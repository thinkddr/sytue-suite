// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit
#include <stdlib.h>
#import <AppKit/AppKit.h>

// sytueLibreMac suelta una cadena de C.CString. Está aquí para que el Go no importe
// `unsafe`: gocritic toma `import "C"` y `unsafe` por el mismo paquete dos veces.
static void sytueLibreMac(char *p) { free(p); }

// El Dock solo se toca desde el hilo principal. La cadena se copia antes de volver.
static void sytueInsignia(const char *texto) {
	@autoreleasepool {
		NSString *s = texto[0] ? [[NSString alloc] initWithUTF8String:texto] : nil;
		dispatch_async(dispatch_get_main_queue(), ^{
			[[NSApp dockTile] setBadgeLabel:s];
			[s release];
		});
	}
}
*/
import "C"

import "strconv"

func insignia(_ string, n int) error {
	texto := ""
	if n > 0 {
		texto = strconv.Itoa(n)
	}
	c := C.CString(texto)
	defer C.sytueLibreMac(c)
	C.sytueInsignia(c)
	return nil
}

func hayBandeja() bool { return true }
