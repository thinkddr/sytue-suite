// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package ficheros

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework UIKit -framework Foundation
#import <UIKit/UIKit.h>
#include <stdlib.h>

static int sytue_copiar_imagen_ios(const char *ruta) {
	@autoreleasepool {
		NSString *path = [NSString stringWithUTF8String:ruta];
		UIImage *img = [UIImage imageWithContentsOfFile:path];
		if (!img) return -1;
		dispatch_async(dispatch_get_main_queue(), ^{
			[UIPasteboard generalPasteboard].image = img;
		});
		return 0;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func copiarImagen(ruta string) error {
	cr := C.CString(ruta)
	defer C.free(unsafe.Pointer(cr))
	if C.sytue_copiar_imagen_ios(cr) != 0 {
		return errors.New("no se pudo copiar la imagen al portapapeles")
	}
	return nil
}
