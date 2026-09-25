//go:build !cgo || !nativedoc

package nativedoc

import (
	"context"
	"errors"

	"github.com/KPO-Tech/seshat/pkg/docling"
	"github.com/KPO-Tech/seshat/pkg/pdfsmart"
)

// ErrUnavailable is returned by the fallback build when the native document
// backend was not compiled in. Build with CGO and the nativedoc tag to enable it.
var ErrUnavailable = errors.New("nativedoc backend unavailable: build with CGO and the nativedoc tag")

// Converter is a placeholder implementation used when the native document
// backend is not compiled in.
type Converter struct {
	ModelDir string
}

// New creates an unavailable placeholder converter.
func New(modelDir string) *Converter {
	return &Converter{ModelDir: modelDir}
}

// InitORT reports that ONNX Runtime support is unavailable in this build.
func InitORT() error {
	return ErrUnavailable
}

// Initialized reports whether ONNX Runtime has been initialized.
func Initialized() bool {
	return false
}

// IsAvailable reports whether this converter can process documents.
func (c *Converter) IsAvailable(context.Context) bool {
	return false
}

// ConvertFile returns ErrUnavailable in non-nativedoc builds.
func (c *Converter) ConvertFile(context.Context, string) (*docling.ConversionResult, error) {
	return nil, ErrUnavailable
}

// ConvertBytes returns ErrUnavailable in non-nativedoc builds.
func (c *Converter) ConvertBytes(context.Context, []byte, string) (*docling.ConversionResult, error) {
	return nil, ErrUnavailable
}

// ConvertURL returns ErrUnavailable in non-nativedoc builds.
func (c *Converter) ConvertURL(context.Context, string) (*docling.ConversionResult, error) {
	return nil, ErrUnavailable
}

// RenderPage returns ErrUnavailable in non-nativedoc builds.
func (c *Converter) RenderPage(context.Context, []byte, int) ([]byte, error) {
	return nil, ErrUnavailable
}

var _ docling.DocumentConverterBackend = (*Converter)(nil)
var _ pdfsmart.PageRenderer = (*Converter)(nil)
