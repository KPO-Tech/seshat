package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Channel is a multiplexed WebSocket connection to a Jupyter kernel.
// It implements the Jupyter Messaging Protocol v5 over a single WebSocket.
type Channel struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	session string
}

// dialChannel opens a WebSocket channel to the kernel at kernelID.
func dialChannel(ctx context.Context, cfg Config, kernelID string) (*Channel, error) {
	u, err := kernelWsURL(cfg.ServerURL, kernelID)
	if err != nil {
		return nil, fmt.Errorf("build websocket URL: %w", err)
	}

	headers := http.Header{}
	if cfg.Token != "" {
		headers.Set("Authorization", "token "+cfg.Token)
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u, headers)
	if err != nil {
		return nil, fmt.Errorf("websocket dial %s: %w", u, err)
	}

	return &Channel{conn: conn, session: uuid.New().String()}, nil
}

// Close closes the WebSocket connection.
func (ch *Channel) Close() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	_ = ch.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	ch.conn.Close()
}

// Execute sends an execute_request for code and collects outputs until the
// kernel becomes idle or timeout elapses. Returns parsed outputs.
func (ch *Channel) Execute(ctx context.Context, code string, timeout time.Duration) ([]Output, error) {
	msgID := uuid.New().String()
	req := ch.buildMessage("execute_request", "shell", map[string]any{
		"code":             code,
		"silent":           false,
		"store_history":    true,
		"user_expressions": map[string]any{},
		"allow_stdin":      false,
		"stop_on_error":    true,
	}, msgID)

	ch.mu.Lock()
	err := ch.conn.WriteJSON(req)
	ch.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("send execute_request: %w", err)
	}

	deadline := time.Now().Add(timeout)
	var outputs []Output

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return outputs, fmt.Errorf("execution timeout after %s", timeout)
		}
		if err := ch.conn.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			return outputs, fmt.Errorf("set read deadline: %w", err)
		}

		var msg jupyterMessage
		if err := ch.conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				break
			}
			return outputs, fmt.Errorf("read message: %w", err)
		}

		// Only process messages that belong to our request.
		if msg.ParentHeader.MsgID != msgID {
			continue
		}

		switch msg.Header.MsgType {
		case "stream":
			var c streamContent
			if err := json.Unmarshal(msg.Content, &c); err == nil && c.Text != "" {
				outputs = append(outputs, Output{Type: "stream", Name: c.Name, Text: c.Text})
			}

		case "execute_result":
			var c resultContent
			if err := json.Unmarshal(msg.Content, &c); err == nil {
				text := extractText(c.Data)
				if text != "" {
					outputs = append(outputs, Output{Type: "execute_result", Text: text, Data: c.Data})
				}
			}

		case "display_data":
			var c resultContent
			if err := json.Unmarshal(msg.Content, &c); err == nil {
				out := Output{Type: "display_data", Data: c.Data}
				if img, ok := c.Data["image/png"].(string); ok && img != "" {
					out.ImagePNG = img
				} else if img, ok := c.Data["image/jpeg"].(string); ok && img != "" {
					out.ImageJPEG = img
				} else {
					out.Text = extractText(c.Data)
				}
				outputs = append(outputs, out)
			}

		case "error":
			var c errorContent
			if err := json.Unmarshal(msg.Content, &c); err == nil {
				outputs = append(outputs, Output{
					Type: "error",
					Text: fmt.Sprintf("%s: %s\n%s", c.Ename, c.Evalue, strings.Join(stripAnsi(c.Traceback), "\n")),
				})
			}

		case "execute_reply":
			// Final reply: execution is complete regardless of status.
			return outputs, nil

		case "status":
			// idle signals the kernel finished, but execute_reply is the authoritative
			// completion message — keep reading until we see it.
		}

		if ctx.Err() != nil {
			return outputs, ctx.Err()
		}
	}
	return outputs, nil
}

// ─── Message building ─────────────────────────────────────────────────────────

type jupyterMessage struct {
	Header       msgHeader       `json:"header"`
	ParentHeader msgHeader       `json:"parent_header"`
	Metadata     json.RawMessage `json:"metadata"`
	Content      json.RawMessage `json:"content"`
	Channel      string          `json:"channel"`
}

type msgHeader struct {
	MsgID    string `json:"msg_id"`
	Session  string `json:"session"`
	Username string `json:"username"`
	Date     string `json:"date"`
	MsgType  string `json:"msg_type"`
	Version  string `json:"version"`
}

func (ch *Channel) buildMessage(msgType, channel string, content any, msgID string) jupyterMessage {
	raw, _ := json.Marshal(content)
	return jupyterMessage{
		Header: msgHeader{
			MsgID:    msgID,
			Session:  ch.session,
			Username: "nexus",
			Date:     time.Now().UTC().Format(time.RFC3339),
			MsgType:  msgType,
			Version:  "5.3",
		},
		ParentHeader: msgHeader{},
		Metadata:     json.RawMessage("{}"),
		Content:      raw,
		Channel:      channel,
	}
}

// ─── Content types ────────────────────────────────────────────────────────────

type streamContent struct {
	Name string `json:"name"` // "stdout" or "stderr"
	Text string `json:"text"`
}

type resultContent struct {
	ExecutionCount int            `json:"execution_count"`
	Data           map[string]any `json:"data"`
	Metadata       map[string]any `json:"metadata"`
}

type errorContent struct {
	Ename     string   `json:"ename"`
	Evalue    string   `json:"evalue"`
	Traceback []string `json:"traceback"`
}

type statusContent struct {
	ExecutionState string `json:"execution_state"`
}

// ─── URL helper ───────────────────────────────────────────────────────────────

func kernelWsURL(serverURL, kernelID string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/kernels/" + kernelID + "/channels"
	return u.String(), nil
}
