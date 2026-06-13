package styles

import (
	"image/color"

	"github.com/charmbracelet/x/exp/charmtone"
)

// hexC parses a "#RRGGBB" string into a color.Color.
func hexC(h string) color.Color {
	var r, g, b uint8
	if len(h) == 7 {
		r = hexByte(h[1], h[2])
		g = hexByte(h[3], h[4])
		b = hexByte(h[5], h[6])
	}
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}

func hexByte(hi, lo byte) uint8 {
	return hexNibble(hi)<<4 | hexNibble(lo)
}

func hexNibble(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

// ThemeForProvider returns the Styles for a given provider ID.
// All providers use the Nexus dark orange theme by default.
func ThemeForProvider(_ string) Styles {
	return NexusDark()
}

// NexusDark is the Nexus brand theme: orange/grey on a dark background.
func NexusDark() Styles {
	return quickStyle(quickStyleOpts{
		primary:   hexC("#E8630A"), // orange
		secondary: hexC("#FF8C42"), // lighter orange
		accent:    hexC("#FF8C42"),
		keyword:   hexC("#3B82F6"), // blue

		fgBase:       hexC("#F9FAFB"), // near-white
		fgSubtle:     hexC("#B2BCCB"),
		fgMoreSubtle: hexC("#6B7280"), // grey
		fgMostSubtle: hexC("#4B5563"),

		onPrimary: hexC("#1C1007"), // dark text on orange bg

		bgBase:         hexC("#0F1117"),
		bgLeastVisible: hexC("#151A21"),
		bgLessVisible:  hexC("#1C2433"),
		bgMostVisible:  hexC("#374151"),

		separator: hexC("#374151"),

		destructive:       hexC("#EF4444"),
		error:             hexC("#EF4444"),
		warningSubtle:     hexC("#D97706"),
		warning:           hexC("#F59E0B"),
		denied:            hexC("#EF4444"),
		busy:              hexC("#F59E0B"),
		info:              hexC("#3B82F6"),
		infoMoreSubtle:    hexC("#2563EB"),
		infoMostSubtle:    hexC("#1D4ED8"),
		success:           hexC("#10B981"),
		successMoreSubtle: hexC("#059669"),
		successMostSubtle: hexC("#047857"),
	})
}

// CharmtonePantera returns the Charmtone dark theme (kept for reference).
func CharmtonePantera() Styles {
	return quickStyle(quickStyleOpts{
		primary:   charmtone.Charple,
		secondary: charmtone.Dolly,
		accent:    charmtone.Bok,
		keyword:   charmtone.Blush,

		fgBase:       charmtone.Sash,
		fgMoreSubtle: charmtone.Squid,
		fgSubtle:     charmtone.Smoke,
		fgMostSubtle: charmtone.Oyster,

		onPrimary: charmtone.Butter,

		bgBase:         charmtone.Pepper,
		bgLeastVisible: charmtone.BBQ,
		bgLessVisible:  charmtone.Char,
		bgMostVisible:  charmtone.Iron,

		separator: charmtone.Char,

		destructive:       charmtone.Coral,
		error:             charmtone.Sriracha,
		warningSubtle:     charmtone.Zest,
		warning:           charmtone.Mustard,
		denied:            charmtone.Tang,
		busy:              charmtone.Citron,
		info:              charmtone.Malibu,
		infoMoreSubtle:    charmtone.Sardine,
		infoMostSubtle:    charmtone.Damson,
		success:           charmtone.Julep,
		successMoreSubtle: charmtone.Bok,
		successMostSubtle: charmtone.Guac,
	})
}

// HyperNexusObsidiana returns the HyperNexus dark theme.
func HyperNexusObsidiana() Styles {
	return quickStyle(quickStyleOpts{
		primary:   charmtone.Charple,
		secondary: charmtone.Dolly,
		accent:    charmtone.Bok,

		fgBase:       charmtone.Sash,
		fgMoreSubtle: charmtone.Squid,
		fgSubtle:     charmtone.Smoke,
		fgMostSubtle: charmtone.Oyster,

		onPrimary: charmtone.Butter,

		bgBase:         charmtone.Pepper,
		bgLeastVisible: charmtone.BBQ,
		bgLessVisible:  charmtone.Char,
		bgMostVisible:  charmtone.Iron,

		separator: charmtone.Char,

		destructive:       charmtone.Coral,
		error:             charmtone.Sriracha,
		warningSubtle:     charmtone.Zest,
		warning:           charmtone.Mustard,
		denied:            charmtone.Tang,
		busy:              charmtone.Citron,
		info:              charmtone.Malibu,
		infoMoreSubtle:    charmtone.Sardine,
		infoMostSubtle:    charmtone.Damson,
		success:           charmtone.Julep,
		successMoreSubtle: charmtone.Bok,
		successMostSubtle: charmtone.Guac,
	})
}
