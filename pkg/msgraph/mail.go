package msgraph

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MailScopes are write-capable, unlike core/connectors' SharePointScopes
// (deliberately read-only, a Knowledge connector never writing back) -
// Outlook Mail is used for Inbox replies and agent-sent mail, so it needs
// Mail.Send too.
var MailScopes = []string{
	"openid",
	"profile",
	"offline_access",
	// Mail.ReadWrite (supersedes Mail.Read) - trash/untrash (move), mark
	// read/unread, categories and drafts all mutate existing messages, not
	// just read them.
	"https://graph.microsoft.com/Mail.ReadWrite",
	"https://graph.microsoft.com/Mail.Send",
	// User.Read backs GetProfile's GET /me call - the openid/profile scopes
	// above only expose id_token claims, not a callable Graph endpoint.
	"https://graph.microsoft.com/User.Read",
}

// Message is one email as returned by a mail delta page.
type Message struct {
	ID               string           `json:"id"`
	ConversationID   string           `json:"conversationId"`
	Subject          string           `json:"subject"`
	BodyPreview      string           `json:"bodyPreview"`
	Body             *MessageBody     `json:"body,omitempty"`
	From             *Recipient       `json:"from,omitempty"`
	ToRecipients     []RecipientEntry `json:"toRecipients,omitempty"`
	CcRecipients     []RecipientEntry `json:"ccRecipients,omitempty"`
	ReceivedDateTime string           `json:"receivedDateTime"`
	// IsRead/Categories/HasAttachments mirror Gmail's Message.Unread/
	// LabelIds/Attachments - Graph's read state is a plain boolean (not a
	// pseudo-label like Gmail's UNREAD), and categories are a flat
	// caller-assigned string array, not IDs from a separate resource the
	// way Gmail label IDs are (see ListCategories's doc comment).
	IsRead         bool      `json:"isRead,omitempty"`
	Categories     []string  `json:"categories,omitempty"`
	HasAttachments bool      `json:"hasAttachments,omitempty"`
	Deleted        *struct{} `json:"@removed,omitempty"`
}

type MessageBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}

type RecipientEntry struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

type Recipient struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

type EmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type mailDeltaResponse struct {
	Value     []Message `json:"value"`
	NextLink  string    `json:"@odata.nextLink"`
	DeltaLink string    `json:"@odata.deltaLink"`
}

type mailListResponse struct {
	Value []Message `json:"value"`
}

const mailFields = "id,conversationId,subject,bodyPreview,from,toRecipients,ccRecipients,receivedDateTime,isRead,categories,hasAttachments"

// ListMessagesDelta returns every inbox message changed since cursor (empty
// cursor = full enumeration, a first-ever sync), paging through
// @odata.nextLink internally and returning once @odata.deltaLink is
// reached - the caller only ever sees one logical page plus the new
// deltaLink to persist as the next cursor. Mirrors core/connectors'
// sharepoint deltaPage shape.
func (c *Client) ListMessagesDelta(ctx context.Context, cursor string) ([]Message, string, error) {
	url := cursor
	if url == "" {
		url = graphBaseURL + "/me/mailFolders/inbox/messages/delta?$select=" + mailFields
	}

	var messages []Message
	for {
		var page mailDeltaResponse
		if err := c.get(ctx, url, &page); err != nil {
			return nil, "", fmt.Errorf("mail delta page: %w", err)
		}
		messages = append(messages, page.Value...)
		if page.DeltaLink != "" {
			return messages, page.DeltaLink, nil
		}
		if page.NextLink == "" {
			return messages, cursor, nil
		}
		url = page.NextLink
	}
}

// LatestMessageInConversation returns the most recent message in a mail
// conversation (Graph's thread-equivalent) - used to build a properly
// threaded reply, since ReplyToMessage needs a specific message ID, not a
// conversation ID (what NormalizedMessage.ExternalThreadID actually stores
// for mail).
func (c *Client) LatestMessageInConversation(ctx context.Context, conversationID string) (*Message, error) {
	filter := "conversationId eq '" + strings.ReplaceAll(conversationID, "'", "''") + "'"
	q := url.Values{}
	q.Set("$filter", filter)
	q.Set("$orderby", "receivedDateTime desc")
	q.Set("$top", "1")
	q.Set("$select", mailFields)
	reqURL := graphBaseURL + "/me/mailFolders/inbox/messages?" + q.Encode()

	var page mailListResponse
	if err := c.get(ctx, reqURL, &page); err != nil {
		return nil, fmt.Errorf("find latest message in conversation %s: %w", conversationID, err)
	}
	if len(page.Value) == 0 {
		return nil, fmt.Errorf("no messages found in conversation %s", conversationID)
	}
	return &page.Value[0], nil
}

// ReplyToMessage replies to an existing message - Graph handles
// In-Reply-To/References threading natively, unlike Gmail's manual header
// construction.
func (c *Client) ReplyToMessage(ctx context.Context, messageID, body string) error {
	url := graphBaseURL + "/me/messages/" + messageID + "/reply"
	payload := map[string]string{"comment": body}
	return c.post(ctx, url, payload, nil)
}

// SendMail sends a fresh (non-reply) message.
func (c *Client) SendMail(ctx context.Context, to []string, subject, body string) error {
	recipients := make([]RecipientEntry, 0, len(to))
	for _, addr := range to {
		recipients = append(recipients, RecipientEntry{EmailAddress: EmailAddress{Address: addr}})
	}
	payload := map[string]any{
		"message": map[string]any{
			"subject":      subject,
			"body":         MessageBody{ContentType: "Text", Content: body},
			"toRecipients": recipients,
		},
		"saveToSentItems": true,
	}
	url := graphBaseURL + "/me/sendMail"
	return c.post(ctx, url, payload, nil)
}

// GetMessage fetches one message by ID with Graph's full default property
// set (unlike ListMessagesDelta's $select-trimmed page, this includes the
// full Body).
func (c *Client) GetMessage(ctx context.Context, id string) (*Message, error) {
	var msg Message
	if err := c.get(ctx, graphBaseURL+"/me/messages/"+id, &msg); err != nil {
		return nil, fmt.Errorf("get message %s: %w", id, err)
	}
	return &msg, nil
}

// searchMessageLimit caps SearchMessages the same way seshat-core/gmail's
// Search caps its own result count - an agent-facing search tool with no
// cap could otherwise pull in years of history on a broad query.
const searchMessageLimit = 25

// SearchMessages runs a free-text Graph $search query (matches subject,
// body and sender, closest Graph equivalent to Gmail's search operators -
// Graph has no from:/is:unread/has:attachment query syntax of its own, so
// this is plain full-text search, not a structured query language).
// maxResults <= 0 uses searchMessageLimit.
func (c *Client) SearchMessages(ctx context.Context, query string, maxResults int) ([]Message, error) {
	if maxResults <= 0 {
		maxResults = searchMessageLimit
	}
	q := url.Values{}
	q.Set("$search", strconv.Quote(query))
	q.Set("$top", strconv.Itoa(maxResults))
	q.Set("$select", mailFields)
	var page mailListResponse
	if err := c.get(ctx, graphBaseURL+"/me/messages?"+q.Encode(), &page); err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return page.Value, nil
}

// ListMessagesInConversation returns every message in a conversation
// (Graph's thread-equivalent), oldest first - the multi-message counterpart
// to LatestMessageInConversation's single-message lookup.
func (c *Client) ListMessagesInConversation(ctx context.Context, conversationID string) ([]Message, error) {
	filter := "conversationId eq '" + strings.ReplaceAll(conversationID, "'", "''") + "'"
	q := url.Values{}
	q.Set("$filter", filter)
	q.Set("$orderby", "receivedDateTime asc")
	q.Set("$select", mailFields)
	reqURL := graphBaseURL + "/me/mailFolders/inbox/messages?" + q.Encode()

	var page mailListResponse
	if err := c.get(ctx, reqURL, &page); err != nil {
		return nil, fmt.Errorf("list messages in conversation %s: %w", conversationID, err)
	}
	return page.Value, nil
}

// SetMessageRead sets a message's read/unread state.
func (c *Client) SetMessageRead(ctx context.Context, id string, read bool) error {
	if err := c.patch(ctx, graphBaseURL+"/me/messages/"+id, map[string]bool{"isRead": read}); err != nil {
		return fmt.Errorf("set message %s read=%v: %w", id, read, err)
	}
	return nil
}

// moveResponse is the moved message Graph returns from a move call - only
// its (possibly new) ID matters to TrashMessage/UntrashMessage's callers.
type moveResponse struct {
	ID string `json:"id"`
}

// MoveMessage moves a message to a well-known or custom mail folder,
// returning the moved message's ID (Graph mints a new one on move) - the
// primitive TrashMessage/UntrashMessage are built on.
func (c *Client) MoveMessage(ctx context.Context, id, destinationFolderID string) (string, error) {
	var moved moveResponse
	if err := c.post(ctx, graphBaseURL+"/me/messages/"+id+"/move", map[string]string{"destinationId": destinationFolderID}, &moved); err != nil {
		return "", fmt.Errorf("move message %s to %s: %w", id, destinationFolderID, err)
	}
	return moved.ID, nil
}

// TrashMessage moves a message to the well-known Deleted Items folder -
// Graph has no separate trash flag the way Gmail does, so this (like
// UntrashMessage) is a MoveMessage call, and the returned ID differs from
// the original (see MoveMessage's doc comment).
func (c *Client) TrashMessage(ctx context.Context, id string) (string, error) {
	return c.MoveMessage(ctx, id, "deleteditems")
}

// UntrashMessage moves a message from Deleted Items back to the Inbox.
func (c *Client) UntrashMessage(ctx context.Context, id string) (string, error) {
	return c.MoveMessage(ctx, id, "inbox")
}

// Category is an Outlook "master category" - the closest Graph equivalent
// to a Gmail label, but structurally different: it's a flat, org-wide named
// color swatch (ListCategories/CreateCategory/DeleteCategory manage the
// swatch itself), and a message's Categories field is a plain string array
// of names Graph replaces wholesale on PATCH, not an add/remove of IDs the
// way Gmail's ModifyMessageRequest works (see ModifyMessageCategories).
type Category struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName"`
	Color       string `json:"color,omitempty"`
}

type categoryListResponse struct {
	Value []Category `json:"value"`
}

// ListCategories returns every master category defined for this mailbox.
func (c *Client) ListCategories(ctx context.Context) ([]Category, error) {
	var page categoryListResponse
	if err := c.get(ctx, graphBaseURL+"/me/outlook/masterCategories", &page); err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return page.Value, nil
}

// defaultCategoryColor is Graph's first preset color swatch - Color is a
// required field on create, and callers (an agent-facing tool included)
// shouldn't need to know Graph's "presetN" color naming just to create a
// category.
const defaultCategoryColor = "preset0"

// CreateCategory creates a new master category.
func (c *Client) CreateCategory(ctx context.Context, name string) (Category, error) {
	var created Category
	if err := c.post(ctx, graphBaseURL+"/me/outlook/masterCategories", Category{DisplayName: name, Color: defaultCategoryColor}, &created); err != nil {
		return Category{}, fmt.Errorf("create category %q: %w", name, err)
	}
	return created, nil
}

// DeleteCategory deletes a master category.
func (c *Client) DeleteCategory(ctx context.Context, id string) error {
	if err := c.delete(ctx, graphBaseURL+"/me/outlook/masterCategories/"+id); err != nil {
		return fmt.Errorf("delete category %s: %w", id, err)
	}
	return nil
}

// ModifyMessageCategories adds and/or removes category names on a message -
// Gmail-like add/remove ergonomics layered over Graph's actual full-replace
// PATCH (see Category's doc comment): reads the message's current
// Categories, computes the new set, then PATCHes the whole array. Not
// atomic against a concurrent modification (Graph offers no partial-update
// primitive here), acceptable for an agent-driven triage action.
func (c *Client) ModifyMessageCategories(ctx context.Context, id string, addCategories, removeCategories []string) error {
	msg, err := c.GetMessage(ctx, id)
	if err != nil {
		return fmt.Errorf("modify categories on message %s: %w", id, err)
	}
	remove := make(map[string]bool, len(removeCategories))
	for _, name := range removeCategories {
		remove[name] = true
	}
	next := make([]string, 0, len(msg.Categories)+len(addCategories))
	seen := make(map[string]bool, len(msg.Categories)+len(addCategories))
	for _, name := range msg.Categories {
		if remove[name] || seen[name] {
			continue
		}
		seen[name] = true
		next = append(next, name)
	}
	for _, name := range addCategories {
		if seen[name] {
			continue
		}
		seen[name] = true
		next = append(next, name)
	}
	if err := c.patch(ctx, graphBaseURL+"/me/messages/"+id, map[string][]string{"categories": next}); err != nil {
		return fmt.Errorf("modify categories on message %s: %w", id, err)
	}
	return nil
}

// Attachment is one attachment's metadata (no bytes - see
// Client.GetAttachment for the on-demand download).
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Size        int    `json:"size"`
}

type attachmentListResponse struct {
	Value []Attachment `json:"value"`
}

// ListAttachments returns attachment metadata for a message.
func (c *Client) ListAttachments(ctx context.Context, messageID string) ([]Attachment, error) {
	var page attachmentListResponse
	if err := c.get(ctx, graphBaseURL+"/me/messages/"+messageID+"/attachments?$select=id,name,contentType,size", &page); err != nil {
		return nil, fmt.Errorf("list attachments for message %s: %w", messageID, err)
	}
	return page.Value, nil
}

// GetAttachment downloads one attachment's bytes. Unlike Gmail (a separate
// download-by-ID call returning base64url), Graph's file attachment
// resource carries its bytes as standard base64 inline on the same
// attachment object - fetched here via $select to avoid also pulling every
// other attachment's bytes that a plain ListAttachments call would.
func (c *Client) GetAttachment(ctx context.Context, messageID, attachmentID string) ([]byte, error) {
	var raw struct {
		ContentBytes string `json:"contentBytes"`
	}
	url := graphBaseURL + "/me/messages/" + messageID + "/attachments/" + attachmentID + "?$select=contentBytes"
	if err := c.get(ctx, url, &raw); err != nil {
		return nil, fmt.Errorf("get attachment %s on message %s: %w", attachmentID, messageID, err)
	}
	data, err := base64.StdEncoding.DecodeString(raw.ContentBytes)
	if err != nil {
		return nil, fmt.Errorf("decode attachment data: %w", err)
	}
	return data, nil
}

// ComposeOptions is the CC/BCC-capable header set Compose/CreateDraft
// accept - SendMail's minimal to/subject/body is kept as-is since existing
// callers (inbox/outlook's connector) only need that shape. Unlike Gmail's
// ComposeOptions, there is no ThreadID: Graph has no client-settable
// "send within this thread" field on message creation - conversationId is
// server-assigned from the Subject/In-Reply-To Graph infers, not something
// a caller can request.
type ComposeOptions struct {
	To, Cc, Bcc []string
	Subject     string
	Body        string
}

func composeMessagePayload(opts ComposeOptions) map[string]any {
	toRecipients := make([]RecipientEntry, 0, len(opts.To))
	for _, addr := range opts.To {
		toRecipients = append(toRecipients, RecipientEntry{EmailAddress: EmailAddress{Address: addr}})
	}
	ccRecipients := make([]RecipientEntry, 0, len(opts.Cc))
	for _, addr := range opts.Cc {
		ccRecipients = append(ccRecipients, RecipientEntry{EmailAddress: EmailAddress{Address: addr}})
	}
	bccRecipients := make([]RecipientEntry, 0, len(opts.Bcc))
	for _, addr := range opts.Bcc {
		bccRecipients = append(bccRecipients, RecipientEntry{EmailAddress: EmailAddress{Address: addr}})
	}
	return map[string]any{
		"subject":       opts.Subject,
		"body":          MessageBody{ContentType: "Text", Content: opts.Body},
		"toRecipients":  toRecipients,
		"ccRecipients":  ccRecipients,
		"bccRecipients": bccRecipients,
	}
}

// Compose sends a message with CC/BCC, unlike SendMail's minimal shape.
func (c *Client) Compose(ctx context.Context, opts ComposeOptions) error {
	if len(opts.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	payload := map[string]any{"message": composeMessagePayload(opts), "saveToSentItems": true}
	if err := c.post(ctx, graphBaseURL+"/me/sendMail", payload, nil); err != nil {
		return fmt.Errorf("compose message: %w", err)
	}
	return nil
}

// CreateDraft saves a message as a draft instead of sending it - a plain
// POST /me/messages creates a draft by default in Graph, unlike Gmail's
// dedicated Drafts.Create endpoint.
func (c *Client) CreateDraft(ctx context.Context, opts ComposeOptions) (*Message, error) {
	if len(opts.To) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	var created Message
	if err := c.post(ctx, graphBaseURL+"/me/messages", composeMessagePayload(opts), &created); err != nil {
		return nil, fmt.Errorf("create draft: %w", err)
	}
	return &created, nil
}

// ListDrafts returns messages in the well-known Drafts folder.
func (c *Client) ListDrafts(ctx context.Context, top int) ([]Message, error) {
	if top <= 0 {
		top = listDraftsLimit
	}
	q := url.Values{}
	q.Set("$top", strconv.Itoa(top))
	q.Set("$select", mailFields)
	var page mailListResponse
	if err := c.get(ctx, graphBaseURL+"/me/mailFolders('drafts')/messages?"+q.Encode(), &page); err != nil {
		return nil, fmt.Errorf("list drafts: %w", err)
	}
	return page.Value, nil
}

// listDraftsLimit caps ListDrafts the same way searchMessageLimit caps
// SearchMessages.
const listDraftsLimit = 25

// SendDraft sends a previously saved draft.
func (c *Client) SendDraft(ctx context.Context, draftID string) error {
	if err := c.post(ctx, graphBaseURL+"/me/messages/"+draftID+"/send", nil, nil); err != nil {
		return fmt.Errorf("send draft %s: %w", draftID, err)
	}
	return nil
}

// DeleteDraft discards a draft without sending it.
func (c *Client) DeleteDraft(ctx context.Context, draftID string) error {
	if err := c.delete(ctx, graphBaseURL+"/me/messages/"+draftID); err != nil {
		return fmt.Errorf("delete draft %s: %w", draftID, err)
	}
	return nil
}

// Profile is the connected account's own mailbox identity.
type Profile struct {
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

// GetProfile returns the connected account's own identity.
func (c *Client) GetProfile(ctx context.Context) (Profile, error) {
	var profile Profile
	q := url.Values{}
	q.Set("$select", "displayName,mail,userPrincipalName")
	if err := c.get(ctx, graphBaseURL+"/me?"+q.Encode(), &profile); err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return profile, nil
}
