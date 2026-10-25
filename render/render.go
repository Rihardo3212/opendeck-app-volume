// Package render draws the key images for the App Volume plugin.
// It is pure Go so it can be tested on any OS.
package render

import (
	"bytes"
	"encoding/base64"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"unicode"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const Size = 144

var (
	ColBG         = color.RGBA{0x14, 0x14, 0x18, 0xff}
	ColBGSelected = color.RGBA{0x1d, 0x2a, 0x45, 0xff}
	ColAccent     = color.RGBA{0x4f, 0x8c, 0xff, 0xff}
	ColMuted      = color.RGBA{0xef, 0x44, 0x44, 0xff}
	ColText       = color.RGBA{0xf2, 0xf2, 0xf5, 0xff}
	ColDim        = color.RGBA{0x9a, 0x9a, 0xa5, 0xff}
	ColTrack      = color.RGBA{0x3a, 0x3a, 0x44, 0xff}
	ColEmpty      = color.RGBA{0x0c, 0x0c, 0x0f, 0xff}
)

var boldTT, mediumTT *opentype.Font

func init() {
	boldTT, _ = opentype.Parse(gobold.TTF)
	mediumTT, _ = opentype.Parse(gomedium.TTF)
}

func face(f *opentype.Font, size float64) font.Face {
	fc, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	return fc
}

// App is what a key needs to know to draw one application.
type App struct {
	Name   string
	Icon   image.Image // may be nil
	Volume float64     // 0..1
	Muted  bool
}

func fill(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

// DrawText draws s centered horizontally at baseline y, shrinking the font until it fits maxW.
func DrawText(img *image.RGBA, s string, f *opentype.Font, size float64, cx, y, maxW int, c color.Color) {
	for size > 8 {
		fc := face(f, size)
		w := font.MeasureString(fc, s).Ceil()
		if w <= maxW {
			d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: fc,
				Dot: fixed.P(cx-w/2, y)}
			d.DrawString(s)
			return
		}
		size -= 1
	}
}

func roundedRect(z *vector.Rasterizer, x0, y0, x1, y1, r float32) {
	const k = 0.5523 // bezier circle approximation
	z.MoveTo(x0+r, y0)
	z.LineTo(x1-r, y0)
	z.CubeTo(x1-r+r*k, y0, x1, y0+r-r*k, x1, y0+r)
	z.LineTo(x1, y1-r)
	z.CubeTo(x1, y1-r+r*k, x1-r+r*k, y1, x1-r, y1)
	z.LineTo(x0+r, y1)
	z.CubeTo(x0+r-r*k, y1, x0, y1-r+r*k, x0, y1-r)
	z.LineTo(x0, y0+r)
	z.CubeTo(x0, y0+r-r*k, x0+r-r*k, y0, x0+r, y0)
	z.ClosePath()
}

func fillShape(dst *image.RGBA, c color.Color, build func(z *vector.Rasterizer)) {
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	build(z)
	z.Draw(dst, b, image.NewUniform(c), image.Point{})
}

func tileColor(name string) color.RGBA {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	hue := float64(h.Sum32()%360) / 360
	return hsl(hue, 0.55, 0.45)
}

func hsl(h, s, l float64) color.RGBA {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * math.Min(l, 1-l)
		v := l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))
		return uint8(math.Round(v * 255))
	}
	return color.RGBA{f(0), f(8), f(4), 0xff}
}

func initial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}

// Key renders one application slot.
func Key(app *App, selected bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	if app == nil {
		fill(img, img.Bounds(), ColEmpty)
		return img
	}
	bg := ColBG
	if selected {
		bg = ColBGSelected
	}
	fill(img, img.Bounds(), bg)

	// Icon area (big)
	iconRect := image.Rect(14, 10, 130, 126)
	layer := image.NewRGBA(img.Bounds())
	if app.Icon != nil {
		draw.CatmullRom.Scale(layer, iconRect, app.Icon, app.Icon.Bounds(), draw.Over, nil)
	} else {
		tc := tileColor(app.Name)
		fillShape(layer, tc, func(z *vector.Rasterizer) {
			roundedRect(z, float32(iconRect.Min.X), float32(iconRect.Min.Y), float32(iconRect.Max.X), float32(iconRect.Max.Y), 16)
		})
		DrawText(layer, initial(app.Name), boldTT, 70, 72, 92, 100, ColText)
	}
	alpha := uint8(255)
	if app.Muted {
		alpha = 80
	}
	draw.DrawMask(img, img.Bounds(), layer, image.Point{}, image.NewUniform(color.Alpha{alpha}), image.Point{}, draw.Over)

	if app.Muted {
		// red slash over the icon
		fillShape(img, ColMuted, func(z *vector.Rasterizer) {
			z.MoveTo(20, 26)
			z.LineTo(30, 16)
			z.LineTo(124, 110)
			z.LineTo(114, 120)
			z.ClosePath()
		})
	}

	// Volume badge in the bottom-right corner of the icon
	label := itoa(int(math.Round(app.Volume*100))) + "%"
	badgeBG := color.RGBA{0x10, 0x10, 0x14, 0xe6}
	textCol := color.Color(ColText)
	if app.Muted {
		label = "MUTE"
		badgeBG = ColMuted
	}
	fc := face(boldTT, 30)
	tw := font.MeasureString(fc, label).Ceil()
	bx1, by1 := 138, 136
	bx0, by0 := bx1-tw-16, by1-38
	fillShape(img, badgeBG, func(z *vector.Rasterizer) {
		roundedRect(z, float32(bx0), float32(by0), float32(bx1), float32(by1), 10)
	})
	d := &font.Drawer{Dst: img, Src: image.NewUniform(textCol), Face: fc, Dot: fixed.P(bx0+8, by1-9)}
	d.DrawString(label)

	if selected {
		dashedBorder(img, color.White, 4, 12, 8)
	}
	return img
}

// dashedBorder draws a dashed frame of thickness t (dash length dl, gap gl) around the image edge.
func dashedBorder(img *image.RGBA, c color.Color, t, dl, gl int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	per := 2*(w-t) + 2*(h-t)
	for p := 0; p < per; p += dl + gl {
		for q := p; q < p+dl && q < per; q++ {
			var x, y int
			switch {
			case q < w-t:
				x, y = q, 0
			case q < w-t+h-t:
				x, y = w-t, q-(w-t)
			case q < 2*(w-t)+h-t:
				x, y = w-t-(q-(w-t+h-t)), h-t
			default:
				x, y = 0, h-t-(q-(2*(w-t)+h-t))
			}
			fill(img, image.Rect(x, y, x+t, y+t), c)
		}
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// PNG encodes an image.
func PNG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// DataURL encodes an image as a data: URL for setImage.
func DataURL(img image.Image) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(PNG(img))
}

// ---------- Static glyphs (used for action icons and fallback key images) ----------

// Glyph kinds
const (
	GlyphSpeaker = iota
	GlyphPlus
	GlyphMinus
	GlyphMute
	GlyphKnob
	GlyphGrid
)

// Glyph draws a white symbol on a transparent (or bg) square of the given size.
func Glyph(kind, size int, bg color.Color, fg color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	if bg != nil {
		fill(img, img.Bounds(), bg)
	}
	s := float32(size) / 100 // design on a 100x100 grid
	fillShape(img, fg, func(z *vector.Rasterizer) {
		speaker := func(ox float32) {
			z.MoveTo((ox+10)*s, 38*s)
			z.LineTo((ox+26)*s, 38*s)
			z.LineTo((ox+46)*s, 20*s)
			z.LineTo((ox+46)*s, 80*s)
			z.LineTo((ox+26)*s, 62*s)
			z.LineTo((ox+10)*s, 62*s)
			z.ClosePath()
		}
		arc := func(cx, cy, r, th float32) {
			// annular sector from -50deg to 50deg
			n := 16
			a0, a1 := -50.0*math.Pi/180, 50.0*math.Pi/180
			for i := 0; i <= n; i++ {
				a := a0 + (a1-a0)*float64(i)/float64(n)
				x, y := cx+(r+th/2)*float32(math.Cos(a)), cy+(r+th/2)*float32(math.Sin(a))
				if i == 0 {
					z.MoveTo(x*s, y*s)
				} else {
					z.LineTo(x*s, y*s)
				}
			}
			for i := n; i >= 0; i-- {
				a := a0 + (a1-a0)*float64(i)/float64(n)
				z.LineTo((cx+(r-th/2)*float32(math.Cos(a)))*s, (cy+(r-th/2)*float32(math.Sin(a)))*s)
			}
			z.ClosePath()
		}
		bar := func(x0, y0, x1, y1 float32) {
			z.MoveTo(x0*s, y0*s)
			z.LineTo(x1*s, y0*s)
			z.LineTo(x1*s, y1*s)
			z.LineTo(x0*s, y1*s)
			z.ClosePath()
		}
		line := func(xa, ya, xb, yb, th float32) {
			dx, dy := xb-xa, yb-ya
			l := float32(math.Hypot(float64(dx), float64(dy)))
			nx, ny := -dy/l*th/2, dx/l*th/2
			z.MoveTo((xa+nx)*s, (ya+ny)*s)
			z.LineTo((xb+nx)*s, (yb+ny)*s)
			z.LineTo((xb-nx)*s, (yb-ny)*s)
			z.LineTo((xa-nx)*s, (ya-ny)*s)
			z.ClosePath()
		}
		switch kind {
		case GlyphSpeaker:
			speaker(4)
			arc(50, 50, 16, 7)
			arc(50, 50, 30, 7)
		case GlyphPlus:
			speaker(0)
			bar(58, 46, 92, 54)
			bar(71, 33, 79, 67)
		case GlyphMinus:
			speaker(0)
			bar(58, 46, 92, 54)
		case GlyphMute:
			speaker(0)
			line(60, 36, 88, 64, 8)
			line(88, 36, 60, 64, 8)
		case GlyphKnob:
			// ring
			n := 48
			for i := 0; i <= n; i++ {
				a := 2 * math.Pi * float64(i) / float64(n)
				x, y := 50+36*float32(math.Cos(a)), 50+36*float32(math.Sin(a))
				if i == 0 {
					z.MoveTo(x*s, y*s)
				} else {
					z.LineTo(x*s, y*s)
				}
			}
			for i := n; i >= 0; i-- {
				a := 2 * math.Pi * float64(i) / float64(n)
				z.LineTo((50+27*float32(math.Cos(a)))*s, (50+27*float32(math.Sin(a)))*s)
			}
			z.ClosePath()
			line(50, 50, 50, 22, 8)
		case GlyphGrid:
			for _, x := range []float32{14, 54} {
				for _, y := range []float32{14, 54} {
					bar(x, y, x+32, y+32)
				}
			}
		}
	})
	return img
}

// LabeledKey draws a static key image: glyph on top, label below.
func LabeledKey(kind int, label string) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Size, Size))
	fill(img, img.Bounds(), ColBG)
	g := Glyph(kind, 84, nil, ColText)
	draw.Draw(img, image.Rect(30, 10, 114, 94), g, image.Point{}, draw.Over)
	if label != "" {
		DrawText(img, label, mediumTT, 20, 72, 124, 130, ColDim)
	}
	return img
}
