package workspace

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/message"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

func TestPersistEphemeralSessionAttachmentsWritesToSessionPastes(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	attachments := []message.Attachment{
		{FilePath: "paste_1.txt", FileName: "paste_1.txt", MimeType: "text/plain", Content: []byte("hello")},
		{FilePath: "paste_2.png", FileName: "paste_2.png", MimeType: "image/png", Content: []byte("png")},
		{FilePath: "/tmp/existing.txt", FileName: "existing.txt", MimeType: "text/plain", Content: []byte("keep")},
	}

	persisted, err := persistEphemeralSessionAttachments("session-1", attachments)
	if err != nil {
		t.Fatalf("persistEphemeralSessionAttachments: %v", err)
	}

	textPath := persisted[0].FilePath
	if dir := filepath.Dir(textPath); dir != runtimepath.SessionPastesTextDir("", "session-1") {
		t.Fatalf("expected text paste dir %q, got %q", runtimepath.SessionPastesTextDir("", "session-1"), dir)
	}
	textBody, err := os.ReadFile(textPath)
	if err != nil {
		t.Fatalf("read persisted text paste: %v", err)
	}
	if string(textBody) != "hello" {
		t.Fatalf("unexpected persisted text body %q", string(textBody))
	}

	imagePath := persisted[1].FilePath
	if dir := filepath.Dir(imagePath); dir != runtimepath.SessionPastesImagesDir("", "session-1") {
		t.Fatalf("expected image paste dir %q, got %q", runtimepath.SessionPastesImagesDir("", "session-1"), dir)
	}
	imageBody, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatalf("read persisted image paste: %v", err)
	}
	if string(imageBody) != "png" {
		t.Fatalf("unexpected persisted image body %q", string(imageBody))
	}

	if got := persisted[2].FilePath; got != "/tmp/existing.txt" {
		t.Fatalf("expected non-ephemeral attachment path to stay unchanged, got %q", got)
	}
}

func TestConvertSDKMessagesRestoresUserAttachments(t *testing.T) {
	attachedText := message.PromptWithTextAttachments("review this", []message.Attachment{{
		FilePath: filepath.Join("/tmp", "session-1", "pastes", "text", "paste_1.txt"),
		FileName: "paste_1.txt",
		MimeType: "text/plain",
		Content:  []byte("file body"),
	}})
	image := sdk.ImageContent{}
	image.Source.Type = "base64"
	image.Source.MediaType = "image/png"
	image.Source.Data = base64.StdEncoding.EncodeToString([]byte("png-bytes"))

	msgs := convertSDKMessages("session-1", []sdk.Message{{
		ID:        sdk.MessageID("msg-1"),
		Role:      sdk.RoleUser,
		Timestamp: time.Now(),
		Content: []sdk.ContentBlock{
			sdk.TextContent{Text: attachedText},
			image,
		},
	}})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 converted message, got %d", len(msgs))
	}
	msg := msgs[0]
	if got := msg.Content().Text; got != "review this" {
		t.Fatalf("expected cleaned prompt text, got %q", got)
	}
	binary := msg.BinaryContent()
	if len(binary) != 2 {
		t.Fatalf("expected 2 attachments after conversion, got %d", len(binary))
	}
	if got := string(binary[0].Data); got != "file body" {
		t.Fatalf("expected restored text attachment body, got %q", got)
	}
	if got := filepath.Base(binary[0].Path); got != "paste_1.txt" {
		t.Fatalf("expected restored text attachment path, got %q", got)
	}
	if got := binary[1].MIMEType; got != "image/png" {
		t.Fatalf("expected restored image mime type, got %q", got)
	}
	if got := string(binary[1].Data); got != "png-bytes" {
		t.Fatalf("expected restored image payload, got %q", got)
	}
}
