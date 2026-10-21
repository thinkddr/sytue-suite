// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && tienda

package ficheros

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework UniformTypeIdentifiers
#include <stdlib.h>
#include <string.h>
#import <AppKit/AppKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>

// Las rutas elegidas, una por línea, o NULL si se canceló. `extensiones` va separada por
// comas (vacía, cualquier fichero). Quien llama libera el resultado.
//
// EN EL HILO PRINCIPAL, que es donde AppKit deja enseñar un panel: el bucle de Fyne
// atiende la cola principal (es por donde ya van `escritorio.Aparecer` y el atajo
// global), y `runModal` se queda ahí hasta que la persona cierra el panel.
static char *sytueElegir(const char *titulo, const char *extensiones) {
	__block char *salida = NULL;
	void (^panel)(void) = ^{
		@autoreleasepool {
			NSOpenPanel *p = [NSOpenPanel openPanel];
			p.canChooseFiles = YES;
			p.canChooseDirectories = NO;
			p.allowsMultipleSelection = YES;
			if (titulo[0] != 0) {
				p.message = [NSString stringWithUTF8String:titulo];
			}
			NSMutableArray<UTType *> *tipos = [NSMutableArray array];
			for (NSString *e in [[NSString stringWithUTF8String:extensiones] componentsSeparatedByString:@","]) {
				UTType *t = e.length > 0 ? [UTType typeWithFilenameExtension:e] : nil;
				if (t != nil) {
					[tipos addObject:t];
				}
			}
			if (tipos.count > 0) {
				p.allowedContentTypes = tipos;
			}
			if ([p runModal] == NSModalResponseOK) {
				NSMutableString *rutas = [NSMutableString string];
				for (NSURL *u in p.URLs) {
					[rutas appendFormat:@"%@\n", u.path];
				}
				salida = strdup(rutas.UTF8String);
			}
		}
	};
	if ([NSThread isMainThread]) {
		panel();
	} else {
		dispatch_sync(dispatch_get_main_queue(), panel);
	}
	return salida;
}
*/
import "C"

import (
	"strings"
	"unsafe"
)

// En la variante de la Mac App Store el panel de abrir es NSOpenPanel dentro de este
// proceso. No es por gusto: con el App Sandbox, el sistema da permiso sobre lo elegido al
// proceso que enseña el panel, y el `choose file` de AppleScript (elegir_darwin.go) lo
// enseña un `osascript` hijo, que se quedaría con el permiso mientras la app recibe una
// ruta que no puede leer. El permiso dura lo que el proceso: lo que se elige se lee en el
// acto (subir, adjuntar, importar), que es lo que hacen todos los que llaman.
//
// COMPILADO EN macOS EL 2026-10-06 Y NO EJECUTADO: ningún Mac ha corrido la variante.
// Lo primero que hay que ver en uno es que el panel sale y que lo elegido se lee.
func init() {
	panelPropio = func(titulo string, extensiones []string) ([]string, error) {
		t, e := C.CString(titulo), C.CString(strings.Join(extensiones, ","))
		defer C.free(unsafe.Pointer(t))
		defer C.free(unsafe.Pointer(e))
		salida := C.sytueElegir(t, e)
		if salida == nil {
			return nil, nil // cancelar no es un error
		}
		defer C.free(unsafe.Pointer(salida))
		return rutasDe(C.GoString(salida)), nil
	}
}
