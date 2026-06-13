package common

import (
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/diffview"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
	"github.com/alecthomas/chroma/v2"
)

// DiffFormatter returns a diff formatter with the given styles that can be
// used to format diff outputs.
func DiffFormatter(s *styles.Styles) *diffview.DiffView {
	formatDiff := diffview.New()
	style := chroma.MustNewStyle("nexus", s.ChromaTheme())
	diff := formatDiff.ChromaStyle(style).Style(s.Diff).TabWidth(4)
	return diff
}
