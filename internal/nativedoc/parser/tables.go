//go:build cgo && nativedoc

package parser

import (
	"context"
	"fmt"

	"github.com/KPO-Tech/seshat/internal/nativedoc"
	"github.com/KPO-Tech/seshat/internal/nativedoc/pdfium"
	"github.com/KPO-Tech/seshat/internal/pdftext"
)

// FindTables renders a page, lets the layout and table-structure models find the tables on it and read their
// rows and columns, and returns them in points from the top left corner of the page. It is what internal/pdfsmart
// asks for, through its TableFinder interface, to read tables that have no ruling lines: the models say where the
// cells are and the page's own text fills them.
//
// With no ONNX runtime or no models it returns nothing, and the page is read as it would be without them.
func (c *Converter) FindTables(ctx context.Context, pdfData []byte, pageIndex int) ([]pdftext.TableStructure, error) {
	if !nativedoc.Initialized() || c.ModelDir == "" {
		return nil, nil
	}
	width, _, err := pdfium.PageSize(pdfData, pageIndex)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: size of page %d: %w", pageIndex+1, err)
	}
	rgba, err := pdfium.RenderPage(pdfData, pageIndex, c.renderDPI())
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: render page %d: %w", pageIndex+1, err)
	}
	img, err := nativedoc.FromImage(rgba)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: convert page %d: %w", pageIndex+1, err)
	}
	layouts, err := nativedoc.FindTables(ctx, c.ModelDir, img)
	if err != nil {
		return nil, fmt.Errorf("nativedoc/parser: tables on page %d: %w", pageIndex+1, err)
	}

	// Pixels to points: the page was rendered at a size that gives this many pixels to a point.
	scale := float64(img.W) / width
	if scale <= 0 {
		return nil, nil
	}
	pts := func(b [4]float32) [4]float64 {
		return [4]float64{float64(b[0]) / scale, float64(b[1]) / scale, float64(b[2]) / scale, float64(b[3]) / scale}
	}
	var out []pdftext.TableStructure
	for _, l := range layouts {
		whole := pts([4]float32{l.X0, l.Y0, l.X1, l.Y1})
		s := pdftext.TableStructure{X0: whole[0], Top: whole[1], X1: whole[2], Bottom: whole[3], Score: float64(l.Score)}
		for _, b := range l.Columns {
			s.Columns = append(s.Columns, pts(b))
		}
		for _, b := range l.Rows {
			s.Rows = append(s.Rows, pts(b))
		}
		for _, b := range l.Headers {
			s.Headers = append(s.Headers, pts(b))
		}
		out = append(out, s)
	}
	return out, nil
}
