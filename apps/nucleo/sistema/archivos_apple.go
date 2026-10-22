// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (ios || (darwin && tienda)) && cgo

package sistema

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Security -framework FileProvider
#include <stdlib.h>
#import <Foundation/Foundation.h>
#import <FileProvider/FileProvider.h>
#import <Security/Security.h>

// La consulta de la entrada de la extensión: «generic password» con su servicio, su
// cuenta y el grupo compartido EXPLÍCITO —sin él, SecItem guarda en el primer grupo de la
// app, que en el Mac es el de la sesión de la suite y la extensión no ve—.
static NSMutableDictionary *sytueArchivosConsulta(const char *grupo, const char *servicio, const char *cuenta) {
	NSMutableDictionary *q = [NSMutableDictionary dictionary];
	q[(__bridge id)kSecClass] = (__bridge id)kSecClassGenericPassword;
	q[(__bridge id)kSecAttrService] = [NSString stringWithUTF8String:servicio];
	q[(__bridge id)kSecAttrAccount] = [NSString stringWithUTF8String:cuenta];
	q[(__bridge id)kSecAttrAccessGroup] = [NSString stringWithUTF8String:grupo];
	// En el Mac, el llavero de protección de datos: es el que entiende de grupos (ver
	// apps/nucleo/auth/store_apple.c). En iOS es el único.
	q[(__bridge id)kSecUseDataProtectionKeychain] = @YES;
	return q;
}

static int sytueArchivosBorrar(const char *grupo, const char *servicio, const char *cuenta) {
	@autoreleasepool {
		OSStatus st = SecItemDelete((__bridge CFDictionaryRef)sytueArchivosConsulta(grupo, servicio, cuenta));
		return (st == errSecSuccess || st == errSecItemNotFound) ? 0 : (int)st;
	}
}

static int sytueArchivosGuardar(const char *grupo, const char *servicio, const char *cuenta, const void *datos, int n) {
	int err = sytueArchivosBorrar(grupo, servicio, cuenta);
	if (err) {
		return err;
	}
	@autoreleasepool {
		NSMutableDictionary *q = sytueArchivosConsulta(grupo, servicio, cuenta);
		q[(__bridge id)kSecValueData] = [NSData dataWithBytes:datos length:(NSUInteger)n];
		// Después del primer desbloqueo y no «solo desbloqueado»: la extensión sincroniza
		// con el teléfono bloqueado en el bolsillo. Ni copia de seguridad ni otro equipo.
		q[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly;
		OSStatus st = SecItemAdd((__bridge CFDictionaryRef)q, NULL);
		return st == errSecSuccess ? 0 : (int)st;
	}
}

// 1 si hay entrada, 0 si no, y el OSStatus si el llavero falla.
static int sytueArchivosHay(const char *grupo, const char *servicio, const char *cuenta) {
	@autoreleasepool {
		NSMutableDictionary *q = sytueArchivosConsulta(grupo, servicio, cuenta);
		q[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
		OSStatus st = SecItemCopyMatching((__bridge CFDictionaryRef)q, NULL);
		if (st == errSecItemNotFound) {
			return 0;
		}
		return st == errSecSuccess ? 1 : (int)st;
	}
}

// La extensión es NSFileProviderReplicatedExtension: macOS 13 e iOS 16.
static int sytueArchivosDisponible(void) {
	if (@available(macOS 13.0, iOS 16.0, *)) {
		return 1;
	}
	return 0;
}

// El alta y la baja son asíncronas; se esperan aquí, fuera del hilo principal (las llama
// una rutina de Go). Devuelven 0, el código del NSError, o -1 si no hay extensión.
static int sytueArchivosDominio(const char *identificador, const char *nombre, int alta) {
	if (@available(macOS 13.0, iOS 16.0, *)) {
		@autoreleasepool {
			NSFileProviderDomain *d = [[NSFileProviderDomain alloc]
				initWithIdentifier:[NSString stringWithUTF8String:identificador]
				displayName:[NSString stringWithUTF8String:nombre]];
			dispatch_semaphore_t listo = dispatch_semaphore_create(0);
			__block NSInteger codigo = 0;
			void (^fin)(NSError *) = ^(NSError *e) {
				codigo = e ? (e.code ? e.code : -2) : 0;
				dispatch_semaphore_signal(listo);
			};
			if (alta) {
				[NSFileProviderManager addDomain:d completionHandler:fin];
			} else {
				[NSFileProviderManager removeDomain:d completionHandler:fin];
			}
			if (dispatch_semaphore_wait(listo, dispatch_time(DISPATCH_TIME_NOW, 60 * NSEC_PER_SEC)) != 0) {
				return -3;
			}
			return (int)codigo;
		}
	}
	return -1;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// dominioArchivos es el identificador del NSFileProviderDomain: uno por app.
const dominioArchivos = "sytue-drive"

// cadenas convierte a C los tres nombres de la entrada; quien llama suelta con `soltar`.
func cadenas() (grupo, servicio, cuenta *C.char, soltar func()) {
	grupo, servicio, cuenta = C.CString(GrupoLlaveroArchivos), C.CString(servicioArchivos), C.CString(cuentaArchivos)
	return grupo, servicio, cuenta, func() {
		C.free(unsafe.Pointer(grupo))
		C.free(unsafe.Pointer(servicio))
		C.free(unsafe.Pointer(cuenta))
	}
}

func archivosDisponible() bool { return C.sytueArchivosDisponible() == 1 }

func guardarCredencialArchivos(crudo []byte) error {
	g, s, c, soltar := cadenas()
	defer soltar()
	if st := C.sytueArchivosGuardar(g, s, c, unsafe.Pointer(&crudo[0]), C.int(len(crudo))); st != 0 {
		return fmt.Errorf("sistema: el llavero no deja guardar la credencial de archivos (OSStatus %d)", int(st))
	}
	return nil
}

func hayCredencialArchivos() (bool, error) {
	g, s, c, soltar := cadenas()
	defer soltar()
	switch st := C.sytueArchivosHay(g, s, c); st {
	case 0, 1:
		return st == 1, nil
	default:
		return false, fmt.Errorf("sistema: el llavero no deja leer la credencial de archivos (OSStatus %d)", int(st))
	}
}

func dominio(nombre string, alta bool) error {
	id, n := C.CString(dominioArchivos), C.CString(nombre)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(n))
	a, que := C.int(0), "dar de baja"
	if alta {
		a, que = 1, "dar de alta"
	}
	switch st := C.sytueArchivosDominio(id, n, a); st {
	case 0:
		return nil
	case -1:
		return ErrArchivosNoDisponible
	default:
		return fmt.Errorf("sistema: NSFileProviderManager no deja %s el dominio (código %d)", que, int(st))
	}
}

func registrarArchivos(nombre string) error { return dominio(nombre, true) }

func olvidarArchivos() error {
	// El nombre no cuenta para la baja: el dominio se reconoce por su identificador.
	errDominio := dominio("Sytue Drive", false)
	if errors.Is(errDominio, ErrArchivosNoDisponible) {
		errDominio = nil
	}
	g, s, c, soltar := cadenas()
	defer soltar()
	var errLlavero error
	if st := C.sytueArchivosBorrar(g, s, c); st != 0 {
		errLlavero = fmt.Errorf("sistema: el llavero no deja borrar la credencial de archivos (OSStatus %d)", int(st))
	}
	return errors.Join(errDominio, errLlavero)
}
