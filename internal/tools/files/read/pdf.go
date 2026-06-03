package read

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDF constants
const (
	// MaxPagesPerRead is the maximum number of pages per read
	MaxPagesPerRead = 50

	// PDFATMentionInlineThreshold is the page count threshold for inline PDF reading
	PDFATMentionInlineThreshold = 20
)

// PDFPageRange represents a parsed PDF page range
type PDFPageRange struct {
	FirstPage int
	LastPage  int
	// LastPage can be -1 to mean "to the end"
}

// PDFResult represents the result of reading a PDF
type PDFResult struct {
	FilePath     string
	Base64       string
	OriginalSize int64
	PageCount    int
}

// PDFExtractionResult represents the result of extracting PDF pages
type PDFExtractionResult struct {
	FilePath     string
	OriginalSize int64
	Count        int
	OutputDir    string
}

// ParsePDFPageRange parses a PDF page range string
// Supports formats: "1-5", "3", "10-20", "5-" (5 to end)
func ParsePDFPageRange(pages string) (*PDFPageRange, error) {
	pages = strings.TrimSpace(pages)

	// Single page: "3"
	if !strings.Contains(pages, "-") {
		pageNum, err := strconv.Atoi(pages)
		if err != nil {
			return nil, fmt.Errorf("invalid page number: %s", pages)
		}
		if pageNum < 1 {
			return nil, fmt.Errorf("page numbers must be >= 1, got: %d", pageNum)
		}
		return &PDFPageRange{
			FirstPage: pageNum,
			LastPage:  pageNum,
		}, nil
	}

	// Range: "1-5", "10-", "3-end"
	parts := strings.SplitN(pages, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid page range format: %s", pages)
	}

	firstPage, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid first page number: %s", parts[0])
	}
	if firstPage < 1 {
		return nil, fmt.Errorf("page numbers must be >= 1, got: %d", firstPage)
	}

	lastPart := strings.TrimSpace(parts[1])
	if lastPart == "" || lastPart == "end" {
		// "5-" or "5-end" means to the end
		return &PDFPageRange{
			FirstPage: firstPage,
			LastPage:  -1, // -1 means "to the end"
		}, nil
	}

	lastPage, err := strconv.Atoi(lastPart)
	if err != nil {
		return nil, fmt.Errorf("invalid last page number: %s", lastPart)
	}
	if lastPage < 1 {
		return nil, fmt.Errorf("page numbers must be >= 1, got: %d", lastPage)
	}
	if lastPage < firstPage {
		return nil, fmt.Errorf("last page (%d) must be >= first page (%d)", lastPage, firstPage)
	}

	return &PDFPageRange{
		FirstPage: firstPage,
		LastPage:  lastPage,
	}, nil
}

// IsPDFExtension checks if a file extension is a PDF extension
func IsPDFExtension(ext string) bool {
	return strings.ToLower(ext) == ".pdf"
}

// ReadPDF reads a PDF file and returns base64-encoded data
func ReadPDF(filePath string) (*PDFResult, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	// Validate PDF and get page count
	ctx, err := pdfcpu.Read(bytes.NewReader(data), &model.Configuration{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse PDF: %w", err)
	}

	// Get page count
	pageCount := ctx.XRefTable.PageCount

	// Encode to base64
	base64Data := base64StdEncode(data)

	return &PDFResult{
		FilePath:     filePath,
		Base64:       base64Data,
		OriginalSize: int64(len(data)),
		PageCount:    pageCount,
	}, nil
}

// GetPDFPageCount returns the number of pages in a PDF file
func GetPDFPageCount(filePath string) (int, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PDF: %w", err)
	}

	// Parse PDF
	ctx, err := pdfcpu.Read(bytes.NewReader(data), &model.Configuration{})
	if err != nil {
		return 0, fmt.Errorf("failed to parse PDF: %w", err)
	}

	return ctx.XRefTable.PageCount, nil
}

// ExtractPDFPages extracts specific pages from a PDF and returns images
func ExtractPDFPages(filePath string, pageRange *PDFPageRange) (*PDFExtractionResult, error) {
	// Create temporary directory for output
	outputDir, err := os.MkdirTemp("", "pdf_extract_*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Build page range string for pdfcpu
	var pageRanges []string
	if pageRange.LastPage == -1 {
		// "to end" - extract all pages from FirstPage to end (max 50)
		maxPage := pageRange.FirstPage + MaxPagesPerRead
		if maxPage > MaxPagesPerRead {
			maxPage = MaxPagesPerRead
		}
		for i := pageRange.FirstPage; i <= maxPage; i++ {
			pageRanges = append(pageRanges, fmt.Sprintf("%d", i))
		}
	} else {
		// Extract specific range
		for i := pageRange.FirstPage; i <= pageRange.LastPage; i++ {
			pageRanges = append(pageRanges, fmt.Sprintf("%d", i))
		}
	}

	// Extract pages as separate PDFs
	conf := &model.Configuration{}
	err = api.ExtractPagesFile(filePath, outputDir, pageRanges, conf)
	if err != nil {
		os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to extract pages: %w", err)
	}

	// Count extracted files
	files, err := os.ReadDir(outputDir)
	if err != nil {
		os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to read output dir: %w", err)
	}

	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &PDFExtractionResult{
		FilePath:     filePath,
		OriginalSize: fileInfo.Size(),
		Count:        len(files),
		OutputDir:    outputDir,
	}, nil
}

// base64StdEncode encodes data to base64 using standard encoding
func base64StdEncode(data []byte) string {
	// Simple base64 encoding
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result strings.Builder

	for i := 0; i < len(data); i += 3 {
		// Convert 3 bytes to 4 base64 characters
		var n uint32
		if i+3 <= len(data) {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
		} else if i+2 == len(data) {
			n = uint32(data[i])<<16 | uint32(data[i+1])<<8
		} else {
			n = uint32(data[i]) << 16
		}

		result.WriteByte(chars[n>>18&0x3F])
		result.WriteByte(chars[n>>12&0x3F])
		if i+1 < len(data) {
			result.WriteByte(chars[n>>6&0x3F])
		} else {
			result.WriteByte('=')
		}
		if i+2 < len(data) {
			result.WriteByte(chars[n&0x3F])
		} else {
			result.WriteByte('=')
		}
	}

	return result.String()
}
