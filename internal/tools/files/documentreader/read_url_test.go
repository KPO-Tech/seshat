package documentreader

import (
	"context"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/documentreader"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

func TestReadURLTool_Call_ReportsNotConfiguredWithoutDocumentReader(t *testing.T) {
	tl := NewReadURLTool(Config{})
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"url": "https://example.com/paper.pdf"},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("Call returned an error result: %v", result.Error)
	}
	if !strings.Contains(result.Content, "requires a configured document reader") {
		t.Errorf("expected a 'requires a configured document reader' message, got:\n%s", result.Content)
	}
}

func TestReadURLTool_Call_RejectsMissingURL(t *testing.T) {
	tl := NewReadURLTool(Config{})
	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{},
	}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected an error for missing url")
	}
}

func TestReadURLTool_ValidateInput(t *testing.T) {
	tl := NewReadURLTool(Config{})
	ctx := context.Background()

	cases := []struct {
		name    string
		input   map[string]any
		wantErr bool
	}{
		{"missing url", map[string]any{}, true},
		{"scheme-less url", map[string]any{"url": "example.com/doc.pdf"}, true},
		{"ftp scheme", map[string]any{"url": "ftp://example.com/doc.pdf"}, true},
		{"https url", map[string]any{"url": "https://example.com/doc.pdf"}, false},
		{"http url", map[string]any{"url": "http://example.com/doc.pdf"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := tl.ValidateInput(ctx, c.input)
			if c.wantErr && err == nil {
				t.Errorf("expected an error for input %v", c.input)
			}
			if !c.wantErr && err != nil {
				t.Errorf("expected no error for input %v, got: %v", c.input, err)
			}
		})
	}
}

func TestFormatURLResult(t *testing.T) {
	out := formatURLResult("https://example.com/paper.pdf", &documentreader.ConversionResult{
		Markdown:  "# Paper\n\nAbstract here.",
		PageCount: 8,
		Images: []documentreader.ExtractedImage{
			{Filename: "image_0001.png", MimeType: "image/png", Base64: "iVBORw0KGgo"},
		},
	}, "/tmp/paper.md")

	if !strings.Contains(out, "Source: https://example.com/paper.pdf") {
		t.Errorf("expected source URL in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Pages: 8") {
		t.Errorf("expected page count in output, got:\n%s", out)
	}
	if !strings.Contains(out, "# Paper") {
		t.Errorf("expected markdown content in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Saved to: /tmp/paper.md") {
		t.Errorf("expected saved-to line in output, got:\n%s", out)
	}
}
