// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package escritorio

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework CoreServices -framework UniformTypeIdentifiers
#include <stdlib.h>
#import <AppKit/AppKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#import <CoreServices/CoreServices.h>

static void sytueLibrePredeterminada(char *p) { free(p); }

// 0 hecho; 1 no corre desde un .app; 2 el sistema no conoce la extensión; otro, el
// OSStatus de Launch Services.
//
// LSSetDefaultRoleHandlerForContentType y no `-[NSWorkspace
// setDefaultApplicationAtURL:toOpenContentType:]`, que es su sustituta: medido en macOS
// 27, la nueva no llega a contestar para com.adobe.pdf (con Foxit de predeterminada) y la
// vieja lo hace en el acto y sin preguntar. Está obsoleta desde macOS 12 pero sigue ahí.
static int sytuePredeterminada(const char *ext) {
	@autoreleasepool {
		NSBundle *app = [NSBundle mainBundle];
		if (![[[app bundleURL] pathExtension] isEqualToString:@"app"] || [app bundleIdentifier] == nil) {
			return 1;
		}
		UTType *tipo = [UTType typeWithFilenameExtension:[NSString stringWithUTF8String:ext]];
		if (tipo == nil || [tipo isDynamic]) {
			return 2;
		}
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		OSStatus r = LSSetDefaultRoleHandlerForContentType((__bridge CFStringRef)[tipo identifier], kLSRolesAll,
			(__bridge CFStringRef)[app bundleIdentifier]);
#pragma clang diagnostic pop
		return (int)r;
	}
}
*/
import "C"

import (
	"errors"
	"fmt"
)

func predeterminada(_ string, extensiones []string) error {
	var errs []error
	for _, ext := range extensiones {
		c := C.CString(ext)
		r := C.sytuePredeterminada(c)
		C.sytueLibrePredeterminada(c)
		switch r {
		case 0:
		case 1:
			return ErrNoDisponible // `go run` o un binario suelto: no hay .app que registrar
		default:
			errs = append(errs, fmt.Errorf("escritorio: .%s no se ha podido poner por defecto (OSStatus %d)", ext, int(r)))
		}
	}
	return errors.Join(errs...)
}
