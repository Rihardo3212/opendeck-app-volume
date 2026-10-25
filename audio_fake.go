//go:build !windows

package main

import (
	"math"
	"sort"
	"time"
)

// fakeBackend lets the plugin run on non-Windows machines for testing.
type fakeBackend struct {
	apps   map[string]*AppInfo
	start  time.Time
	master float64
	mMuted bool
}

func newBackend() (Backend, error) {
	f := &fakeBackend{apps: map[string]*AppInfo{}, start: time.Now(), master: 0.5}
	for i, n := range []string{"Spotify", "Discord", "Firefox", "Steam"} {
		f.apps[n] = &AppInfo{Key: n, Name: n, Volume: 0.2 + 0.2*float64(i), Active: true}
	}
	return f, nil
}

func (f *fakeBackend) Snapshot() []AppInfo {
	// Steam stops "playing" after 3 seconds to exercise the hide logic.
	if time.Since(f.start) > 3*time.Second {
		f.apps["Steam"].Active = false
	}
	var out []AppInfo
	for _, a := range f.apps {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (f *fakeBackend) AdjustVolume(key string, d float64) {
	if a, ok := f.apps[key]; ok {
		a.Volume = math.Max(0, math.Min(1, a.Volume+d))
	}
}

func (f *fakeBackend) ToggleMute(key string) {
	if a, ok := f.apps[key]; ok {
		a.Muted = !a.Muted
	}
}

func (f *fakeBackend) AdjustMaster(d float64) {
	f.master = math.Max(0, math.Min(1, f.master+d))
	logf("master volume %.2f", f.master)
}

func (f *fakeBackend) ToggleMasterMute() {
	f.mMuted = !f.mMuted
	logf("master muted %v", f.mMuted)
}
