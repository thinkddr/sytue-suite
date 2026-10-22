// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && cgo && sytuepruebas

package portapapeles

/*
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <stdlib.h>
#include <sys/select.h>

static Display *sytue_abrir_prueba(void) {
	Display *d = NULL;
	for (int i = 0; i < 5 && !d; i++) d = XOpenDisplay(NULL);
	return d;
}

// sytue_servir hace de OTRA aplicación que ha copiado `datos` como `tipo`: se queda con
// CLIPBOARD y contesta a quien lo pida hasta que *parar. Con `trozo` > 0 y más datos, los
// manda por INCR, trozo a trozo, como hacen los navegadores con un HTML grande. Pone
// *listo a 1 cuando ya es el dueño. Solo para los guardianes (etiqueta sytuepruebas).
static int sytue_servir(const char *tipo, const unsigned char *datos, long tam, long trozo,
                        volatile int *parar, volatile int *listo) {
	Display *d = sytue_abrir_prueba();
	if (!d) return -1;
	Atom sel = XInternAtom(d, "CLIPBOARD", False);
	Atom quiero = XInternAtom(d, tipo, False);
	Atom targets = XInternAtom(d, "TARGETS", False);
	Atom incr = XInternAtom(d, "INCR", False);
	Window w = XCreateSimpleWindow(d, DefaultRootWindow(d), 0, 0, 1, 1, 0, 0, 0);
	XSetSelectionOwner(d, sel, w, CurrentTime);
	XSync(d, False);
	if (XGetSelectionOwner(d, sel) != w) {
		XCloseDisplay(d);
		return -2;
	}
	*listo = 1;
	Window req = None;
	Atom reqProp = None;
	long enviado = 0;
	int enIncr = 0;
	int fd = ConnectionNumber(d);
	while (!*parar) {
		while (XPending(d)) {
			XEvent ev;
			XNextEvent(d, &ev);
			if (ev.type == SelectionRequest) {
				XSelectionRequestEvent *r = &ev.xselectionrequest;
				XSelectionEvent resp = {0};
				resp.type = SelectionNotify;
				resp.display = d;
				resp.requestor = r->requestor;
				resp.selection = r->selection;
				resp.target = r->target;
				resp.time = r->time;
				resp.property = r->property;
				if (r->target == targets) {
					Atom lista[2] = {targets, quiero};
					XChangeProperty(d, r->requestor, r->property, XA_ATOM, 32, PropModeReplace, (unsigned char *)lista, 2);
				} else if (r->target == quiero && trozo > 0 && tam > trozo) {
					long t = tam;
					XSelectInput(d, r->requestor, PropertyChangeMask);
					XChangeProperty(d, r->requestor, r->property, incr, 32, PropModeReplace, (unsigned char *)&t, 1);
					req = r->requestor;
					reqProp = r->property;
					enviado = 0;
					enIncr = 1;
				} else if (r->target == quiero) {
					XChangeProperty(d, r->requestor, r->property, quiero, 8, PropModeReplace, datos, (int)tam);
				} else {
					resp.property = None;
				}
				XSendEvent(d, r->requestor, False, 0, (XEvent *)&resp);
				XFlush(d);
			} else if (ev.type == PropertyNotify && enIncr && ev.xproperty.window == req &&
			           ev.xproperty.atom == reqProp && ev.xproperty.state == PropertyDelete) {
				long n = tam - enviado;
				if (n > trozo) n = trozo;
				XChangeProperty(d, req, reqProp, quiero, 8, PropModeReplace, datos + enviado, (int)n);
				enviado += n;
				if (n == 0) enIncr = 0;
				XFlush(d);
			}
		}
		struct timeval t = {0, 20000};
		fd_set fds;
		FD_ZERO(&fds);
		FD_SET(fd, &fds);
		select(fd + 1, &fds, NULL, NULL, &t);
	}
	XDestroyWindow(d, w);
	XCloseDisplay(d);
	return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

// servirParaPrueba hace de otra aplicación que ha copiado `datos` como `tipo`, hasta que
// se llama a lo que devuelve.
func servirParaPrueba(tipo string, datos []byte, trozo int) (func(), error) {
	ct := C.CString(tipo)
	cd := C.CBytes(datos)
	senales := (*[2]C.int)(C.malloc(C.size_t(2 * C.sizeof_int)))
	senales[0], senales[1] = 0, 0
	liberar := func() {
		C.free(unsafe.Pointer(ct))
		C.free(cd)
		C.free(unsafe.Pointer(senales))
	}
	hecho := make(chan C.int, 1)
	go func() {
		hecho <- C.sytue_servir(ct, (*C.uchar)(cd), C.long(len(datos)), C.long(trozo), &senales[0], &senales[1])
	}()
	for range 400 {
		if senales[1] == 1 {
			return func() { senales[0] = 1; <-hecho; liberar() }, nil
		}
		select {
		case r := <-hecho:
			liberar()
			return nil, fmt.Errorf("no se ha podido ser el dueño del portapapeles (%d)", int(r))
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	senales[0] = 1
	<-hecho
	liberar()
	return nil, errors.New("el dueño de prueba no arranca")
}
