// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package sistema

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework UIKit -framework Foundation
#import <UIKit/UIKit.h>
#include <stdlib.h>

// sytueSeguir pide a iOS unos segundos más con la app en segundo plano y devuelve el
// identificador de la tarea. El aviso de que se acaba el tiempo la cierra él mismo: una
// tarea que no se cierra a tiempo hace que iOS mate la app. Se llama desde cualquier hilo
// (UIApplication lo admite para este método).
static unsigned long sytueSeguir(const char *nombre) {
	@autoreleasepool {
		__block UIBackgroundTaskIdentifier id = UIBackgroundTaskInvalid;
		UIApplication *app = [UIApplication sharedApplication];
		id = [app beginBackgroundTaskWithName:[NSString stringWithUTF8String:nombre] expirationHandler:^{
			if (id != UIBackgroundTaskInvalid) {
				[app endBackgroundTask:id];
				id = UIBackgroundTaskInvalid;
			}
		}];
		return (unsigned long)id;
	}
}

static void sytueTerminar(unsigned long id) {
	if ((UIBackgroundTaskIdentifier)id == UIBackgroundTaskInvalid) return;
	dispatch_async(dispatch_get_main_queue(), ^{
		[[UIApplication sharedApplication] endBackgroundTask:(UIBackgroundTaskIdentifier)id];
	});
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

func seguirEnFondo(nombre string) func() {
	n := C.CString(nombre)
	defer C.free(unsafe.Pointer(n))
	id := C.sytueSeguir(n)
	return sync.OnceFunc(func() { C.sytueTerminar(id) })
}
