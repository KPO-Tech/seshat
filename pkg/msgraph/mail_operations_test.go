package msgraph

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGetMessageFetchesByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/messages/m1" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Message{ID: "m1", Subject: "Hello"})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	msg, err := c.GetMessage(context.Background(), "m1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if msg.ID != "m1" || msg.Subject != "Hello" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestSearchMessagesPassesSearchParam(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mailListResponse{Value: []Message{{ID: "m1"}}})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	messages, err := c.SearchMessages(context.Background(), "invoice", 0)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if q.Get("$search") != `"invoice"` {
		t.Fatalf("expected a quoted search term, got %q", q.Get("$search"))
	}
}

func TestListMessagesInConversationOrdersOldestFirst(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mailListResponse{Value: []Message{{ID: "m1"}, {ID: "m2"}}})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	messages, err := c.ListMessagesInConversation(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("ListMessagesInConversation: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	q, _ := url.ParseQuery(gotQuery)
	if q.Get("$orderby") != "receivedDateTime asc" {
		t.Fatalf("expected oldest-first ordering, got %q", q.Get("$orderby"))
	}
}

func TestSetMessageReadPatchesIsRead(t *testing.T) {
	var gotMethod string
	var gotBody map[string]bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.SetMessageRead(context.Background(), "m1", true); err != nil {
		t.Fatalf("SetMessageRead: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Fatalf("expected PATCH, got %s", gotMethod)
	}
	if !gotBody["isRead"] {
		t.Fatalf("expected isRead=true, got %+v", gotBody)
	}
}

func TestTrashAndUntrashMessageMoveFolders(t *testing.T) {
	var gotPaths []string
	var gotDestinations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotDestinations = append(gotDestinations, body["destinationId"])
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(moveResponse{ID: "m1-moved"})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	newID, err := c.TrashMessage(context.Background(), "m1")
	if err != nil {
		t.Fatalf("TrashMessage: %v", err)
	}
	if newID != "m1-moved" {
		t.Fatalf("expected the moved id to be returned, got %q", newID)
	}
	if _, err := c.UntrashMessage(context.Background(), "m1-moved"); err != nil {
		t.Fatalf("UntrashMessage: %v", err)
	}
	if len(gotPaths) != 2 || gotPaths[0] != "/me/messages/m1/move" || gotPaths[1] != "/me/messages/m1-moved/move" {
		t.Fatalf("unexpected move paths: %v", gotPaths)
	}
	if gotDestinations[0] != "deleteditems" || gotDestinations[1] != "inbox" {
		t.Fatalf("expected trash->deleteditems then untrash->inbox, got %v", gotDestinations)
	}
}

func TestCategoriesListCreateDelete(t *testing.T) {
	var createdName, createdColor string
	var deletedID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/me/outlook/masterCategories":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(categoryListResponse{Value: []Category{{ID: "c1", DisplayName: "Newsletters"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/me/outlook/masterCategories":
			var body Category
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdName, createdColor = body.DisplayName, body.Color
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Category{ID: "c2", DisplayName: body.DisplayName, Color: body.Color})
		case r.Method == http.MethodDelete && r.URL.Path == "/me/outlook/masterCategories/c1":
			deletedID = "c1"
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	categories, err := c.ListCategories(context.Background())
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(categories) != 1 || categories[0].DisplayName != "Newsletters" {
		t.Fatalf("unexpected categories: %+v", categories)
	}

	created, err := c.CreateCategory(context.Background(), "Promo")
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if createdName != "Promo" || createdColor == "" || created.ID != "c2" {
		t.Fatalf("unexpected create: name=%q color=%q created=%+v", createdName, createdColor, created)
	}

	if err := c.DeleteCategory(context.Background(), "c1"); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}
	if deletedID != "c1" {
		t.Fatal("expected the delete endpoint to be called with c1")
	}
}

func TestModifyMessageCategoriesAddsAndRemoves(t *testing.T) {
	var gotCategories []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Message{ID: "m1", Categories: []string{"Old", "Keep"}})
			return
		}
		var body struct {
			Categories []string `json:"categories"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotCategories = body.Categories
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.ModifyMessageCategories(context.Background(), "m1", []string{"New"}, []string{"Old"}); err != nil {
		t.Fatalf("ModifyMessageCategories: %v", err)
	}
	want := map[string]bool{"Keep": true, "New": true}
	if len(gotCategories) != 2 {
		t.Fatalf("expected 2 categories after modify, got %v", gotCategories)
	}
	for _, c := range gotCategories {
		if !want[c] {
			t.Fatalf("unexpected category %q in %v", c, gotCategories)
		}
	}
}

func TestListAndGetAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me/messages/m1/attachments":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(attachmentListResponse{Value: []Attachment{{ID: "a1", Name: "file.txt", ContentType: "text/plain", Size: 5}}})
		case "/me/messages/m1/attachments/a1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"contentBytes": base64.StdEncoding.EncodeToString([]byte("hello"))})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	attachments, err := c.ListAttachments(context.Background(), "m1")
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(attachments) != 1 || attachments[0].Name != "file.txt" {
		t.Fatalf("unexpected attachments: %+v", attachments)
	}

	data, err := c.GetAttachment(context.Background(), "m1", "a1")
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected decoded bytes 'hello', got %q", data)
	}
}

func TestComposeRequiresAtLeastOneRecipient(t *testing.T) {
	c := New(httptest.NewServer(http.NewServeMux()).Client())
	if err := c.Compose(context.Background(), ComposeOptions{Subject: "Hi", Body: "Body"}); err == nil {
		t.Fatal("expected an error when no recipients are given")
	}
}

func TestComposeIncludesCcAndBcc(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.Compose(context.Background(), ComposeOptions{To: []string{"a@example.com"}, Cc: []string{"c@example.com"}, Bcc: []string{"b@example.com"}, Subject: "Hi", Body: "Body"}); err != nil {
		t.Fatalf("Compose: %v", err)
	}
	message, ok := gotBody["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected a message object, got %+v", gotBody)
	}
	if cc, ok := message["ccRecipients"].([]any); !ok || len(cc) != 1 {
		t.Fatalf("expected 1 cc recipient, got %+v", message["ccRecipients"])
	}
	if bcc, ok := message["bccRecipients"].([]any); !ok || len(bcc) != 1 {
		t.Fatalf("expected 1 bcc recipient, got %+v", message["bccRecipients"])
	}
}

func TestCreateDraftPostsToMessages(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Message{ID: "draft-1"})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	draft, err := c.CreateDraft(context.Background(), ComposeOptions{To: []string{"a@example.com"}, Subject: "Hi", Body: "Body"})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if draft.ID != "draft-1" {
		t.Fatalf("expected draft-1, got %+v", draft)
	}
	if gotPath != "/me/messages" || gotMethod != http.MethodPost {
		t.Fatalf("expected POST /me/messages, got %s %s", gotMethod, gotPath)
	}
}

func TestListDraftsUsesDraftsFolder(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mailListResponse{Value: []Message{{ID: "draft-1"}}})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	drafts, err := c.ListDrafts(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(drafts))
	}
	if gotPath != "/me/mailFolders('drafts')/messages" {
		t.Fatalf("expected the drafts folder path, got %q", gotPath)
	}
}

func TestSendAndDeleteDraft(t *testing.T) {
	var gotPaths []string
	var gotMethods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		gotMethods = append(gotMethods, r.Method)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	if err := c.SendDraft(context.Background(), "draft-1"); err != nil {
		t.Fatalf("SendDraft: %v", err)
	}
	if err := c.DeleteDraft(context.Background(), "draft-1"); err != nil {
		t.Fatalf("DeleteDraft: %v", err)
	}
	if gotPaths[0] != "/me/messages/draft-1/send" || gotMethods[0] != http.MethodPost {
		t.Fatalf("expected POST send, got %s %s", gotMethods[0], gotPaths[0])
	}
	if gotPaths[1] != "/me/messages/draft-1" || gotMethods[1] != http.MethodDelete {
		t.Fatalf("expected DELETE message, got %s %s", gotMethods[1], gotPaths[1])
	}
}

func TestGetProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Profile{DisplayName: "Jane Doe", Mail: "jane@example.com", UserPrincipalName: "jane@example.com"})
	}))
	defer server.Close()
	swapGraphBaseURLForTest(t, server)

	c := New(server.Client())
	profile, err := c.GetProfile(context.Background())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile.Mail != "jane@example.com" || profile.DisplayName != "Jane Doe" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}
