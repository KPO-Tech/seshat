//go:build cgo && nativedoc

package nativedoc

import (
	"context"
	"sort"
	"strings"
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

// CropImage cuts a box, widened by pad pixels each way, out of an image, and returns where the cut starts.
func CropImage(img *Image, x0, y0, x1, y1, pad float32) (*Image, float32, float32) {
	return cropPadded(img, x0, y0, x1, y1, pad)
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

// TextBox is a line of text read from an image, with its box in pixels of that image.
type TextBox struct {
	X0, Y0, X1, Y1 float32
	Text           string
}

// ReadText finds the text in an image and reads it: the text-detection model finds where the lines are, the
// recognition model reads each. A line that cannot be read is left out.
func ReadText(ctx context.Context, modelDir string, img *Image) ([]TextBox, error) {
	det, err := RunDet(ctx, modelDir, img)
	if err != nil {
		return nil, err
	}
	var out []TextBox
	for _, box := range det.Boxes {
		minX, minY, maxX, maxY := box.Pts[0][0], box.Pts[0][1], box.Pts[0][0], box.Pts[0][1]
		for _, p := range box.Pts {
			minX, maxX = minf(minX, p[0]), maxf(maxX, p[0])
			minY, maxY = minf(minY, p[1]), maxf(maxY, p[1])
		}
		crop, _, _ := cropPadded(img, minX, minY, maxX, maxY, 0)
		read, err := RunOCRRec(ctx, modelDir, crop)
		if err != nil || strings.TrimSpace(read.Text) == "" {
			continue
		}
		out = append(out, TextBox{X0: minX, Y0: minY, X1: maxX, Y1: maxY, Text: read.Text})
	}
	return out, nil
}
