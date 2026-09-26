//go:build cgo && nativedoc

package parser

import (
	"fmt"

	"github.com/KPO-Tech/seshat/internal/documentreader"
	"github.com/KPO-Tech/seshat/internal/officetext"
)

// convertDOCX converts a DOCX to markdown. Delegates to
// internal/officetext.ExtractDOCX - seshat already had a pure-Go,
// zero-CGO DOCX extractor (paragraphs, headings, tables) that produces the
// same output a CGO-based office_oxide port would, tested head-to-head
// against the same fixture before choosing this over porting RAGFlow's
// office_oxide wrapper. No point adding a native dependency for a format
// this repo already handles.
func (c *Converter) convertDOCX(data []byte, filename string) (*documentreader.ConversionResult, error) {
	md, err := officetext.ExtractDOCX(data)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: extract %s: %w", filename, err)
	}
	return &documentreader.ConversionResult{Markdown: md}, nil
}

// convertXLSX converts an XLSX workbook to markdown. Delegates to
// internal/officetext.ExtractXLSX (excelize, pure Go, no CGO) - same
// rationale as convertDOCX above.
func (c *Converter) convertXLSX(data []byte, filename string) (*documentreader.ConversionResult, error) {
	md, err := officetext.ExtractXLSX(data)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: extract %s: %w", filename, err)
	}
	return &documentreader.ConversionResult{Markdown: md}, nil
}
