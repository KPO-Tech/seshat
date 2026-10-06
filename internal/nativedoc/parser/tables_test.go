//go:build cgo && nativedoc

package parser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	nativedoc "github.com/KPO-Tech/seshat/internal/nativedoc"
	"github.com/KPO-Tech/seshat/internal/pdfsmart"
)

// The layout and table-structure models run here for real, on a page whose table has no ruling lines
// (internal/pdftext/testdata/borderless.pdf). They need the models: set SESHAT_NATIVEDOC_TEST_MODELS_DIR (see
// scripts/install-deepdoc-models.sh).
func modelsOrSkip(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("SESHAT_NATIVEDOC_TEST_MODELS_DIR")
	if dir == "" {
		t.Skip("SESHAT_NATIVEDOC_TEST_MODELS_DIR not set - see scripts/install-deepdoc-models.sh")
	}
	if err := nativedoc.InitORT(); err != nil {
		t.Fatalf("InitORT: %v", err)
	}
	return dir
}

func TestABorderlessTableIsFoundByTheModelsAndFilledFromThePageText(t *testing.T) {
	c := New(modelsOrSkip(t))
	data, err := os.ReadFile(filepath.Join("..", "..", "pdftext", "testdata", "borderless.pdf"))
	if err != nil {
		t.Fatal(err)
	}

	tables, err := c.FindTables(context.Background(), data, 0)
	if err != nil {
		t.Fatalf("FindTables: %v", err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected the one table of page 1, got %d: %+v", len(tables), tables)
	}
	if len(tables[0].Rows) < 5 || len(tables[0].Columns) < 4 {
		t.Errorf("structure too thin: %d rows, %d columns", len(tables[0].Rows), len(tables[0].Columns))
	}

	result, ok, err := pdfsmart.ReadPages(context.Background(), data, []int{1}, pdfsmart.Options{}, c, pdfsmart.VisionFallback{})
	if err != nil || !ok {
		t.Fatalf("ReadPages: ok=%v err=%v", ok, err)
	}
	md := result.Markdown
	for _, want := range []string{"| Product line | Q1 | Q2 | Q3 | Q4 |", "| Hardware | 120 | 135 | 128 | 141 |", "| Training | 12 | 9 | 14 | 16 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Count(md, "Hardware") != 1 {
		t.Errorf("the table's text is repeated outside it:\n%s", md)
	}

	// A page of prose is never sent to the models.
	prose, ok, err := pdfsmart.ReadPages(context.Background(), data, []int{2}, pdfsmart.Options{}, c, pdfsmart.VisionFallback{})
	if err != nil || !ok || strings.Contains(prose.Markdown, "|") {
		t.Fatalf("page 2: ok=%v err=%v\n%s", ok, err, prose.Markdown)
	}
}

func TestWithoutOnnxRuntimeOrModelsThePageIsReadAsBefore(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "pdftext", "testdata", "borderless.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	c := New("") // no model directory
	tables, err := c.FindTables(context.Background(), data, 0)
	if err != nil || tables != nil {
		t.Fatalf("tables=%v err=%v", tables, err)
	}
}

var _ pdfsmart.TableFinder = (*Converter)(nil)

// testdata/table_image.pdf has a table pasted in as a picture on a page with a text layer; scanned_table.pdf is the
// same table on a page that is one picture as a whole. Both are read by the models' OCR.
func TestATableThatIsAPictureIsReadByOCR(t *testing.T) {
	c := New(modelsOrSkip(t))
	for _, name := range []string{"table_image.pdf", "scanned_table.pdf"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "pdftext", "testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			result, ok, err := pdfsmart.ReadPages(context.Background(), data, []int{1}, pdfsmart.Options{}, c, pdfsmart.VisionFallback{})
			if err != nil || !ok {
				t.Fatalf("ReadPages: ok=%v err=%v", ok, err)
			}
			for _, want := range []string{"| Product | Q1 | Q2 | Q3 |", "| Hardware | 120 | 135 | 128 |", "| Training | 12 | 9 | 14 |"} {
				if !strings.Contains(result.Markdown, want) {
					t.Errorf("missing %q in:\n%s", want, result.Markdown)
				}
			}
		})
	}
}

// Converting a whole document (what ingestion does, and what a page sent to the engine gets) writes a page that has a
// text layer as the page reader does: with its tables, a picture's included, not as the plain text of the layer.
func TestConvertingADocumentWritesItsTablesToo(t *testing.T) {
	c := New(modelsOrSkip(t))
	for name, want := range map[string]string{
		"table_image.pdf": "| Hardware | 120 | 135 | 128 |",
		"header_rule.pdf": "| E103184 | Maintenance kit CA6707/10 | 49,99 | 21 % | 1 PCS | 49,99 |",
		"borderless.pdf":  "| Hardware | 120 | 135 | 128 | 141 |",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "pdftext", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		result, err := c.ConvertBytes(context.Background(), data, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(result.Markdown, want) {
			t.Errorf("%s: missing %q in:\n%s", name, want, result.Markdown)
		}
	}
}
