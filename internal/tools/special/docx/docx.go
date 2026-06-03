package docx

import (
	"context"
	"fmt"
	"os"
	"strings"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	docx "github.com/mmonterroca/docxgo/v2"
	"github.com/mmonterroca/docxgo/v2/domain"
)

const Description = `Creates and modifies Microsoft Word (.docx) documents. Supports creating new documents, adding formatted text, tables, and images.

Creates and modifies Microsoft Word (.docx) documents.

This tool supports four actions:
- create: Create a new .docx document with content
- read: Read and display the content of an existing document
- append: Add new content to an existing document
- replace: Replace the first paragraph's content in an existing document

Features supported:
- Text formatting (bold, italic, underline, color, font size)
- Paragraph alignment (left, center, right, justify)
- Tables with tab-separated data
- Images (PNG, JPEG, GIF, etc.)
- Document metadata (title, author/creator)

Usage Examples:

Example 1 - Create a simple document:
  document_path: /home/user/report.docx
  action: create
  content: "Hello World - My First Document"
  bold: true
  font_size: 16

Example 2 - Create document with metadata:
  document_path: /home/user/report.docx
  action: create
  content: "Annual Report 2024"
  title: "Annual Report 2024"
  author: "John Doe"
  alignment: center
  bold: true
  font_size: 24

Example 3 - Create document with table:
  document_path: /home/user/table.docx
  action: create
  table_data: "Name\tAge\tCity\nAlice\t30\tNYC\nBob\t25\tLA"

Example 4 - Append content to existing document:
  document_path: /home/user/report.docx
  action: append
  content: "This is additional content."
  italic: true
  color: blue

Example 5 - Read document content:
  document_path: /home/user/report.docx
  action: read

Example 6 - Add image to document:
  document_path: /home/user/report.docx
  action: create
  content: "Report with image"
  image_path: /home/user/image.png
  image_width: 4
  image_height: 3

Parameters:
- document_path: Path to the .docx file (required)
- action: Action to perform: create, read, append, replace (required)
- content: Text content to add
- bold: Apply bold formatting (default: false)
- italic: Apply italic formatting (default: false)
- underline: Apply underline formatting (default: false)
- color: Text color (black, white, red, blue, green, yellow, purple, orange, pink, gray)
- font_size: Font size in points (1-72)
- font_family: Font family (e.g., Arial, Calibri)
- alignment: Text alignment (left, center, right, justify)
- table_data: Tab-separated table data (rows separated by newlines)
- image_path: Path to image file to insert
- image_width: Image width in inches
- image_height: Image height in inches
- title: Document title metadata
- author: Document author (Creator) metadata`

type Tool struct {
	workingDir string
}

func NewDocxTool(workingDir string) *Tool {
	if workingDir == "" {
		wd, _ := os.Getwd()
		workingDir = wd
	}
	return &Tool{workingDir: workingDir}
}

func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:               ToolName,
		DisplayName:        DisplayName,
		SearchHint:         SearchHint,
		Description:        Description,
		Category:           "filesystem",
		IsReadOnly:         false,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: true,
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"document_path": map[string]any{
					"type":        "string",
					"description": "Path to the .docx file (required)",
				},
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform: create, append, replace",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Text content to add to the document",
				},
				"bold": map[string]any{
					"type":        "boolean",
					"description": "Apply bold formatting",
				},
				"italic": map[string]any{
					"type":        "boolean",
					"description": "Apply italic formatting",
				},
				"underline": map[string]any{
					"type":        "boolean",
					"description": "Apply underline formatting",
				},
				"color": map[string]any{
					"type":        "string",
					"description": "Text color: black, white, red, blue, green, yellow, purple, orange, pink, gray",
				},
				"font_size": map[string]any{
					"type":        "number",
					"description": "Font size in points (1-72)",
				},
				"font_family": map[string]any{
					"type":        "string",
					"description": "Font family (e.g., Arial, Calibri, Times New Roman)",
				},
				"alignment": map[string]any{
					"type":        "string",
					"description": "Text alignment: left, center, right, justify",
				},
				"table_data": map[string]any{
					"type":        "string",
					"description": "Tab-separated table data (rows separated by newlines)",
				},
				"image_path": map[string]any{
					"type":        "string",
					"description": "Path to image file to insert",
				},
				"image_width": map[string]any{
					"type":        "number",
					"description": "Image width in inches",
				},
				"image_height": map[string]any{
					"type":        "number",
					"description": "Image height in inches",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Document title metadata",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Document author metadata (Creator)",
				},
			},
			"required": []string{"document_path", "action"},
		}),
	}
}

func (t *Tool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsedInput, err := parseCallInput(input.Parsed)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}
	if err := parsedInput.Validate(); err != nil {
		return tool.NewErrorResult(err), nil
	}

	result, err := t.executeAction(parsedInput)
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	return tool.NewTextResult(result), nil
}

func (t *Tool) executeAction(input *Input) (string, error) {
	switch input.Action {
	case "create":
		return t.createDocument(input)
	case "append":
		return t.appendToDocument(input)
	case "replace":
		return t.replaceContent(input)
	default:
		return "", fmt.Errorf("unknown action: %s", input.Action)
	}
}

func (t *Tool) createDocument(input *Input) (string, error) {
	builder := docx.NewDocumentBuilder()

	if input.Title != "" || input.Author != "" {
		meta := &domain.Metadata{}
		if input.Title != "" {
			meta.Title = input.Title
		}
		if input.Author != "" {
			meta.Creator = input.Author
		}
		builder.SetMetadata(meta)
	}

	if input.Content != "" {
		para := builder.AddParagraph()
		para.Text(input.Content)
		applyTextFormatting(para, input)
		para.End()
	}

	if input.TableData != "" {
		rows := strings.Split(strings.TrimSpace(input.TableData), "\n")
		if len(rows) > 0 {
			cols := strings.Split(rows[0], "\t")
			tb := builder.AddTable(len(rows), len(cols))
			for r, row := range rows {
				cells := strings.Split(row, "\t")
				rb := tb.Row(r)
				for c := 0; c < len(cols); c++ {
					cellText := ""
					if c < len(cells) {
						cellText = cells[c]
					}
					cb := rb.Cell(c)
					cb.Text(cellText)
					cb.End()
				}
				rb.End()
			}
			tb.End()
		}
	}

	if input.ImagePath != "" {
		width := float64(input.ImageWidth)
		height := float64(input.ImageHeight)
		if width == 0 {
			width = 4.0
		}
		if height == 0 {
			height = 3.0
		}
		size := domain.NewImageSizeInches(width, height)
		builder.AddParagraph().AddImageWithSize(input.ImagePath, size).End()
	}

	doc, err := builder.Build()
	if err != nil {
		return "", fmt.Errorf("failed to build document: %w", err)
	}

	if err := doc.SaveAs(input.DocumentPath); err != nil {
		return "", fmt.Errorf("failed to save document: %w", err)
	}

	return formatOutput(Output{
		DocumentPath: input.DocumentPath,
		Success:      true,
		Message:      "Document created successfully",
	}), nil
}

func (t *Tool) appendToDocument(input *Input) (string, error) {
	doc, err := docx.OpenDocument(input.DocumentPath)
	if err != nil {
		doc = docx.NewDocument()
	}

	para, err := doc.AddParagraph()
	if err != nil {
		return "", fmt.Errorf("failed to add paragraph: %w", err)
	}

	run, err := para.AddRun()
	if err != nil {
		return "", fmt.Errorf("failed to add run: %w", err)
	}
	if err := run.SetText(input.Content); err != nil {
		return "", fmt.Errorf("failed to set text: %w", err)
	}

	applyTextFormattingToRun(run, input)

	if err := doc.SaveAs(input.DocumentPath); err != nil {
		return "", fmt.Errorf("failed to save document: %w", err)
	}

	return formatOutput(Output{
		DocumentPath: input.DocumentPath,
		Success:      true,
		Message:      "Content appended successfully",
	}), nil
}

func (t *Tool) replaceContent(input *Input) (string, error) {
	doc, err := docx.OpenDocument(input.DocumentPath)
	if err != nil {
		return "", fmt.Errorf("failed to open document: %w", err)
	}

	paragraphs := doc.Paragraphs()
	if len(paragraphs) == 0 {
		return "", fmt.Errorf("document has no paragraphs to replace")
	}

	firstPara := paragraphs[0]
	runs := firstPara.Runs()
	if len(runs) == 0 {
		run, _ := firstPara.AddRun()
		run.SetText(input.Content)
	} else {
		runs[0].SetText(input.Content)
	}

	if err := doc.SaveAs(input.DocumentPath); err != nil {
		return "", fmt.Errorf("failed to save document: %w", err)
	}

	return formatOutput(Output{
		DocumentPath: input.DocumentPath,
		Success:      true,
		Message:      "Content replaced successfully",
	}), nil
}

func applyTextFormatting(p *docx.ParagraphBuilder, input *Input) {
	if input.Bold {
		p.Bold()
	}
	if input.Italic {
		p.Italic()
	}
	if input.Underline {
		p.Underline(domain.UnderlineSingle)
	}
	if input.Color != "" {
		p.Color(getColor(input.Color))
	}
	if input.FontSize > 0 {
		p.FontSize(input.FontSize)
	}
	if input.Alignment != "" {
		p.Alignment(getAlignment(input.Alignment))
	}
}

func applyTextFormattingToRun(r domain.Run, input *Input) {
	if input.Bold {
		r.SetBold(true)
	}
	if input.Italic {
		r.SetItalic(true)
	}
	if input.Underline {
		r.SetUnderline(domain.UnderlineSingle)
	}
	if input.Color != "" {
		r.SetColor(getColor(input.Color))
	}
	if input.FontSize > 0 {
		r.SetSize(input.FontSize * 2)
	}
}

func getColor(colorName string) domain.Color {
	pink := domain.Color{R: 255, G: 192, B: 203}
	colorMap := map[string]domain.Color{
		"black":   docx.Black,
		"white":   docx.White,
		"red":     docx.Red,
		"blue":    docx.Blue,
		"green":   docx.Green,
		"yellow":  docx.Yellow,
		"purple":  docx.Purple,
		"orange":  docx.Orange,
		"pink":    pink,
		"gray":    docx.Gray,
		"cyan":    docx.Cyan,
		"magenta": docx.Magenta,
	}
	if c, ok := colorMap[strings.ToLower(colorName)]; ok {
		return c
	}
	return docx.Black
}

func getAlignment(alignment string) domain.Alignment {
	switch strings.ToLower(alignment) {
	case "center":
		return domain.AlignmentCenter
	case "right":
		return domain.AlignmentRight
	case "justify":
		return domain.AlignmentDistribute
	default:
		return domain.AlignmentLeft
	}
}

func parseCallInput(parsed map[string]any) (*Input, error) {
	input := &Input{}

	if v, ok := parsed["document_path"].(string); ok {
		input.DocumentPath = v
	}
	if v, ok := parsed["action"].(string); ok {
		input.Action = v
	}
	if v, ok := parsed["content"].(string); ok {
		input.Content = v
	}
	if v, ok := parsed["bold"].(bool); ok {
		input.Bold = v
	}
	if v, ok := parsed["italic"].(bool); ok {
		input.Italic = v
	}
	if v, ok := parsed["underline"].(bool); ok {
		input.Underline = v
	}
	if v, ok := parsed["color"].(string); ok {
		input.Color = v
	}
	if v, ok := parsed["font_size"].(float64); ok {
		input.FontSize = int(v)
	}
	if v, ok := parsed["font_family"].(string); ok {
		input.FontFamily = v
	}
	if v, ok := parsed["alignment"].(string); ok {
		input.Alignment = v
	}
	if v, ok := parsed["table_data"].(string); ok {
		input.TableData = v
	}
	if v, ok := parsed["image_path"].(string); ok {
		input.ImagePath = v
	}
	if v, ok := parsed["image_width"].(float64); ok {
		input.ImageWidth = int(v)
	}
	if v, ok := parsed["image_height"].(float64); ok {
		input.ImageHeight = int(v)
	}
	if v, ok := parsed["title"].(string); ok {
		input.Title = v
	}
	if v, ok := parsed["author"].(string); ok {
		input.Author = v
	}

	return input, nil
}

func formatOutput(output Output) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Document: %s\n", output.DocumentPath))
	b.WriteString(fmt.Sprintf("Success: %v\n", output.Success))
	if output.Message != "" {
		b.WriteString(fmt.Sprintf("Message: %s\n", output.Message))
	}
	if output.Content != "" {
		b.WriteString(fmt.Sprintf("Content:\n%s\n", output.Content))
	}
	return b.String()
}

func (t *Tool) IsEnabled() bool {
	return true
}

func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	parsed, err := parseCallInput(input)
	if err != nil {
		return nil, err
	}
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	return input, nil
}

func (t *Tool) IsReadOnly(input map[string]any) bool {
	return false
}

func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, ctx2 tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{
		Behavior: types.PermissionBehaviorPassthrough,
	}
}

func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

func (t *Tool) FormatResult(result any) string {
	if s, ok := result.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", result)
}

func (t *Tool) Description(ctx context.Context) (string, error) {
	return Description, nil
}
