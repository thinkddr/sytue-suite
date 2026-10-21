// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package camara

/*
#cgo LDFLAGS: -lmfplat -lmf -lmfreadwrite -lmfuuid -lole32 -luuid
#include <stdlib.h>
#include "camara_windows.h"
*/
import "C"

import (
	"image"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fuenteWindows es la cámara de Media Foundation (camara_windows.c).
type fuenteWindows struct {
	mu   sync.Mutex
	buf  []byte
	sec  C.long
	cerr bool
}

func abrir(p Pedido) (Fuente, error) {
	id, err := windows.UTF16PtrFromString(p.ID)
	if err != nil {
		return nil, ErrSinCamara
	}
	switch C.sytueCamAbrir((*C.wchar_t)(unsafe.Pointer(id)), C.int(p.Ancho), C.int(p.Alto), C.int(p.Fotogramas)) {
	case 0:
		return &fuenteWindows{buf: make([]byte, 1920*1080*3/2)}, nil
	case -2:
		return nil, ErrSinPermiso
	}
	return nil, ErrSinCamara
}

func (f *fuenteWindows) Leer() (*image.YCbCr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cerr {
		return nil, false
	}
	var w, h C.int
	if C.sytueCamLeer((*C.uint8_t)(unsafe.Pointer(&f.buf[0])), C.int(len(f.buf)), &w, &h, &f.sec) == 0 {
		if n := int(w) * int(h) * 3 / 2; n > len(f.buf) {
			f.buf = make([]byte, n)
		}
		return nil, false
	}
	return deI420(f.buf, int(w), int(h)), true
}

func (f *fuenteWindows) Cerrar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cerr {
		f.cerr = true
		C.sytueCamCerrar()
	}
}

func dispositivos() []Dispositivo {
	buf := make([]uint16, 16384)
	n := C.sytueCamLista((*C.wchar_t)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	var out []Dispositivo
	for _, l := range strings.Split(windows.UTF16ToString(buf[:n]), "\n") {
		if id, nombre, ok := strings.Cut(l, "\t"); ok {
			out = append(out, Dispositivo{ID: id, Nombre: nombre})
		}
	}
	return out
}
