// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package ficheros

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework UIKit -framework Foundation -framework MobileCoreServices -framework CoreGraphics
#import <UIKit/UIKit.h>
#import <MobileCoreServices/MobileCoreServices.h>
#include <stdlib.h>
#include <string.h>

extern void sytue_elegir_ios_retorno(char *rutas);

static UIViewController* sytue_top_view_controller(void) {
	UIViewController *root = nil;
	for (UIWindow *w in [UIApplication sharedApplication].windows) {
		if (w.isKeyWindow) { root = w.rootViewController; break; }
	}
	if (!root) {
		root = [UIApplication sharedApplication].keyWindow.rootViewController;
	}
	while (root.presentedViewController && !root.presentedViewController.isBeingDismissed) {
		root = root.presentedViewController;
	}
	return root;
}

static int sytue_imprimir_ios(const char *ruta) {
	@autoreleasepool {
		NSString *path = [NSString stringWithUTF8String:ruta];
		NSURL *url = [NSURL fileURLWithPath:path];
		if (![UIPrintInteractionController canPrintURL:url]) {
			return -1;
		}
		dispatch_async(dispatch_get_main_queue(), ^{
			UIPrintInteractionController *pic = [UIPrintInteractionController sharedPrintController];
			UIPrintInfo *printInfo = [UIPrintInfo printInfo];
			printInfo.outputType = UIPrintInfoOutputGeneral;
			printInfo.jobName = [url lastPathComponent];
			pic.printInfo = printInfo;
			pic.printingItem = url;

			UIViewController *root = sytue_top_view_controller();
			if (!root) return;
			[pic presentAnimated:YES completionHandler:nil];
		});
		return 0;
	}
}

@interface SytueDocDelegate : NSObject <UIDocumentInteractionControllerDelegate>
@end

@implementation SytueDocDelegate
- (void)documentInteractionControllerDidDismissOpenInMenu:(UIDocumentInteractionController *)controller {
}
- (void)documentInteractionControllerDidDismissOptionsMenu:(UIDocumentInteractionController *)controller {
}
@end

static UIDocumentInteractionController *sytue_retained_doc = nil;
static SytueDocDelegate *sytue_doc_del = nil;

static int sytue_abrir_ios(const char *ruta, int compartir) {
	@autoreleasepool {
		NSString *path = [NSString stringWithUTF8String:ruta];
		NSURL *url = [NSURL fileURLWithPath:path];
		dispatch_async(dispatch_get_main_queue(), ^{
			UIViewController *root = sytue_top_view_controller();
			if (!root) return;

			UIView *v = root.view;
			CGRect rect = CGRectMake(CGRectGetMidX(v.bounds), CGRectGetMidY(v.bounds), 1, 1);

			if (!compartir) {
				sytue_retained_doc = [UIDocumentInteractionController interactionControllerWithURL:url];
				if (!sytue_doc_del) {
					sytue_doc_del = [[SytueDocDelegate alloc] init];
				}
				sytue_retained_doc.delegate = sytue_doc_del;
				if ([sytue_retained_doc presentOpenInMenuFromRect:rect inView:v animated:YES]) {
					return;
				}
				sytue_retained_doc = nil;
			}

			UIActivityViewController *avc = [[UIActivityViewController alloc] initWithActivityItems:@[url] applicationActivities:nil];
			if (avc.popoverPresentationController) {
				avc.popoverPresentationController.sourceView = v;
				avc.popoverPresentationController.sourceRect = rect;
			}
			[root presentViewController:avc animated:YES completion:nil];
		});
		return 0;
	}
}

static void* sytue_leer_fichero_ios(const char *ruta, int *longitud) {
	if (!ruta || !longitud) {
		if (longitud) *longitud = 0;
		return NULL;
	}
	@autoreleasepool {
		NSString *path = [NSString stringWithUTF8String:ruta];
		NSURL *url = nil;
		if ([path hasPrefix:@"file://"]) {
			url = [NSURL URLWithString:path];
		} else {
			url = [NSURL fileURLWithPath:path];
		}
		if (!url) {
			*longitud = 0;
			return NULL;
		}
		BOOL secured = NO;
		@try {
			secured = [url startAccessingSecurityScopedResource];
		} @catch (NSException *e) {
			secured = NO;
		}
		NSError *err = nil;
		NSData *datos = [NSData dataWithContentsOfURL:url options:0 error:&err];
		if (secured) {
			[url stopAccessingSecurityScopedResource];
		}
		if (!datos || datos.length == 0) {
			*longitud = 0;
			return NULL;
		}
		void *buf = malloc(datos.length);
		if (!buf) {
			*longitud = 0;
			return NULL;
		}
		memcpy(buf, datos.bytes, datos.length);
		*longitud = (int)datos.length;
		return buf;
	}
}

@interface SytuePickerDelegate : NSObject <UIDocumentPickerDelegate>
@property (nonatomic, copy) void (^onDone)(NSArray<NSString *> *paths);
@end

@implementation SytuePickerDelegate
- (void)documentPicker:(UIDocumentPickerViewController *)controller didPickDocumentsAtURLs:(NSArray<NSURL *> *)urls {
	NSMutableArray<NSString *> *paths = [NSMutableArray array];
	NSString *tmpDir = [NSTemporaryDirectory() stringByAppendingPathComponent:[[NSUUID UUID] UUIDString]];
	[[NSFileManager defaultManager] createDirectoryAtPath:tmpDir withIntermediateDirectories:YES attributes:nil error:nil];

	for (NSURL *url in urls) {
		BOOL secured = NO;
		@try {
			secured = [url startAccessingSecurityScopedResource];
		} @catch (NSException *e) {
			secured = NO;
		}
		NSString *destPath = [tmpDir stringByAppendingPathComponent:[url lastPathComponent]];
		NSURL *destURL = [NSURL fileURLWithPath:destPath];
		NSError *copyErr = nil;
		[[NSFileManager defaultManager] copyItemAtURL:url toURL:destURL error:&copyErr];
		if (secured) {
			[url stopAccessingSecurityScopedResource];
		}
		if (!copyErr && [[NSFileManager defaultManager] fileExistsAtPath:destPath]) {
			[paths addObject:destPath];
		}
	}
	if (self.onDone) {
		self.onDone(paths);
		self.onDone = nil;
	}
}

- (void)documentPickerWasCancelled:(UIDocumentPickerViewController *)controller {
	if (self.onDone) {
		self.onDone(@[]);
		self.onDone = nil;
	}
}
@end

static SytuePickerDelegate *sytue_picker_del = nil;

static void sytue_elegir_ios(const char *exts_csv) {
	dispatch_async(dispatch_get_main_queue(), ^{
		UIViewController *root = sytue_top_view_controller();
		if (!root) {
			sytue_elegir_ios_retorno(NULL);
			return;
		}

		NSMutableArray *docTypes = [NSMutableArray array];
		if (exts_csv && strlen(exts_csv) > 0) {
			NSString *str = [NSString stringWithUTF8String:exts_csv];
			for (NSString *ext in [str componentsSeparatedByString:@","]) {
				NSString *e = [ext stringByTrimmingCharactersInSet:[NSCharacterSet whitespaceAndNewlineCharacterSet]];
				if ([e hasPrefix:@"."]) e = [e substringFromIndex:1];
				if (e.length > 0) {
					CFStringRef uti = UTTypeCreatePreferredIdentifierForTag(kUTTagClassFilenameExtension, (__bridge CFStringRef)e, NULL);
					if (uti) {
						[docTypes addObject:(__bridge_transfer NSString *)uti];
					}
				}
			}
		}
		if (docTypes.count == 0) {
			[docTypes addObject:@"public.data"];
			[docTypes addObject:@"public.content"];
		}

		UIDocumentPickerViewController *picker = [[UIDocumentPickerViewController alloc]
			initWithDocumentTypes:docTypes inMode:UIDocumentPickerModeImport];
		picker.allowsMultipleSelection = YES;

		sytue_picker_del = [[SytuePickerDelegate alloc] init];
		sytue_picker_del.onDone = ^(NSArray<NSString *> *paths) {
			NSString *joined = [paths componentsJoinedByString:@"\n"];
			sytue_elegir_ios_retorno((char *)[joined UTF8String]);
			sytue_picker_del = nil;
		};
		picker.delegate = sytue_picker_del;

		[root presentViewController:picker animated:YES completion:nil];
	});
}
*/
import "C"

import (
	"errors"
	"strings"
	"sync"
	"unsafe"
)

var (
	elegirMu    sync.Mutex
	elegirCanal chan []string
)

func elegirMovilNativo(extensiones []string) ([]string, error) {
	elegirMu.Lock()
	ch := make(chan []string, 1)
	elegirCanal = ch
	elegirMu.Unlock()

	csv := strings.Join(extensiones, ",")
	ccsv := C.CString(csv)
	defer C.free(unsafe.Pointer(ccsv))
	C.sytue_elegir_ios(ccsv)
	rutas := <-ch
	return rutas, nil
}

func imprimirMovil(ruta string) error {
	cr := C.CString(ruta)
	defer C.free(unsafe.Pointer(cr))
	if C.sytue_imprimir_ios(cr) != 0 {
		return errors.New("no se puede imprimir este fichero")
	}
	return nil
}

func abrirMovil(ruta string) error {
	cr := C.CString(ruta)
	defer C.free(unsafe.Pointer(cr))
	if C.sytue_abrir_ios(cr, 0) != 0 {
		return errors.New("no se pudo abrir el fichero")
	}
	return nil
}

func revelarMovil(ruta string) error {
	cr := C.CString(ruta)
	defer C.free(unsafe.Pointer(cr))
	if C.sytue_abrir_ios(cr, 1) != 0 {
		return errors.New("no se pudo compartir el fichero")
	}
	return nil
}

func leerFicheroMovil(ruta string) ([]byte, error) {
	cr := C.CString(ruta)
	defer C.free(unsafe.Pointer(cr))
	var longitud C.int
	ptr := C.sytue_leer_fichero_ios(cr, &longitud)
	if ptr == nil || longitud <= 0 {
		return nil, errors.New("ficheros: no se pudo leer el fichero en iOS")
	}
	defer C.free(ptr)
	return C.GoBytes(ptr, longitud), nil
}

func leerLocalSeguro(ruta string) ([]byte, error) {
	return leerFicheroMovil(ruta)
}

func init() {
	EnElMovil = elegirMovilNativo
}
