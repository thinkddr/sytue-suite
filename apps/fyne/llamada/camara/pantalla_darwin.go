// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package camara

/*
#cgo LDFLAGS: -framework ScreenCaptureKit -framework CoreGraphics
#include "camara_darwin.h"
*/
import "C"

import (
	"image"
	"sync"
	"unsafe"
)

type pantallaDarwin struct {
	mu      sync.Mutex
	buf     []byte
	cerrada bool
}

func abrirPantalla() (Fuente, error) {
	if C.sytuePantallaPermiso() == 0 {
		return nil, ErrSinPermiso
	}
	return &pantallaDarwin{buf: make([]byte, 2560*1600*4)}, nil
}

func (p *pantallaDarwin) Leer() (*image.YCbCr, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cerrada {
		return nil, false
	}
	var w, h C.int
	r := C.sytuePantallaCapturar((*C.uint8_t)(unsafe.Pointer(&p.buf[0])), C.int(len(p.buf)), &w, &h)
	if r == -3 {
		p.buf = make([]byte, int(w)*int(h)*4) // una pantalla 5K: la próxima vez cabe
		return nil, false
	}
	if r != 0 {
		return nil, false
	}
	rgba := &image.RGBA{Pix: p.buf[:int(w)*int(h)*4], Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}
	return DeRGBA(rgba), true
}

func (p *pantallaDarwin) Cerrar() {
	p.mu.Lock()
	p.cerrada = true
	p.mu.Unlock()
}
