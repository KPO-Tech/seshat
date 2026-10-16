package model

import (
	"bytes"
	_ "embed" // landing_mark.png
	"image"
	"image/color"
	_ "image/png" // decodes landing_mark.png
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/disintegration/imaging"

	fimage "github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/ui/image"
)

// landingMarkPNG is the Seshat Runtime mark (docs/brand), cropped to its
// bounds, on a transparent background.
//
//go:embed landing_mark.png
var landingMarkPNG []byte

const (
	// landingMarkRows is the height of the mark in terminal rows. Each row
	// holds four braille dots, so the mark is 4*landingMarkRows dots tall.
	landingMarkRows = 12
	// landingMarkMinHeight is the smallest main area that still shows the mark
	// above the tagline and the shortcuts.
	landingMarkMinHeight = 24
	// landingMarkOpaque is the alpha from which a dot is drawn.
	landingMarkOpaque = 90
	// landingMarkID identifies the mark in the terminal image cache.
	landingMarkID = "seshat-landing-mark"
	// landingMarkRetry is how long a transmission is given before it is sent again
	// because the image cache was reset in between (for example by the file picker).
	landingMarkRetry = 3 * time.Second
)

var (
	landingMarkOnce sync.Once
	landingMarkImg  image.Image
)

// landingMarkImage returns the decoded mark, or nil when the asset cannot be
// decoded (the landing page then falls back to the wordmark alone).
func landingMarkImage() image.Image {
	landingMarkOnce.Do(func() {
		img, _, err := image.Decode(bytes.NewReader(landingMarkPNG))
		if err == nil {
			landingMarkImg = img
		}
	})
	return landingMarkImg
}

// landingMarkCols returns the width, in columns, that keeps the mark's aspect
// ratio. A braille cell is 2 dots wide and 4 dots tall, and a terminal cell is
// about twice as tall as it is wide, so dots are square and the mark is
// landingMarkRows*4 dots tall.
func landingMarkCols(img image.Image) int {
	b := img.Bounds()
	if b.Dy() == 0 {
		return 0
	}
	return max(1, (b.Dx()*landingMarkRows*4/2+b.Dy()/2)/b.Dy())
}

// brailleBit maps a dot (column, row) of a braille cell to its bit.
var brailleBit = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// renderLandingMarkBlocks draws the mark with braille characters: every cell
// holds 2x4 dots, which keeps the thin flame blades that half blocks lose, and
// takes the average color of its dots. It works in any terminal that has color
// and returns an empty string without the asset.
func renderLandingMarkBlocks() string {
	img := landingMarkImage()
	if img == nil {
		return ""
	}
	cols := landingMarkCols(img)
	px := imaging.Resize(img, cols*2, landingMarkRows*4, imaging.CatmullRom)

	var sb strings.Builder
	for row := 0; row < landingMarkRows; row++ {
		for col := 0; col < cols; col++ {
			var bits rune
			var r, g, b, n int
			for dx := 0; dx < 2; dx++ {
				for dy := 0; dy < 4; dy++ {
					c := px.NRGBAAt(col*2+dx, row*4+dy)
					if c.A < landingMarkOpaque {
						continue
					}
					bits |= brailleBit[dx][dy]
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					n++
				}
			}
			if n == 0 {
				sb.WriteByte(' ')
				continue
			}
			avg := color.RGBA{R: uint8(r / n), G: uint8(g / n), B: uint8(b / n), A: 0xff} //nolint:gosec // an average of bytes fits in a byte
			sb.WriteString(ansi.NewStyle().ForegroundColor(avg).Styled(string(0x2800 + bits)))
		}
		if row < landingMarkRows-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// withLandingMark adds the command that sends the landing page mark to the
// terminal, when there is one.
func (m *UI) withLandingMark(cmds []tea.Cmd) []tea.Cmd {
	if cmd := m.transmitLandingMark(); cmd != nil {
		return append(cmds, cmd)
	}
	return cmds
}

// transmitLandingMark sends the mark to a terminal that supports the Kitty
// graphics protocol, so the landing page can show the real image instead of
// the braille drawing. It returns nil when there is nothing to send.
func (m *UI) transmitLandingMark() tea.Cmd {
	if m.state != uiLanding || !m.caps.SupportsKittyGraphics() {
		return nil
	}
	img := landingMarkImage()
	if img == nil {
		return nil
	}
	cellW, cellH := m.caps.CellSize()
	if cellW == 0 || cellH == 0 {
		return nil
	}
	cols := landingMarkCols(img)
	if fimage.HasTransmitted(landingMarkID, cols, landingMarkRows) {
		return nil
	}
	if time.Since(m.landingMarkSentAt) < landingMarkRetry {
		return nil
	}
	m.landingMarkSentAt = time.Now()
	_, isTmux := m.caps.Env.LookupEnv("TMUX")
	return fimage.EncodingKitty.Transmit(landingMarkID, img, fimage.CellSize{Width: cellW, Height: cellH}, cols, landingMarkRows, isTmux)
}

// renderLandingMark returns the mark for the landing page: the real image when
// the terminal can show it and it has arrived, the braille drawing otherwise.
func (m *UI) renderLandingMark() string {
	if m.caps.SupportsKittyGraphics() {
		if img := landingMarkImage(); img != nil {
			if out := fimage.EncodingKitty.Render(landingMarkID, landingMarkCols(img), landingMarkRows); out != "" {
				return out
			}
		}
	}
	return renderLandingMarkBlocks()
}
