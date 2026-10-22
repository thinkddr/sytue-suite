// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && cgo

package portapapeles

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

// sytue_html_mac lee el HTML del portapapeles `nombre` (NULL, el general): `public.html`,
// y si solo trae el archivo web de Safari, el HTML de su recurso principal. Devuelve los
// bytes (malloc) y deja su tamaño en *tam; NULL si no hay.
static unsigned char *sytue_html_mac(const char *nombre, long *tam) {
	*tam = 0;
	@autoreleasepool {
		NSPasteboard *pb = nombre ? [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:nombre]]
		                          : [NSPasteboard generalPasteboard];
		NSData *datos = [pb dataForType:NSPasteboardTypeHTML];
		if (datos == nil) {
			NSData *archivo = [pb dataForType:@"com.apple.webarchive"];
			if (archivo != nil) {
				id plist = [NSPropertyListSerialization propertyListWithData:archivo options:0 format:NULL error:NULL];
				if ([plist isKindOfClass:[NSDictionary class]]) {
					id principal = ((NSDictionary *)plist)[@"WebMainResource"];
					if ([principal isKindOfClass:[NSDictionary class]]) {
						id d = ((NSDictionary *)principal)[@"WebResourceData"];
						if ([d isKindOfClass:[NSData class]]) datos = d;
					}
				}
			}
		}
		if (datos == nil || datos.length == 0) return NULL;
		unsigned char *out = malloc(datos.length);
		memcpy(out, datos.bytes, datos.length);
		*tam = (long)datos.length;
		return out;
	}
}
*/
import "C" //nolint:gocritic // CGO exige importar C por separado

import "unsafe" //nolint:gocritic // dupImport falso positivo con import C

// NSPasteboard se lee desde cualquier hilo: el servidor del portapapeles es otro proceso
// y lo pedido llega por mensaje, así que no hace falta el hilo principal.
func leer() ([]byte, error) { return leerMac("") }

// leerMac lee el portapapeles `nombre` (vacío, el general). Los guardianes usan uno con
// nombre propio, para no tocar el de quien está delante.
func leerMac(nombre string) ([]byte, error) {
	var cn *C.char
	if nombre != "" {
		cn = C.CString(nombre)
		defer C.free(unsafe.Pointer(cn))
	}
	var tam C.long
	p := C.sytue_html_mac(cn, &tam)
	if p == nil {
		return nil, ErrSinHTML
	}
	defer C.free(unsafe.Pointer(p))
	if int64(tam) > MaxHTML*2 {
		return nil, ErrSinHTML
	}
	return C.GoBytes(unsafe.Pointer(p), C.int(tam)), nil
}
