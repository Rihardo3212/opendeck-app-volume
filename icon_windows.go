//go:build windows

package main

import (
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32                = windows.NewLazySystemDLL("user32.dll")
	modGdi32                 = windows.NewLazySystemDLL("gdi32.dll")
	procPrivateExtractIconsW = modUser32.NewProc("PrivateExtractIconsW")
	procGetIconInfo          = modUser32.NewProc("GetIconInfo")
	procDestroyIcon          = modUser32.NewProc("DestroyIcon")
	procGetDC                = modUser32.NewProc("GetDC")
	procReleaseDC            = modUser32.NewProc("ReleaseDC")
	procGetObjectW           = modGdi32.NewProc("GetObjectW")
	procGetDIBits            = modGdi32.NewProc("GetDIBits")
	procDeleteObject         = modGdi32.NewProc("DeleteObject")
)

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

type bitmap struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

type bitmapInfo struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	Colors        [2]uint32
}

// extractIcon returns the exe's main icon at up to 128px, or nil.
func extractIcon(path string) (img image.Image) {
	defer func() {
		if r := recover(); r != nil {
			logf("icon extraction panic for %s: %v", path, r)
			img = nil
		}
	}()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	for _, size := range []int{128, 64, 48, 32} {
		var hicon uintptr
		var id uint32
		n, _, _ := procPrivateExtractIconsW.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(size), uintptr(size),
			uintptr(unsafe.Pointer(&hicon)), uintptr(unsafe.Pointer(&id)), 1, 0)
		if n == 0 || n == 0xFFFFFFFF || hicon == 0 {
			continue
		}
		img = hiconToImage(hicon)
		procDestroyIcon.Call(hicon)
		if img != nil {
			return img
		}
	}
	return nil
}

func readBits(hdc, hbm uintptr, w, h int) []byte {
	bi := bitmapInfo{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	buf := make([]byte, w*h*4)
	r, _, _ := procGetDIBits.Call(hdc, hbm, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0)
	if r == 0 {
		return nil
	}
	return buf
}

func hiconToImage(hicon uintptr) image.Image {
	var ii iconInfo
	if r, _, _ := procGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil
	}
	defer func() {
		if ii.HbmColor != 0 {
			procDeleteObject.Call(ii.HbmColor)
		}
		if ii.HbmMask != 0 {
			procDeleteObject.Call(ii.HbmMask)
		}
	}()
	if ii.HbmColor == 0 {
		return nil
	}
	var bm bitmap
	if r, _, _ := procGetObjectW.Call(ii.HbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil
	}
	w, h := int(bm.Width), int(bm.Height)
	if w <= 0 || h <= 0 || w > 512 || h > 512 {
		return nil
	}
	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc)

	color := readBits(hdc, ii.HbmColor, w, h)
	if color == nil {
		return nil
	}
	hasAlpha := false
	for i := 3; i < len(color); i += 4 {
		if color[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha && ii.HbmMask != 0 {
		mask = readBits(hdc, ii.HbmMask, w, h)
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		b, g, r, a := color[i*4], color[i*4+1], color[i*4+2], color[i*4+3]
		if !hasAlpha {
			a = 255
			if mask != nil && mask[i*4] != 0 { // white mask pixel = transparent
				a = 0
			}
		}
		out.Pix[i*4], out.Pix[i*4+1], out.Pix[i*4+2], out.Pix[i*4+3] = r, g, b, a
	}
	return out
}
