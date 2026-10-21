// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

package portapapeles

import (
	"bytes"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// El portapapeles de Windows con su API de siempre: el formato registrado «HTML Format»
// (CF_HTML) se pide por su nombre, y el bloque de memoria que devuelve se copia con el
// portapapeles abierto. OpenClipboard y CloseClipboard son del HILO que los llama, así que
// todo va en un hilo del sistema fijo.

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClipboardFormat    = user32.NewProc("RegisterClipboardFormatW")
	procIsClipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procGetClipboardData           = user32.NewProc("GetClipboardData")
	procGlobalLock                 = kernel32.NewProc("GlobalLock")
	procGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	procGlobalSize                 = kernel32.NewProc("GlobalSize")
)

func leer() ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	nombre, err := windows.UTF16PtrFromString("HTML Format")
	if err != nil {
		return nil, ErrNoDisponible
	}
	formato, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(nombre)))
	if formato == 0 {
		return nil, ErrNoDisponible
	}
	if hay, _, _ := procIsClipboardFormatAvailable.Call(formato); hay == 0 {
		return nil, ErrSinHTML
	}
	// Otro proceso puede tenerlo abierto un instante (el que acaba de copiar): se
	// reintenta un poco antes de rendirse, como hacen las demás aplicaciones.
	abierto := false
	for range 10 {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			abierto = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !abierto {
		return nil, ErrSinHTML
	}
	defer procCloseClipboard.Call() //nolint:errcheck // cerrar no tiene nada que decir
	h, _, _ := procGetClipboardData.Call(formato)
	if h == 0 {
		return nil, ErrSinHTML
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return nil, ErrSinHTML
	}
	defer procGlobalUnlock.Call(h) //nolint:errcheck // desbloquear no tiene nada que decir
	n, _, _ := procGlobalSize.Call(h)
	if n == 0 || n > MaxHTML {
		return nil, ErrSinHTML
	}
	datos := bytes.Clone(unsafe.Slice((*byte)(punteroDe(p)), n))
	return []byte(DeCFHTML(datos)), nil
}

// punteroDe es un puntero a memoria del SISTEMA (la de GlobalLock, que el recolector no
// mueve ni libera). La conversión directa `unsafe.Pointer(p)` la marca `go vet`, que no
// sabe de dónde sale.
func punteroDe(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }
