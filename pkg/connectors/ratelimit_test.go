package connectors

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
)

func TestParseRetryAfter_NumericSeconds(t *testing.T) {
	h := http.Header{"Retry-After": []string{"30"}}
	d, ok := parseRetryAfter(h)
	if !ok || d != 30*time.Second {
		t.Fatalf("got (%v, %v), want (30s, true)", d, ok)
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().UTC().Add(2 * time.Minute)
	h := http.Header{"Retry-After": []string{future.Format(http.TimeFormat)}}
	d, ok := parseRetryAfter(h)
	if !ok {
		t.Fatal("expected a future HTTP-date to parse successfully")
	}
	// Allow a little slack for the round trip through second-precision formatting.
	if d < 100*time.Second || d > 130*time.Second {
		t.Fatalf("got %v, want ~2m", d)
	}
}

func TestParseRetryAfter_AbsentOrGarbage(t *testing.T) {
	if _, ok := parseRetryAfter(http.Header{}); ok {
		t.Fatal("expected no header to report ok=false")
	}
	if _, ok := parseRetryAfter(http.Header{"Retry-After": []string{"not-a-value"}}); ok {
		t.Fatal("expected an unparseable value to report ok=false")
	}
}

func TestParseRetryAfter_PastDateIsIgnored(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	h := http.Header{"Retry-After": []string{past.Format(http.TimeFormat)}}
	if _, ok := parseRetryAfter(h); ok {
		t.Fatal("expected a past HTTP-date to report ok=false, not a negative delay")
	}
}

func TestSPGraphClientGet_RateLimitWithRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "45")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := &spGraphClient{http: server.Client()}
	_, err := client.get(context.Background(), server.URL, nil)
	if err == nil {
		t.Fatal("expected an error for a 429 response")
	}
	var rl RateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected the error to satisfy RateLimited, got %T", err)
	}
	delay, ok := rl.RetryAfter()
	if !ok || delay != 45*time.Second {
		t.Fatalf("got (%v, %v), want (45s, true)", delay, ok)
	}
}

func TestSPGraphClientGet_RateLimitWithoutRetryAfterUsesDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &spGraphClient{http: server.Client()}
	_, err := client.get(context.Background(), server.URL, nil)
	var rl RateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected a 503 to satisfy RateLimited, got %T: %v", err, err)
	}
	delay, ok := rl.RetryAfter()
	if !ok || delay != spGraphDefaultRateLimitBackoff {
		t.Fatalf("got (%v, %v), want (%v, true)", delay, ok, spGraphDefaultRateLimitBackoff)
	}
}

func TestSPGraphClientGet_NonRateLimitErrorIsNotRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &spGraphClient{http: server.Client()}
	_, err := client.get(context.Background(), server.URL, nil)
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	// *spGraphError always structurally satisfies RateLimited (it always
	// has a RetryAfter method) - what distinguishes a real rate-limit case
	// is RetryAfter's own ok return, which must be false here.
	var rl RateLimited
	if errors.As(err, &rl) {
		if _, ok := rl.RetryAfter(); ok {
			t.Fatalf("expected a 404 to report RetryAfter ok=false, got ok=true: %v", err)
		}
	}
	var gerr *spGraphError
	if !errors.As(err, &gerr) || gerr.Status != http.StatusNotFound {
		t.Fatalf("expected a *spGraphError with Status=404, got %+v", gerr)
	}
}

func TestConfluenceClientGet_RateLimitWithRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := &confluenceClient{http: server.Client()}
	err := client.get(context.Background(), server.URL, nil)
	var rl RateLimited
	if !errors.As(err, &rl) {
		t.Fatalf("expected the error to satisfy RateLimited, got %T", err)
	}
	delay, ok := rl.RetryAfter()
	if !ok || delay != 12*time.Second {
		t.Fatalf("got (%v, %v), want (12s, true)", delay, ok)
	}
}

func TestConfluenceClientGet_NonRateLimitErrorIsNotRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &confluenceClient{http: server.Client()}
	err := client.get(context.Background(), server.URL, nil)
	// *confluenceError always structurally satisfies RateLimited - a real
	// rate-limit case is distinguished by RetryAfter's ok return.
	var rl RateLimited
	if errors.As(err, &rl) {
		if _, ok := rl.RetryAfter(); ok {
			t.Fatalf("expected a 401 to report RetryAfter ok=false, got ok=true: %v", err)
		}
	}
}

func TestWrapGDriveRateLimit_429(t *testing.T) {
	base := &googleapi.Error{Code: http.StatusTooManyRequests, Header: http.Header{"Retry-After": []string{"20"}}}
	wrapped := wrapGDriveRateLimit(fmt.Errorf("list drive files: %w", base))
	var rl RateLimited
	if !errors.As(wrapped, &rl) {
		t.Fatalf("expected a 429 googleapi.Error to be wrapped as RateLimited, got %T", wrapped)
	}
	delay, ok := rl.RetryAfter()
	if !ok || delay != 20*time.Second {
		t.Fatalf("got (%v, %v), want (20s, true)", delay, ok)
	}
}

func TestWrapGDriveRateLimit_403QuotaReason(t *testing.T) {
	base := &googleapi.Error{Code: http.StatusForbidden, Errors: []googleapi.ErrorItem{{Reason: "userRateLimitExceeded"}}}
	wrapped := wrapGDriveRateLimit(base)
	var rl RateLimited
	if !errors.As(wrapped, &rl) {
		t.Fatalf("expected a 403/userRateLimitExceeded googleapi.Error to be wrapped as RateLimited, got %T", wrapped)
	}
	delay, ok := rl.RetryAfter()
	if !ok || delay != gdriveDefaultRateLimitBackoff {
		t.Fatalf("got (%v, %v), want (%v, true) - no Retry-After header, should use the default", delay, ok, gdriveDefaultRateLimitBackoff)
	}
}

func TestWrapGDriveRateLimit_PlainPermissionErrorIsNotWrapped(t *testing.T) {
	base := &googleapi.Error{Code: http.StatusForbidden, Errors: []googleapi.ErrorItem{{Reason: "insufficientPermissions"}}}
	wrapped := wrapGDriveRateLimit(base)
	var rl RateLimited
	if errors.As(wrapped, &rl) {
		t.Fatalf("expected a genuine 403 permission error to NOT be wrapped as RateLimited, got one anyway: %v", wrapped)
	}
	if wrapped != error(base) {
		t.Fatal("expected a non-rate-limit error to be returned completely unchanged")
	}
}

func TestWrapGDriveRateLimit_NilAndNonGoogleErrorsPassThrough(t *testing.T) {
	if wrapGDriveRateLimit(nil) != nil {
		t.Fatal("expected nil to pass through as nil")
	}
	plain := errors.New("boom")
	if wrapGDriveRateLimit(plain) != error(plain) {
		t.Fatal("expected a non-googleapi.Error to pass through unchanged")
	}
}
