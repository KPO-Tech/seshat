package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComposeIncludesCcAndBcc(t *testing.T) {
	var gotRaw string
	mux := http.NewServeMux()
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
	id, err := c.Compose(context.Background(), ComposeOptions{
		To: []string{"jane@example.com"}, Cc: []string{"cc@example.com"}, Bcc: []string{"bcc@example.com"},
		Subject: "Hi", Body: "Body",
	})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if id != "sent-1" {
		t.Fatalf("expected sent-1, got %q", id)
	}
	if gotRaw == "" {
		t.Fatal("expected a raw message to have been sent")
	}
}

func TestComposeRequiresAtLeastOneRecipient(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.NewServeMux()))
	if _, err := c.Compose(context.Background(), ComposeOptions{Subject: "Hi", Body: "Body"}); err == nil {
		t.Fatal("expected an error when no recipients are given")
	}
}

func TestCreateDraftReturnsMessageAndThreadIDs(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gmail/v1/users/me/drafts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"id": "draft-1", "message": map[string]any{"id": "m1", "threadId": "t1"}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	draft, err := c.CreateDraft(context.Background(), ComposeOptions{To: []string{"jane@example.com"}, Subject: "Hi", Body: "Body"})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if draft.ID != "draft-1" || draft.MessageID != "m1" || draft.ThreadID != "t1" {
		t.Fatalf("unexpected draft: %+v", draft)
	}
}

func TestCreateDraftRequiresAtLeastOneRecipient(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.NewServeMux()))
	if _, err := c.CreateDraft(context.Background(), ComposeOptions{Subject: "Hi", Body: "Body"}); err == nil {
		t.Fatal("expected an error when no recipients are given")
	}
}

func TestListDraftsTranslatesEachEntry(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/drafts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"drafts": []map[string]any{
			{"id": "draft-1", "message": map[string]any{"id": "m1", "threadId": "t1"}},
			{"id": "draft-2", "message": map[string]any{"id": "m2", "threadId": "t2"}},
		}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	drafts, err := c.ListDrafts(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 2 || drafts[0].ID != "draft-1" || drafts[1].MessageID != "m2" {
		t.Fatalf("unexpected drafts: %+v", drafts)
	}
}

func TestSendDraftReturnsSentMessageID(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gmail/v1/users/me/drafts/send", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"id": "sent-1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	id, err := c.SendDraft(context.Background(), "draft-1")
	if err != nil {
		t.Fatalf("SendDraft: %v", err)
	}
	if id != "sent-1" {
		t.Fatalf("expected sent-1, got %q", id)
	}
}

func TestDeleteDraft(t *testing.T) {
	var deletedID string
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /gmail/v1/users/me/drafts/draft-1", func(w http.ResponseWriter, r *http.Request) {
		deletedID = "draft-1"
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.DeleteDraft(context.Background(), "draft-1"); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if deletedID != "draft-1" {
		t.Fatal("expected the delete endpoint to be called with draft-1")
	}
}
