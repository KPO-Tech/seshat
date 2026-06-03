package read

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileType represents the type of file
type FileType string

const (
	FileTypeText         FileType = "text"
	FileTypeImage        FileType = "image"
	FileTypePDF          FileType = "pdf"
	FileTypePDFExtracted FileType = "pdf_extracted"
	FileTypePDFMarkdown  FileType = "pdf_markdown"
	FileTypeNotebook     FileType = "notebook"
	FileTypeUnchanged    FileType = "file_unchanged"
	FileTypeBinary       FileType = "binary"
	// FileTypeDocling represents formats converted via docling-serve (DOCX, PPTX, XLSX, audio).
	FileTypeDocling FileType = "docling"
)

// DoclingExtensions lists binary formats that require docling-serve for extraction.
// PDF has its own dedicated path (FileTypePDF); images go through the multimodal path.
// Text-based formats (.tex, .html) remain in TextExtensions and are read directly.
var DoclingExtensions = map[string]bool{
	".docx": true,
	".pptx": true,
	".xlsx": true,
	".wav":  true,
	".mp3":  true,
}

// ImageMimeTypes maps extensions to MIME types
var ImageMimeTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".svg":  "image/svg+xml",
}

// TextExtensions are common text file extensions
var TextExtensions = map[string]bool{
	".txt":        true,
	".md":         true,
	".markdown":   true,
	".json":       true,
	".xml":        true,
	".html":       true,
	".css":        true,
	".js":         true,
	".ts":         true,
	".go":         true,
	".py":         true,
	".rs":         true,
	".c":          true,
	".cpp":        true,
	".h":          true,
	".hpp":        true,
	".java":       true,
	".sh":         true,
	".bash":       true,
	".zsh":        true,
	".fish":       true,
	".yaml":       true,
	".yml":        true,
	".toml":       true,
	".ini":        true,
	".cfg":        true,
	".conf":       true,
	".log":        true,
	".csv":        true,
	".tsv":        true,
	".sql":        true,
	".php":        true,
	".rb":         true,
	".swift":      true,
	".kt":         true,
	".scala":      true,
	".dart":       true,
	".lua":        true,
	".r":          true,
	".m":          true,
	".pl":         true,
	".tcl":        true,
	".vim":        true,
	".dockerfile": true,
}

// DetectFileType detects the type of file
func DetectFileType(filePath string) (FileType, error) {
	// Check by extension first
	ext := strings.ToLower(filepath.Ext(filePath))

	// Check for PDF first
	if IsPDFExtension(ext) {
		return FileTypePDF, nil
	}

	// Check for notebooks
	if IsNotebookExtension(ext) {
		return FileTypeNotebook, nil
	}

	if ImageMimeTypes[ext] != "" {
		return FileTypeImage, nil
	}

	if TextExtensions[ext] {
		return FileTypeText, nil
	}

	// Docling-convertible binary formats (DOCX, PPTX, XLSX, audio)
	if DoclingExtensions[ext] {
		return FileTypeDocling, nil
	}

	// Read first bytes to detect binary
	file, err := os.Open(filePath)
	if err != nil {
		return FileTypeBinary, err
	}
	defer file.Close()

	// Read first 512 bytes for magic number detection
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return FileTypeBinary, err
	}

	// Check for PDF magic number (%PDF-)
	if n >= 4 {
		if buffer[0] == 0x25 && buffer[1] == 0x50 && buffer[2] == 0x44 && buffer[3] == 0x46 {
			return FileTypePDF, nil
		}
	}

	// Check for common image magic numbers
	if n >= 4 {
		// JPEG: FF D8 FF
		if buffer[0] == 0xFF && buffer[1] == 0xD8 && buffer[2] == 0xFF {
			return FileTypeImage, nil
		}
		// PNG: 89 50 4E 47
		if buffer[0] == 0x89 && buffer[1] == 0x50 && buffer[2] == 0x4E && buffer[3] == 0x47 {
			return FileTypeImage, nil
		}
		// GIF: 47 49 46 38
		if buffer[0] == 0x47 && buffer[1] == 0x49 && buffer[2] == 0x46 && buffer[3] == 0x38 {
			return FileTypeImage, nil
		}
	}

	// Check for binary content
	if isBinaryContent(buffer[:n]) {
		return FileTypeBinary, nil
	}

	return FileTypeText, nil
}

// isBinaryContent checks if content appears to be binary
func isBinaryContent(data []byte) bool {
	if len(data) == 0 {
		return false
	}

	// Check for null bytes (common in binary files)
	if bytes.Contains(data, []byte{0}) {
		return true
	}

	// Check ratio of non-text characters
	textChars := 0
	for _, b := range data {
		if isTextChar(b) {
			textChars++
		}
	}

	// If less than 80% are text characters, consider it binary
	ratio := float64(textChars) / float64(len(data))
	return ratio < 0.8
}

// isTextChar checks if a byte is a text character
func isTextChar(b byte) bool {
	// ASCII text range (32-126) plus common whitespace (9, 10, 13, 32)
	return (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13
}

// GetImageMimeType returns the MIME type for an image file
func GetImageMimeType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	return ImageMimeTypes[ext]
}

// IsTextByExtension checks if a file is likely text based on extension
func IsTextByExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return TextExtensions[ext]
}

// IsImageByExtension checks if a file is likely an image based on extension
func IsImageByExtension(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	return ImageMimeTypes[ext] != ""
}
