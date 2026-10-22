// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android && cgo

package escritorio

/*
#cgo LDFLAGS: -lX11
#include <stdlib.h>
#include <string.h>
#include <X11/Xutil.h>
#include <X11/Xlib.h>

// Una conexión propia y de un solo uso: la de GLFW no sale de su paquete, y las
// propiedades de una ventana las puede cambiar cualquier cliente. XCloseDisplay vacía
// la cola, así que la propiedad llega antes de volver.
static void sytueVarianteGTK(unsigned long w, int oscuro) {
	Display *d = XOpenDisplay(NULL);
	if (!d) return;
	const char *v = oscuro ? "dark" : "light";
	XChangeProperty(d, (Window)w, XInternAtom(d, "_GTK_THEME_VARIANT", False),
		XInternAtom(d, "UTF8_STRING", False), 8, PropModeReplace,
		(const unsigned char *)v, (int)strlen(v));
	XCloseDisplay(d);
}

// Dice de qué app es la ventana, que es como la une a su .desktop el escritorio: la
// clase ICCCM (GLFW pone el TÍTULO) y las dos propiedades que leen KWin y Mutter. Sin
// esto, en Plasma la ventana no se juntaba con su lanzador y el número no salía.
static void sytueIdentificar(unsigned long w, char *id) {
	Display *d = XOpenDisplay(NULL);
	if (!d) { free(id); return; }
	XClassHint h = { (char *)id, (char *)id };
	XSetClassHint(d, (Window)w, &h);
	Atom utf8 = XInternAtom(d, "UTF8_STRING", False);
	XChangeProperty(d, (Window)w, XInternAtom(d, "_KDE_NET_WM_DESKTOP_FILE", False), utf8, 8,
		PropModeReplace, (const unsigned char *)id, (int)strlen(id));
	XChangeProperty(d, (Window)w, XInternAtom(d, "_GTK_APPLICATION_ID", False), utf8, 8,
		PropModeReplace, (const unsigned char *)id, (int)strlen(id));
	XCloseDisplay(d);
	free(id);
}
*/
import "C"

import "image/color"

func colorearBarraTitulo(w uintptr, _ color.NRGBA, oscuro bool) {
	o := 0
	if oscuro {
		o = 1
	}
	C.sytueVarianteGTK(C.ulong(w), C.int(o))
}

func identificarVentana(w uintptr, id string) {
	C.sytueIdentificar(C.ulong(w), C.CString(id)) // la libera la función de C
}
