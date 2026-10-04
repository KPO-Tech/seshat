// Package documentreading decides how to turn a file's bytes into markdown
// text, cheapest path first. It is the shared default reader for any host
// (the local backend, a server) that embeds the engine.
//
// Convert tries the SDK's native, dependency-free extractors first (DOCX,
// PPTX, XLSX, HTML, and PDFs page by page through pdfsmart) and only reaches for an
// external converter, which may be nil, when a format genuinely needs it:
// scans, audio, images, or a PDF page with no usable text layer. Output from
// an external converter is checked for garbled text too. A host that has
// no converter configured still reads every native format.
package documentreading

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/KPO-Tech/seshat/internal/htmltext"
	"github.com/KPO-Tech/seshat/pkg/documentreader"
	"github.com/KPO-Tech/seshat/pkg/officetext"
	"github.com/KPO-Tech/seshat/pkg/pdfsmart"
	"github.com/KPO-Tech/seshat/pkg/textquality"
)

// HTMLExtensions are the HTML formats Convert turns into markdown: a saved page, an exported report, a
// documentation page, as a document. They are not in officetext.SupportedExtensions on purpose: the Read tool
// shows an HTML file as source, because the tags are what someone editing a web page needs to see.
var HTMLExtensions = map[string]bool{".html": true, ".htm": true, ".xhtml": true}

// NativeExtensions lists formats converted locally, no external service needed: the Office formats and HTML.
var NativeExtensions = nativeExtensions()

func nativeExtensions() map[string]bool {
	all := map[string]bool{}
	for ext := range officetext.SupportedExtensions { // .docx .pptx .xlsx
		all[ext] = true
	}
	for ext := range HTMLExtensions {
		all[ext] = true
	}
	return all
}

// extractNative converts a native format. sparse is only ever true for a deck that is mostly pictures.
func extractNative(filename string, data []byte) (markdown string, ok bool, sparse bool, err error) {
	if HTMLExtensions[strings.ToLower(filepath.Ext(filename))] {
		result, err := htmltext.Convert(data)
		if err != nil {
			return "", true, false, err
		}
		if strings.TrimSpace(result.Markdown) == "" {
			return "", true, false, officetext.ErrEmpty
		}
		return result.Markdown, true, false, nil
	}
	return officetext.Extract(filename, data)
}

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

// nativeRead is what the native extractor made of a file: the markdown when it is enough, the thin markdown when it is
// too little to trust on its own (a deck of pictures with a title or two), and whether the file parsed but held no text.
type nativeRead struct {
	markdown string
	thin     string
	empty    bool
}

func readNative(filename string, data []byte) nativeRead {
	md, ok, sparse, err := extractNative(filename, data)
	switch {
	case !ok:
		return nativeRead{}
	case err != nil:
		return nativeRead{empty: errors.Is(err, officetext.ErrEmpty)}
	case textquality.IsGarbledText(md):
		return nativeRead{}
	case sparse:
		return nativeRead{thin: md}
	}
	return nativeRead{markdown: md}
}

// external converts with the external reader, and falls back on the thin native text when the reader cannot do better:
// a few titles are worth more than nothing. A file the native extractor parsed and found empty is not an error when
// the external reader fails on it: there is nothing to extract.
func external(ctx context.Context, native nativeRead, converter documentreader.Converter, convert func(documentreader.Converter) (*documentreader.ConversionResult, error)) (Result, bool, error) {
	thin := func() (Result, bool, error) {
		if native.thin != "" {
			return Result{Markdown: native.thin, Source: SourceNative}, true, nil
		}
		return Result{}, false, nil
	}
	if converter == nil || !converter.IsAvailable(ctx) {
		return thin()
	}
	conversion, err := convert(converter)
	if err != nil {
		if native.thin != "" || native.empty {
			return thin()
		}
		return Result{}, false, err
	}
	// Rare, but a real failure mode worth guarding: an external converter producing garbled output (e.g. a broken
	// embedded font it couldn't OCR around). Treat it the same as empty - "nothing usable came out of this" - rather
	// than silently returning bad text as if it succeeded.
	if strings.TrimSpace(conversion.Markdown) == "" || textquality.IsGarbledText(conversion.Markdown) {
		return thin()
	}
	return Result{Markdown: conversion.Markdown, Images: conversion.Images, PageCount: conversion.PageCount, Source: SourceExternal}, true, nil
}

// Convert turns filePath's content into markdown. It tries native extraction
// first for DOCX/PPTX/XLSX/HTML (always) and PDF (page by page, see pdfsmart),
// then falls back to externalConverter (which may be nil) for everything else:
// scans, audio, images, a PDF page that needs OCR, and any native-extraction
// failure. A document whose native text is thin (a deck that is mostly
// pictures) is returned as it is when the external reader cannot improve on it.
// ok is false when no path could produce text - callers should not treat that
// as an error, just "nothing extracted".
func Convert(ctx context.Context, filePath string, externalConverter documentreader.Converter) (Result, bool, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	var native nativeRead
	if NativeExtensions[ext] {
		if data, err := os.ReadFile(filePath); err == nil {
			native = readNative(filePath, data)
			if native.markdown != "" {
				return Result{Markdown: native.markdown, Source: SourceNative}, true, nil
			}
		}
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

	return external(ctx, native, externalConverter, func(c documentreader.Converter) (*documentreader.ConversionResult, error) {
		return c.ConvertFile(ctx, filePath)
	})
}

// ConvertBytes is Convert's counterpart for callers that only have the raw
// bytes and a filename, not a path on disk (e.g. RAG ingestion, which reads
// blobs out of storage rather than the workspace filesystem).
func ConvertBytes(ctx context.Context, data []byte, filename string, externalConverter documentreader.Converter) (Result, bool, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	var native nativeRead
	if NativeExtensions[ext] {
		native = readNative(filename, data)
		if native.markdown != "" {
			return Result{Markdown: native.markdown, Source: SourceNative}, true, nil
		}
	}

	if ext == ".pdf" {
		if result, ok, err := convertPDFSmart(ctx, data, externalConverter); err != nil {
			return Result{}, false, err
		} else if ok {
			return result, true, nil
		}
	}

	return external(ctx, native, externalConverter, func(c documentreader.Converter) (*documentreader.ConversionResult, error) {
		return c.ConvertBytes(ctx, data, filename)
	})
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
