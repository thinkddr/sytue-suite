// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin

package escritorio

// Los avisos con clic de macOS e iOS: el mismo `UNUserNotificationCenter` en los dos, y
// por eso un solo fichero (en Go, `ios` implica `darwin`). La insignia sí es de cada uno:
// el Dock en macOS (escritorio_darwin.go) y el número del icono en iOS
// (escritorio_ios.go).

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#include <stdlib.h>
#include <TargetConditionals.h>
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

extern void sytueAvisoPulsado(char *clave);

// sytueLibre suelta una cadena de C.CString. Está aquí para que el Go no importe
// `unsafe`: gocritic toma `import "C"` y `unsafe` por el mismo paquete dos veces.
static void sytueLibre(char *p) { free(p); }

// Lo que se pide al preguntar: en iOS también el número del icono, que allí va con el
// mismo permiso; en macOS el Dock no lo necesita.
#if TARGET_OS_IPHONE
static const UNAuthorizationOptions sytuePermisos = UNAuthorizationOptionAlert | UNAuthorizationOptionBadge;
#else
static const UNAuthorizationOptions sytuePermisos = UNAuthorizationOptionAlert;
#endif

// El delegado es lo que entrega el clic: sin él, pulsar el aviso solo trae la app al
// frente. En el `userInfo` va el ENLACE del aviso entero —clave y destino—, y es lo
// único que vuelve a Go: con la app viva vale la clave; con la app recién abierta por el
// clic, el destino.
//
// No implementa `willPresentNotification`, a propósito: sin ese método, con la app
// delante el aviso no se enseña, que es lo que hacía el de Fyne y lo que hace cualquier
// app del Mac —la ventana ya lo dice—.
@interface SytueDelegadoAvisos : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation SytueDelegadoAvisos
- (void)userNotificationCenter:(UNUserNotificationCenter *)centro
    didReceiveNotificationResponse:(UNNotificationResponse *)respuesta
             withCompletionHandler:(void (^)(void))listo {
	id enlace = respuesta.notification.request.content.userInfo[@"sytue"];
	if ([enlace isKindOfClass:[NSString class]] &&
	    [respuesta.actionIdentifier isEqualToString:UNNotificationDefaultActionIdentifier]) {
		sytueAvisoPulsado((char *)[enlace UTF8String]);
	}
	listo();
}
@end

// sytueCentro es el centro de avisos con el delegado ya puesto, o nil sin identificador
// de paquete —un `go run`, un binario suelto, las pruebas—: ahí
// `currentNotificationCenter` ABORTA el proceso, así que ni se toca.
static UNUserNotificationCenter *sytueCentro(void) {
	if ([[NSBundle mainBundle] bundleIdentifier] == nil) {
		return nil;
	}
	static SytueDelegadoAvisos *delegado;
	static dispatch_once_t una;
	UNUserNotificationCenter *centro = [UNUserNotificationCenter currentNotificationCenter];
	dispatch_once(&una, ^{
		delegado = [SytueDelegadoAvisos new];
		centro.delegate = delegado;
	});
	return centro;
}

// sytuePrepararAvisos pone el delegado AL ARRANCAR. Un aviso pulsado con la app cerrada
// la abre, y el sistema solo le entrega ese clic si el delegado está puesto antes de que
// acabe `applicationDidFinishLaunching`; con el primer aviso ya sería tarde.
static void sytuePrepararAvisos(void) {
	@autoreleasepool {
		sytueCentro();
	}
}

// sytueAvisar enseña el aviso con su enlace y devuelve 0 si no se puede. La primera vez
// el sistema pregunta si se permiten avisos; si se dice que no, no sale nada y no hay a
// quién avisar.
static int sytueAvisar(const char *enlace, const char *titulo, const char *cuerpo) {
	@autoreleasepool {
		UNUserNotificationCenter *centro = sytueCentro();
		if (centro == nil) {
			return 0;
		}
		NSString *e = [NSString stringWithUTF8String:enlace];
		UNMutableNotificationContent *c = [[UNMutableNotificationContent new] autorelease];
		c.title = [NSString stringWithUTF8String:titulo] ?: @"";
		c.body = [NSString stringWithUTF8String:cuerpo] ?: @"";
		c.userInfo = @{@"sytue" : e};
		UNNotificationRequest *pedido = [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
		                                                                     content:c
		                                                                     trigger:nil];
		[centro requestAuthorizationWithOptions:sytuePermisos
		                      completionHandler:^(BOOL permitido, NSError *error) {
			if (permitido) {
				[centro addNotificationRequest:pedido withCompletionHandler:nil];
			} else {
				NSLog(@"sytue: sin permiso para avisos: %@", error);
			}
		}];
	}
	return 1;
}
// sytuePedirPermiso pregunta si se permiten avisos sin poner ninguno (en iOS, al entrar).
// No es `static`: la llama escritorio_ios.go desde su propio preámbulo.
void sytuePedirPermiso(void) {
	@autoreleasepool {
		UNUserNotificationCenter *centro = sytueCentro();
		[centro requestAuthorizationWithOptions:sytuePermisos completionHandler:^(BOOL p, NSError *e) {}];
	}
}
*/
import "C"

import "cmp"

// El delegado se pone al cargar el paquete, antes de que el toolkit arranque la
// aplicación: es la única forma de llegar antes del fin de
// `applicationDidFinishLaunching` (macOS) o de `application:didFinishLaunchingWithOptions:`
// (iOS) sea cual sea el orden del `main` de cada app.
func init() { C.sytuePrepararAvisos() }

// avisar lo enseña por `UNUserNotificationCenter` con el delegado de arriba, que
// devuelve el enlace a [Pulsado] al hacer clic.
func avisar(a Aviso) error {
	enlace := EnlaceDeAviso(cmp.Or(a.Esquema, "sytue"), apuntar(a.AlPulsar), a.Destino)
	e, titulo, cuerpo := C.CString(enlace), C.CString(a.Titulo), C.CString(a.Cuerpo)
	defer C.sytueLibre(e)
	defer C.sytueLibre(titulo)
	defer C.sytueLibre(cuerpo)
	if C.sytueAvisar(e, titulo, cuerpo) == 0 {
		return ErrNoDisponible
	}
	return nil
}
