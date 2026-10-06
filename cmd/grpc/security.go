package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "github.com/KPO-Tech/seshat/pkg/grpc/seshat"
)

// The gRPC server runs an agent for whoever reaches it, and ConnectMCP can start a process: it must not be open by default.
//
//   - It listens on the loopback address unless SESHAT_GRPC_HOST says otherwise.
//   - On any other address it refuses to start without SESHAT_GRPC_AUTH_TOKEN, unless SESHAT_GRPC_ALLOW_INSECURE_REMOTE=true
//     says that the network in front of it is the protection.
//   - With a token, every call but HealthCheck must carry "authorization: Bearer <token>".
//   - SESHAT_GRPC_TLS_CERT and SESHAT_GRPC_TLS_KEY turn TLS on (the token is a secret: on a network, send it only over TLS).
//   - A stdio MCP server is a command line: ConnectMCP refuses it unless SESHAT_GRPC_ALLOW_STDIO_MCP=true.

// healthCheckMethod is the one call that needs no token, so that an orchestrator can probe the server.
const healthCheckMethod = pb.SeshatService_HealthCheck_FullMethodName

// isLoopbackHost reports whether a listen host only accepts connections from the machine itself.
func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// validateListenConfig refuses a configuration that would expose the server with no protection.
func validateListenConfig(cfg GRPCConfig) error {
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return fmt.Errorf("SESHAT_GRPC_TLS_CERT and SESHAT_GRPC_TLS_KEY go together: set both or neither")
	}
	if isLoopbackHost(cfg.Host) || cfg.AuthToken != "" || cfg.AllowInsecureRemote {
		return nil
	}
	return fmt.Errorf("refusing to listen on %q without authentication: set SESHAT_GRPC_AUTH_TOKEN (and SESHAT_GRPC_TLS_CERT/KEY), "+
		"listen on 127.0.0.1, or set SESHAT_GRPC_ALLOW_INSECURE_REMOTE=true if the network in front of the server is the protection", cfg.Host)
}

// listenAddress is the address the server listens on.
func listenAddress(cfg GRPCConfig) string {
	return net.JoinHostPort(strings.Trim(cfg.Host, "[]"), fmt.Sprint(cfg.Port))
}

// serverOptions are the gRPC options of the listen configuration: TLS when a certificate is given, and the token check.
func serverOptions(cfg GRPCConfig) ([]grpc.ServerOption, error) {
	var opts []grpc.ServerOption
	if cfg.TLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load the TLS certificate: %w", err)
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})))
	}
	if cfg.AuthToken != "" {
		unary, stream := tokenInterceptors(cfg.AuthToken)
		opts = append(opts, grpc.ChainUnaryInterceptor(unary), grpc.ChainStreamInterceptor(stream))
	}
	return opts, nil
}

// tokenInterceptors check "authorization: Bearer <token>" on every call but HealthCheck.
func tokenInterceptors(token string) (grpc.UnaryServerInterceptor, grpc.StreamServerInterceptor) {
	want := sha256.Sum256([]byte(token))
	check := func(ctx context.Context, method string) error {
		if method == healthCheckMethod {
			return nil
		}
		md, _ := metadata.FromIncomingContext(ctx)
		for _, value := range md.Get("authorization") {
			got := sha256.Sum256([]byte(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(value, "Bearer "), "bearer "))))
			// compared as digests of equal length, so that the length of the token is not learnt from the time
			if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
				return nil
			}
		}
		return status.Error(codes.Unauthenticated, "a valid authorization: Bearer token is required")
	}
	unary := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := check(ctx, info.FullMethod); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
	stream := func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := check(ss.Context(), info.FullMethod); err != nil {
			return err
		}
		return handler(srv, ss)
	}
	return unary, stream
}
