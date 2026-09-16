package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/option"
)

// newTestClient builds a Client whose requests all land on server - the
// official Gmail SDK supports exactly this override via WithEndpoint (only
// - NOT WithHTTPClient, which was tried first and found to bypass the
// TokenSource-driven Authorization header entirely, confirmed by a request
// arriving with no Authorization header at all; dropping it restores the
// SDK's own oauth2-wrapped transport, exercising the same code path
// production actually uses), so these tests exercise the real
// request/response cycle, not a hand-rolled substitute.
func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fake-token"})
	c, err := New(context.Background(), ts, option.WithEndpoint(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestSyncBootstrapWhenCursorEmpty(t *testing.T) {
	var gotAuthHeader string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		writeJSON(w, map[string]any{"historyId": "555"})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"messages": []map[string]string{{"id": "m1"}}})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/messages/m1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"id": "m1", "threadId": "t1", "internalDate": "1700000000000", "snippet": "hi",
			"payload": map[string]any{"mimeType": "text/plain", "headers": []map[string]string{
				{"name": "From", "value": "jane@example.com"},
			}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	messages, nextCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "m1" {
		t.Fatalf("expected one message m1, got %+v", messages)
	}
	if nextCursor != "555" {
		t.Fatalf("expected the bootstrap cursor to be the profile's historyId, got %q", nextCursor)
	}
	if gotAuthHeader != "Bearer fake-token" {
		t.Fatalf("expected the token source's Authorization header to reach the server, got %q", gotAuthHeader)
	}
}

func TestSyncIncrementalFallsBackToBootstrapOnHistoryExpired(t *testing.T) {
	var historyCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/history", func(w http.ResponseWriter, r *http.Request) {
		historyCalls++
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"error": map[string]any{"code": 404, "message": "history expired"}})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"historyId": "999"})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"messages": []map[string]string{}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	messages, nextCursor, err := c.Sync(context.Background(), "123")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if historyCalls != 1 {
		t.Fatalf("expected exactly one history call before falling back, got %d", historyCalls)
	}
	if len(messages) != 0 || nextCursor != "999" {
		t.Fatalf("expected a fresh bootstrap after history-expired, got messages=%+v cursor=%q", messages, nextCursor)
	}
}

func TestReplyBuildsThreadedMessageAndReturnsID(t *testing.T) {
	var gotRaw string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/threads/t1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"messages": []map[string]any{{
				"payload": map[string]any{"headers": []map[string]string{
					{"name": "Subject", "value": "Table for 4"},
					{"name": "Message-Id", "value": "<msg1@mail.gmail.com>"},
				}},
			}},
		})
	})
	mux.HandleFunc("POST /gmail/v1/users/me/messages/send", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Raw string `json:"raw"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotRaw = body.Raw
		writeJSON(w, map[string]any{"id": "sent-1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	id, err := c.Reply(context.Background(), "t1", "jane@example.com", "Yes!")
	if err != nil {
		t.Fatalf("Reply: %v", err)
	}
	if id != "sent-1" {
		t.Fatalf("expected the sent message id to be returned, got %q", id)
	}
	if gotRaw == "" {
		t.Fatal("expected a raw message body to have been sent")
	}
}

func TestSendRequiresAtLeastOneRecipient(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.NewServeMux()))
	if _, err := c.Send(context.Background(), nil, "Subject", "Body"); err == nil {
		t.Fatal("expected an error when no recipients are given")
	}
}

func TestFetchAttachmentDecodesBase64URLData(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/messages/m1/attachments/a1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": b64("attachment bytes")})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	data, err := c.FetchAttachment(context.Background(), "m1", "a1")
	if err != nil {
		t.Fatalf("FetchAttachment: %v", err)
	}
	if string(data) != "attachment bytes" {
		t.Fatalf("expected decoded attachment bytes, got %q", data)
	}
}
