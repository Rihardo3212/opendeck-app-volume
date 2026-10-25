//go:build windows

package main

import (
	"fmt"
	"image"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modOle32             = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")

	clsidMMDeviceEnumerator  = mustGUID("{BCDE0395-E52F-467C-8E3D-C4579291692E}")
	iidIMMDeviceEnumerator   = mustGUID("{A95664D2-9614-4F35-A746-DE8DB63617E6}")
	iidIAudioSessionManager2 = mustGUID("{77AA99A0-1BD6-484F-8BC7-2C654C9A9B6F}")
	iidIAudioSessionControl2 = mustGUID("{BFB7FF88-7239-4FC9-8FA2-07C950BE9C6D}")
	iidIAudioEndpointVolume  = mustGUID("{5CDF2C82-841E-4546-9722-0CF74078229A}")
	iidISimpleAudioVolume    = mustGUID("{87CE5498-68D6-44E5-9215-6DA47EF883D8}")
)

const (
	clsctxAll         = 0x17
	eRender           = 0
	deviceStateActive = 1
	sessionActive     = 1
	sessionExpired    = 2
)

func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}

// comCall calls method #idx of the COM object's vtable.
func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	all := append([]uintptr{obj}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

func comQI(obj uintptr, iid *windows.GUID) uintptr {
	var out uintptr
	if hr := comCall(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))); int32(hr) < 0 {
		return 0
	}
	return out
}

func failed(hr uintptr) bool { return int32(hr) < 0 }

type procEntry struct {
	path string
	seen time.Time
}

type winBackend struct {
	enumerator uintptr
	procs      map[uint32]procEntry
	names      map[string]string
	icons      map[string]image.Image
}

func newBackend() (Backend, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_MULTITHREADED); err != nil {
		// S_FALSE (already initialised) is fine
		logf("CoInitializeEx: %v", err)
	}
	var enum uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&enum)))
	if failed(hr) || enum == 0 {
		return nil, fmt.Errorf("CoCreateInstance(MMDeviceEnumerator) hr=0x%x", hr)
	}
	return &winBackend{
		enumerator: enum,
		procs:      map[uint32]procEntry{},
		names:      map[string]string{},
		icons:      map[string]image.Image{},
	}, nil
}

type session struct {
	pid    uint32
	state  uint32
	volume uintptr // ISimpleAudioVolume*
}

// forEachSession visits every non-system audio session on every active output device.
// The ISimpleAudioVolume pointer is only valid during the callback.
func (b *winBackend) forEachSession(fn func(s session)) {
	var coll uintptr
	if failed(comCall(b.enumerator, 3, eRender, deviceStateActive, uintptr(unsafe.Pointer(&coll)))) || coll == 0 {
		return
	}
	defer comRelease(coll)
	var nDev uint32
	comCall(coll, 3, uintptr(unsafe.Pointer(&nDev)))
	for d := uint32(0); d < nDev; d++ {
		var dev uintptr
		if failed(comCall(coll, 4, uintptr(d), uintptr(unsafe.Pointer(&dev)))) || dev == 0 {
			continue
		}
		var mgr uintptr
		hr := comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioSessionManager2)), clsctxAll, 0, uintptr(unsafe.Pointer(&mgr)))
		comRelease(dev)
		if failed(hr) || mgr == 0 {
			continue
		}
		var senum uintptr
		hr = comCall(mgr, 5, uintptr(unsafe.Pointer(&senum)))
		comRelease(mgr)
		if failed(hr) || senum == 0 {
			continue
		}
		var n int32
		comCall(senum, 3, uintptr(unsafe.Pointer(&n)))
		for i := int32(0); i < n; i++ {
			var ctl uintptr
			if failed(comCall(senum, 4, uintptr(i), uintptr(unsafe.Pointer(&ctl)))) || ctl == 0 {
				continue
			}
			b.visit(ctl, fn)
			comRelease(ctl)
		}
		comRelease(senum)
	}
}

func (b *winBackend) visit(ctl uintptr, fn func(s session)) {
	ctl2 := comQI(ctl, &iidIAudioSessionControl2)
	if ctl2 == 0 {
		return
	}
	defer comRelease(ctl2)
	if comCall(ctl2, 15) == 0 { // IsSystemSoundsSession: S_OK means it is
		return
	}
	var state uint32
	comCall(ctl, 3, uintptr(unsafe.Pointer(&state)))
	if state == sessionExpired {
		return
	}
	var pid uint32
	if failed(comCall(ctl2, 14, uintptr(unsafe.Pointer(&pid)))) || pid == 0 {
		return
	}
	vol := comQI(ctl, &iidISimpleAudioVolume)
	if vol == 0 {
		return
	}
	defer comRelease(vol)
	fn(session{pid: pid, state: state, volume: vol})
}

func getVolume(vol uintptr) (float64, bool) {
	var level float32
	var mute int32
	comCall(vol, 4, uintptr(unsafe.Pointer(&level)))
	comCall(vol, 6, uintptr(unsafe.Pointer(&mute)))
	return float64(level), mute != 0
}

func setVolume(vol uintptr, level float64) {
	comCall(vol, 3, uintptr(math.Float32bits(float32(level))), 0)
}

func setMute(vol uintptr, mute bool) {
	var m uintptr
	if mute {
		m = 1
	}
	comCall(vol, 5, m, 0)
}

// keyFor maps a process id to the app identity (its exe path).
func (b *winBackend) keyFor(pid uint32) (key, path string) {
	now := time.Now()
	e, ok := b.procs[pid]
	if !ok || now.Sub(e.seen) > 20*time.Second {
		e = procEntry{path: processPath(pid), seen: now}
		b.procs[pid] = e
	}
	if e.path == "" {
		return fmt.Sprintf("pid:%d", pid), ""
	}
	return strings.ToLower(e.path), e.path
}

func (b *winBackend) Snapshot() []AppInfo {
	apps := map[string]*AppInfo{}
	var order []string
	b.forEachSession(func(s session) {
		key, path := b.keyFor(s.pid)
		a, ok := apps[key]
		vol, mute := getVolume(s.volume)
		if !ok {
			a = &AppInfo{Key: key, Volume: vol, Muted: mute}
			a.Name, a.Icon = b.describe(key, path, s.pid)
			apps[key] = a
			order = append(order, key)
		}
		if s.state == sessionActive {
			a.Active = true
		}
	})
	out := make([]AppInfo, 0, len(order))
	for _, k := range order {
		out = append(out, *apps[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	// forget stale pids so the map doesn't grow forever
	if len(b.procs) > 512 {
		b.procs = map[uint32]procEntry{}
	}
	return out
}

func (b *winBackend) describe(key, path string, pid uint32) (string, image.Image) {
	if path == "" {
		return fmt.Sprintf("PID %d", pid), nil
	}
	name, ok := b.names[key]
	if !ok {
		name = fileDescription(path)
		if name == "" {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if base != "" {
				name = strings.ToUpper(base[:1]) + base[1:]
			}
		}
		b.names[key] = name
	}
	icon, ok := b.icons[key]
	if !ok {
		icon = extractIcon(path)
		b.icons[key] = icon
	}
	return name, icon
}

func (b *winBackend) withApp(key string, fn func(vols []uintptr)) {
	var vols []uintptr
	b.forEachSession(func(s session) {
		if k, _ := b.keyFor(s.pid); k == key {
			comCall(s.volume, 1) // AddRef: keep it alive past the callback
			vols = append(vols, s.volume)
		}
	})
	if len(vols) > 0 {
		fn(vols)
	}
	for _, v := range vols {
		comRelease(v)
	}
}

func (b *winBackend) AdjustVolume(key string, delta float64) {
	b.withApp(key, func(vols []uintptr) {
		cur, _ := getVolume(vols[0])
		next := math.Max(0, math.Min(1, math.Round((cur+delta)*100)/100))
		for _, v := range vols {
			setVolume(v, next)
		}
	})
}

func (b *winBackend) ToggleMute(key string) {
	b.withApp(key, func(vols []uintptr) {
		_, muted := getVolume(vols[0])
		for _, v := range vols {
			setMute(v, !muted)
		}
	})
}

func processPath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func fileDescription(path string) string {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 {
		return ""
	}
	data := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&data[0])); err != nil {
		return ""
	}
	type langCP struct{ Lang, CP uint16 }
	var tptr unsafe.Pointer
	var tlen uint32
	var candidates []string
	if windows.VerQueryValue(unsafe.Pointer(&data[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tptr), &tlen) == nil && tlen >= 4 {
		t := (*langCP)(tptr)
		candidates = append(candidates, fmt.Sprintf("%04x%04x", t.Lang, t.CP))
	}
	candidates = append(candidates, "040904b0", "040904e4", "000004b0")
	for _, field := range []string{"FileDescription", "ProductName"} {
		for _, c := range candidates {
			var sptr unsafe.Pointer
			var slen uint32
			if windows.VerQueryValue(unsafe.Pointer(&data[0]), `\StringFileInfo\`+c+`\`+field, unsafe.Pointer(&sptr), &slen) == nil && slen > 0 {
				s := strings.TrimSpace(windows.UTF16PtrToString((*uint16)(sptr)))
				if s != "" {
					return s
				}
			}
		}
	}
	return ""
}

// withMaster runs fn with the default output device's IAudioEndpointVolume.
func (b *winBackend) withMaster(fn func(ev uintptr)) {
	var dev uintptr
	// GetDefaultAudioEndpoint(eRender, eMultimedia)
	if failed(comCall(b.enumerator, 4, eRender, 1, uintptr(unsafe.Pointer(&dev)))) || dev == 0 {
		return
	}
	defer comRelease(dev)
	var ev uintptr
	if failed(comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)), clsctxAll, 0, uintptr(unsafe.Pointer(&ev)))) || ev == 0 {
		return
	}
	defer comRelease(ev)
	fn(ev)
}

func (b *winBackend) AdjustMaster(delta float64) {
	b.withMaster(func(ev uintptr) {
		var level float32
		comCall(ev, 9, uintptr(unsafe.Pointer(&level))) // GetMasterVolumeLevelScalar
		next := math.Max(0, math.Min(1, math.Round((float64(level)+delta)*100)/100))
		comCall(ev, 7, uintptr(math.Float32bits(float32(next))), 0) // SetMasterVolumeLevelScalar
	})
}

func (b *winBackend) ToggleMasterMute() {
	b.withMaster(func(ev uintptr) {
		var mute int32
		comCall(ev, 15, uintptr(unsafe.Pointer(&mute))) // GetMute
		var m uintptr
		if mute == 0 {
			m = 1
		}
		comCall(ev, 14, m, 0) // SetMute
	})
}
