package docx

import "fmt"

type Input struct {
	DocumentPath string `json:"document_path,omitempty"`
	Action       string `json:"action,omitempty"`
	Content      string `json:"content,omitempty"`
	Bold         bool   `json:"bold,omitempty"`
	Italic       bool   `json:"italic,omitempty"`
	Underline    bool   `json:"underline,omitempty"`
	Color        string `json:"color,omitempty"`
	FontSize     int    `json:"font_size,omitempty"`
	FontFamily   string `json:"font_family,omitempty"`
	Alignment    string `json:"alignment,omitempty"`
	TableData    string `json:"table_data,omitempty"`
	ImagePath    string `json:"image_path,omitempty"`
	ImageWidth   int    `json:"image_width,omitempty"`
	ImageHeight  int    `json:"image_height,omitempty"`
	Title        string `json:"title,omitempty"`
	Author       string `json:"author,omitempty"`
}

type Output struct {
	DocumentPath string `json:"document_path,omitempty"`
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
	Content      string `json:"content,omitempty"`
}

func (i *Input) Validate() error {
	if i.Action == "" {
		return fmt.Errorf("action is required")
	}

	validActions := map[string]bool{
		"create":  true,
		"append":  true,
		"replace": true,
	}
	if !validActions[i.Action] {
		return fmt.Errorf("invalid action: %s (must be create, append, or replace)", i.Action)
	}

	if i.DocumentPath == "" {
		return fmt.Errorf("document_path is required")
	}

	if i.Action == "create" && i.Content == "" && i.TableData == "" && i.ImagePath == "" {
		return fmt.Errorf("content, table_data, or image_path is required for create action")
	}

	if i.FontSize < 0 || i.FontSize > 72 {
		return fmt.Errorf("font_size must be between 0 and 72")
	}

	if i.ImageWidth < 0 || i.ImageHeight < 0 {
		return fmt.Errorf("image dimensions must be non-negative")
	}

	return nil
}

var ValidColors = []string{
	"black", "white", "red", "blue", "green", "yellow",
	"purple", "orange", "pink", "gray", "cyan", "magenta",
}

var ValidAlignments = []string{"left", "center", "right", "justify"}

func IsValidColor(color string) bool {
	for _, c := range ValidColors {
		if color == c {
			return true
		}
	}
	return false
}

func IsValidAlignment(alignment string) bool {
	for _, a := range ValidAlignments {
		if alignment == a {
			return true
		}
	}
	return false
}
