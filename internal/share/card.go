package share

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Stat is one labeled figure on the wrapped card.
type Stat struct {
	Label string
	Value string
}

// Card is the presentation model shared by the terminal and PNG renderers.
// All strings are already localized.
type Card struct {
	Title     string // e.g. "flow wrapped"
	Subtitle  string // e.g. "Week of Jun 9"
	HeroValue string // e.g. "12h 30m"
	HeroLabel string // e.g. "of deep focus"
	Stats     []Stat // rendered as a 2-column grid (up to 6 shown in the PNG)
	FunFact   string // optional
	Footer    string // e.g. "github.com/xvierd/flow-cli"
}

// Theme is a PNG color palette.
type Theme struct {
	Background color.RGBA
	Panel      color.RGBA
	Text       color.RGBA
	Muted      color.RGBA
	Accent     color.RGBA
	AccentSoft color.RGBA
}

// DarkTheme and LightTheme are the built-in PNG palettes.
var (
	DarkTheme = Theme{
		Background: rgb(0x16, 0x14, 0x22),
		Panel:      rgb(0x22, 0x1F, 0x33),
		Text:       rgb(0xF3, 0xF1, 0xFF),
		Muted:      rgb(0x9C, 0x97, 0xB8),
		Accent:     rgb(0xA7, 0x9B, 0xFF),
		AccentSoft: rgb(0x7C, 0x6F, 0xE0),
	}
	LightTheme = Theme{
		Background: rgb(0xF7, 0xF5, 0xFF),
		Panel:      rgb(0xFF, 0xFF, 0xFF),
		Text:       rgb(0x1E, 0x1B, 0x2E),
		Muted:      rgb(0x6B, 0x67, 0x80),
		Accent:     rgb(0x5B, 0x4B, 0xD6),
		AccentSoft: rgb(0x7C, 0x6F, 0xE0),
	}
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 0xFF} }

// ThemeByName returns the palette for "dark" or "light".
func ThemeByName(name string) (Theme, error) {
	switch strings.ToLower(name) {
	case "", "dark":
		return DarkTheme, nil
	case "light":
		return LightTheme, nil
	default:
		return Theme{}, fmt.Errorf("unknown theme %q: use dark or light", name)
	}
}

// Card image size: the 1.91:1 ratio social networks use for link previews.
const (
	CardWidth  = 1200
	CardHeight = 630
)

var (
	fontsOnce            sync.Once
	fontsErr             error
	regularFont, boldFnt *opentype.Font
)

func loadFonts() error {
	fontsOnce.Do(func() {
		if regularFont, fontsErr = opentype.Parse(goregular.TTF); fontsErr != nil {
			return
		}
		boldFnt, fontsErr = opentype.Parse(gobold.TTF)
	})
	return fontsErr
}

func face(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

// RenderPNG draws the card and encodes it as PNG.
func RenderPNG(w io.Writer, c Card, t Theme) error {
	img, err := RenderImage(c, t)
	if err != nil {
		return err
	}
	return png.Encode(w, img)
}

// RenderImage draws the card onto a new CardWidth x CardHeight image.
func RenderImage(c Card, t Theme) (*image.RGBA, error) {
	if err := loadFonts(); err != nil {
		return nil, fmt.Errorf("load fonts: %w", err)
	}
	faces := map[string]font.Face{}
	for name, spec := range map[string]struct {
		f    *opentype.Font
		size float64
	}{
		"title": {boldFnt, 34}, "subtitle": {regularFont, 26}, "hero": {boldFnt, 104},
		"heroLabel": {regularFont, 30}, "label": {regularFont, 21},
		"fact": {regularFont, 24}, "footer": {regularFont, 20},
	} {
		fc, err := face(spec.f, spec.size)
		if err != nil {
			return nil, err
		}
		defer func() { _ = fc.Close() }()
		faces[name] = fc
	}

	img := image.NewRGBA(image.Rect(0, 0, CardWidth, CardHeight))
	draw.Draw(img, img.Bounds(), &image.Uniform{t.Background}, image.Point{}, draw.Src)

	// Accent stripe across the top.
	for x := 0; x < CardWidth; x++ {
		frac := float64(x) / float64(CardWidth)
		col := lerp(t.AccentSoft, t.Accent, frac)
		for y := 0; y < 10; y++ {
			img.SetRGBA(x, y, col)
		}
	}

	const margin = 64
	leftW := 500
	drawText(img, faces["title"], t.Accent, margin, 104, fit(faces["title"], c.Title, leftW))
	drawText(img, faces["subtitle"], t.Muted, margin, 146, fit(faces["subtitle"], c.Subtitle, leftW))
	if err := drawShrunk(img, boldFnt, []float64{104, 92, 80, 68, 56}, t.Text, margin-4, 300, c.HeroValue, leftW); err != nil {
		return nil, err
	}
	drawText(img, faces["heroLabel"], t.Muted, margin, 348, fit(faces["heroLabel"], c.HeroLabel, leftW))

	if c.FunFact != "" {
		y := 430
		for i, line := range wrap(faces["fact"], c.FunFact, leftW) {
			if i == 3 {
				break
			}
			drawText(img, faces["fact"], t.Text, margin, y, line)
			y += 34
		}
		fillRect(img, image.Rect(margin-22, 404, margin-16, 404+34*min(3, len(wrap(faces["fact"], c.FunFact, leftW)))), t.Accent)
	}
	drawText(img, faces["footer"], t.Muted, margin, CardHeight-44, fit(faces["footer"], c.Footer, leftW))

	// Stat tiles: 2 columns x 3 rows on the right.
	const (
		gridX, gridY = 612, 64
		tileW, tileH = 254, 152
		gap          = 16
	)
	for i, s := range c.Stats {
		if i >= 6 {
			break
		}
		col, row := i%2, i/2
		x0 := gridX + col*(tileW+gap)
		y0 := gridY + row*(tileH+gap)
		roundRect(img, image.Rect(x0, y0, x0+tileW, y0+tileH), 18, t.Panel)
		inner := tileW - 2*26
		drawText(img, faces["label"], t.Muted, x0+26, y0+56, fit(faces["label"], s.Label, inner))
		if err := drawShrunk(img, boldFnt, []float64{32, 28, 24}, t.Accent, x0+26, y0+108, s.Value, inner); err != nil {
			return nil, err
		}
	}
	return img, nil
}

// drawShrunk draws s with the largest of sizes (descending) at which it fits
// maxW, truncating with an ellipsis at the smallest size if none does.
func drawShrunk(dst draw.Image, f *opentype.Font, sizes []float64, c color.Color, x, y int, s string, maxW int) error {
	for i, size := range sizes {
		fc, err := face(f, size)
		if err != nil {
			return err
		}
		if i == len(sizes)-1 || font.MeasureString(fc, s) <= fixed.I(maxW) {
			drawText(dst, fc, c, x, y, fit(fc, s, maxW))
			return fc.Close()
		}
		_ = fc.Close()
	}
	return nil
}

func lerp(a, b color.RGBA, f float64) color.RGBA {
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xFF}
}

func drawText(dst draw.Image, f font.Face, c color.Color, x, y int, s string) {
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

// fit truncates s with an ellipsis so it renders within maxW pixels.
func fit(f font.Face, s string, maxW int) string {
	limit := fixed.I(maxW)
	if font.MeasureString(f, s) <= limit {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		cand := strings.TrimRight(string(r), " ") + "…"
		if font.MeasureString(f, cand) <= limit {
			return cand
		}
	}
	return ""
}

// wrap splits s into lines no wider than maxW pixels. It breaks only at
// regular spaces, so non-breaking spaces (U+00A0) keep words together.
func wrap(f font.Face, s string, maxW int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Split(s, " ") {
		if word == "" {
			continue
		}
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if font.MeasureString(f, cand) <= fixed.I(maxW) || cur == "" {
			cur = cand
			continue
		}
		lines = append(lines, cur)
		cur = word
	}
	if cur != "" {
		lines = append(lines, fit(f, cur, maxW))
	}
	return lines
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

// roundRect fills r with rounded corners of the given radius (antialiased).
func roundRect(img *image.RGBA, r image.Rectangle, radius int, c color.RGBA) {
	rad := float64(radius)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			// Distance from the nearest corner center, when inside a corner box.
			cx, cy := float64(x)+0.5, float64(y)+0.5
			ox, oy := 0.0, 0.0
			switch {
			case cx < float64(r.Min.X)+rad:
				ox = float64(r.Min.X) + rad - cx
			case cx > float64(r.Max.X)-rad:
				ox = cx - (float64(r.Max.X) - rad)
			}
			switch {
			case cy < float64(r.Min.Y)+rad:
				oy = float64(r.Min.Y) + rad - cy
			case cy > float64(r.Max.Y)-rad:
				oy = cy - (float64(r.Max.Y) - rad)
			}
			cov := 1.0
			if ox > 0 && oy > 0 {
				d := ox*ox + oy*oy
				switch {
				case d >= (rad+0.5)*(rad+0.5):
					continue
				case d > (rad-0.5)*(rad-0.5):
					cov = rad + 0.5 - math.Sqrt(d)
				}
			}
			if cov >= 1 {
				img.SetRGBA(x, y, c)
				continue
			}
			bg := img.RGBAAt(x, y)
			img.SetRGBA(x, y, lerp(bg, c, cov))
		}
	}
}
