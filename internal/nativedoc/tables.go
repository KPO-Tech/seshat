//go:build cgo && nativedoc

package nativedoc

import (
	"context"
	"sort"
)

// tables.go finds the tables on a page image and reads their structure, by running the layout model (which says
// where the tables are) and then the table-structure model on each one. It is the part of RAGFlow's table pipeline
// that decides where the cells are; what goes in them is not read here, because the page's own text is exact and an
// OCR of the picture is not.

const (
	// tableMinScore is the layout model's confidence for a table to be looked at. Real tables of a borderless
	// kind come back around 0.5, and a figure taken for a table scores lower than that.
	tableMinScore = 0.4
	// structureMinScore is the confidence for one row or column of a table.
	structureMinScore = 0.25
	// tablePad is how many pixels around the layout model's box the table's crop takes, so that a header or a last
	// row it cut a little short is still in the picture the structure model sees.
	tablePad = 8
)

// TableLayout is a table found on a page image. Boxes are {x0, y0, x1, y1} in pixels of that image.
type TableLayout struct {
	X0, Y0, X1, Y1 float32
	Score          float32
	Columns        [][4]float32
	Rows           [][4]float32
	Headers        [][4]float32
}

// FindTables returns the tables on a page image, with the columns, rows and header boxes of each, best first by
// position on the page (top to bottom).
func FindTables(ctx context.Context, modelDir string, page *Image) ([]TableLayout, error) {
	layout, err := RunDLA(ctx, modelDir, page)
	if err != nil {
		return nil, err
	}
	var out []TableLayout
	for _, box := range layout.Boxes {
		if box.Class != dlaClassMap["table"] || box.Score < tableMinScore {
			continue
		}
		crop, x0, y0 := cropPadded(page, box.X0, box.Y0, box.X1, box.Y1, tablePad)
		structure, err := RunTSR(ctx, modelDir, crop)
		if err != nil {
			return nil, err
		}
		table := TableLayout{X0: box.X0, Y0: box.Y0, X1: box.X1, Y1: box.Y1, Score: box.Score}
		shift := func(b TSRBox) [4]float32 { return [4]float32{b.X0 + x0, b.Top + y0, b.X1 + x0, b.Bottom + y0} }
		for _, b := range structure.Boxes {
			if b.Score < structureMinScore {
				continue
			}
			switch b.Label {
			case "table column":
				table.Columns = append(table.Columns, shift(b))
			case "table row":
				table.Rows = append(table.Rows, shift(b))
			case "table column header":
				table.Headers = append(table.Headers, shift(b))
			}
		}
		out = append(out, table)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Y0 < out[j].Y0 })
	return out, nil
}

// cropPadded cuts the box, widened by pad pixels each way, out of an image, and returns where the cut starts.
func cropPadded(img *Image, x0, y0, x1, y1 float32, pad float32) (*Image, float32, float32) {
	left, top := int(maxf(x0-pad, 0)), int(maxf(y0-pad, 0))
	right, bottom := int(minf(x1+pad, float32(img.W))), int(minf(y1+pad, float32(img.H)))
	w, h := right-left, bottom-top
	if w <= 0 || h <= 0 {
		return &Image{W: 1, H: 1, Pix: make([]byte, 3)}, float32(left), float32(top)
	}
	pix := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		src := ((top+y)*img.W + left) * 3
		copy(pix[y*w*3:(y+1)*w*3], img.Pix[src:src+w*3])
	}
	return &Image{W: w, H: h, Pix: pix}, float32(left), float32(top)
}
