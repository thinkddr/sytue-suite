// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdint.h>
#import <AppKit/AppKit.h>

// El color va en el espacio de color de la VENTANA y no en sRGB: GLFW no etiqueta lo que
// pinta, así que el cromo de Fyne sale en ese espacio tal cual, y un sRGB convertido a
// Display P3 quedaba unos puntos más saturado que la barra de debajo.
//
// Sin NSWindowStyleMaskFullSizeContentView: el contenido empieza DEBAJO de la barra del
// sistema y los semáforos no pisan nada de la app, ni el logotipo ni los clics del
// buscador (una barra transparente sobre el contenido se sigue quedando los clics de
// sus 28 px de alto).
//
// Y sin el título (dueño, 2026-09-25): la franja es solo cromo, de la misma pieza que la
// barra de la suite que lleva debajo el nombre de la app. Mission Control, el menú
// Ventana y los lectores de pantalla lo siguen leyendo: se oculta, no se borra.
static void sytueBarraTitulo(uintptr_t w, double r, double g, double b, int oscuro) {
	NSWindow *win = (NSWindow *)w;
	void (^poner)(void) = ^{
		CGFloat comp[4] = {r, g, b, 1};
		NSColorSpace *cs = [win colorSpace] ?: [NSColorSpace sRGBColorSpace];
		win.titlebarAppearsTransparent = YES;
		win.titleVisibility = NSWindowTitleHidden;
		win.backgroundColor = [NSColor colorWithColorSpace:cs components:comp count:4];
		win.appearance = [NSAppearance appearanceNamed:(oscuro ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua)];
		if (@available(macOS 11.0, *)) {
			win.titlebarSeparatorStyle = NSTitlebarSeparatorStyleNone;
		}
	};
	if ([NSThread isMainThread]) {
		poner();
	} else {
		dispatch_async(dispatch_get_main_queue(), poner);
	}
}

// Para el guardián, porque cgo no entra en un _test.go: una ventana sin enseñar y si
// lleva el título oculto. Se llaman FUERA del hilo principal, con este vaciando su cola
// (`bucleprincipal`); la cola es en serie, así que la lectura va detrás de `poner`.
static uintptr_t sytueVentanaPrueba(void) {
	__block NSWindow *w;
	dispatch_sync(dispatch_get_main_queue(), ^{
		w = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 100, 100)
		                                styleMask:NSWindowStyleMaskTitled
		                                  backing:NSBackingStoreBuffered
		                                    defer:YES];
	});
	return (uintptr_t)w;
}

static int sytueTituloOculto(uintptr_t w) {
	__block int r;
	dispatch_sync(dispatch_get_main_queue(), ^{
		r = ((NSWindow *)w).titleVisibility == NSWindowTitleHidden;
	});
	return r;
}

*/
import "C"

import "image/color"

func colorearBarraTitulo(w uintptr, c color.NRGBA, oscuro bool) {
	o := 0
	if oscuro {
		o = 1
	}
	C.sytueBarraTitulo(C.uintptr_t(w), C.double(c.R)/255, C.double(c.G)/255, C.double(c.B)/255, C.int(o))
}

// ventanaPrueba, tituloOculto y bucleprincipal solo las usa el guardián
// (barratitulo_darwin_test.go).
func ventanaPrueba() uintptr      { return uintptr(C.sytueVentanaPrueba()) }
func tituloOculto(w uintptr) bool { return C.sytueTituloOculto(C.uintptr_t(w)) != 0 }
func bucleprincipal()             { C.CFRunLoopRun() }

func identificarVentana(uintptr, string) {}
