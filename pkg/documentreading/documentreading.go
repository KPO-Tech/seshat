// Package documentreading decides how to turn a file's bytes into markdown
// text, cheapest path first. It is the shared default reader for any host
// (the local backend, a server) that embeds the engine.
//
// Convert tries the SDK's native, dependency-free extractors first (DOCX,
// PPTX, XLSX, and PDFs page by page through pdfsmart) and only reaches for an
// external converter, which may be nil, when a format genuinely needs it:
// scans, audio, images, or a PDF page with no usable text layer. Output from
// an external converter is checked for garbled text too. A host that has
// no converter configured still reads every native format.
package documentreading

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/KPO-Tech/seshat/pkg/documentreader"
	"github.com/KPO-Tech/seshat/pkg/officetext"
	"github.com/KPO-Tech/seshat/pkg/pdfsmart"
	"github.com/KPO-Tech/seshat/pkg/textquality"
)

// NativeExtensions lists formats converted locally, no external service needed.
var NativeExtensions = officetext.SupportedExtensions // .docx .pptx .xlsx

// ExternalOnlyExtensions lists formats that always need an external reader: audio
// transcription and image OCR have no native equivalent here. PDF is
// intentionally absent - it's handled by Convert's own native-first/external
// fallback logic below, not a blanket "always try external" entry, since most
// PDFs have a real text layer and need no OCR at all.
var ExternalOnlyExtensions = map[string]bool{
	".wav":  true,
	".mp3":  true,
	".png":  true,
	".tiff": true,
	".tif":  true,
	".jpeg": true,
	".jpg":  true,
}

// AllConvertibleExtensions is every extension Convert can turn into markdown
// given a reachable external reader - native formats, PDF, and the
// external-only formats. Callers that need "is this worth attempting at all"
// (e.g. the upload-time conversion gate, the prompt annotation) should use
// this rather than re-deriving their own list.
var AllConvertibleExtensions = buildAllConvertibleExtensions()

func buildAllConvertibleExtensions() map[string]bool {
	all := map[string]bool{".pdf": true}
	for ext := range NativeExtensions {
		all[ext] = true
	}
	for ext := range ExternalOnlyExtensions {
		all[ext] = true
	}
	return all
}

// PageReadResult records how one PDF page was read.
type PageReadResult struct {
	Page     int    `json:"page"`
	Source   string `json:"source,omitempty"`
	HasImage bool   `json:"has_image,omitempty"`
}

// Source identifies which engine produced a Result's markdown.
type Source string

const (
	SourceNative   Source = "native"
	SourceExternal Source = "external"
	SourcePDFSmart Source = "pdfsmart"
)

// Result is the outcome of a successful conversion.
type Result struct {
	Markdown  string
	Images    []documentreader.ExtractedImage
	PageCount int
	Pages     []PageReadResult
	Source    Source
}

// Convert turns filePath's content into markdown. It tries native extraction
// first for DOCX/PPTX/XLSX (always) and PDF (only when *every* page has a
// real text layer - see pdfEveryPageHasText), then falls back to
// externalConverter (which may be nil) for everything else: scans, audio,
// images, a PDF where even one page looks like it needs OCR, and any
// native-extraction failure. ok is false when no path could produce text -
// callers should not treat that as an error, just "nothing extracted".
func Convert(ctx context.Context, filePath string, externalConverter documentreader.Converter) (Result, bool, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	if NativeExtensions[ext] {
		if data, err := os.ReadFile(filePath); err == nil {
			if md, ok, sparse, extractErr := officetext.Extract(filePath, data); ok && extractErr == nil && !sparse && !textquality.IsGarbledText(md) {
				return Result{Markdown: md, Source: SourceNative}, true, nil
			}
		}
		// Read failure, a corrupt file, or an office document extraction
		// deemed too thin to trust (e.g. a PPTX that's mostly images) -
		// fall through to the external converter below, which may still salvage something.
	}

	if ext == ".pdf" {
		if data, err := os.ReadFile(filePath); err == nil {
			if result, ok, smartErr := convertPDFSmart(ctx, data, externalConverter); smartErr != nil {
				return Result{}, false, smartErr
			} else if ok {
				return result, true, nil
			}
		}
	}

	if externalConverter == nil || !externalConverter.IsAvailable(ctx) {
		return Result{}, false, nil
	}

	conversion, err := externalConverter.ConvertFile(ctx, filePath)
	if err != nil {
		return Result{}, false, err
	}
	if strings.TrimSpace(conversion.Markdown) == "" {
		return Result{}, false, nil
	}
	// Rare, but a real failure mode worth guarding: an external converter
	// producing garbled output (e.g. a broken embedded font it couldn't
	// OCR around). Treat it the same as empty - "nothing usable came out
	// of this" - rather than silently returning bad text as if it succeeded.
	if textquality.IsGarbledText(conversion.Markdown) {
		return Result{}, false, nil
	}
	return Result{Markdown: conversion.Markdown, Images: conversion.Images, PageCount: conversion.PageCount, Source: SourceExternal}, true, nil
}

// ConvertBytes is Convert's counterpart for callers that only have the raw
// bytes and a filename, not a path on disk (e.g. RAG ingestion, which reads
// blobs out of storage rather than the workspace filesystem).
func ConvertBytes(ctx context.Context, data []byte, filename string, externalConverter documentreader.Converter) (Result, bool, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	if NativeExtensions[ext] {
		if md, ok, sparse, extractErr := officetext.Extract(filename, data); ok && extractErr == nil && !sparse && !textquality.IsGarbledText(md) {
			return Result{Markdown: md, Source: SourceNative}, true, nil
		}
	}

	if ext == ".pdf" {
		if result, ok, err := convertPDFSmart(ctx, data, externalConverter); err != nil {
			return Result{}, false, err
		} else if ok {
			return result, true, nil
		}
	}

	if externalConverter == nil || !externalConverter.IsAvailable(ctx) {
		return Result{}, false, nil
	}

	conversion, err := externalConverter.ConvertBytes(ctx, data, filename)
	if err != nil {
		return Result{}, false, err
	}
	if strings.TrimSpace(conversion.Markdown) == "" {
		return Result{}, false, nil
	}
	// Rare, but a real failure mode worth guarding: an external converter
	// producing garbled output (e.g. a broken embedded font it couldn't
	// OCR around). Treat it the same as empty - "nothing usable came out
	// of this" - rather than silently returning bad text as if it succeeded.
	if textquality.IsGarbledText(conversion.Markdown) {
		return Result{}, false, nil
	}
	return Result{Markdown: conversion.Markdown, Images: conversion.Images, PageCount: conversion.PageCount, Source: SourceExternal}, true, nil
}

func convertPDFSmart(ctx context.Context, data []byte, externalConverter documentreader.Converter) (Result, bool, error) {
	smart, ok, err := pdfsmart.Convert(ctx, data, externalConverter, pdfsmart.VisionFallback{})
	if err != nil {
		return Result{}, false, err
	}
	if !ok || strings.TrimSpace(smart.Markdown) == "" || textquality.IsGarbledText(smart.Markdown) {
		return Result{}, false, nil
	}
	source := SourcePDFSmart
	if onlyPDFSmartSource(smart, pdfsmart.PageSourceNative) {
		source = SourceNative
	} else if onlyPDFSmartSource(smart, pdfsmart.PageSourceDocling) {
		source = SourceExternal
	}
	pages := make([]PageReadResult, 0, len(smart.Pages))
	for _, page := range smart.Pages {
		pages = append(pages, PageReadResult{
			Page:     page.Page,
			Source:   string(page.Source),
			HasImage: page.HasEmbeddedImage,
		})
	}
	return Result{Markdown: smart.Markdown, PageCount: len(smart.Pages), Pages: pages, Source: source}, true, nil
}

func onlyPDFSmartSource(result pdfsmart.Result, source pdfsmart.PageSource) bool {
	if len(result.Pages) == 0 {
		return false
	}
	for _, page := range result.Pages {
		if page.Source != source {
			return false
		}
	}
	return true
}
