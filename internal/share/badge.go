// Package share renders shareable artifacts: a shields-style SVG badge and
// the "wrapped" summary card as a PNG. It is pure Go with embedded fonts and
// has no system dependencies.
package share

import (
	"fmt"
	"math"
	"strings"
)

// Badge colors.
const (
	BadgeColorActive = "#7C6FE0" // flow's accent purple
	BadgeColorIdle   = "#9F9F9F" // no focus in the period
	badgeLabelColor  = "#555"
	badgeHeight      = 20
	badgePadding     = 6 // horizontal padding on each side of each half
)

// verdana11 approximates Verdana advance widths at 11px (the font shields.io
// badges are measured in). Unknown runes fall back to defaultRuneWidth; the
// SVG pins each text run with textLength, so the estimate only drives layout.
var verdana11 = map[rune]float64{
	' ': 3.87, '!': 4.33, '"': 5.03, '#': 9.0, '%': 11.84, '&': 7.99, '\'': 2.95,
	'(': 5.49, ')': 5.49, '*': 7.0, '+': 9.0, ',': 3.64, '-': 4.99, '.': 3.64,
	'/': 5.49, ':': 4.99, ';': 4.99, '?': 5.94, '@': 11.0, '_': 7.0, '|': 4.99,
	'0': 7.0, '1': 7.0, '2': 7.0, '3': 7.0, '4': 7.0, '5': 7.0, '6': 7.0, '7': 7.0, '8': 7.0, '9': 7.0,
	'a': 6.61, 'b': 6.85, 'c': 5.73, 'd': 6.85, 'e': 6.55, 'f': 3.87, 'g': 6.85, 'h': 6.96,
	'i': 3.02, 'j': 3.79, 'k': 6.51, 'l': 3.02, 'm': 10.7, 'n': 6.96, 'o': 6.68, 'p': 6.85,
	'q': 6.85, 'r': 4.69, 's': 5.73, 't': 4.33, 'u': 6.96, 'v': 6.51, 'w': 8.98, 'x': 6.51,
	'y': 6.51, 'z': 5.77,
	'A': 7.52, 'B': 7.52, 'C': 7.67, 'D': 8.48, 'E': 6.96, 'F': 6.32, 'G': 8.53, 'H': 8.26,
	'I': 4.62, 'J': 5.0, 'K': 7.62, 'L': 6.12, 'M': 9.27, 'N': 8.23, 'O': 8.66, 'P': 6.63,
	'Q': 8.66, 'R': 7.64, 'S': 7.52, 'T': 6.78, 'U': 8.05, 'V': 7.52, 'W': 10.88, 'X': 7.53,
	'Y': 6.78, 'Z': 7.53,
}

const defaultRuneWidth = 6.8

// textWidth estimates the rendered width of s in Verdana 11px.
func textWidth(s string) float64 {
	w := 0.0
	for _, r := range s {
		if v, ok := verdana11[r]; ok {
			w += v
		} else {
			w += defaultRuneWidth
		}
	}
	return w
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// Badge renders a flat, shields.io-style SVG badge: a gray label half and a
// colored value half, e.g. "focus this week | 12h". Output is deterministic.
func Badge(label, value, color string) []byte {
	lw := int(math.Ceil(textWidth(label))) + 2*badgePadding
	vw := int(math.Ceil(textWidth(value))) + 2*badgePadding
	total := lw + vw
	l, v, c := xmlEscape(label), xmlEscape(value), xmlEscape(color)
	ltl := lw - 2*badgePadding
	vtl := vw - 2*badgePadding
	lx := float64(lw) / 2
	vx := float64(lw) + float64(vw)/2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" role="img" aria-label="%s: %s">`, total, badgeHeight, l, v)
	b.WriteString("\n")
	fmt.Fprintf(&b, "  <title>%s: %s</title>\n", l, v)
	b.WriteString(`  <linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>` + "\n")
	fmt.Fprintf(&b, `  <clipPath id="r"><rect width="%d" height="%d" rx="3" fill="#fff"/></clipPath>`+"\n", total, badgeHeight)
	b.WriteString(`  <g clip-path="url(#r)">` + "\n")
	fmt.Fprintf(&b, `    <rect width="%d" height="%d" fill="%s"/>`+"\n", lw, badgeHeight, badgeLabelColor)
	fmt.Fprintf(&b, `    <rect x="%d" width="%d" height="%d" fill="%s"/>`+"\n", lw, vw, badgeHeight, c)
	fmt.Fprintf(&b, `    <rect width="%d" height="%d" fill="url(#s)"/>`+"\n", total, badgeHeight)
	b.WriteString("  </g>\n")
	b.WriteString(`  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="11">` + "\n")
	for _, t := range []struct {
		x   float64
		tl  int
		txt string
	}{{lx, ltl, l}, {vx, vtl, v}} {
		fmt.Fprintf(&b, `    <text aria-hidden="true" x="%.1f" y="15" fill="#010101" fill-opacity=".3" textLength="%d">%s</text>`+"\n", t.x, t.tl, t.txt)
		fmt.Fprintf(&b, `    <text x="%.1f" y="14" textLength="%d">%s</text>`+"\n", t.x, t.tl, t.txt)
	}
	b.WriteString("  </g>\n</svg>\n")
	return []byte(b.String())
}
