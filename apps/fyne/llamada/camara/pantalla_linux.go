// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

// gocritic lee este fichero ya pasado por cgo y da avisos de otro código («dupImport»
// de `C` y `unsafe`): se silencia en el fichero, y el resto de linters sí lo miran.
//
//nolint:gocritic // cgo: los avisos no son de este fichero, ver arriba
package camara

/*
#cgo LDFLAGS: -lX11
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <stdint.h>
#include <string.h>

static Display *sytueX11;

// sytuePantallaX11 copia la pantalla de X11 (XGetImage de la raíz) a dst en RGBA.
// 0 bien, -1 sin X11 (Wayland sin XWayland), -3 no cabe.
static int sytuePantallaX11(uint8_t *dst, int cap, int *ancho, int *alto) {
	if (sytueX11 == NULL) sytueX11 = XOpenDisplay(NULL);
	if (sytueX11 == NULL) return -1;
	Window raiz = DefaultRootWindow(sytueX11);
	XWindowAttributes a;
	XGetWindowAttributes(sytueX11, raiz, &a);
	*ancho = a.width; *alto = a.height;
	if (a.width * a.height * 4 > cap) return -3;
	XImage *img = XGetImage(sytueX11, raiz, 0, 0, a.width, a.height, AllPlanes, ZPixmap);
	if (img == NULL) return -1;
	for (int y = 0; y < a.height; y++) {
		for (int x = 0; x < a.width; x++) {
			unsigned long p = XGetPixel(img, x, y);
			uint8_t *o = dst + (y * a.width + x) * 4;
			o[0] = (p & img->red_mask) >> 16;
			o[1] = (p & img->green_mask) >> 8;
			o[2] = p & img->blue_mask;
			o[3] = 255;
		}
	}
	XDestroyImage(img);
	return 0;
}
*/
import "C"

import (
	"image"
	"os"
	"sync"
	"unsafe"
)

// La pantalla de Linux por X11. En Wayland no hay XGetImage que valga (XWayland solo ve
// sus propias ventanas): ahí va por el portal de escritorio y PipeWire (portal_linux.go).
//
// ponytail: XGetPixel píxel a píxel (unos 60 ms a 1080p); a 5 fps sobra. Con XShm se
// quitaría la copia si algún día se nota.
type pantallaX11 struct {
	mu      sync.Mutex
	buf     []byte
	cerrada bool
}

func abrirPantalla() (Fuente, error) {
	if esWayland() {
		return abrirPantallaPortal()
	}
	if os.Getenv("DISPLAY") == "" {
		return nil, ErrSinPantalla
	}
	return &pantallaX11{buf: make([]byte, 1920*1080*4)}, nil
}

func (p *pantallaX11) Leer() (*image.YCbCr, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cerrada {
		return nil, false
	}
	var w, h C.int
	switch C.sytuePantallaX11((*C.uint8_t)(unsafe.Pointer(&p.buf[0])), C.int(len(p.buf)), &w, &h) {
	case 0:
	case -3:
		p.buf = make([]byte, int(w)*int(h)*4)
		return nil, false
	default:
		return nil, false
	}
	n := int(w) * int(h) * 4
	return DeRGBA(&image.RGBA{Pix: p.buf[:n], Stride: int(w) * 4, Rect: image.Rect(0, 0, int(w), int(h))}), true
}

func (p *pantallaX11) Cerrar() {
	p.mu.Lock()
	p.cerrada = true
	p.mu.Unlock()
}
