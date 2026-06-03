package read

// TextFileResult represents the result of reading a text file
type TextFileResult struct {
	// FilePath is the path to the file that was read
	FilePath string `json:"file_path"`

	// Content is the content of the file
	Content string `json:"content"`

	// NumLines is the number of lines in the returned content
	NumLines int `json:"num_lines"`

	// StartLine is the starting line number (1-indexed)
	StartLine int `json:"start_line"`

	// TotalLines is the total number of lines in the file
	TotalLines int `json:"total_lines"`

	// Truncated indicates if the output was truncated
	Truncated bool `json:"truncated,omitempty"`
}

// ImageFileResult represents the result of reading an image file
type ImageFileResult struct {
	// FilePath is the path to the file that was read
	FilePath string `json:"file_path"`

	// Base64 is the base64-encoded image data
	Base64 string `json:"base64"`

	// MimeType is the MIME type of the image
	MimeType string `json:"type"`

	// OriginalSize is the original file size in bytes
	OriginalSize int64 `json:"original_size"`

	// Dimensions contains the image dimensions (optional)
	Dimensions *ImageDimensions `json:"dimensions,omitempty"`
}

// ImageDimensions represents the dimensions of an image
type ImageDimensions struct {
	// Width is the image width in pixels
	Width int `json:"width"`

	// Height is the image height in pixels
	Height int `json:"height"`
}

// BinaryFileResult represents the result of trying to read a binary file
type BinaryFileResult struct {
	// FilePath is the path to the file
	FilePath string `json:"file_path"`

	// Reason is the reason why the file cannot be read
	Reason string `json:"reason"`

	// Suggestion is a suggested alternative tool or action
	Suggestion string `json:"suggestion,omitempty"`
}

// PDFFileResult represents the result of reading a PDF file
type PDFFileResult struct {
	// FilePath is the path to the file that was read
	FilePath string `json:"file_path"`

	// Base64 is the base64-encoded PDF data
	Base64 string `json:"base64"`

	// OriginalSize is the original file size in bytes
	OriginalSize int64 `json:"original_size"`

	// PageCount is the number of pages in the PDF
	PageCount int `json:"page_count"`
}

// PDFExtractedResult represents the result of extracting pages from a PDF
type PDFExtractedResult struct {
	// FilePath is the path to the file that was read
	FilePath string `json:"file_path"`

	// OriginalSize is the original file size in bytes
	OriginalSize int64 `json:"original_size"`

	// Count is the number of pages extracted
	Count int `json:"count"`

	// OutputDir is the directory containing extracted page images
	OutputDir string `json:"output_dir"`
}

// UnchangedFileResult represents the result when a file hasn't changed since last read
type UnchangedFileResult struct {
	// FilePath is the path to the file
	FilePath string `json:"file_path"`

	// Message indicates the file is unchanged
	Message string `json:"message"`
}

// PDFMarkdownFileResult is the result of a docling-converted PDF.
type PDFMarkdownFileResult struct {
	FilePath     string     `json:"file_path"`
	Markdown     string     `json:"markdown"`
	OriginalSize int64      `json:"original_size"`
	PageCount    int        `json:"page_count"`
	Images       []PDFImage `json:"images,omitempty"`
}

// PDFImage is one picture extracted from a converted PDF.
type PDFImage struct {
	Filename string `json:"filename"`
	MimeType string `json:"mime_type"`
	Base64   string `json:"base64"`
}

// DoclingFileResult is the result of converting a non-PDF file (DOCX, PPTX, XLSX, audio) via docling-serve.
type DoclingFileResult struct {
	FilePath     string     `json:"file_path"`
	Format       string     `json:"format"` // lowercase extension without dot, e.g. "docx"
	Markdown     string     `json:"markdown"`
	OriginalSize int64      `json:"original_size"`
	PageCount    int        `json:"page_count,omitempty"` // 0 for audio
	Images       []PDFImage `json:"images,omitempty"`
}

// FileReadResult is the union type for file read results
type FileReadResult struct {
	// Type indicates the type of result
	Type FileType `json:"type"`

	// Text contains the text file result (if Type == FileTypeText)
	Text *TextFileResult `json:"text,omitempty"`

	// Image contains the image file result (if Type == FileTypeImage)
	Image *ImageFileResult `json:"image,omitempty"`

	// PDF contains the PDF file result (if Type == FileTypePDF)
	PDF *PDFFileResult `json:"pdf,omitempty"`

	// PDFExtracted contains the PDF extracted pages result (if Type == FileTypePDFExtracted)
	PDFExtracted *PDFExtractedResult `json:"pdf_extracted,omitempty"`

	// PDFMarkdown contains the docling-converted PDF result (if Type == FileTypePDFMarkdown)
	PDFMarkdown *PDFMarkdownFileResult `json:"pdf_markdown,omitempty"`

	// Docling contains the result of a docling conversion of a non-PDF format (if Type == FileTypeDocling)
	Docling *DoclingFileResult `json:"docling,omitempty"`

	// Notebook contains the notebook result (if Type == FileTypeNotebook)
	Notebook *NotebookResult `json:"notebook,omitempty"`

	// Unchanged contains the unchanged file result (if Type == FileTypeUnchanged)
	Unchanged *UnchangedFileResult `json:"unchanged,omitempty"`

	// Binary contains the binary file result (if Type == FileTypeBinary)
	Binary *BinaryFileResult `json:"binary,omitempty"`

	// Error contains any error that occurred
	Error string `json:"error,omitempty"`
}
