// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#include <string.h>
#include "camara_darwin.h"

// La pantalla de macOS con ScreenCaptureKit (macOS 14+): una captura por llamada, que
// es lo que pide la llamada a 5 fotogramas por segundo. CGDisplayCreateImage, la de
// siempre, ya no está en el SDK de macOS 15.
int sytuePantallaPermiso(void) {
	if (CGPreflightScreenCaptureAccess()) return 1;
	// Enseña el aviso del sistema la primera vez; el permiso vale al volver a abrir la app.
	return CGRequestScreenCaptureAccess() ? 1 : 0;
}

int sytuePantallaCapturar(uint8_t *dst, int cap, int *ancho, int *alto) {
	if (@available(macOS 14.0, *)) {
		__block int r = -1;
		dispatch_semaphore_t listo = dispatch_semaphore_create(0);
		[SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent *c, NSError *e) {
			if (c == nil || c.displays.count == 0) {
				r = e != nil ? -2 : -1;
				dispatch_semaphore_signal(listo);
				return;
			}
			SCDisplay *d = c.displays.firstObject;
			SCContentFilter *f = [[SCContentFilter alloc] initWithDisplay:d excludingWindows:@[]];
			SCStreamConfiguration *cfg = [SCStreamConfiguration new];
			cfg.width = d.width;
			cfg.height = d.height;
			cfg.showsCursor = YES;
			[SCScreenshotManager captureImageWithFilter:f configuration:cfg completionHandler:^(CGImageRef img, NSError *e2) {
				if (img != NULL) {
					int w = (int)CGImageGetWidth(img), h = (int)CGImageGetHeight(img);
					if (w * h * 4 <= cap) {
						CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
						CGContextRef ctx = CGBitmapContextCreate(dst, w, h, 8, w * 4, cs,
						                                         kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big);
						CGContextDrawImage(ctx, CGRectMake(0, 0, w, h), img);
						CGContextRelease(ctx);
						CGColorSpaceRelease(cs);
						r = 0;
					} else {
						r = -3;
					}
					*ancho = w;
					*alto = h;
				} else {
					r = e2 != nil ? -2 : -1;
				}
				dispatch_semaphore_signal(listo);
			}];
		}];
		dispatch_semaphore_wait(listo, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
		return r;
	}
	return -1;
}
