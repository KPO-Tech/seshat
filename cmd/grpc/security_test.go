package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	appconfig "github.com/KPO-Tech/seshat/pkg/config"
	pb "github.com/KPO-Tech/seshat/pkg/grpc/seshat"
)

func TestIsLoopbackHost(t *testing.T) {
	t.Parallel()
	for host, want := range map[string]bool{
		"127.0.0.1": true, "127.5.5.5": true, "::1": true, "[::1]": true, "localhost": true, "LOCALHOST": true,
		"0.0.0.0": false, "": false, "::": false, "192.168.1.10": false, "10.0.0.1": false, "example.com": false, "[::]": false,
	} {
		if got := isLoopbackHost(host); got != want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestDefaultsListenOnLoopbackOnly(t *testing.T) {
	t.Setenv("SESHAT_GRPC_HOST", "")
	t.Setenv("SESHAT_GRPC_AUTH_TOKEN", "")
	t.Setenv("SESHAT_GRPC_ALLOW_INSECURE_REMOTE", "")
	cfg := loadGRPCConfigFromEnv()
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("the default host must be the loopback address, got %q", cfg.Host)
	}
	if err := validateListenConfig(cfg); err != nil {
		t.Fatalf("the default configuration must start: %v", err)
	}
	if addr := listenAddress(cfg); addr != "127.0.0.1:50051" {
		t.Fatalf("listenAddress = %q", addr)
	}
}

func TestValidateListenConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     GRPCConfig
		wantErr string
	}{
		{"loopback, nothing else", GRPCConfig{Host: "127.0.0.1"}, ""},
		{"all interfaces without a token", GRPCConfig{Host: "0.0.0.0"}, "refusing to listen"},
		{"empty host (all interfaces) without a token", GRPCConfig{Host: ""}, "refusing to listen"},
		{"a LAN address without a token", GRPCConfig{Host: "192.168.1.10"}, "refusing to listen"},
		{"all interfaces with a token", GRPCConfig{Host: "0.0.0.0", AuthToken: "s3cret"}, ""},
		{"all interfaces, risk accepted", GRPCConfig{Host: "0.0.0.0", AllowInsecureRemote: true}, ""},
		{"a certificate without its key", GRPCConfig{Host: "127.0.0.1", TLSCertFile: "c.pem"}, "go together"},
		{"a key without its certificate", GRPCConfig{Host: "127.0.0.1", TLSKeyFile: "k.pem"}, "go together"},
	}
	for _, tc := range cases {
		err := validateListenConfig(tc.cfg)
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: error = %v, want one containing %q", tc.name, err, tc.wantErr)
		}
	}
}

func TestConfigFromEnvReadsTheSecuritySettings(t *testing.T) {
	t.Setenv("SESHAT_GRPC_HOST", "0.0.0.0")
	t.Setenv("SESHAT_GRPC_AUTH_TOKEN", "  tok  ")
	t.Setenv("SESHAT_GRPC_TLS_CERT", "c.pem")
	t.Setenv("SESHAT_GRPC_TLS_KEY", "k.pem")
	t.Setenv("SESHAT_GRPC_ALLOW_INSECURE_REMOTE", "true")
	t.Setenv("SESHAT_GRPC_ALLOW_STDIO_MCP", "yes")
	cfg := loadGRPCConfigFromEnv()
	if cfg.Host != "0.0.0.0" || cfg.AuthToken != "tok" || cfg.TLSCertFile != "c.pem" || cfg.TLSKeyFile != "k.pem" || !cfg.AllowInsecureRemote || !cfg.AllowStdioMCP {
		t.Fatalf("config = %+v", cfg)
	}
}

func authContext(values ...string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(authPairs(values)...))
}

func authPairs(values []string) []string {
	var pairs []string
	for _, v := range values {
		pairs = append(pairs, "authorization", v)
	}
	return pairs
}

func TestTokenInterceptorUnary(t *testing.T) {
	t.Parallel()
	unary, _ := tokenInterceptors("s3cret")
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	info := func(method string) *grpc.UnaryServerInfo { return &grpc.UnaryServerInfo{FullMethod: method} }
	query := pb.SeshatService_Query_FullMethodName

	cases := []struct {
		name   string
		ctx    context.Context
		method string
		want   codes.Code
	}{
		{"no metadata", context.Background(), query, codes.Unauthenticated},
		{"no authorization header", authContext(), query, codes.Unauthenticated},
		{"wrong token", authContext("Bearer nope"), query, codes.Unauthenticated},
		{"empty token", authContext("Bearer "), query, codes.Unauthenticated},
		{"a prefix of the token", authContext("Bearer s3cre"), query, codes.Unauthenticated},
		{"the token and more", authContext("Bearer s3cret!"), query, codes.Unauthenticated},
		{"right token", authContext("Bearer s3cret"), query, codes.OK},
		{"right token, lower-case scheme", authContext("bearer s3cret"), query, codes.OK},
		{"right token among others", authContext("Bearer nope", "Bearer s3cret"), query, codes.OK},
		{"health check needs no token", authContext(), healthCheckMethod, codes.OK},
		{"connect MCP needs the token", authContext(), pb.SeshatService_ConnectMCP_FullMethodName, codes.Unauthenticated},
	}
	for _, tc := range cases {
		_, err := unary(tc.ctx, nil, info(tc.method), handler)
		if got := status.Code(err); got != tc.want {
			t.Errorf("%s: code = %v, want %v (%v)", tc.name, got, tc.want, err)
		}
	}
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeServerStream) Context() context.Context { return f.ctx }

func TestTokenInterceptorStream(t *testing.T) {
	t.Parallel()
	_, stream := tokenInterceptors("s3cret")
	called := false
	handler := func(any, grpc.ServerStream) error { called = true; return nil }
	info := &grpc.StreamServerInfo{FullMethod: pb.SeshatService_QueryStream_FullMethodName}

	if err := stream(nil, fakeServerStream{ctx: authContext("Bearer nope")}, info, handler); status.Code(err) != codes.Unauthenticated || called {
		t.Fatalf("a wrong token must be refused before the handler runs: err=%v called=%v", err, called)
	}
	if err := stream(nil, fakeServerStream{ctx: authContext("Bearer s3cret")}, info, handler); err != nil || !called {
		t.Fatalf("the right token must pass: err=%v called=%v", err, called)
	}
}

func TestConnectMCPRefusesAStdioServerByDefault(t *testing.T) {
	t.Parallel()
	server := NewSeshatServer(appconfig.Config{})
	for _, kind := range []string{"", "stdio", "STDIO"} {
		resp, err := server.ConnectMCP(context.Background(), &pb.ConnectMCPRequest{Name: "x", Type: kind, Command: "touch", Args: []string{"/tmp/pwned"}})
		if status.Code(err) != codes.PermissionDenied || resp != nil {
			t.Errorf("type %q: want PermissionDenied before anything is written or started, got resp=%v err=%v", kind, resp, err)
		}
	}
}

// startTestServer runs the real server with the security options of cfg, and returns its address.
func startTestServer(t *testing.T, cfg GRPCConfig) string {
	t.Helper()
	opts, err := serverOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(opts...)
	pb.RegisterSeshatServiceServer(server, NewSeshatServer(appconfig.Config{}))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String()
}

func dialTest(t *testing.T, addr string, creds credentials.TransportCredentials) pb.SeshatServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewSeshatServiceClient(conn)
}

func withToken(t *testing.T, token string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
}

func TestServerWithATokenOverTheWire(t *testing.T) {
	addr := startTestServer(t, GRPCConfig{AuthToken: "s3cret"})
	client := dialTest(t, addr, insecure.NewCredentials())

	if _, err := client.HealthCheck(withToken(t, ""), &pb.HealthCheckRequest{}); err != nil {
		t.Fatalf("HealthCheck needs no token: %v", err)
	}
	if _, err := client.GetModels(withToken(t, ""), &pb.GetModelsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("a call with no token must be Unauthenticated, got %v", err)
	}
	if _, err := client.GetModels(withToken(t, "wrong"), &pb.GetModelsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("a call with a wrong token must be Unauthenticated, got %v", err)
	}
	if _, err := client.GetModels(withToken(t, "s3cret"), &pb.GetModelsRequest{}); err != nil {
		t.Fatalf("a call with the right token must pass: %v", err)
	}
	// The stream interceptor is covered by TestTokenInterceptorStream. It is not exercised over the wire here: QueryStream is the
	// only streaming call, and marshalling its request message, pb.QueryRequest, trips checkptr under `go test -race` on Windows
	// (even an empty one; the other messages do not), so a test that sends one crashes there.
}

func selfSignedCert(t *testing.T) (certFile, keyFile string, pool *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool = x509.NewCertPool()
	pool.AddCert(cert)
	return certFile, keyFile, pool
}

func TestServerWithTLSAndATokenOverTheWire(t *testing.T) {
	certFile, keyFile, pool := selfSignedCert(t)
	addr := startTestServer(t, GRPCConfig{AuthToken: "s3cret", TLSCertFile: certFile, TLSKeyFile: keyFile})

	// a plaintext client cannot talk to a TLS server
	plain := dialTest(t, addr, insecure.NewCredentials())
	if _, err := plain.HealthCheck(withToken(t, ""), &pb.HealthCheckRequest{}); err == nil {
		t.Fatal("a plaintext client must not reach a TLS server")
	}
	secure := dialTest(t, addr, credentials.NewTLS(&tls.Config{RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS12}))
	if _, err := secure.GetModels(withToken(t, ""), &pb.GetModelsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("TLS does not replace the token: got %v", err)
	}
	if _, err := secure.GetModels(withToken(t, "s3cret"), &pb.GetModelsRequest{}); err != nil {
		t.Fatalf("TLS and the right token must pass: %v", err)
	}
}

func TestServerOptionsRejectAnUnreadableCertificate(t *testing.T) {
	t.Parallel()
	if _, err := serverOptions(GRPCConfig{TLSCertFile: filepath.Join(t.TempDir(), "missing.pem"), TLSKeyFile: filepath.Join(t.TempDir(), "missing.key")}); err == nil {
		t.Fatal("a certificate that cannot be read must be an error, not a server with no TLS")
	}
}
