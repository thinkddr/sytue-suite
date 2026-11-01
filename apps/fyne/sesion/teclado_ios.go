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

static void sytue_actualizar_alto_teclado(NSNotification *note) {
	NSDictionary *info = note.userInfo;
	CGRect endFrame = [[info objectForKey:UIKeyboardFrameEndUserInfoKey] CGRectValue];
	CGRect screenBounds = [UIScreen mainScreen].bounds;
	float solape = (float)(screenBounds.size.height - endFrame.origin.y);
	if (solape <= 0.0f || endFrame.size.height <= 0.0f) {
		sytue_alto_teclado_ios = 0.0f;
	} else {
		sytue_alto_teclado_ios = solape;
	}
}

static void sytue_iniciar_teclado() {
	if (sytue_teclado_iniciado) return;
	sytue_teclado_iniciado = 1;
	NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
	NSOperationQueue *mq = [NSOperationQueue mainQueue];
	[nc addObserverForName:UIKeyboardWillChangeFrameNotification
	                object:nil
	                 queue:mq
	            usingBlock:^(NSNotification *note) {
		sytue_actualizar_alto_teclado(note);
	}];
	[nc addObserverForName:UIKeyboardDidChangeFrameNotification
	                object:nil
	                 queue:mq
	            usingBlock:^(NSNotification *note) {
		sytue_actualizar_alto_teclado(note);
	}];
	[nc addObserverForName:UIKeyboardWillHideNotification
	                object:nil
	                 queue:mq
	            usingBlock:^(NSNotification *note) {
		sytue_alto_teclado_ios = 0.0f;
	}];
}

__attribute__((constructor))
static void sytue_preparar_teclado_ios(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		sytue_iniciar_teclado();
	});
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
