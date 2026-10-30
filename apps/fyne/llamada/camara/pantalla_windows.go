// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build windows

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package camara

/*
#cgo LDFLAGS: -lgdi32 -luser32
#include <windows.h>
#include <stdint.h>

// sytuePantallaWin copia la pantalla principal (GDI, BitBlt del escritorio) a dst en
// BGRA de arriba abajo. 0 bien, -3 no cabe (ancho y alto dicen cuánto hace falta).
static int sytuePantallaWin(uint8_t *dst, int cap, int *ancho, int *alto) {
	int w = GetSystemMetrics(SM_CXSCREEN), h = GetSystemMetrics(SM_CYSCREEN);
	*ancho = w; *alto = h;
	if (w <= 0 || h <= 0) return -1;
	if (w * h * 4 > cap) return -3;
	HDC pantalla = GetDC(NULL), mem = CreateCompatibleDC(pantalla);
	HBITMAP bmp = CreateCompatibleBitmap(pantalla, w, h);
	HGDIOBJ viejo = SelectObject(mem, bmp);
	BitBlt(mem, 0, 0, w, h, pantalla, 0, 0, SRCCOPY | CAPTUREBLT);
	BITMAPINFO bi = {0};
	bi.bmiHeader.biSize = sizeof(bi.bmiHeader);
	bi.bmiHeader.biWidth = w;
	bi.bmiHeader.biHeight = -h; // de arriba abajo
	bi.bmiHeader.biPlanes = 1;
	bi.bmiHeader.biBitCount = 32;
	bi.bmiHeader.biCompression = BI_RGB;
	int r = GetDIBits(mem, bmp, 0, h, dst, &bi, DIB_RGB_COLORS) == h ? 0 : -1;
	SelectObject(mem, viejo);
	DeleteObject(bmp);
	DeleteDC(mem);
	ReleaseDC(NULL, pantalla);
	return r;
}
*/
import "C"

import (
	"image"
	"sync"
	"unsafe"
)

// La pantalla de Windows por GDI. ponytail: BitBlt a 5 fps; si una pantalla 4K se nota
// lenta, Desktop Duplication (DXGI), que es la de los grabadores.
type pantallaWindows struct {
	mu      sync.Mutex
	buf     []byte
	cerrada bool
}

// PantallaDisponible indica que este destino implementa la captura; los permisos
// y el acceso al equipo se comprueban al abrirla.
const PantallaDisponible = true

func abrirPantalla() (Fuente, error) { return &pantallaWindows{buf: make([]byte, 1920*1080*4)}, nil }

func (p *pantallaWindows) Leer() (*image.YCbCr, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cerrada {
		return nil, false
	}
	var w, h C.int
	switch C.sytuePantallaWin((*C.uint8_t)(unsafe.Pointer(&p.buf[0])), C.int(len(p.buf)), &w, &h) {
	case 0:
	case -3:
		p.buf = make([]byte, int(w)*int(h)*4)
		return nil, false
	default:
		return nil, false
	}
	n := int(w) * int(h) * 4
	for i := 0; i < n; i += 4 { // BGRA → RGBA
		p.buf[i], p.buf[i+2] = p.buf[i+2], p.buf[i]
	}
	return DeRGBA(&image.RGBA{Pix: p.buf[:n], Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}), true
}

func (p *pantallaWindows) Cerrar() {
	p.mu.Lock()
	p.cerrada = true
	p.mu.Unlock()
}
