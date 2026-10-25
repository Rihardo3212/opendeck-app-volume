package main

import (
	"image"
	"image/color"
	"image/draw"
	"os"

	"appvolume/render"
)

func main() {
	icon := render.Glyph(render.GlyphSpeaker, 128, color.RGBA{0x1d, 0xb9, 0x54, 0xff}, color.White)
	keys := []*image.RGBA{
		render.Key(&render.App{Name: "Spotify", Icon: icon, Volume: 0.62}, false),
		render.Key(&render.App{Name: "Discord", Volume: 0.35}, true),
		render.Key(&render.App{Name: "Firefox", Volume: 1}, false),
		render.Key(&render.App{Name: "Steam", Icon: icon, Volume: 0.4, Muted: true}, true),
		render.Key(nil, false),
		render.LabeledKey(render.GlyphPlus, "Vol +"),
		render.LabeledKey(render.GlyphMute, "Mute"),
		render.LabeledKey(render.GlyphKnob, ""),
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 4*154+10, 2*154+10))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{60, 60, 60, 255}), image.Point{}, draw.Src)
	for i, k := range keys {
		x, y := 10+(i%4)*154, 10+(i/4)*154
		draw.Draw(sheet, image.Rect(x, y, x+144, y+144), k, image.Point{}, draw.Src)
	}
	os.WriteFile(os.Args[1], render.PNG(sheet), 0o644)
}
