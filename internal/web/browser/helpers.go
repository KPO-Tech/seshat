package browser

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultBlankURL
	}
	return trimmed
}

func validateNavigationURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid browser URL: %w", err)
	}
	if parsed.Scheme == "" && raw == defaultBlankURL {
		return nil
	}
	switch parsed.Scheme {
	case "http", "https", "about":
		if parsed.Scheme == "about" {
			return nil
		}
		return rejectLocalNavigationTarget(parsed)
	default:
		return fmt.Errorf("unsupported browser URL scheme %q", parsed.Scheme)
	}
}

func rejectLocalNavigationTarget(parsed *url.URL) error {
	host := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), "www.")
	switch host {
	case "", "localhost", "localhost.localdomain", "host.docker.internal":
		return fmt.Errorf("local network targets are not allowed")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("private or loopback IP targets are not allowed")
		}
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("local network targets are not allowed")
	}
	return nil
}

func withRod(fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("browser runtime panic: %v", recovered)
		}
	}()
	return fn()
}

func withRodResult[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("browser runtime panic: %v", recovered)
		}
	}()
	return fn()
}
