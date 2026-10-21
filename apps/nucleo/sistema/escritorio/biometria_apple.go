// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

package escritorio

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework LocalAuthentication
#cgo darwin,!ios LDFLAGS: -framework AppKit
#include <stdlib.h>
#include <TargetConditionals.h>
#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#if TARGET_OS_OSX
#import <AppKit/AppKit.h>
#endif

// sytueBiometria: 0 no sabe verificar; 1 Touch ID; 2 Face ID; 3 Optic ID; 4 solo con la
// contraseña del equipo (o el código del teléfono).
static int sytueBiometria(void) {
	@autoreleasepool {
		LAContext *c = [[LAContext new] autorelease];
		if (![c canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:nil]) {
			return 0;
		}
		if ([c canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:nil]) {
			if (c.biometryType == LABiometryTypeTouchID) {
				return 1;
			}
			if (c.biometryType == LABiometryTypeFaceID) {
				return 2;
			}
			if (@available(macOS 14.0, iOS 17.0, *)) {
				if (c.biometryType == LABiometryTypeOpticID) {
					return 3;
				}
			}
		}
		return 4;
	}
}

// sytueVerificar: 0 verificado, 1 no, 2 este equipo no sabe. La respuesta llega en una
// cola privada; aquí se espera, que es lo que pide quien llama.
//
// Al ABRIR la app con el bloqueo puesto se llama desde el hilo principal y antes de que
// exista la aplicación de AppKit: esperar ahí con el semáforo dejaba el hilo principal
// parado, el sistema no llegaba a enseñar Touch ID y la app se quedaba abierta sin
// ventana para siempre (visto en un Mac el 2026-09-27). En el hilo principal se hace
// girar su bucle mientras tanto, y en macOS la app se pone delante: Touch ID solo
// pregunta a la app activa.
static int sytueVerificar(const char *motivo) {
	__block int r = 1;
	__block volatile BOOL hecho = NO;
	@autoreleasepool {
		LAContext *c = [[LAContext new] autorelease];
		if (![c canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:nil]) {
			return 2;
		}
		BOOL principal = [NSThread isMainThread];
#if TARGET_OS_OSX
		if (principal) {
			[NSApplication sharedApplication];
			[NSApp activateIgnoringOtherApps:YES];
		}
#endif
		dispatch_semaphore_t listo = dispatch_semaphore_create(0);
		[c evaluatePolicy:LAPolicyDeviceOwnerAuthentication
		  localizedReason:[NSString stringWithUTF8String:motivo] ?: @""
		            reply:^(BOOL si, NSError *e) {
			r = si ? 0 : 1;
			hecho = YES;
			dispatch_semaphore_signal(listo);
		}];
		if (principal) {
			while (!hecho) {
				[[NSRunLoop currentRunLoop] runMode:NSDefaultRunLoopMode
				                         beforeDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
			}
		} else {
			dispatch_semaphore_wait(listo, DISPATCH_TIME_FOREVER);
		}
		dispatch_release(listo);
	}
	return r;
}

// La cadena se suelta aquí para que el Go no importe `unsafe` (gocritic, dupImport).
static void sytueLibreLA(char *p) { free(p); }
*/
import "C"

import "context"

func biometria() (string, bool) {
	switch C.sytueBiometria() {
	case 1:
		return "Touch ID", false
	case 2:
		return "Face ID", false
	case 3:
		return "Optic ID", false
	case 4:
		return "", true
	}
	return "", false
}

// verificar pide la huella, la cara o la contraseña con el diálogo del sistema.
//
// ponytail: `ctx` no cierra el diálogo; LAContext.invalidate lo haría, y nadie cancela
// esto hoy.
func verificar(_ context.Context, motivo string) error {
	m := C.CString(motivo)
	defer C.sytueLibreLA(m)
	switch C.sytueVerificar(m) {
	case 0:
		return nil
	case 2:
		return ErrNoDisponible
	}
	return ErrNoVerificado
}
