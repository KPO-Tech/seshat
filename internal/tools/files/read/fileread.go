package read

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/docling"
	"github.com/EngineerProjects/nexus-engine/internal/sandbox"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// Tool represents the FileRead tool
type Tool struct {
	// config is the tool configuration
	config *ToolConfig

	// filesystemPolicy centralizes common path access checks.
	filesystemPolicy *sandbox.FilesystemPolicy

	// doclingClient converts PDFs to structured markdown when set.
	// When nil the tool falls back to base64 pass-through.
	doclingClient *docling.Client
}

// ToolConfig represents the FileRead tool configuration
type ToolConfig struct {
	// MaxFileSize is the maximum file size to read
	MaxFileSize int64

	// MaxImageSize is the maximum image size to read
	MaxImageSize int64

	// DefaultLimit is the default number of lines to read
	DefaultLimit int

	// MaxLimit is the maximum number of lines to read
	MaxLimit int

	// DoclingURL is the base URL of a running docling-serve instance.
	// When non-empty a Client is created and PDFs are converted to markdown.
	// Example: "http://localhost:5001"
	DoclingURL string
}

// DefaultToolConfig returns default tool configuration
func DefaultToolConfig() *ToolConfig {
	return &ToolConfig{
		MaxFileSize:  MaxFileSize,
		MaxImageSize: MaxImageSize,
		DefaultLimit: DefaultLimit,
		MaxLimit:     MaxLimit,
	}
}

// NewTool creates a new FileRead tool
func NewTool(config *ToolConfig) *Tool {
	if config == nil {
		config = DefaultToolConfig()
	}

	t := &Tool{
		config:           config,
		filesystemPolicy: sandbox.NewDefaultFilesystemPolicy(),
	}
	if config.DoclingURL != "" {
		t.doclingClient = docling.NewClient(config.DoclingURL)
	}
	return t
}

// Definition returns the tool definition
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Read File",
		SearchHint:  SearchHint,
		Description: Description,
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_path": map[string]any{
					"type":        "string",
					"description": "The absolute path to the file to read",
				},
				"offset": map[string]any{
					"type":        "number",
					"description": "The line number to start reading from (1-indexed). Only provide if the file is too large to read at once.",
				},
				"limit": map[string]any{
					"type":        "number",
					"description": "The number of lines to read. Only provide if the file is too large to read at once.",
				},
				"pages": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("Page range for PDF files (e.g., \"1-5\", \"3\", \"10-20\"). Only applicable to PDF files. Maximum %d pages per request.", MaxPagesPerRead),
				},
			},
			"required": []string{"file_path"},
		}),
		IsReadOnly:         true,
		IsConcurrencySafe:  true,
		IsDestructive:      false,
		RequiresPermission: true,
	}
}

// Call executes the tool
func (t *Tool) Call(
	ctx context.Context,
	input tool.CallInput,
	permissionCheck types.CanUseToolFn,
) (tool.CallResult, error) {
	directCall := input.ToolContext == nil
	// Extract file path
	filePath, ok := input.Parsed["file_path"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return tool.NewErrorResult(fmt.Errorf("file_path is required and must be a string")), nil
	}
	filePath, err := resolveFilePath(filePath, input.ToolContextValue())
	if err != nil {
		return tool.NewErrorResult(err), nil
	}

	// Step 1: Security validation - check if path is blocked
	if t.isBlockedDevicePath(filePath) {
		return tool.NewErrorResult(fmt.Errorf("cannot read device path: %s", filePath)), nil
	}

	// Step 2: Path validation
	err = t.validateReadPath(input.ToolContextValue(), filePath)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("path validation failed: %w", err)), nil
	}
	if directCall && permissionCheck != nil {
		req := sandbox.PermissionRequest{
			ToolName:      ToolName,
			Environment:   sandbox.EnvironmentLocal,
			Access:        sandbox.AccessRead,
			Paths:         []string{filePath},
			Justification: "Read file contents",
			Scope:         sandbox.ApprovalScopeToolCall,
			Metadata: map[string]any{
				"pages": input.Parsed["pages"],
			},
		}
		toolCtx := input.ToolContextValue()
		permissionResult, err := sandbox.ResolveToolPermission(ctx, permissionCheck, req, sandbox.ToolPermissionOptions{
			ToolInput: map[string]any{
				"file_path": filePath,
				"offset":    input.Parsed["offset"],
				"limit":     input.Parsed["limit"],
				"pages":     input.Parsed["pages"],
			},
			ToolUseID:              toolCtx.ToolUseID,
			SessionID:              toolCtx.SessionID,
			TurnID:                 toolCtx.TurnID,
			PermissionMode:         toolCtx.PermissionMode,
			WorkingDirectory:       strings.TrimSpace(toolCtx.WorkingDirectory),
			IsToolRunningInSandbox: toolCtx.EnableSandbox,
		})
		if err != nil {
			return tool.NewErrorResult(err), nil
		}
		if err := sandbox.ErrorForPermissionResult(permissionResult, "file read requires approval"); err != nil {
			return tool.NewErrorResult(err), nil
		}
	}

	// Step 3: Validate pages parameter (if provided)
	pagesParam := ""
	if p, ok := input.Parsed["pages"].(string); ok && p != "" {
		pagesParam = p
		// Parse and validate page range
		parsedRange, err := ParsePDFPageRange(pagesParam)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("invalid pages parameter: %w", err)), nil
		}

		// Check page range size
		if parsedRange.LastPage == -1 {
			// "to end" - count pages
			pageCount, err := GetPDFPageCount(filePath)
			if err != nil {
				return tool.NewErrorResult(fmt.Errorf("failed to get PDF page count: %w", err)), nil
			}
			rangeSize := pageCount - parsedRange.FirstPage + 1
			if rangeSize > MaxPagesPerRead {
				return tool.NewErrorResult(fmt.Errorf("page range \"%s\" exceeds maximum of %d pages per request. Please use a smaller range.", pagesParam, MaxPagesPerRead)), nil
			}
		} else {
			rangeSize := parsedRange.LastPage - parsedRange.FirstPage + 1
			if rangeSize > MaxPagesPerRead {
				return tool.NewErrorResult(fmt.Errorf("page range \"%s\" exceeds maximum of %d pages per request. Please use a smaller range.", pagesParam, MaxPagesPerRead)), nil
			}
		}
	}

	// Step 5: Check if file exists and get info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to provide helpful suggestions
			workingDir, _ := os.Getwd()
			return tool.NewErrorResult(FormatNotFoundError(filePath, workingDir)), nil
		}
		return tool.NewErrorResult(fmt.Errorf("failed to access file: %w", err)), nil
	}

	// Step 6: Detect file type
	fileType, err := DetectFileType(filePath)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to detect file type: %w", err)), nil
	}

	// Step 6.5: Check deduplication for text files
	if fileType == FileTypeText {
		// Parse offset and limit for cache key
		offset := 0
		limit := 0
		if offsetVal, ok := input.Parsed["offset"].(float64); ok {
			offset = int(offsetVal)
		}
		if limitVal, ok := input.Parsed["limit"].(float64); ok {
			limit = int(limitVal)
		}

		// Check if file has been read before with same parameters
		if _, found := CheckFileUnchanged(filePath, offset, limit); found {
			// File unchanged - return cached result
			result := &FileReadResult{
				Type: FileTypeUnchanged,
				Unchanged: &UnchangedFileResult{
					FilePath: filePath,
					Message:  "File unchanged since last read (using cached content)",
				},
			}
			return tool.NewTextResult(t.formatUnchangedResult(result)), nil
		}
	}

	// Step 7: Read based on file type
	switch fileType {
	case FileTypePDF:
		return t.readPDFFile(ctx, filePath, fileInfo, pagesParam)
	case FileTypeNotebook:
		return t.readNotebookFile(ctx, filePath, fileInfo)
	case FileTypeText:
		return t.readTextFile(ctx, filePath, fileInfo, input.Parsed)
	case FileTypeImage:
		return t.readImageFile(ctx, filePath, fileInfo)
	case FileTypeDocling:
		return t.readDoclingFile(ctx, filePath, fileInfo)
	default:
		return t.handleBinaryFile(filePath)
	}
}

// readTextFile reads a text file with optional offset/limit
func (t *Tool) readTextFile(
	ctx context.Context,
	filePath string,
	fileInfo os.FileInfo,
	parsed map[string]any,
) (tool.CallResult, error) {
	// Parse offset and limit
	offset := 0
	limit := t.config.DefaultLimit

	if offsetVal, ok := parsed["offset"].(float64); ok {
		offset = int(offsetVal)
		if offset < 1 {
			offset = 1
		}
	}

	if limitVal, ok := parsed["limit"].(float64); ok {
		limit = int(limitVal)
		if limit < 1 {
			limit = t.config.DefaultLimit
		}
		if limit > t.config.MaxLimit {
			limit = t.config.MaxLimit
		}
	}

	// Check file size
	if fileInfo.Size() > t.config.MaxFileSize {
		return tool.NewErrorResult(fmt.Errorf("file too large (%d bytes, max %d bytes)", fileInfo.Size(), t.config.MaxFileSize)), nil
	}

	// Read file with cancellation support
	content, lineCount, totalLines, _, _, _, err := ReadFileInRange(
		ctx,
		filePath,
		offset,
		limit,
		t.config.MaxFileSize,
	)

	if err != nil {
		// Check if it's a cancellation error
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		}
		return tool.NewErrorResult(fmt.Errorf("failed to read file: %w", err)), nil
	}

	// Calculate truncation
	isTruncated := limit > 0 && lineCount == limit && totalLines > offset+lineCount

	// Create result
	result := &FileReadResult{
		Type: FileTypeText,
		Text: &TextFileResult{
			FilePath:   filePath,
			Content:    content,
			NumLines:   lineCount,
			StartLine:  offset,
			TotalLines: totalLines,
			Truncated:  isTruncated,
		},
	}

	// Cache the result for deduplication
	isPartialView := offset != 0 || limit != t.config.DefaultLimit
	CacheFileRead(filePath, content, fileInfo, offset, limit, isPartialView)

	return tool.NewTextResult(t.formatTextResult(result)), nil
}

// readImageFile reads an image file with processing and token validation
func (t *Tool) readImageFile(
	ctx context.Context,
	filePath string,
	fileInfo os.FileInfo,
) (tool.CallResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
	default:
	}

	// Check file size
	if fileInfo.Size() > t.config.MaxImageSize {
		return tool.NewErrorResult(fmt.Errorf("image too large (%d bytes, max %d bytes)", fileInfo.Size(), t.config.MaxImageSize)), nil
	}

	// Process image with resize/compression and token budget
	processed, err := ReadAndProcessImage(filePath, DefaultMaxImageTokens)
	if err != nil {
		// Check if it's a cancellation error
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		}
		return tool.NewErrorResult(fmt.Errorf("failed to process image: %w", err)), nil
	}

	// Create result
	result := &FileReadResult{
		Type: FileTypeImage,
		Image: &ImageFileResult{
			FilePath:     filePath,
			Base64:       processed.Base64,
			MimeType:     processed.MimeType,
			OriginalSize: processed.OriginalSize,
			Dimensions:   processed.Dimensions,
		},
	}

	return tool.NewTextResult(t.formatImageResult(result)), nil
}

// handleBinaryFile handles binary files that cannot be read
func (t *Tool) handleBinaryFile(filePath string) (tool.CallResult, error) {
	result := &FileReadResult{
		Type: FileTypeBinary,
		Binary: &BinaryFileResult{
			FilePath:   filePath,
			Reason:     "Binary files cannot be displayed as text",
			Suggestion: "Use a specialized tool or download the file to view its contents",
		},
	}

	return tool.NewTextResult(t.formatBinaryResult(result)), nil
}

// readPDFFile reads a PDF file.
// When a docling client is configured and reachable it converts the PDF to
// structured markdown (preserving tables, images, headings).  Otherwise it
// falls back to base64 pass-through so the model can at least read the raw PDF.
func (t *Tool) readPDFFile(
	ctx context.Context,
	filePath string,
	fileInfo os.FileInfo,
	pagesParam string,
) (tool.CallResult, error) {
	select {
	case <-ctx.Done():
		return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
	default:
	}

	if fileInfo.Size() > t.config.MaxFileSize {
		return tool.NewErrorResult(fmt.Errorf("PDF too large (%d bytes, max %d bytes)", fileInfo.Size(), t.config.MaxFileSize)), nil
	}

	// Docling path: convert to markdown.
	if t.doclingClient != nil && t.doclingClient.IsAvailable(ctx) {
		conversion, err := t.doclingClient.ConvertFile(ctx, filePath)
		if err != nil {
			if ctx.Err() != nil {
				return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
			}
			// Docling failed — fall through to the base64 path.
			goto fallback
		}
		images := make([]PDFImage, 0, len(conversion.Images))
		for _, img := range conversion.Images {
			images = append(images, PDFImage{
				Filename: img.Filename,
				MimeType: img.MimeType,
				Base64:   img.Base64,
			})
		}
		result := &FileReadResult{
			Type: FileTypePDFMarkdown,
			PDFMarkdown: &PDFMarkdownFileResult{
				FilePath:     filePath,
				Markdown:     conversion.Markdown,
				OriginalSize: fileInfo.Size(),
				PageCount:    conversion.PageCount,
				Images:       images,
			},
		}
		return tool.NewTextResult(t.formatPDFMarkdownResult(result)), nil
	}

fallback:
	// Base64 path (no docling).
	if pagesParam != "" {
		parsedRange, err := ParsePDFPageRange(pagesParam)
		if err != nil {
			return tool.NewErrorResult(fmt.Errorf("failed to parse page range: %w", err)), nil
		}
		select {
		case <-ctx.Done():
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		default:
		}
		extractResult, err := ExtractPDFPages(filePath, parsedRange)
		if err != nil {
			if ctx.Err() != nil {
				return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
			}
			return tool.NewErrorResult(fmt.Errorf("failed to extract PDF pages: %w", err)), nil
		}
		result := &FileReadResult{
			Type: FileTypePDFExtracted,
			PDFExtracted: &PDFExtractedResult{
				FilePath:     filePath,
				OriginalSize: extractResult.OriginalSize,
				Count:        extractResult.Count,
				OutputDir:    extractResult.OutputDir,
			},
		}
		return tool.NewTextResult(t.formatPDFExtractedResult(result)), nil
	}

	pageCount, err := GetPDFPageCount(filePath)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to get PDF page count: %w", err)), nil
	}
	if pageCount > PDFATMentionInlineThreshold {
		return tool.NewErrorResult(fmt.Errorf("this PDF has %d pages, which is too many to read at once. Use the pages parameter to read specific page ranges (e.g., pages: \"1-5\"). Maximum %d pages per request.", pageCount, MaxPagesPerRead)), nil
	}

	pdfResult, err := ReadPDF(filePath)
	if err != nil {
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		}
		return tool.NewErrorResult(fmt.Errorf("failed to read PDF: %w", err)), nil
	}
	result := &FileReadResult{
		Type: FileTypePDF,
		PDF: &PDFFileResult{
			FilePath:     filePath,
			Base64:       pdfResult.Base64,
			OriginalSize: pdfResult.OriginalSize,
			PageCount:    pdfResult.PageCount,
		},
	}
	return tool.NewTextResult(t.formatPDFResult(result)), nil
}

// readDoclingFile converts a docling-supported binary file (DOCX, PPTX, XLSX, WAV, MP3)
// to markdown via docling-serve. When docling is not configured it returns a descriptive
// message so the agent understands why the file can't be read directly.
func (t *Tool) readDoclingFile(
	ctx context.Context,
	filePath string,
	fileInfo os.FileInfo,
) (tool.CallResult, error) {
	select {
	case <-ctx.Done():
		return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
	default:
	}

	if fileInfo.Size() > t.config.MaxFileSize {
		return tool.NewErrorResult(fmt.Errorf("file too large (%d bytes, max %d bytes)", fileInfo.Size(), t.config.MaxFileSize)), nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	format := strings.TrimPrefix(ext, ".")

	if t.doclingClient == nil || !t.doclingClient.IsAvailable(ctx) {
		return tool.NewTextResult(fmt.Sprintf(
			"File: %s\nFormat: %s | Size: %d bytes\n\nThis file format requires docling-serve for text extraction. Configure the DOCLING_URL setting to enable automatic conversion of DOCX, PPTX, XLSX, and audio transcription.",
			filePath, strings.ToUpper(format), fileInfo.Size(),
		)), nil
	}

	conversion, err := t.doclingClient.ConvertFile(ctx, filePath)
	if err != nil {
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		}
		return tool.NewErrorResult(fmt.Errorf("docling conversion failed for %s: %w", strings.ToUpper(format), err)), nil
	}

	images := make([]PDFImage, 0, len(conversion.Images))
	for _, img := range conversion.Images {
		images = append(images, PDFImage{
			Filename: img.Filename,
			MimeType: img.MimeType,
			Base64:   img.Base64,
		})
	}

	result := &FileReadResult{
		Type: FileTypeDocling,
		Docling: &DoclingFileResult{
			FilePath:     filePath,
			Format:       format,
			Markdown:     conversion.Markdown,
			OriginalSize: fileInfo.Size(),
			PageCount:    conversion.PageCount,
			Images:       images,
		},
	}
	return tool.NewTextResult(t.formatDoclingResult(result)), nil
}

// readNotebookFile reads a Jupyter notebook file
func (t *Tool) readNotebookFile(
	ctx context.Context,
	filePath string,
	fileInfo os.FileInfo,
) (tool.CallResult, error) {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
	default:
	}

	// Check file size
	if fileInfo.Size() > t.config.MaxFileSize {
		return tool.NewErrorResult(fmt.Errorf("notebook too large (%d bytes, max %d bytes)", fileInfo.Size(), t.config.MaxFileSize)), nil
	}

	// Read and parse notebook
	notebook, err := ReadNotebook(filePath)
	if err != nil {
		// Check if it's a cancellation error
		if ctx.Err() != nil {
			return tool.NewErrorResult(fmt.Errorf("file read cancelled")), nil
		}
		return tool.NewErrorResult(fmt.Errorf("failed to read notebook: %w", err)), nil
	}

	// Parse notebook into result
	notebookResult, err := ParseNotebook(notebook, filePath)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("failed to parse notebook: %w", err)), nil
	}

	// Create result
	result := &FileReadResult{
		Type:     FileTypeNotebook,
		Notebook: notebookResult,
	}

	return tool.NewTextResult(t.formatNotebookResult(result)), nil
}

// Helper methods

func (t *Tool) isBlockedDevicePath(filePath string) bool {
	// Check exact matches
	if BlockedDevicePaths[filePath] {
		return true
	}

	// Check /proc paths for stdio
	if strings.HasPrefix(filePath, "/proc/") && strings.HasSuffix(filePath, "/fd/0") {
		return true
	}
	if strings.HasPrefix(filePath, "/proc/") && strings.HasSuffix(filePath, "/fd/1") {
		return true
	}
	if strings.HasPrefix(filePath, "/proc/") && strings.HasSuffix(filePath, "/fd/2") {
		return true
	}

	return false
}

func (t *Tool) formatTextResult(result *FileReadResult) string {
	// Use the enhanced formatting with line numbers
	formatted := FormatTextWithLineNumbers(result, false) // false = padded format

	// Add cyber risk reminder if applicable
	if ShouldIncludeCyberRiskReminder() {
		formatted += CyberRiskMitigationReminder
	}

	return formatted
}

func (t *Tool) formatImageResult(result *FileReadResult) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("Image: %s\n", result.Image.FilePath))
	builder.WriteString(fmt.Sprintf("Type: %s\n", result.Image.MimeType))
	builder.WriteString(fmt.Sprintf("Size: %d bytes\n", result.Image.OriginalSize))

	if result.Image.Dimensions != nil {
		builder.WriteString(fmt.Sprintf("Dimensions: %dx%d\n", result.Image.Dimensions.Width, result.Image.Dimensions.Height))
	}

	builder.WriteString(fmt.Sprintf("\n[data:image/%s;base64,%s]", result.Image.MimeType, result.Image.Base64))

	return builder.String()
}

func (t *Tool) formatBinaryResult(result *FileReadResult) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("File: %s\n", result.Binary.FilePath))
	builder.WriteString(fmt.Sprintf("Reason: %s\n", result.Binary.Reason))

	if result.Binary.Suggestion != "" {
		builder.WriteString(fmt.Sprintf("Suggestion: %s\n", result.Binary.Suggestion))
	}

	return builder.String()
}

func (t *Tool) formatPDFResult(result *FileReadResult) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("PDF: %s\n", result.PDF.FilePath))
	builder.WriteString(fmt.Sprintf("Pages: %d\n", result.PDF.PageCount))
	builder.WriteString(fmt.Sprintf("Size: %d bytes\n", result.PDF.OriginalSize))
	builder.WriteString(fmt.Sprintf("\n[data:application/pdf;base64,%s]", result.PDF.Base64))

	return builder.String()
}

func (t *Tool) formatPDFMarkdownResult(result *FileReadResult) string {
	r := result.PDFMarkdown
	var b strings.Builder
	b.WriteString(fmt.Sprintf("PDF: %s\n", r.FilePath))
	b.WriteString(fmt.Sprintf("Pages: %d | Size: %d bytes\n", r.PageCount, r.OriginalSize))
	if len(r.Images) > 0 {
		b.WriteString(fmt.Sprintf("Extracted images: %d\n", len(r.Images)))
		for _, img := range r.Images {
			b.WriteString(fmt.Sprintf("  - %s (%s) [data:%s;base64,%s]\n",
				img.Filename, img.MimeType, img.MimeType, img.Base64))
		}
	}
	b.WriteString("\n")
	b.WriteString(r.Markdown)
	return b.String()
}

func (t *Tool) formatDoclingResult(result *FileReadResult) string {
	r := result.Docling
	var b strings.Builder
	b.WriteString(fmt.Sprintf("File: %s\n", r.FilePath))
	b.WriteString(fmt.Sprintf("Format: %s | Size: %d bytes", strings.ToUpper(r.Format), r.OriginalSize))
	if r.PageCount > 0 {
		b.WriteString(fmt.Sprintf(" | Pages: %d", r.PageCount))
	}
	b.WriteString("\n")
	if len(r.Images) > 0 {
		b.WriteString(fmt.Sprintf("Extracted images: %d\n", len(r.Images)))
		for _, img := range r.Images {
			b.WriteString(fmt.Sprintf("  - %s (%s) [data:%s;base64,%s]\n",
				img.Filename, img.MimeType, img.MimeType, img.Base64))
		}
	}
	b.WriteString("\n")
	b.WriteString(r.Markdown)
	return b.String()
}

func (t *Tool) formatPDFExtractedResult(result *FileReadResult) string {
	var builder strings.Builder

	builder.WriteString(fmt.Sprintf("PDF pages extracted: %d page(s) from %s (%d bytes)\n",
		result.PDFExtracted.Count,
		result.PDFExtracted.FilePath,
		result.PDFExtracted.OriginalSize))

	return builder.String()
}

func (t *Tool) formatNotebookResult(result *FileReadResult) string {
	return FormatNotebookResult(result.Notebook)
}

func (t *Tool) formatUnchangedResult(result *FileReadResult) string {
	return fmt.Sprintf("File: %s\n%s", result.Unchanged.FilePath, result.Unchanged.Message)
}

// Description returns a human-readable description
func (t *Tool) Description(ctx context.Context) (string, error) {
	return "Read file contents with support for text, images, and automatic type detection", nil
}

// ValidateInput validates and normalizes file-read input.
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	_ = ctx
	filePath, ok := input["file_path"].(string)
	if !ok || strings.TrimSpace(filePath) == "" {
		return nil, fmt.Errorf("file_path is required and must be a string")
	}

	normalized := make(map[string]any, len(input))
	for k, v := range input {
		normalized[k] = v
	}
	if offset, ok := normalized["offset"].(float64); ok && offset < 0 {
		normalized["offset"] = float64(0)
	}
	return normalized, nil
}

// CheckPermissions performs file-read-specific permission checks before the global pipeline.
func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	_ = ctx
	filePath, _ := input["file_path"].(string)
	if strings.TrimSpace(filePath) == "" {
		return types.Deny("file_path is required and must be a string")
	}
	resolvedPath, err := resolveFilePath(filePath, toolCtx)
	if err != nil {
		return types.Deny(err.Error())
	}
	filePath = resolvedPath
	if t.isBlockedDevicePath(filePath) {
		return types.Deny(fmt.Sprintf("cannot read device path: %s", filePath))
	}
	if err := t.validateReadPath(toolCtx, filePath); err != nil {
		return types.Deny(fmt.Sprintf("path validation failed: %v", err))
	}
	return types.Passthrough(input)
}

func (t *Tool) validateReadPath(toolCtx tool.ToolUseContext, path string) error {
	ctx := sandbox.Context{
		WorkingDirectory: strings.TrimSpace(toolCtx.WorkingDirectory),
		Environment:      sandbox.EnvironmentLocal,
		SandboxEnabled:   toolCtx.EnableSandbox,
	}
	if toolCtx.Workspace != nil {
		ctx.WorkspaceRoot = toolCtx.Workspace.Root
	}
	for _, dir := range toolCtx.AdditionalWorkingDirectories {
		if strings.TrimSpace(dir) != "" {
			ctx.AdditionalRoots = append(ctx.AdditionalRoots, dir)
		}
	}

	decision, err := t.filesystemPolicy.EvaluatePath(ctx, path, sandbox.AccessRead)
	if err != nil {
		return err
	}
	if decision.Decision == sandbox.DecisionDeny {
		return errors.New(decision.Reason)
	}
	return nil
}

func resolveFilePath(path string, toolCtx tool.ToolUseContext) (string, error) {
	if toolCtx.Workspace != nil {
		return toolCtx.Workspace.Resolve(path)
	}
	workingDir := strings.TrimSpace(toolCtx.WorkingDirectory)
	if filepath.IsAbs(path) || workingDir == "" {
		return path, nil
	}
	return filepath.Join(workingDir, path), nil
}

// IsConcurrencySafe returns whether this tool use can run concurrently.
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	_ = input
	return true
}

// IsReadOnly returns whether this tool use is read-only.
func (t *Tool) IsReadOnly(input map[string]any) bool {
	_ = input
	return true
}

// IsEnabled returns whether this tool is currently active.
func (t *Tool) IsEnabled() bool { return true }

// FormatResult serialises the tool output into the tool_result content string.
func (t *Tool) FormatResult(data any) string {
	if s, ok := data.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", data)
}

// BackfillInput enriches a shallow clone of the parsed input with derived fields.
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// ── Tool metadata ──────────────────────────────────────────────────────────────

// ToolName is the name of the file read tool.
const ToolName = "read_file"

// Description is the description of the file read tool.
var Description = fmt.Sprintf(`Read the contents of a file. Supports text files, images, PDFs, and - when docling-serve is configured - DOCX, PPTX, XLSX documents and audio transcription (WAV, MP3). For large text files, use offset/limit to read specific ranges. For PDFs, use the pages parameter to read specific page ranges.

Reads a file from the local filesystem. You can access any file directly by using this tool. Assume this tool is able to read all files on the machine. If the user provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to %d lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files), but it is recommended to read the whole file by not providing these parameters
- Results are returned using cat -n format, with line numbers starting at 1
- When you already know which part of the file you need, only read that part. This can be important for larger files.
- This tool allows reading images (eg PNG, JPG, etc). When reading an image file the contents are presented visually.
- This tool can read PDF files (.pdf). For large PDFs (more than 10 pages), you must provide the pages parameter to read specific page ranges (e.g., pages: "1-5"). Reading a large PDF without the pages parameter will fail. Maximum 20 pages per request.
- This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.
- When docling-serve is configured, this tool can convert DOCX, PPTX, XLSX documents and transcribe WAV/MP3 audio files to markdown automatically.
- This tool can only read files, not directories. To read a directory, use an ls command via the Bash tool.
- You will regularly be asked to read screenshots. If the user provides a path to a screenshot, always use this tool to view the file at the path.
- If you read a file that exists but has empty contents you will receive a warning in place of file contents.`, MaxLinesToRead)

// SearchHint is a hint for tool search functionality.
const SearchHint = "read files from the filesystem"

// MaxLinesToRead is the maximum number of lines to read.
const MaxLinesToRead = 2000
