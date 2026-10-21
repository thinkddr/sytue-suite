// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios && cgo && sytuepruebas

package portapapeles

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

// sytue_poner_mac deja `datos` como `tipo` en el portapapeles `nombre` (uno propio de la
// prueba: el general es de quien está delante). Con `archivo`, como archivo web de Safari.
static void sytue_poner_mac(const char *nombre, const char *tipo, const void *datos, long tam, int archivo) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:nombre]];
		[pb clearContents];
		NSData *d = [NSData dataWithBytes:datos length:tam];
		if (archivo) {
			NSDictionary *plist = @{@"WebMainResource" : @{
				@"WebResourceData" : d,
				@"WebResourceMIMEType" : @"text/html",
				@"WebResourceTextEncodingName" : @"UTF-8",
				@"WebResourceURL" : @"about:blank",
				@"WebResourceFrameName" : @"",
			}};
			d = [NSPropertyListSerialization dataWithPropertyList:plist format:NSPropertyListBinaryFormat_v1_0 options:0 error:NULL];
		}
		[pb setData:d forType:[NSString stringWithUTF8String:tipo]];
	}
}

static void sytue_soltar_mac(const char *nombre) {
	@autoreleasepool {
		[[NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:nombre]] releaseGlobally];
	}
}
*/
import "C"

import "unsafe"

func ponerParaPrueba(nombre, tipo string, datos []byte, archivo bool) {
	cn, ct := C.CString(nombre), C.CString(tipo)
	defer C.free(unsafe.Pointer(cn))
	defer C.free(unsafe.Pointer(ct))
	cd := C.CBytes(datos)
	defer C.free(cd)
	a := C.int(0)
	if archivo {
		a = 1
	}
	C.sytue_poner_mac(cn, ct, cd, C.long(len(datos)), a)
}

func soltarParaPrueba(nombre string) {
	cn := C.CString(nombre)
	defer C.free(unsafe.Pointer(cn))
	C.sytue_soltar_mac(cn)
}
