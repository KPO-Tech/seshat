package connectors

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// fakeDriveServer answers the few Drive v3 calls the connector makes, so the
// connector can be driven end to end without Google.
func fakeDriveServer(t *testing.T) *httptest.Server {
	t.Helper()
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/drive/v3/changes/startPageToken":
			writeJSON(w, map[string]any{"startPageToken": "start-1"})
		case path == "/drive/v3/changes":
			writeJSON(w, map[string]any{
				"newStartPageToken": "start-2",
				"changes": []any{
					map[string]any{"fileId": "gone", "removed": true},
					map[string]any{"fileId": "f2", "file": map[string]any{"id": "f2", "name": "edited.txt", "mimeType": "text/plain", "size": "7"}},
				},
			})
		case path == "/drive/v3/files":
			writeJSON(w, map[string]any{"files": []any{
				map[string]any{"id": "f1", "name": "plan.txt", "mimeType": "text/plain", "size": "8", "modifiedTime": "2026-10-01T10:00:00Z"},
				map[string]any{"id": "skip", "name": "script.py", "mimeType": "text/plain", "size": "3"},
			}})
		case strings.HasSuffix(path, "/permissions"):
			writeJSON(w, map[string]any{"permissions": []any{
				map[string]any{"type": "user", "emailAddress": "alice@example.com"},
				map[string]any{"type": "group", "emailAddress": "eng@example.com"},
			}})
		case path == "/drive/v3/files/f1":
			_, _ = w.Write([]byte("the plan"))
		case path == "/drive/v3/files/f2":
			_, _ = w.Write([]byte("edited!"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestGDriveConnectorRunsAgainstAnotherEndpoint(t *testing.T) {
	server := fakeDriveServer(t)
	conn := NewGDriveConnector(&oauth2.Config{}).WithEndpoint(server.URL + "/drive/v3/")
	secret := Secret{AccessToken: "tok"}

	items, cursor, err := conn.Sync(context.Background(), secret, "")
	if err != nil {
		t.Fatalf("bootstrap sync: %v", err)
	}
	if cursor != "start-1" {
		t.Fatalf("expected the start page token as cursor, got %q", cursor)
	}
	if len(items) != 1 || items[0].ID != "f1" || items[0].Text != "the plan" {
		t.Fatalf("expected the one allowed file with its text, got %+v", items)
	}
	if got := items[0].AccessControl; len(got) != 2 || got[0] != "user:alice@example.com" || got[1] != "group:eng@example.com" {
		t.Fatalf("expected the file's access, got %v", got)
	}

	items, cursor, err = conn.Sync(context.Background(), secret, cursor)
	if err != nil {
		t.Fatalf("incremental sync: %v", err)
	}
	if cursor != "start-2" || len(items) != 1 || items[0].ID != "f2" || items[0].Text != "edited!" {
		t.Fatalf("expected the edited file and the new cursor, got %q %+v", cursor, items)
	}

	acl, err := conn.RefreshPermissions(context.Background(), secret, []string{"f1"})
	if err != nil || len(acl["f1"]) != 2 {
		t.Fatalf("expected the refreshed access of f1, got %v (%v)", acl, err)
	}
}

func TestGDriveConnectorWithoutEndpointKeepsGoogle(t *testing.T) {
	if c := NewGDriveConnector(&oauth2.Config{}); c.endpoint != "" {
		t.Fatalf("a connector must talk to Google unless told otherwise, got %q", c.endpoint)
	}
}
