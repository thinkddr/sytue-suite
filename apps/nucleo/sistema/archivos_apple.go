// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (ios || darwin) && cgo

package sistema

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Security -framework FileProvider
#cgo darwin,!ios LDFLAGS: -framework AppKit
#cgo ios LDFLAGS: -framework UIKit -framework UniformTypeIdentifiers
#include <stdlib.h>
#import <Foundation/Foundation.h>
#import <FileProvider/FileProvider.h>
#import <Security/Security.h>
#import <TargetConditionals.h>
#if TARGET_OS_IPHONE
#import <UIKit/UIKit.h>
#import <UniformTypeIdentifiers/UniformTypeIdentifiers.h>
#else
#import <AppKit/AppKit.h>
#endif

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
static int sytueArchivosHay(const char *grupo, const char *servicio, const char *cuenta, const char *esperada) {
	@autoreleasepool {
		NSMutableDictionary *q = sytueArchivosConsulta(grupo, servicio, cuenta);
        q[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
        q[(__bridge id)kSecReturnData] = @YES;
        CFTypeRef salida = NULL;
        OSStatus st = SecItemCopyMatching((__bridge CFDictionaryRef)q, &salida);
        if (st == errSecItemNotFound) { return 0; }
        if (st != errSecSuccess) { return (int)st; }
        NSData *datos = CFBridgingRelease(salida);
        NSDictionary *credencial = [NSJSONSerialization JSONObjectWithData:datos options:0 error:nil];
        NSString *identidad = [credencial isKindOfClass:NSDictionary.class] ? credencial[@"identidad"] : nil;
        NSCharacterSet *otros = [[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet];
        BOOL valida = [identidad isKindOfClass:NSString.class] && identidad.length == 64 && [identidad rangeOfCharacterFromSet:otros].location == NSNotFound;
        if (!valida) { return 0; }
        return (!esperada || [identidad isEqualToString:[NSString stringWithUTF8String:esperada]]) ? 1 : 0;
	}
}

// La extensión es NSFileProviderReplicatedExtension: macOS 13 e iOS 16.
static int sytueArchivosDisponible(void) {
	if (@available(macOS 13.0, iOS 16.0, *)) {
        @autoreleasepool {
            NSURL *plugins = NSBundle.mainBundle.builtInPlugInsURL;
            NSBundle *extension = plugins ? [NSBundle bundleWithURL:[plugins URLByAppendingPathComponent:@"Archivos.appex"]] : nil;
            return [extension.bundleIdentifier isEqualToString:@"eu.sytue.drive.archivos"] ? 1 : 0;
        }
	}
	return 0;
}

// El alta y la baja son asíncronas; se esperan aquí, fuera del hilo principal (las llama
// una rutina de Go). Devuelven 0, el código del NSError, o -1 si no hay extensión.
static int sytueArchivosDominio(const char *identificador, const char *nombre, int alta) {
	if (@available(macOS 13.0, iOS 16.0, *)) {
		@autoreleasepool {
			NSFileProviderDomain *d = alta ? [[NSFileProviderDomain alloc]
				initWithIdentifier:[NSString stringWithUTF8String:identificador]
				displayName:[NSString stringWithUTF8String:nombre]] : nil;
			dispatch_semaphore_t listo = dispatch_semaphore_create(0);
			__block NSInteger codigo = 0;
			void (^fin)(NSError *) = ^(NSError *e) {
				codigo = e ? (e.code ? e.code : -2) : 0;
				dispatch_semaphore_signal(listo);
			};
            [NSFileProviderManager getDomainsWithCompletionHandler:^(NSArray<NSFileProviderDomain *> *domains, NSError *error) {
                if (error) { fin(error); return; }
                // El dominio representa cuenta Y autorización. Retirar los anteriores
                // evita que una URI/caché del SO de A se pueda resolver con la clave de B.
                dispatch_group_t retirados = dispatch_group_create();
                __block NSError *falloRetirada = nil;
                BOOL existing = NO;
                for (NSFileProviderDomain *candidate in domains) {
                    if (alta && [candidate.identifier isEqualToString:d.identifier]) { existing = YES; continue; }
                    if (![candidate.identifier isEqualToString:@"sytue-drive"] && ![candidate.identifier hasPrefix:@"sytue-drive."]) { continue; }
                    dispatch_group_enter(retirados);
                    void (^retirado)(NSError *) = ^(NSError *fallo) {
                        if (fallo) { @synchronized (retirados) { falloRetirada = fallo; } }
                        dispatch_group_leave(retirados);
                    };
#if TARGET_OS_IPHONE
                    // iOS no ofrece PreserveDirtyUserData. Su baja elimina la caché;
                    // nunca se reactiva A para subir contenido tras cerrar la sesión.
                    [NSFileProviderManager removeDomain:candidate completionHandler:retirado];
#else
                    // El SO conserva las ediciones pendientes en una carpeta recuperada.
                    // No se importa ni se vuelve a subir al dominio nuevo, de otra cuenta.
                    [NSFileProviderManager removeDomain:candidate mode:NSFileProviderDomainRemovalModePreserveDirtyUserData completionHandler:^(NSURL *preservados, NSError *fallo) {
                        (void)preservados;
                        retirado(fallo);
                    }];
#endif
                }
                dispatch_group_notify(retirados, dispatch_get_global_queue(QOS_CLASS_UTILITY, 0), ^{
                    if (falloRetirada) { fin(falloRetirada); }
                    else if (alta && !existing) { [NSFileProviderManager addDomain:d completionHandler:fin]; }
                    else { fin(nil); }
                });
            }];
			if (dispatch_semaphore_wait(listo, dispatch_time(DISPATCH_TIME_NOW, 60 * NSEC_PER_SEC)) != 0) {
				return -3;
			}
			return (int)codigo;
		}
	}
	return -1;
}
// Se pide al File Provider su URL; nunca se deduce ~/Library/CloudStorage ni se usa
// un esquema privado de Archivos. Se llama fuera del hilo principal.
static int sytueArchivosAbrir(const char *identificador, int estricto) {
    if (@available(macOS 13.0, iOS 16.0, *)) {
        @autoreleasepool {
            NSFileProviderDomain *domain = [[NSFileProviderDomain alloc]
                initWithIdentifier:[NSString stringWithUTF8String:identificador] displayName:@"Sytue Drive"];
            NSFileProviderManager *manager = [NSFileProviderManager managerForDomain:domain];
            if (!manager) { return -1; }
            dispatch_semaphore_t listo = dispatch_semaphore_create(0);
            __block NSInteger codigo = 0;
            [manager getUserVisibleURLForItemIdentifier:NSFileProviderRootContainerItemIdentifier completionHandler:^(NSURL *url, NSError *error) {
                // Una apertura ligada a una cuenta no cae al selector global de iOS.
                if (estricto && (error || !url)) {
                    codigo = error.code ?: -2;
                    dispatch_semaphore_signal(listo);
                    return;
                }
#if !TARGET_OS_IPHONE
                if (error || !url) {
                    codigo = error.code ?: -2;
                    dispatch_semaphore_signal(listo);
                    return;
                }
#endif
                // iOS puede mantener el dominio desactivado hasta que la persona lo
                // habilite en Ubicaciones. El selector nativo sigue disponible sin URL.
                dispatch_async(dispatch_get_main_queue(), ^{
#if TARGET_OS_IPHONE
                    UIViewController *root = nil;
                    for (UIScene *scene in UIApplication.sharedApplication.connectedScenes) {
                        if (![scene isKindOfClass:UIWindowScene.class] || scene.activationState != UISceneActivationStateForegroundActive) { continue; }
                        for (UIWindow *window in ((UIWindowScene *)scene).windows) {
                            if (window.isKeyWindow) { root = window.rootViewController; break; }
                        }
                        if (root) { break; }
                    }
                    while (root.presentedViewController && !root.presentedViewController.isBeingDismissed) { root = root.presentedViewController; }
                    if (!root) { codigo = -4; }
                    else {
                        UIDocumentPickerViewController *picker = [[UIDocumentPickerViewController alloc] initForOpeningContentTypes:@[UTTypeFolder] asCopy:NO];
                        if (url && !error) { picker.directoryURL = url; }
                        [root presentViewController:picker animated:YES completion:nil];
                    }
#else
                    if (![NSWorkspace.sharedWorkspace openURL:url]) { codigo = -4; }
#endif
                    dispatch_semaphore_signal(listo);
                });
            }];
            if (dispatch_semaphore_wait(listo, dispatch_time(DISPATCH_TIME_NOW, 30 * NSEC_PER_SEC)) != 0) { return -3; }
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

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
)

// Una autorización tiene su propio dominio y caché del SO. La revisión es estable
// al renovar, pero cambia al salir o cambiar de cuenta, incluso A→B→A.
func dominioActualArchivos() (string, error) {
	a, err := ajustes.LeerAccesoDrive()
	if err != nil {
		return "", err
	}
	if !a.Activo {
		return "", errors.New("sistema: el acceso a archivos de Drive no está activo")
	}
	return identificadorDominioArchivos(a.Identidad, a.Revision)
}

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

func hayCredencialArchivos() (bool, error) { return hayCredencialArchivosDe("") }

func hayCredencialArchivosDe(esperada string) (bool, error) {
	var dueno *C.char
	if esperada != "" {
		dueno = C.CString(esperada)
	}
	defer C.free(unsafe.Pointer(dueno))
	g, s, c, soltar := cadenas()
	defer soltar()
	switch st := C.sytueArchivosHay(g, s, c, dueno); st {
	case 0, 1:
		return st == 1, nil
	default:
		return false, fmt.Errorf("sistema: el llavero no deja leer la credencial de archivos (OSStatus %d)", int(st))
	}
}

func dominio(identificador, nombre string, alta bool) error {
	id, n := C.CString(identificador), C.CString(nombre)
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

func registrarArchivos(nombre string) error {
	id, err := dominioActualArchivos()
	if err != nil {
		return err
	}
	return dominio(id, nombre, true)
}

func olvidarArchivos() error {
	// Retira también dominios anteriores, aunque el marcador ya esté cerrado.
	errDominio := dominio("", "Sytue Drive", false)
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

func abrirArchivos() error {
	if !archivosDisponible() {
		return ErrArchivosNoDisponible
	}
	dominio, err := dominioActualArchivos()
	if err != nil {
		return err
	}
	return abrirDominioArchivos(dominio, false)
}

// El dominio viene ya resuelto; esta función no consulta el marcador global.
func abrirDominioArchivos(dominio string, estricto bool) error {
	if !archivosDisponible() {
		return ErrArchivosNoDisponible
	}
	id := C.CString(dominio)
	defer C.free(unsafe.Pointer(id))
	modo := C.int(0)
	if estricto {
		modo = 1
	}
	if st := C.sytueArchivosAbrir(id, modo); st != 0 {
		return fmt.Errorf("sistema: no se pudo abrir el dominio de Drive (código %d)", int(st))
	}
	return nil
}
