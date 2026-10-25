package main

import (
	"image"
	"runtime"
	"time"
)

// AppInfo is one application that has audio sessions (all of its sessions grouped together).
type AppInfo struct {
	Key    string // stable identity (lower-case exe path)
	Name   string
	Volume float64 // 0..1
	Muted  bool
	Active bool // currently playing sound
	Icon   image.Image
}

// Backend talks to the OS mixer. All methods are called from a single locked OS thread.
type Backend interface {
	Snapshot() []AppInfo
	AdjustVolume(key string, delta float64)
	ToggleMute(key string)
}

// AudioWorker owns the backend on one OS thread (COM likes that) and polls it.
type AudioWorker struct {
	cmds     chan func(Backend)
	onUpdate func([]AppInfo)
}

func StartAudioWorker(onUpdate func([]AppInfo)) *AudioWorker {
	w := &AudioWorker{cmds: make(chan func(Backend), 64), onUpdate: onUpdate}
	go w.run()
	return w
}

func (w *AudioWorker) run() {
	runtime.LockOSThread()
	b, err := newBackend()
	if err != nil {
		logf("audio backend init failed: %v", err)
		return
	}
	t := time.NewTicker(400 * time.Millisecond)
	defer t.Stop()
	w.onUpdate(safeSnapshot(b))
	for {
		select {
		case <-t.C:
			w.onUpdate(safeSnapshot(b))
		case c := <-w.cmds:
			func() {
				defer recoverLog("command")
				c(b)
			}()
			// drain any queued commands (fast knob turns) before refreshing
		drain:
			for {
				select {
				case c2 := <-w.cmds:
					func() {
						defer recoverLog("command")
						c2(b)
					}()
				default:
					break drain
				}
			}
			w.onUpdate(safeSnapshot(b))
		}
	}
}

func safeSnapshot(b Backend) (apps []AppInfo) {
	defer recoverLog("snapshot")
	return b.Snapshot()
}

func recoverLog(where string) {
	if r := recover(); r != nil {
		logf("panic in %s: %v", where, r)
	}
}

func (w *AudioWorker) Do(f func(Backend)) {
	select {
	case w.cmds <- f:
	default:
		logf("command queue full, dropping")
	}
}
