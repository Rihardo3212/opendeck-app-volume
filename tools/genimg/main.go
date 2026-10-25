// genimg writes the static images for the plugin bundle.
package main

import (
	"image/color"
	"os"
	"path/filepath"

	"appvolume/render"
)

func write(dir, name string, b []byte) {
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		panic(err)
	}
}

func main() {
	dir := os.Args[1]
	os.MkdirAll(dir, 0o755)
	white := color.RGBA{0xff, 0xff, 0xff, 0xff}
	icons := map[string]int{
		"plugin": render.GlyphSpeaker, "category": render.GlyphSpeaker,
		"slot": render.GlyphGrid, "dial": render.GlyphKnob,
		"up": render.GlyphPlus, "down": render.GlyphMinus, "mute": render.GlyphMute, "master": render.GlyphSpeaker,
	}
	for name, g := range icons {
		write(dir, name+".png", render.PNG(render.Glyph(g, 28, nil, white)))
		write(dir, name+"@2x.png", render.PNG(render.Glyph(g, 56, nil, white)))
	}
	keys := map[string]struct {
		g     int
		label string
	}{
		"key-slot": {render.GlyphSpeaker, "App"}, "key-dial": {render.GlyphKnob, ""},
		"key-up": {render.GlyphPlus, "Vol +"}, "key-down": {render.GlyphMinus, "Vol -"},
		"key-mute": {render.GlyphMute, "Mute"}, "key-master": {render.GlyphSpeaker, "Master"},
	}
	for name, k := range keys {
		write(dir, name+".png", render.PNG(render.LabeledKey(k.g, k.label)))
	}
}
