// fakehost imitates OpenDeck: starts the plugin, sends events, records images.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"appvolume/render"

	"github.com/gorilla/websocket"
)

func main() {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	var mu sync.Mutex
	images := map[string]string{}
	counts := map[string]int{}
	connCh := make(chan *websocket.Conn, 1)
	up := websocket.Upgrader{}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := up.Upgrade(w, r, nil)
		_, reg, _ := c.ReadMessage()
		fmt.Println("register:", string(reg))
		connCh <- c
		for {
			_, m, err := c.ReadMessage()
			if err != nil {
				return
			}
			var ev struct {
				Event, Context string
				Payload        map[string]any
			}
			json.Unmarshal(m, &ev)
			mu.Lock()
			counts[ev.Event]++
			if ev.Event == "setImage" {
				images[ev.Context] = ev.Payload["image"].(string)
			}
			if ev.Event != "setImage" {
				fmt.Println("plugin ->", string(m))
			}
			mu.Unlock()
		}
	}))
	cmd := exec.Command(os.Args[1], "-port", fmt.Sprint(port), "-pluginUUID", "com.rihardo.appvolume", "-registerEvent", "registerPlugin", "-info", "{}")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Start()
	defer cmd.Process.Kill()
	c := <-connCh
	send := func(v map[string]any) { c.WriteJSON(v); time.Sleep(150 * time.Millisecond) }
	ctx := func(pos int, ctl string) string { return fmt.Sprintf("dev.prof.%s.%d.0", ctl, pos) }
	for i := 0; i < 6; i++ {
		send(map[string]any{"event": "willAppear", "action": "com.rihardo.appvolume.slot", "context": ctx(i, "Keypad"), "device": "dev",
			"payload": map[string]any{"settings": map[string]any{}, "coordinates": map[string]any{"row": i / 3, "column": i % 3}, "controller": "Keypad"}})
	}
	send(map[string]any{"event": "willAppear", "action": "com.rihardo.appvolume.dial", "context": ctx(0, "Encoder"), "device": "dev",
		"payload": map[string]any{"settings": map[string]any{}, "coordinates": map[string]any{"row": 0, "column": 0}, "controller": "Encoder"}})
	time.Sleep(time.Second)
	// knob before selecting -> expect showAlert
	send(map[string]any{"event": "dialRotate", "action": "com.rihardo.appvolume.dial", "context": ctx(0, "Encoder"), "device": "dev", "payload": map[string]any{"ticks": 1, "settings": map[string]any{}}})
	// select 2nd key (Discord), turn knob +5 ticks at step 3
	send(map[string]any{"event": "keyDown", "action": "com.rihardo.appvolume.slot", "context": ctx(1, "Keypad"), "device": "dev", "payload": map[string]any{}})
	for i := 0; i < 5; i++ {
		send(map[string]any{"event": "dialRotate", "action": "com.rihardo.appvolume.dial", "context": ctx(0, "Encoder"), "device": "dev", "payload": map[string]any{"ticks": 1, "settings": map[string]any{"step": 3}}})
	}
	// select first key then press it again -> mute
	send(map[string]any{"event": "keyDown", "action": "com.rihardo.appvolume.slot", "context": ctx(0, "Keypad"), "device": "dev", "payload": map[string]any{}})
	send(map[string]any{"event": "keyDown", "action": "com.rihardo.appvolume.slot", "context": ctx(0, "Keypad"), "device": "dev", "payload": map[string]any{}})
	send(map[string]any{"event": "willAppear", "action": "com.rihardo.appvolume.master", "context": ctx(1, "Encoder"), "device": "dev", "payload": map[string]any{"settings": map[string]any{}, "coordinates": map[string]any{"row": 0, "column": 1}, "controller": "Encoder"}})
	send(map[string]any{"event": "dialRotate", "action": "com.rihardo.appvolume.master", "context": ctx(1, "Encoder"), "device": "dev", "payload": map[string]any{"ticks": -3, "settings": map[string]any{}}})
	send(map[string]any{"event": "dialDown", "action": "com.rihardo.appvolume.master", "context": ctx(1, "Encoder"), "device": "dev", "payload": map[string]any{}})
	time.Sleep(4 * time.Second) // Steam goes quiet but stays (30s window)
	mu.Lock()
	fmt.Println("counts:", counts)
	sheet := image.NewRGBA(image.Rect(0, 0, 3*154+10, 2*154+10))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{60, 60, 60, 255}), image.Point{}, draw.Src)
	for i := 0; i < 6; i++ {
		d := images[ctx(i, "Keypad")]
		b, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(d, "data:image/png;base64,"))
		img, _, err := image.Decode(strings.NewReader(string(b)))
		if err != nil {
			continue
		}
		x, y := 10+(i%3)*154, 10+(i/3)*154
		draw.Draw(sheet, image.Rect(x, y, x+144, y+144), img, image.Point{}, draw.Src)
	}
	mu.Unlock()
	os.WriteFile(os.Args[2], render.PNG(sheet), 0o644)
}
