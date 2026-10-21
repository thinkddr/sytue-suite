// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios

package escritorio

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework UserNotifications
#import <UserNotifications/UserNotifications.h>

void sytuePedirPermiso(void); // avisos_apple.go

// El número del icono. `setBadgeCount` es de iOS 16; antes, sin número.
static void sytueInsigniaIOS(long n) {
	if (@available(iOS 16.0, *)) {
		[[UNUserNotificationCenter currentNotificationCenter] setBadgeCount:n withCompletionHandler:nil];
	}
}

// Los avisos, con su clic, están en avisos_apple.go.
*/
import "C"

// insignia pone el número sobre el icono de la app; lo decide `ui.insigniaDe`.
//
// ponytail: compilado con el SDK de Xcode 27 y sin verlo en un iPhone (no hay cuenta de
// Apple con la que firmar la app para instalarla).
func insignia(_ string, n int) error {
	C.sytueInsigniaIOS(C.long(n))
	return nil
}

func hayBandeja() bool { return true }

// prepararAvisos pregunta al entrar si se permiten avisos y el número del icono, como en
// Android: sin preguntar antes, el primer aviso se quedaría esperando a la respuesta.
func prepararAvisos(string) error {
	C.sytuePedirPermiso()
	return nil
}
