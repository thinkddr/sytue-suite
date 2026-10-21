// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package sesion

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework UIKit -framework Foundation
#import <UIKit/UIKit.h>

static float sytue_alto_teclado_ios = 0.0f;
static int sytue_teclado_iniciado = 0;

static void sytue_iniciar_teclado() {
	if (sytue_teclado_iniciado) return;
	sytue_teclado_iniciado = 1;
	[[NSNotificationCenter defaultCenter] addObserverForName:UIKeyboardWillChangeFrameNotification
	                                                  object:nil
	                                                   queue:[NSOperationQueue mainQueue]
	                                              usingBlock:^(NSNotification *note) {
		NSDictionary *info = note.userInfo;
		CGRect endFrame = [[info objectForKey:UIKeyboardFrameEndUserInfoKey] CGRectValue];
		UIScreen *screen = [UIScreen mainScreen];
		if (endFrame.origin.y >= screen.bounds.size.height) {
			sytue_alto_teclado_ios = 0.0f;
		} else {
			sytue_alto_teclado_ios = (float)endFrame.size.height;
		}
	}];
	[[NSNotificationCenter defaultCenter] addObserverForName:UIKeyboardWillHideNotification
	                                                  object:nil
	                                                   queue:[NSOperationQueue mainQueue]
	                                              usingBlock:^(NSNotification *note) {
		sytue_alto_teclado_ios = 0.0f;
	}];
}

static float sytue_obtener_alto_teclado_ios() {
	if (!sytue_teclado_iniciado) {
		if ([NSThread isMainThread]) {
			sytue_iniciar_teclado();
		} else {
			dispatch_sync(dispatch_get_main_queue(), ^{
				sytue_iniciar_teclado();
			});
		}
	}
	return sytue_alto_teclado_ios;
}
*/
import "C"

// AltoTeclado devuelve los puntos que tapa el teclado en iOS.
func AltoTeclado() float32 {
	return float32(C.sytue_obtener_alto_teclado_ios())
}
