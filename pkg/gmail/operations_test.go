package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func rawMessagePayload(id, threadID string) map[string]any {
	return map[string]any{
		"id": id, "threadId": threadID, "internalDate": "1700000000000", "labelIds": []string{"UNREAD", "INBOX"},
		"payload": map[string]any{"mimeType": "text/plain", "headers": []map[string]string{
			{"name": "From", "value": "jane@example.com"},
			{"name": "Subject", "value": "Hello"},
		}},
	}
}

func TestGetMessageTranslatesLabelsAndUnread(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/messages/m1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, rawMessagePayload("m1", "t1"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	msg, err := c.GetMessage(context.Background(), "m1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if !msg.Unread {
		t.Fatal("expected Unread to be true when labelIds contains UNREAD")
	}
	if len(msg.LabelIds) != 2 {
		t.Fatalf("expected labelIds to be carried through, got %+v", msg.LabelIds)
	}
}

func TestGetThreadReturnsEveryMessage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/threads/t1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"messages": []map[string]any{rawMessagePayload("m1", "t1"), rawMessagePayload("m2", "t1")}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	messages, err := c.GetThread(context.Background(), "t1")
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestSearchPassesQueryAndTranslatesResults(t *testing.T) {
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		writeJSON(w, map[string]any{"messages": []map[string]string{{"id": "m1"}}})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/messages/m1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, rawMessagePayload("m1", "t1"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	messages, err := c.Search(context.Background(), "is:unread from:jane@example.com", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotQuery != "is:unread from:jane@example.com" {
		t.Fatalf("expected the query to reach Gmail's q param, got %q", gotQuery)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

func TestTrashAndUntrashMessage(t *testing.T) {
	var trashed, untrashed bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gmail/v1/users/me/messages/m1/trash", func(w http.ResponseWriter, r *http.Request) {
		trashed = true
		writeJSON(w, map[string]any{"id": "m1"})
	})
	mux.HandleFunc("POST /gmail/v1/users/me/messages/m1/untrash", func(w http.ResponseWriter, r *http.Request) {
		untrashed = true
		writeJSON(w, map[string]any{"id": "m1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.TrashMessage(context.Background(), "m1"); err != nil {
		t.Fatalf("TrashMessage: %v", err)
	}
	if !trashed {
		t.Fatal("expected the trash endpoint to be called")
	}
	if err := c.UntrashMessage(context.Background(), "m1"); err != nil {
		t.Fatalf("UntrashMessage: %v", err)
	}
	if !untrashed {
		t.Fatal("expected the untrash endpoint to be called")
	}
}

func TestMarkReadAndUnreadModifyTheUnreadLabel(t *testing.T) {
	var gotAdd, gotRemove []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gmail/v1/users/me/messages/m1/modify", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AddLabelIds    []string `json:"addLabelIds"`
			RemoveLabelIds []string `json:"removeLabelIds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotAdd, gotRemove = body.AddLabelIds, body.RemoveLabelIds
		writeJSON(w, map[string]any{"id": "m1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.MarkRead(context.Background(), "m1"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if len(gotRemove) != 1 || gotRemove[0] != "UNREAD" {
		t.Fatalf("expected MarkRead to remove UNREAD, got remove=%v add=%v", gotRemove, gotAdd)
	}

	if err := c.MarkUnread(context.Background(), "m1"); err != nil {
		t.Fatalf("MarkUnread: %v", err)
	}
	if len(gotAdd) != 1 || gotAdd[0] != "UNREAD" {
		t.Fatalf("expected MarkUnread to add UNREAD, got remove=%v add=%v", gotRemove, gotAdd)
	}
}

func TestLabelsListCreateDelete(t *testing.T) {
	var created string
	var deletedID string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"labels": []map[string]string{{"id": "Label_1", "name": "Newsletters"}}})
	})
	mux.HandleFunc("POST /gmail/v1/users/me/labels", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		created = body.Name
		writeJSON(w, map[string]any{"id": "Label_2", "name": body.Name})
	})
	mux.HandleFunc("DELETE /gmail/v1/users/me/labels/Label_1", func(w http.ResponseWriter, r *http.Request) {
		deletedID = "Label_1"
		w.WriteHeader(http.StatusNoContent)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	labels, err := c.ListLabels(context.Background())
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 1 || labels[0].Name != "Newsletters" {
		t.Fatalf("unexpected labels: %+v", labels)
	}

	label, err := c.CreateLabel(context.Background(), "Promo")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	if created != "Promo" || label.ID != "Label_2" {
		t.Fatalf("expected the new label to be created with name Promo, got created=%q label=%+v", created, label)
	}

	if err := c.DeleteLabel(context.Background(), "Label_1"); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	if deletedID != "Label_1" {
		t.Fatal("expected the delete endpoint to be called with Label_1")
	}
}

func TestTrashAndUntrashThread(t *testing.T) {
	var trashed, untrashed bool
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gmail/v1/users/me/threads/t1/trash", func(w http.ResponseWriter, r *http.Request) {
		trashed = true
		writeJSON(w, map[string]any{"id": "t1"})
	})
	mux.HandleFunc("POST /gmail/v1/users/me/threads/t1/untrash", func(w http.ResponseWriter, r *http.Request) {
		untrashed = true
		writeJSON(w, map[string]any{"id": "t1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.TrashThread(context.Background(), "t1"); err != nil {
		t.Fatalf("TrashThread: %v", err)
	}
	if !trashed {
		t.Fatal("expected the thread trash endpoint to be called")
	}
	if err := c.UntrashThread(context.Background(), "t1"); err != nil {
		t.Fatalf("UntrashThread: %v", err)
	}
	if !untrashed {
		t.Fatal("expected the thread untrash endpoint to be called")
	}
}

func TestMarkThreadReadAndUnreadModifyTheUnreadLabel(t *testing.T) {
	var gotAdd, gotRemove []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /gmail/v1/users/me/threads/t1/modify", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AddLabelIds    []string `json:"addLabelIds"`
			RemoveLabelIds []string `json:"removeLabelIds"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotAdd, gotRemove = body.AddLabelIds, body.RemoveLabelIds
		writeJSON(w, map[string]any{"id": "t1"})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	if err := c.MarkThreadRead(context.Background(), "t1"); err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
	if len(gotRemove) != 1 || gotRemove[0] != "UNREAD" {
		t.Fatalf("expected MarkThreadRead to remove UNREAD, got remove=%v add=%v", gotRemove, gotAdd)
	}

	if err := c.MarkThreadUnread(context.Background(), "t1"); err != nil {
		t.Fatalf("MarkThreadUnread: %v", err)
	}
	if len(gotAdd) != 1 || gotAdd[0] != "UNREAD" {
		t.Fatalf("expected MarkThreadUnread to add UNREAD, got remove=%v add=%v", gotRemove, gotAdd)
	}
}

func TestGetProfile(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /gmail/v1/users/me/profile", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"emailAddress": "me@example.com", "messagesTotal": 42, "threadsTotal": 10})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	c := newTestClient(t, server)
	profile, err := c.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile.EmailAddress != "me@example.com" || profile.MessagesTotal != 42 || profile.ThreadsTotal != 10 {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}
