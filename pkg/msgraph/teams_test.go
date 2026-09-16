package msgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListChatsReturnsValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/chats" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatsResponse{Value: []Chat{{ID: "c1", Topic: "General"}}})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	chats, err := c.ListChats(context.Background())
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].ID != "c1" {
		t.Fatalf("expected one chat c1, got %+v", chats)
	}
}

func TestListChatMessagesBootstrapAndPagination(t *testing.T) {
	var requests int
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		switch requests {
		case 1:
			if r.URL.Path != "/chats/c1/messages" {
				t.Fatalf("expected the bootstrap chat messages path, got %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(chatMessagesResponse{
				Value:    []ChatMessage{{ID: "msg1"}},
				NextLink: server.URL + "/page2",
			})
		default:
			t.Fatalf("unexpected extra request (ListChatMessages only fetches one page per call): %d", requests)
		}
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	messages, nextCursor, err := c.ListChatMessages(context.Background(), "c1", "")
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != "msg1" {
		t.Fatalf("expected one message, got %+v", messages)
	}
	if nextCursor != server.URL+"/page2" {
		t.Fatalf("expected nextLink returned as the next cursor, got %q", nextCursor)
	}
}

func TestSendChatMessagePostsBody(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chats/c1/messages" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.SendChatMessage(context.Background(), "c1", "hello team"); err != nil {
		t.Fatalf("SendChatMessage: %v", err)
	}
	body, ok := gotBody["body"].(map[string]any)
	if !ok || body["content"] != "hello team" {
		t.Fatalf("expected body.content %q, got %+v", "hello team", gotBody)
	}
}
