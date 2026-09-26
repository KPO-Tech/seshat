//go:build cgo && nativedoc

// Package nativedoc provides in-process document OCR, layout detection, and
// text recognition — no external Python process, no docling-serve, no
// seshat-intelligence required. This is seshat's default document-reading
// path when neither is configured.
//
// Adapted from RAGFlow (https://github.com/infiniflow/ragflow),
// internal/deepdoc/native/, licensed under the Apache License, Version 2.0
// (see https://github.com/infiniflow/ragflow/blob/main/LICENSE). Copyright
// the RAGFlow/InfiniFlow authors. This package is a close port of that
// code: the text-detection (DBNet), text-recognition (CTC), and
// layout-detection (YOLO-style) algorithms, their pre/post-processing, and
// the ONNX Runtime session plumbing are unchanged from the original.
// Changes made here: package renamed from `native` to `nativedoc`; no other
// seshat-internal packages are imported by this package (it was already
// self-contained upstream), so no rewiring was needed at this layer -
// integration with seshat's own documentreader.Converter interface
// lives in a separate file/package that calls into this one, not in these
// ported files themselves. One behavioral change: the "shared weights"
// memory optimization (session.go) is now off by default
// (WeightSharingEnabled), rather than attempted unconditionally - see that
// var's own doc comment for why (it crashes, not errors, against a stock
// onnxruntime build, which is what this repo can rely on outside of a
// deliberately-fetched RAGFlow-patched Linux/macOS build).
//
// Model weights (det.ort, rec.ort, layout.ort, tsr.ort, ocr.res) are not
// vendored here - they're fetched from the InfiniFlow/deepdoc HuggingFace
// repository (also Apache-2.0) at setup time, the same way RAGFlow's own
// build fetches them.
//
// See docs/issues/document-intelligence-roadmap.md for the full adoption
// plan, including what was deliberately NOT ported (RAGFlow's own document
// assembly/box-merging orchestration, which this repo's own
// internal/pdfsmart and internal/rag chunkers already cover differently).
package nativedoc
