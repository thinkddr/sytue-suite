// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && cgo

package sistema

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#include <stdlib.h>
#include <string.h>
#import <Foundation/Foundation.h>

// sytueIdioma es el primero de `preferredLanguages`: el idioma que la persona ha puesto
// en Ajustes (o el que ha elegido solo para esta app, que el sistema pone delante), ya en
// BCP 47 —«es-ES», «pt-BR», «zh-Hans-CN»—. Devuelve una copia que se suelta con free.
static char *sytueIdioma(void) {
	@autoreleasepool {
		NSString *l = [[NSLocale preferredLanguages] firstObject];
		return l ? strdup([l UTF8String]) : NULL;
	}
}

// sytueLibre está aquí para que el Go no importe `unsafe`: gocritic toma `import "C"` y
// `unsafe` por el mismo paquete dos veces.
static void sytueLibre(char *p) { free(p); }
*/
import "C"

// delSistema pregunta a macOS o a iOS en qué idioma habla su dueño.
//
// Existe por el mismo fallo que en Android: una app del Mac abierta desde el Finder o
// el Dock arranca SIN `LANG` —solo lo pone una terminal, y a veces como `C.UTF-8`—, así
// que con el entorno como única fuente las trece apps salían en el idioma por defecto en
// un Mac en español. Compilado también para iOS con el SDK de Xcode 27.
func delSistema() string {
	c := C.sytueIdioma()
	if c == nil {
		return ""
	}
	defer C.sytueLibre(c)
	return C.GoString(c)
}

// El tema del sistema lo pinta Fyne con lo que sabe del escritorio (`tema.De`).
func temaOscuro() (oscuro, sabido bool) { return false, false }
