package officetext

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SupportedExtensions lists the formats this package can extract without any
// external service. Kept as the single source of truth for "is this a
// native-office format" checks - see internal/tools/files/read/detector.go.
var SupportedExtensions = map[string]bool{
	".docx": true,
	".pptx": true,
	".xlsx": true,
}

// MinCharsPerSlide is the threshold below which a PPTX's extracted text is
// treated as "not really there" relative to how many slides the deck has -
// mirrors pdftext.MinCharsPerPage's reasoning, sized up to account for the
// "## Slide N\n\n" heading markup Extract itself writes per slide (~13
// chars) even when a slide renders no real content. A deck that's mostly
// screenshots/diagrams with only a slide title or two of real text will
// parse "successfully" (ok=true, err=nil) but still be nearly useless as a
// text extraction - callers should treat Sparse the same way they already
// treat pdftext's Sparse: prefer OCR (docling) instead of trusting it.
const MinCharsPerSlide = 40

// Extract dispatches to the format-specific extractor based on filename
// extension. ok is false when the extension isn't one this package handles;
// callers should fall back to another conversion path (e.g. docling) in that
// case rather than treating it as an error. sparse is only ever true for
// PPTX (see MinCharsPerSlide) - DOCX/XLSX have no page/slide-count concept
// to measure sparseness against, and are essentially never image-only.
func Extract(filename string, data []byte) (markdown string, ok bool, sparse bool, err error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".docx":
		md, err := ExtractDOCX(data)
		return md, true, false, emptyAsErr(md, err)
	case ".pptx":
		md, slideCount, err := ExtractPPTX(data)
		if extractErr := emptyAsErr(md, err); extractErr != nil {
			return md, true, false, extractErr
		}
		sparse := slideCount > 0 && len(md) < MinCharsPerSlide*slideCount
		return md, true, sparse, nil
	case ".xlsx":
		md, err := ExtractXLSX(data)
		return md, true, false, emptyAsErr(md, err)
	default:
		return "", false, false, nil
	}
}

func emptyAsErr(md string, err error) error {
	if err != nil {
		return err
	}
	if strings.TrimSpace(md) == "" {
		return ErrEmpty
	}
	return nil
}

// ErrEmpty is returned by format extractors when the document parsed
// successfully but yielded no extractable text (e.g. an image-only slide
// deck) - callers can use this to decide whether to still try documentreader.
var ErrEmpty = fmt.Errorf("no extractable text found")
