package common

import (
	"github.com/EngineerProjects/seshat/internal/seshattui/ui/diffview"
	"github.com/EngineerProjects/seshat/internal/seshattui/ui/styles"
	"github.com/alecthomas/chroma/v2"
)

// DiffFormatter returns a diff formatter with the given styles that can be
// used to format diff outputs.
func DiffFormatter(s *styles.Styles) *diffview.DiffView {
	formatDiff := diffview.New()
	style := chroma.MustNewStyle("seshat", s.ChromaTheme())
	diff := formatDiff.ChromaStyle(style).Style(s.Diff).TabWidth(4)
	return diff
}
