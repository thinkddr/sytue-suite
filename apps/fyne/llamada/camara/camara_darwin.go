// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package camara

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AVFoundation -framework CoreMedia -framework CoreVideo -framework Foundation
#include <stdlib.h>
#include "camara_darwin.h"
*/
import "C"

import (
	"image"
	"strings"
	"sync"
	"unsafe"
)

// fuenteDarwin es la cámara de AVFoundation. El buffer es de Go y C copia en él.
type fuenteDarwin struct {
	mu   sync.Mutex
	buf  []byte
	sec  C.long
	cerr bool
}

func abrir(p Pedido) (Fuente, error) {
	id := C.CString(p.ID)
	defer C.free(unsafe.Pointer(id))
	switch C.sytueCamAbrir(id, C.int(p.Ancho), C.int(p.Alto), C.int(p.Fotogramas)) {
	case 0:
		return &fuenteDarwin{buf: make([]byte, 1920*1080*3/2)}, nil
	case -2:
		return nil, ErrSinPermiso
	}
	return nil, ErrSinCamara
}

func (f *fuenteDarwin) Leer() (*image.YCbCr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cerr {
		return nil, false
	}
	var w, h C.int
	if C.sytueCamLeer((*C.uint8_t)(unsafe.Pointer(&f.buf[0])), C.int(len(f.buf)), &w, &h, &f.sec) == 0 {
		if n := int(w) * int(h) * 3 / 2; n > len(f.buf) {
			f.buf = make([]byte, n) // una cámara 4K: la próxima vez cabe
		}
		return nil, false
	}
	return deI420(f.buf, int(w), int(h)), true
}

func (f *fuenteDarwin) Cerrar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cerr {
		f.cerr = true
		C.sytueCamCerrar()
	}
}

func dispositivos() []Dispositivo {
	buf := make([]byte, 8192)
	n := C.sytueCamLista((*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
	var out []Dispositivo
	for _, l := range strings.Split(string(buf[:n]), "\n") {
		if id, nombre, ok := strings.Cut(l, "\t"); ok {
			out = append(out, Dispositivo{ID: id, Nombre: nombre})
		}
	}
	return out
}
