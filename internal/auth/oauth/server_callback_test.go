package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Found by an audit: the error page copied error_description into the HTML as it
// came, so a page that sent the browser to the local callback could run a script
// on it.
func TestCallbackErrorPageEscapesTheDescription(t *testing.T) {
	server := NewServer(nil)
	query := url.Values{}
	query.Set("error", "access_denied")
	query.Set("error_description", `<script>alert(1)</script><img src=x onerror=alert(2)>`)
	request := httptest.NewRequest(http.MethodGet, "/callback?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()

	server.handleCallback(recorder, request)

	body := recorder.Body.String()
	if strings.Contains(body, "<script>") || strings.Contains(body, "<img") {
		t.Fatalf("the description must be escaped, got %q", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("the description must still be shown, escaped, got %q", body)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("an error callback is a bad request, got %d", recorder.Code)
	}
}

func TestCallbackHitTwiceDoesNotPanic(t *testing.T) {
	server := NewServer(nil)
	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodGet, "/callback?code=abc", nil)
		server.handleCallback(httptest.NewRecorder(), request)
	}
	select {
	case <-server.received:
	default:
		t.Error("the waiting caller must be told that the callback arrived")
	}
}
