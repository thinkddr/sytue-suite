// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package escritorio

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RegisterHotKey sin ventana: el WM_HOTKEY llega a la cola del HILO que lo registró, así
// que el registro y el bucle de mensajes van en un hilo del sistema propio y fijo.

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procGetMessage       = user32.NewProc("GetMessageW")
	procPostThreadMsg    = user32.NewProc("PostThreadMessageW")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procCederFrente      = user32.NewProc("AllowSetForegroundWindow")
)

const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modWin      = 0x0008
	modNoRepeat = 0x4000
	vkSpace     = 0x20
	wmHotkey    = 0x0312
	wmQuit      = 0x0012
)

type mensaje struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

func atajo(_, _ string, c Combinacion, alPulsar func()) (func(), error) {
	mods := uintptr(modNoRepeat)
	if c.Ctrl {
		mods |= modControl
	}
	if c.Alt {
		mods |= modAlt
	}
	if c.Cmd {
		mods |= modWin
	}
	listo := make(chan error, 1)
	hilo := make(chan uint32, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		hilo <- windows.GetCurrentThreadId()
		if r, _, err := procRegisterHotKey.Call(0, 1, mods, vkSpace); r == 0 {
			listo <- err // lo más normal: otra aplicación ya tiene la combinación
			return
		}
		defer procUnregisterHotKey.Call(0, 1) //nolint:errcheck // al salir ya no hay a quién decírselo
		listo <- nil
		var m mensaje
		for {
			r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if r == 0 || r == ^uintptr(0) { // 0: WM_QUIT; −1: error
				return
			}
			if m.message == wmHotkey {
				go alPulsar()
			}
		}
	}()
	tid := <-hilo
	if err := <-listo; err != nil {
		return nil, err
	}
	return func() { _, _, _ = procPostThreadMsg.Call(uintptr(tid), wmQuit, 0, 0) }, nil
}

// cederPrimerPlano es AllowSetForegroundWindow(ASFW_ANY): este proceso, que tiene el
// primer plano porque lo ha lanzado un clic, deja que otro lo tome.
func cederPrimerPlano() { _, _, _ = procCederFrente.Call(^uintptr(0)) }
