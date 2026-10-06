//go:build cgo && nativedoc

// Package pdfium renders PDF pages to raster images using libpdfium
// (statically linked at build time). It exists specifically because
// pdf_oxide's own RenderPage/RenderPageZoom has a confirmed rendering bug
// (pattern-filled text paints as solid black blocks instead of real glyphs
// - see https://github.com/yfedoseev/pdf_oxide/issues/1283) that silently
// corrupts OCR input on real-world documents using gradient/pattern text
// fills. pdf_oxide is still used for text/char extraction, which is
// unaffected by this bug - only rendering goes through pdfium.
//
// Adapted from RAGFlow (https://github.com/infiniflow/ragflow),
// internal/deepdoc/parser/pdf/pdfium/, licensed under the Apache License,
// Version 2.0 (see https://github.com/infiniflow/ragflow/blob/main/LICENSE).
// Copyright the RAGFlow/InfiniFlow authors. Changes made here: import path
// for the sibling pdfsync package updated to this module; pdfium.go's
// `#cgo LDFLAGS: -lm -lpthread -ldl` made `!windows`-conditional - those
// are POSIX system libraries with no Windows equivalent, and RAGFlow never
// built this package for Windows (no Windows-specific file existed
// upstream) - see docs/issues/document-intelligence-roadmap.md phase 1.W.
package pdfium
