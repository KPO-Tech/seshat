package components

import (
	"fmt"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
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
type Attachments struct {
	styles   common.Styles
	list     []Attachment
	deleting bool // delete mode — next key removes an item
}

func NewAttachments(styles common.Styles) *Attachments {
	return &Attachments{styles: styles}
}

func (a *Attachments) Add(att Attachment) {
	a.list = append(a.list, att)
}

func (a *Attachments) AddPath(path string) {
	a.list = append(a.list, Attachment{Path: path})
}

func (a *Attachments) List() []Attachment { return a.list }
func (a *Attachments) Count() int         { return len(a.list) }
func (a *Attachments) Reset()             { a.list = nil; a.deleting = false }

// EnterDeleteMode activates attachment-delete mode.
func (a *Attachments) EnterDeleteMode() {
	if len(a.list) > 0 {
		a.deleting = true
	}
}

// ExitDeleteMode cancels delete mode without removing anything.
func (a *Attachments) ExitDeleteMode() { a.deleting = false }

// DeleteAll removes all attachments and exits delete mode.
func (a *Attachments) DeleteAll() {
	a.list = nil
	a.deleting = false
}

// DeleteLast removes the last attachment.
func (a *Attachments) DeleteLast() {
	if len(a.list) > 0 {
		a.list = a.list[:len(a.list)-1]
	}
	if len(a.list) == 0 {
		a.deleting = false
	}
}

// View renders the attachment pills strip. Returns "" when there are no attachments.
func (a *Attachments) View(width int) string {
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
