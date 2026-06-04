package model

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Attachment represents a file attached to a message.
type Attachment struct {
	Path     string
	MimeType string
	Data     []byte
}

// attachments manages the file attachment strip above the input.
// Adapted from crush's ui/attachments/attachments.go.
type attachments struct {
	styles   Styles
	list     []Attachment
	deleting bool // delete mode — next key removes an item
}

func newAttachments(styles Styles) *attachments {
	return &attachments{styles: styles}
}

func (a *attachments) Add(att Attachment) {
	a.list = append(a.list, att)
}

func (a *attachments) AddPath(path string) {
	a.list = append(a.list, Attachment{Path: path})
}

func (a *attachments) List() []Attachment { return a.list }
func (a *attachments) Count() int          { return len(a.list) }
func (a *attachments) Reset()              { a.list = nil; a.deleting = false }

// EnterDeleteMode activates attachment-delete mode.
func (a *attachments) EnterDeleteMode() {
	if len(a.list) > 0 {
		a.deleting = true
	}
}

// ExitDeleteMode cancels delete mode without removing anything.
func (a *attachments) ExitDeleteMode() { a.deleting = false }

// DeleteAll removes all attachments and exits delete mode.
func (a *attachments) DeleteAll() {
	a.list = nil
	a.deleting = false
}

// DeleteLast removes the last attachment.
func (a *attachments) DeleteLast() {
	if len(a.list) > 0 {
		a.list = a.list[:len(a.list)-1]
	}
	if len(a.list) == 0 {
		a.deleting = false
	}
}

// View renders the attachment pills strip. Returns "" when there are no attachments.
func (a *attachments) View(width int) string {
	if len(a.list) == 0 {
		return ""
	}

	var pills []string
	for i, att := range a.list {
		name := filepath.Base(att.Path)
		const maxLen = 20
		if len(name) > maxLen {
			name = name[:maxLen-1] + "…"
		}

		var pill string
		if a.deleting {
			label := fmt.Sprintf("[%d] %s ×", i+1, name)
			pill = a.styles.ToolError.Render(label)
		} else {
			label := fmt.Sprintf("📎 %s", name)
			pill = a.styles.BrowserSelected.Render(label)
		}
		pills = append(pills, pill)
	}

	line := strings.Join(pills, "  ")
	if a.deleting {
		line += "  " + a.styles.MsgTimestamp.Render("(backspace: remove last, esc: cancel)")
	}

	// Truncate if too wide
	if len(line) > width-2 {
		line = line[:width-5] + "…"
	}

	return a.styles.MsgTimestamp.Render("  ") + line
}
