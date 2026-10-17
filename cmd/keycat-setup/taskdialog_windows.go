//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	comctl32               = windows.NewLazySystemDLL("comctl32.dll")
	procTaskDialogIndirect = comctl32.NewProc("TaskDialogIndirect")
	user32                 = windows.NewLazySystemDLL("user32.dll")
	procSendMessageW       = user32.NewProc("SendMessageW")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procMessageBoxW        = user32.NewProc("MessageBoxW")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
)

const (
	tdfAllowDialogCancellation = 0x0008
	tdfVerificationFlagChecked = 0x0100
	tdfShowProgressBar         = 0x0200
	tdfShowMarqueeProgressBar  = 0x0400
	tdfCallbackTimer           = 0x0800

	tdcbfCancel = 0x08
	tdcbfRetry  = 0x10
	tdcbfClose  = 0x20

	idCancel = 2
	idRetry  = 4
	idClose  = 8

	tdnCreated             = 0
	tdnNavigated           = 1
	tdnButtonClicked       = 2
	tdnTimer               = 4
	tdnDialogConstructed   = 7
	tdnVerificationClicked = 8

	wmUser                   = 0x0400
	tdmNavigatePage          = wmUser + 101
	tdmClickButton           = wmUser + 102
	tdmSetProgressBarRange   = wmUser + 105
	tdmSetProgressBarPos     = wmUser + 106
	tdmSetProgressBarMarquee = wmUser + 107
	tdmEnableButton          = wmUser + 111
	tdmUpdateElementText     = wmUser + 114

	tdeContent = 0

	iconApp     = 1
	iconWarning = 0xFFFF
	iconError   = 0xFFFE
	iconInfo    = 0xFFFD

	configSize = 160
	buttonSize = 12
)

type Button struct {
	ID   int32
	Text string
}

type Page struct {
	Instruction  string
	Content      string
	Footer       string
	Verification string
	Checked      bool
	Icon         uintptr
	FooterIcon   uintptr
	Buttons      []Button
	Common       uint32
	Default      int32
	Progress     bool
	Marquee      bool
	Cancellable  bool
	OnCreated    func(d *Dialog)
	OnButton     func(d *Dialog, id int32) bool
	OnTimer      func(d *Dialog)
}

type Dialog struct {
	Title     string
	Offscreen bool
	Shot      func(d *Dialog)
	hwnd      uintptr
	page      *Page
	keep      []any
}

var (
	active   *Dialog
	callback = windows.NewCallback(taskDialogCallback)
)

func taskDialogCallback(hwnd, msg, wParam, lParam, refData uintptr) uintptr {
	d := active
	if d == nil {
		return 0
	}
	d.hwnd = hwnd
	p := d.page
	switch msg {
	case tdnDialogConstructed:
		if d.Offscreen {
			x, y := int64(-32000), int64(-32000)
			procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0001|0x0004|0x0010)
		}
	case tdnCreated, tdnNavigated:
		if p.Progress {
			d.send(tdmSetProgressBarRange, 0, 1000<<16)
		}
		if p.Marquee {
			d.send(tdmSetProgressBarMarquee, 1, 30)
		}
		if p.OnCreated != nil {
			p.OnCreated(d)
		}
		if d.Shot != nil {
			shot := d.Shot
			d.Shot = nil
			shot(d)
		}
	case tdnButtonClicked:
		if p.OnButton != nil && p.OnButton(d, int32(wParam)) {
			return 1
		}
	case tdnTimer:
		if p.OnTimer != nil {
			p.OnTimer(d)
		}
	case tdnVerificationClicked:
		p.Checked = wParam != 0
	}
	return 0
}

func (d *Dialog) send(msg, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(d.hwnd, msg, wParam, lParam)
	return r
}

func (d *Dialog) str(s string) uintptr {
	if s == "" {
		return 0
	}
	u, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	d.keep = append(d.keep, u)
	return uintptr(unsafe.Pointer(u))
}

func (d *Dialog) build(p *Page) *[configSize]byte {
	cfg := new([configSize]byte)
	d.keep = append(d.keep, cfg)
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(cfg[off:], v) }
	put64 := func(off int, v uintptr) { binary.LittleEndian.PutUint64(cfg[off:], uint64(v)) }
	var flags uint32
	if p.Cancellable {
		flags |= tdfAllowDialogCancellation
	}
	if p.Checked {
		flags |= tdfVerificationFlagChecked
	}
	if p.Progress {
		flags |= tdfShowProgressBar
	}
	if p.Marquee {
		flags |= tdfShowMarqueeProgressBar
	}
	if p.OnTimer != nil {
		flags |= tdfCallbackTimer
	}
	hinst, _, _ := procGetModuleHandleW.Call(0)
	put32(0, configSize)
	put64(12, hinst)
	put32(20, flags)
	put32(24, p.Common)
	put64(28, d.str(d.Title))
	put64(36, p.Icon)
	put64(44, d.str(p.Instruction))
	put64(52, d.str(p.Content))
	if len(p.Buttons) > 0 {
		buttons := make([]byte, buttonSize*len(p.Buttons))
		for i, b := range p.Buttons {
			binary.LittleEndian.PutUint32(buttons[i*buttonSize:], uint32(b.ID))
			binary.LittleEndian.PutUint64(buttons[i*buttonSize+4:], uint64(d.str(b.Text)))
		}
		d.keep = append(d.keep, buttons)
		put32(60, uint32(len(p.Buttons)))
		put64(64, uintptr(unsafe.Pointer(&buttons[0])))
	}
	put32(72, uint32(p.Default))
	put64(92, d.str(p.Verification))
	put64(124, p.FooterIcon)
	put64(132, d.str(p.Footer))
	put64(140, callback)
	return cfg
}

func (d *Dialog) Run(first *Page) (int32, bool, error) {
	if err := procTaskDialogIndirect.Find(); err != nil {
		return 0, false, errors.New("this version of Windows lacks task dialogs")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	active, d.page = d, first
	defer func() { active = nil }()
	cfg := d.build(first)
	var button, radio, checked int32
	hr, _, _ := procTaskDialogIndirect.Call(
		uintptr(unsafe.Pointer(&cfg[0])),
		uintptr(unsafe.Pointer(&button)),
		uintptr(unsafe.Pointer(&radio)),
		uintptr(unsafe.Pointer(&checked)))
	runtime.KeepAlive(d.keep)
	if hr != 0 {
		return 0, false, fmt.Errorf("task dialog failed (0x%08x)", uint32(hr))
	}
	return button, checked != 0, nil
}

func (d *Dialog) Navigate(p *Page) {
	d.page = p
	cfg := d.build(p)
	procSendMessageW.Call(d.hwnd, tdmNavigatePage, 0, uintptr(unsafe.Pointer(&cfg[0])))
}

func (d *Dialog) SetProgress(frac float64) {
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	d.send(tdmSetProgressBarPos, uintptr(frac*1000), 0)
}

func (d *Dialog) SetContent(s string) {
	u, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	procSendMessageW.Call(d.hwnd, tdmUpdateElementText, tdeContent, uintptr(unsafe.Pointer(u)))
	runtime.KeepAlive(u)
}

func (d *Dialog) EnableButton(id int32, on bool) {
	var v uintptr
	if on {
		v = 1
	}
	d.send(tdmEnableButton, uintptr(id), v)
}

func (d *Dialog) Click(id int32) { d.send(tdmClickButton, uintptr(id), 0) }

func (d *Dialog) Close() {
	if d.page.Common&tdcbfClose != 0 {
		d.Click(idClose)
	} else {
		d.Click(idCancel)
	}
}

func messageBox(title, text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(icon))
}
