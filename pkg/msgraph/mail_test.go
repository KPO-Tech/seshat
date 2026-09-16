package msgraph

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestListMessagesDeltaBootstrapAndFollowsDeltaLink(t *testing.T) {
	var requests []string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch len(requests) {
		case 1:
			if r.URL.Path != "/me/mailFolders/inbox/messages/delta" {
				t.Fatalf("expected the bootstrap delta path, got %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(mailDeltaResponse{
				Value:    []Message{{ID: "m1", Subject: "Hello"}},
				NextLink: server.URL + "/page2",
			})
		case 2:
			_ = json.NewEncoder(w).Encode(mailDeltaResponse{
				Value:     []Message{{ID: "m2", Subject: "World"}},
				DeltaLink: server.URL + "/delta-cursor",
			})
		default:
			t.Fatalf("unexpected extra request: %v", requests)
		}
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	messages, nextCursor, err := c.ListMessagesDelta(context.Background(), "")
	if err != nil {
		t.Fatalf("ListMessagesDelta: %v", err)
	}
	if len(messages) != 2 || messages[0].ID != "m1" || messages[1].ID != "m2" {
		t.Fatalf("expected both pages' messages combined, got %+v", messages)
	}
	if nextCursor != server.URL+"/delta-cursor" {
		t.Fatalf("expected the deltaLink to be returned as the next cursor, got %q", nextCursor)
	}
}

func TestListMessagesDeltaResumesFromCursor(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mailDeltaResponse{DeltaLink: "https://unused"})
	}))
	defer server.Close()

	c := New(server.Client())
	if _, _, err := c.ListMessagesDelta(context.Background(), server.URL+"/resume-here"); err != nil {
		t.Fatalf("ListMessagesDelta: %v", err)
	}
	if gotPath != "/resume-here" {
		t.Fatalf("expected the request to hit the cursor url directly, got %q", gotPath)
	}
}

func TestReplyToMessagePostsComment(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/me/messages/m1/reply" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.ReplyToMessage(context.Background(), "m1", "thanks!"); err != nil {
		t.Fatalf("ReplyToMessage: %v", err)
	}
	if gotBody["comment"] != "thanks!" {
		t.Fatalf("expected comment %q, got %+v", "thanks!", gotBody)
	}
}

func TestSendMailBuildsRecipientsAndBody(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/me/sendMail" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.SendMail(context.Background(), []string{"a@example.com"}, "Subject", "Body"); err != nil {
		t.Fatalf("SendMail: %v", err)
	}
	message, ok := gotBody["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected a message object, got %+v", gotBody)
	}
	if message["subject"] != "Subject" {
		t.Fatalf("expected subject %q, got %+v", "Subject", message["subject"])
	}
	recipients, ok := message["toRecipients"].([]any)
	if !ok || len(recipients) != 1 {
		t.Fatalf("expected exactly one recipient, got %+v", message["toRecipients"])
	}
}

func TestLatestMessageInConversationFiltersAndOrders(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/mailFolders/inbox/messages" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mailListResponse{Value: []Message{{ID: "latest-msg"}}})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	msg, err := c.LatestMessageInConversation(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("LatestMessageInConversation: %v", err)
	}
	if msg.ID != "latest-msg" {
		t.Fatalf("expected latest-msg, got %+v", msg)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if q.Get("$filter") != "conversationId eq 'conv-1'" {
		t.Fatalf("unexpected $filter: %q", q.Get("$filter"))
	}
	if q.Get("$orderby") != "receivedDateTime desc" || q.Get("$top") != "1" {
		t.Fatalf("expected newest-first, top=1, got orderby=%q top=%q", q.Get("$orderby"), q.Get("$top"))
	}
}

func TestLatestMessageInConversationErrorsWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mailListResponse{})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if _, err := c.LatestMessageInConversation(context.Background(), "conv-1"); err == nil {
		t.Fatal("expected an error when no messages are found in the conversation")
	}
}

func TestGraphErrorSurfacesStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient scope"}}`))
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	_, _, err := c.ListMessagesDelta(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error for a 403 response")
	}
	var gerr *GraphError
	if !errors.As(err, &gerr) {
		t.Fatalf("expected a *GraphError, got %T: %v", err, err)
	}
	if gerr.Status != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", gerr.Status)
	}
}
