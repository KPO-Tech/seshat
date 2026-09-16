package msgraph

import (
	"context"
	"fmt"
)

// TeamsScopes cover 1:1/group chats only for v1 - Team channel messages
// need Channel.ReadBasic.All/ChannelMessage.Send, which require
// tenant-admin consent, a materially different consent story left for a
// follow-up if actually needed.
var TeamsScopes = []string{
	"openid",
	"profile",
	"offline_access",
	"https://graph.microsoft.com/Chat.Read",
	"https://graph.microsoft.com/ChatMessage.Send",
}

// Chat is one 1:1 or group chat the connected account is a member of.
type Chat struct {
	ID    string `json:"id"`
	Topic string `json:"topic"`
}

// ChatMessage is one message within a Chat.
type ChatMessage struct {
	ID              string           `json:"id"`
	ChatID          string           `json:"chatId"`
	From            *ChatMessageFrom `json:"from,omitempty"`
	Body            *MessageBody     `json:"body,omitempty"`
	CreatedDateTime string           `json:"createdDateTime"`
	DeletedDateTime string           `json:"deletedDateTime,omitempty"`
}

type ChatMessageFrom struct {
	User *ChatMessageUser `json:"user,omitempty"`
}

type ChatMessageUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type chatsResponse struct {
	Value []Chat `json:"value"`
}

type chatMessagesResponse struct {
	Value    []ChatMessage `json:"value"`
	NextLink string        `json:"@odata.nextLink"`
}

// ListChats returns every chat the connected account is a member of.
func (c *Client) ListChats(ctx context.Context) ([]Chat, error) {
	var page chatsResponse
	url := graphBaseURL + "/me/chats?$select=id,topic"
	if err := c.get(ctx, url, &page); err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	return page.Value, nil
}

// ListChatMessages returns chatID's messages, paginated via
// @odata.nextLink only - unlike Mail/SharePoint, Teams chat messages have
// no delta endpoint, so cursor here is just "the next page link to resume
// from," not a persistent incremental-sync cursor across calls separated in
// time (a resumed sync after a gap re-lists from the most recent messages).
func (c *Client) ListChatMessages(ctx context.Context, chatID, cursor string) ([]ChatMessage, string, error) {
	url := cursor
	if url == "" {
		url = graphBaseURL + "/chats/" + chatID + "/messages?$top=50"
	}
	var page chatMessagesResponse
	if err := c.get(ctx, url, &page); err != nil {
		return nil, "", fmt.Errorf("list chat messages for chat %s: %w", chatID, err)
	}
	return page.Value, page.NextLink, nil
}

// SendChatMessage posts a new message into an existing chat.
func (c *Client) SendChatMessage(ctx context.Context, chatID, body string) error {
	url := graphBaseURL + "/chats/" + chatID + "/messages"
	payload := map[string]any{"body": MessageBody{ContentType: "Text", Content: body}}
	return c.post(ctx, url, payload, nil)
}
