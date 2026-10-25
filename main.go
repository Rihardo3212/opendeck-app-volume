// App Volume: a Stream Deck SDK v2 plugin (for OpenDeck) that shows the apps
// currently playing sound on keys. Press a key to select that app, then turn
// a knob to change its volume. Press the knob (or the selected key again) to mute.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"appvolume/render"

	"github.com/gorilla/websocket"
)

const (
	pluginID     = "com.rihardo.appvolume"
	actSlot      = pluginID + ".slot"
	actDial      = pluginID + ".dial"
	actUp        = pluginID + ".up"
	actDown      = pluginID + ".down"
	actMute      = pluginID + ".mute"
	actMaster    = pluginID + ".master"
	keepVisible  = 30 * time.Second // keep an app on its key this long after it goes quiet
	defaultDial  = 2.0              // % per knob tick
	defaultKeyUp = 5.0              // % per key press
)

func logf(format string, args ...any) { log.Printf(format, args...) }

type inEvent struct {
	Event   string          `json:"event"`
	Action  string          `json:"action"`
	Context string          `json:"context"`
	Device  string          `json:"device"`
	Payload json.RawMessage `json:"payload"`
}

type payload struct {
	Settings    map[string]any `json:"settings"`
	Coordinates struct {
		Column int `json:"column"`
		Row    int `json:"row"`
	} `json:"coordinates"`
	Ticks int `json:"ticks"`
}

type slot struct {
	context  string
	device   string
	row, col int
	appKey   string // which app this key currently shows
	lastSig  string
}

type Plugin struct {
	mu   sync.Mutex
	wmu  sync.Mutex
	conn *websocket.Conn

	slots      map[string]*slot
	apps       map[string]AppInfo
	order      []string // app keys in slot order
	lastActive map[string]time.Time
	selected   string

	audio *AudioWorker
}

func main() {
	port := flag.Int("port", 0, "")
	uuid := flag.String("pluginUUID", "", "")
	reg := flag.String("registerEvent", "", "")
	flag.String("info", "", "")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	if *port == 0 {
		fmt.Println("This is an OpenDeck / Stream Deck plugin; it is started by OpenDeck.")
		os.Exit(1)
	}

	conn, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d", *port), nil)
	if err != nil {
		logf("connect failed: %v", err)
		os.Exit(1)
	}
	p := &Plugin{
		conn:       conn,
		slots:      map[string]*slot{},
		apps:       map[string]AppInfo{},
		lastActive: map[string]time.Time{},
	}
	p.send(map[string]any{"event": *reg, "uuid": *uuid})
	logf("registered as %s", *uuid)

	p.audio = StartAudioWorker(p.onAudio)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			logf("connection closed: %v", err)
			os.Exit(0)
		}
		var ev inEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		p.handle(ev)
	}
}

func (p *Plugin) send(v any) {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	if err := p.conn.WriteJSON(v); err != nil {
		logf("write failed: %v", err)
	}
}

func (p *Plugin) handle(ev inEvent) {
	var pl payload
	_ = json.Unmarshal(ev.Payload, &pl)

	p.mu.Lock()
	defer p.mu.Unlock()

	switch ev.Event {
	case "willAppear":
		if ev.Action == actSlot {
			p.slots[ev.Context] = &slot{context: ev.Context, device: ev.Device, row: pl.Coordinates.Row, col: pl.Coordinates.Column}
			p.send(map[string]any{"event": "setTitle", "context": ev.Context, "payload": map[string]any{"title": ""}})
			p.refreshSlots()
		}
	case "willDisappear":
		if ev.Action == actSlot {
			delete(p.slots, ev.Context)
			p.refreshSlots()
		}
	case "keyDown":
		switch ev.Action {
		case actSlot:
			s := p.slots[ev.Context]
			if s == nil || s.appKey == "" {
				return
			}
			if p.selected == s.appKey {
				p.toggleMute(ev.Context)
			} else {
				p.selected = s.appKey
				p.refreshSlots()
			}
		case actUp:
			p.adjust(ev.Context, step(pl.Settings, defaultKeyUp)/100)
		case actDown:
			p.adjust(ev.Context, -step(pl.Settings, defaultKeyUp)/100)
		case actMute:
			p.toggleMute(ev.Context)
		}
	case "dialRotate":
		if pl.Ticks == 0 {
			return
		}
		delta := float64(pl.Ticks) * step(pl.Settings, defaultDial) / 100
		switch ev.Action {
		case actDial:
			p.adjust(ev.Context, delta)
		case actMaster:
			p.audio.Do(func(b Backend) { b.AdjustMaster(delta) })
		}
	case "dialDown":
		switch ev.Action {
		case actDial:
			p.toggleMute(ev.Context)
		case actMaster:
			p.audio.Do(func(b Backend) { b.ToggleMasterMute() })
		}
	}
}

func step(settings map[string]any, def float64) float64 {
	if v, ok := settings["step"]; ok {
		switch t := v.(type) {
		case float64:
			if t > 0 && t <= 50 {
				return t
			}
		case string:
			var f float64
			if _, err := fmt.Sscanf(t, "%g", &f); err == nil && f > 0 && f <= 50 {
				return f
			}
		}
	}
	return def
}

// selectedOK reports whether an app is selected and still has audio sessions.
func (p *Plugin) selectedOK(ctx string) bool {
	if _, ok := p.apps[p.selected]; p.selected != "" && ok {
		return true
	}
	p.send(map[string]any{"event": "showAlert", "context": ctx})
	return false
}

func (p *Plugin) adjust(ctx string, delta float64) {
	if !p.selectedOK(ctx) {
		return
	}
	key := p.selected
	// optimistic update so the key reacts instantly
	if a, ok := p.apps[key]; ok {
		a.Volume = max(0, min(1, a.Volume+delta))
		p.apps[key] = a
		p.refreshSlots()
	}
	p.audio.Do(func(b Backend) { b.AdjustVolume(key, delta) })
}

func (p *Plugin) toggleMute(ctx string) {
	if !p.selectedOK(ctx) {
		return
	}
	key := p.selected
	if a, ok := p.apps[key]; ok {
		a.Muted = !a.Muted
		p.apps[key] = a
		p.refreshSlots()
	}
	p.audio.Do(func(b Backend) { b.ToggleMute(key) })
}

// onAudio receives a fresh mixer snapshot from the audio thread.
func (p *Plugin) onAudio(list []AppInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	p.apps = map[string]AppInfo{}
	for _, a := range list {
		p.apps[a.Key] = a
		if a.Active {
			p.lastActive[a.Key] = now
		}
	}
	visible := func(k string) bool {
		if _, ok := p.apps[k]; !ok {
			return false
		}
		t, ok := p.lastActive[k]
		return (ok && now.Sub(t) < keepVisible) || k == p.selected
	}
	// keep existing order, drop vanished apps, append newcomers
	var order []string
	seen := map[string]bool{}
	for _, k := range p.order {
		if visible(k) {
			order = append(order, k)
			seen[k] = true
		}
	}
	for _, a := range list {
		if !seen[a.Key] && visible(a.Key) {
			order = append(order, a.Key)
			seen[a.Key] = true
		}
	}
	p.order = order
	for k, t := range p.lastActive {
		if now.Sub(t) > 10*time.Minute {
			delete(p.lastActive, k)
		}
	}
	p.refreshSlots()
}

// refreshSlots assigns apps to slot keys (top-left to bottom-right) and redraws changed keys.
func (p *Plugin) refreshSlots() {
	list := make([]*slot, 0, len(p.slots))
	for _, s := range p.slots {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.device != b.device {
			return a.device < b.device
		}
		if a.row != b.row {
			return a.row < b.row
		}
		return a.col < b.col
	})
	// each device gets its own copy of the list
	idx := map[string]int{}
	for _, s := range list {
		i := idx[s.device]
		idx[s.device] = i + 1
		var app *AppInfo
		if i < len(p.order) {
			if a, ok := p.apps[p.order[i]]; ok {
				app = &a
			}
		}
		var sig string
		if app == nil {
			s.appKey = ""
			sig = "empty"
		} else {
			s.appKey = app.Key
			sig = fmt.Sprintf("%s|%d|%v|%v|%v", app.Key, int(app.Volume*100+0.5), app.Muted, app.Key == p.selected, app.Icon != nil)
		}
		if sig == s.lastSig {
			continue
		}
		s.lastSig = sig
		var img = render.Key(nil, false)
		if app != nil {
			img = render.Key(&render.App{Name: app.Name, Icon: app.Icon, Volume: app.Volume, Muted: app.Muted}, app.Key == p.selected)
		}
		p.send(map[string]any{"event": "setImage", "context": s.context, "payload": map[string]any{"image": render.DataURL(img), "target": 0}})
	}
}
