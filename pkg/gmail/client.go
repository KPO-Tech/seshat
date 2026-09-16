package gmail

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/oauth2"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// Scopes requested from Google, matching what this client actually does.
// gmail.modify is a documented superset of gmail.readonly (all read/write
// operations except immutable delete and settings) - needed once this
// client started doing more than read/send: trash/untrash, mark read/
// unread, label CRUD and drafts CRUD all mutate existing mail. gmail.send
// stays alongside it since it's the scope Send/Reply/Compose actually rely
// on. These are all "restricted" scopes (Google's sensitive-scope review
// applies), which is also why each deployment registers its own Google
// Cloud OAuth client and consent screen rather than sharing a public one.
var Scopes = []string{
	"https://www.googleapis.com/auth/gmail.modify",
	"https://www.googleapis.com/auth/gmail.send",
	"https://www.googleapis.com/auth/userinfo.email",
}

// bootstrapMessageLimit caps how many messages a brand-new connection
// pulls in on its first sync - enough to seed a useful triage list without
// a multi-minute initial sync on a mailbox with years of history.
const bootstrapMessageLimit = 50

// Client wraps the official Gmail API client.
type Client struct {
	svc *gmailapi.Service
}

// New builds a Client from an oauth2.TokenSource - this package never sees
// a raw OAuth config or does its own token exchange. Extra opts are passed
// through to gmailapi.NewService, letting tests point at a fake server via
// option.WithHTTPClient/option.WithEndpoint.
func New(ctx context.Context, tokenSource oauth2.TokenSource, opts ...option.ClientOption) (*Client, error) {
	allOpts := append([]option.ClientOption{option.WithTokenSource(tokenSource)}, opts...)
	svc, err := gmailapi.NewService(ctx, allOpts...)
	if err != nil {
		return nil, err
	}
	return &Client{svc: svc}, nil
}

// Sync pulls everything new since cursor and returns it plus the cursor
// value to persist for next time. An empty cursor bootstraps (a snapshot
// of recent messages); a history-expired cursor (Gmail only retains
// history for ~7 days) transparently falls back to a fresh bootstrap
// instead of surfacing an error the caller can't act on.
func (c *Client) Sync(ctx context.Context, cursor string) ([]Message, string, error) {
	if cursor == "" {
		return c.bootstrapSync(ctx)
	}
	messages, nextCursor, err := c.incrementalSync(ctx, cursor)
	if err != nil && isHistoryExpired(err) {
		return c.bootstrapSync(ctx)
	}
	return messages, nextCursor, err
}

func (c *Client) bootstrapSync(ctx context.Context) ([]Message, string, error) {
	profile, err := c.svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return nil, "", fmt.Errorf("get profile: %w", err)
	}
	list, err := c.svc.Users.Messages.List("me").MaxResults(bootstrapMessageLimit).Context(ctx).Do()
	if err != nil {
		return nil, "", fmt.Errorf("list messages: %w", err)
	}
	messages, err := c.fetchAndTranslate(ctx, messageIDs(list.Messages))
	if err != nil {
		return nil, "", err
	}
	// The profile's HistoryId is captured after listing, so anything that
	// arrived mid-bootstrap is (at worst) re-observed on the very next
	// incremental sync - persistence on the caller's side is expected to
	// be idempotent per external message ID, so that's a no-op rather than
	// a duplicate.
	return messages, strconv.FormatUint(profile.HistoryId, 10), nil
}

func (c *Client) incrementalSync(ctx context.Context, cursor string) ([]Message, string, error) {
	startHistoryID, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid sync cursor %q: %w", cursor, err)
	}

	var ids []string
	latestHistoryID := startHistoryID
	pageToken := ""
	for {
		call := c.svc.Users.History.List("me").StartHistoryId(startHistoryID).HistoryTypes("messageAdded").Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", fmt.Errorf("list history: %w", err)
		}
		if resp.HistoryId > latestHistoryID {
			latestHistoryID = resp.HistoryId
		}
		for _, h := range resp.History {
			for _, added := range h.MessagesAdded {
				if added.Message != nil {
					ids = append(ids, added.Message.Id)
				}
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	messages, err := c.fetchAndTranslate(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	return messages, strconv.FormatUint(latestHistoryID, 10), nil
}

func (c *Client) fetchAndTranslate(ctx context.Context, ids []string) ([]Message, error) {
	messages := make([]Message, 0, len(ids))
	for _, id := range ids {
		m, err := c.getMessage(ctx, id)
		if err != nil {
			// A single unparseable message (e.g. a malformed header on some
			// automated sender) shouldn't fail the whole sync - skip it and
			// keep going.
			continue
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// getMessage fetches and translates exactly one message - the shared
// primitive behind fetchAndTranslate's loop and the exported GetMessage.
func (c *Client) getMessage(ctx context.Context, id string) (Message, error) {
	msg, err := c.svc.Users.Messages.Get("me", id).Format("full").Context(ctx).Do()
	if err != nil {
		return Message{}, fmt.Errorf("get message %s: %w", id, err)
	}
	return translateRawMessage(msg)
}

// GetMessage fetches one message by ID, fully MIME-parsed - unlike Sync,
// this targets a specific message the caller already knows about (e.g. from
// a prior Search/GetThread call), not "what's new".
func (c *Client) GetMessage(ctx context.Context, id string) (Message, error) {
	return c.getMessage(ctx, id)
}

// GetThread returns every message in a thread, in Gmail's own order
// (oldest first).
func (c *Client) GetThread(ctx context.Context, threadID string) ([]Message, error) {
	thread, err := c.svc.Users.Threads.Get("me", threadID).Format("full").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get thread %s: %w", threadID, err)
	}
	messages := make([]Message, 0, len(thread.Messages))
	for _, raw := range thread.Messages {
		m, err := translateRawMessage(raw)
		if err != nil {
			continue
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// searchMessageLimit caps how many results a single Search call returns -
// an agent-facing search tool with no cap could otherwise pull in years of
// history on a broad query.
const searchMessageLimit = 25

// Search runs a Gmail search query (the same syntax as the Gmail web UI's
// search box, e.g. "from:boss@company.com is:unread") and returns the
// matching messages, most recent first. maxResults <= 0 uses
// searchMessageLimit.
func (c *Client) Search(ctx context.Context, query string, maxResults int64) ([]Message, error) {
	if maxResults <= 0 {
		maxResults = searchMessageLimit
	}
	list, err := c.svc.Users.Messages.List("me").Q(query).MaxResults(maxResults).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return c.fetchAndTranslate(ctx, messageIDs(list.Messages))
}

// TrashMessage moves a message to Trash (Gmail permanently deletes trashed
// mail after ~30 days) - reversible via UntrashMessage until then.
func (c *Client) TrashMessage(ctx context.Context, id string) error {
	if _, err := c.svc.Users.Messages.Trash("me", id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("trash message %s: %w", id, err)
	}
	return nil
}

// UntrashMessage removes a message from Trash.
func (c *Client) UntrashMessage(ctx context.Context, id string) error {
	if _, err := c.svc.Users.Messages.Untrash("me", id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("untrash message %s: %w", id, err)
	}
	return nil
}

// unreadLabel is Gmail's system label ID for "unread" - toggling it is the
// only mechanism Gmail exposes for read/unread state, there is no separate
// boolean field to PATCH (see Message.Unread's doc comment).
const unreadLabel = "UNREAD"

// MarkRead removes the UNREAD label from a message.
func (c *Client) MarkRead(ctx context.Context, id string) error {
	return c.ModifyLabels(ctx, id, nil, []string{unreadLabel})
}

// MarkUnread adds the UNREAD label to a message.
func (c *Client) MarkUnread(ctx context.Context, id string) error {
	return c.ModifyLabels(ctx, id, []string{unreadLabel}, nil)
}

// ModifyLabels adds and/or removes label IDs on a message in one call -
// the general primitive MarkRead/MarkUnread are built on, also usable
// directly for user-created labels (see ListLabels/CreateLabel).
func (c *Client) ModifyLabels(ctx context.Context, id string, addLabelIds, removeLabelIds []string) error {
	req := &gmailapi.ModifyMessageRequest{AddLabelIds: addLabelIds, RemoveLabelIds: removeLabelIds}
	if _, err := c.svc.Users.Messages.Modify("me", id, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("modify labels on message %s: %w", id, err)
	}
	return nil
}

// ListLabels returns every label in the mailbox, system and user-created.
func (c *Client) ListLabels(ctx context.Context) ([]Label, error) {
	resp, err := c.svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list labels: %w", err)
	}
	labels := make([]Label, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		labels = append(labels, Label{ID: l.Id, Name: l.Name})
	}
	return labels, nil
}

// CreateLabel creates a new user label.
func (c *Client) CreateLabel(ctx context.Context, name string) (Label, error) {
	created, err := c.svc.Users.Labels.Create("me", &gmailapi.Label{Name: name}).Context(ctx).Do()
	if err != nil {
		return Label{}, fmt.Errorf("create label %q: %w", name, err)
	}
	return Label{ID: created.Id, Name: created.Name}, nil
}

// DeleteLabel deletes a user label (Gmail rejects deleting a system label
// with an API error, which this surfaces unchanged rather than special-
// casing).
func (c *Client) DeleteLabel(ctx context.Context, id string) error {
	if err := c.svc.Users.Labels.Delete("me", id).Context(ctx).Do(); err != nil {
		return fmt.Errorf("delete label %s: %w", id, err)
	}
	return nil
}

// TrashThread moves every message in a thread to Trash in one call - Gmail's
// own thread-level primitive (Outlook has no equivalent; a caller there
// must enumerate the conversation's messages and move each individually,
// see coremsgraph's ListMessagesInConversation).
func (c *Client) TrashThread(ctx context.Context, threadID string) error {
	if _, err := c.svc.Users.Threads.Trash("me", threadID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("trash thread %s: %w", threadID, err)
	}
	return nil
}

// UntrashThread removes every message in a thread from Trash.
func (c *Client) UntrashThread(ctx context.Context, threadID string) error {
	if _, err := c.svc.Users.Threads.Untrash("me", threadID).Context(ctx).Do(); err != nil {
		return fmt.Errorf("untrash thread %s: %w", threadID, err)
	}
	return nil
}

// ModifyThreadLabels adds and/or removes label IDs on every message in a
// thread in one call - the thread-level counterpart to ModifyLabels.
func (c *Client) ModifyThreadLabels(ctx context.Context, threadID string, addLabelIds, removeLabelIds []string) error {
	req := &gmailapi.ModifyThreadRequest{AddLabelIds: addLabelIds, RemoveLabelIds: removeLabelIds}
	if _, err := c.svc.Users.Threads.Modify("me", threadID, req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("modify labels on thread %s: %w", threadID, err)
	}
	return nil
}

// MarkThreadRead removes the UNREAD label from every message in a thread.
func (c *Client) MarkThreadRead(ctx context.Context, threadID string) error {
	return c.ModifyThreadLabels(ctx, threadID, nil, []string{unreadLabel})
}

// MarkThreadUnread adds the UNREAD label to every message in a thread.
func (c *Client) MarkThreadUnread(ctx context.Context, threadID string) error {
	return c.ModifyThreadLabels(ctx, threadID, []string{unreadLabel}, nil)
}

// GetProfile returns the connected account's own mailbox summary.
func (c *Client) GetProfile(ctx context.Context) (Profile, error) {
	profile, err := c.svc.Users.GetProfile("me").Context(ctx).Do()
	if err != nil {
		return Profile{}, fmt.Errorf("get profile: %w", err)
	}
	return Profile{EmailAddress: profile.EmailAddress, MessagesTotal: profile.MessagesTotal, ThreadsTotal: profile.ThreadsTotal}, nil
}

func messageIDs(refs []*gmailapi.Message) []string {
	ids := make([]string, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.Id)
	}
	return ids
}

func isHistoryExpired(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == 404
}
