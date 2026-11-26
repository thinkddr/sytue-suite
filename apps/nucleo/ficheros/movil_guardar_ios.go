// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package ficheros

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework UIKit -framework Foundation
#import <UIKit/UIKit.h>
#include <stdlib.h>

static UIViewController *sytue_guardar_raiz(void) {
	UIViewController *root = nil;
	for (UIWindow *w in [UIApplication sharedApplication].windows) {
		if (w.isKeyWindow) { root = w.rootViewController; break; }
	}
	while (root.presentedViewController && !root.presentedViewController.isBeingDismissed) {
		root = root.presentedViewController;
	}
	return root;
}

// Por copia de trabajo: el estado de su exportación (0 el selector abierto, 1 elegido,
// -1 cancelado) y el destino elegido, con su acceso de seguridad abierto mientras viva
// el proceso para poder volver a escribir ahí al guardar otra vez.
static NSMutableDictionary<NSString *, NSNumber *> *sytue_estados = nil;
static NSMutableDictionary<NSString *, NSURL *> *sytue_destinos = nil;
static NSMutableSet *sytue_delegados = nil;

static void sytue_poner_estado(NSString *ruta, int e, NSURL *destino) {
	@synchronized ([NSFileManager class]) {
		if (!sytue_estados) {
			sytue_estados = [NSMutableDictionary dictionary];
			sytue_destinos = [NSMutableDictionary dictionary];
		}
		sytue_estados[ruta] = @(e);
		if (destino) sytue_destinos[ruta] = destino;
	}
}

@interface SytueExportarDelegate : NSObject <UIDocumentPickerDelegate>
@property (nonatomic, copy) NSString *ruta;
@end

@implementation SytueExportarDelegate
- (void)documentPicker:(UIDocumentPickerViewController *)controller didPickDocumentsAtURLs:(NSArray<NSURL *> *)urls {
	NSURL *u = urls.firstObject;
	if (u) {
		[u startAccessingSecurityScopedResource]; // se queda abierto: es el sitio de los próximos Guardar
		sytue_poner_estado(self.ruta, 1, u);
	} else {
		sytue_poner_estado(self.ruta, -1, nil);
	}
	[sytue_delegados removeObject:self];
}
- (void)documentPickerWasCancelled:(UIDocumentPickerViewController *)controller {
	sytue_poner_estado(self.ruta, -1, nil);
	[sytue_delegados removeObject:self];
}
@end

// El selector de EXPORTAR de «Archivos» con la copia de trabajo: la persona elige
// carpeta (en el iPhone, iCloud Drive o la nube que tenga) y el sistema la copia ahí.
static void sytue_exportar_ios(const char *ruta) {
	NSString *r = [NSString stringWithUTF8String:ruta];
	sytue_poner_estado(r, 0, nil);
	dispatch_async(dispatch_get_main_queue(), ^{
		UIViewController *root = sytue_guardar_raiz();
		if (!root) { sytue_poner_estado(r, -1, nil); return; }
		NSURL *url = [NSURL fileURLWithPath:r];
		UIDocumentPickerViewController *p = [[UIDocumentPickerViewController alloc] initForExportingURLs:@[url] asCopy:YES];
		SytueExportarDelegate *d = [[SytueExportarDelegate alloc] init];
		d.ruta = r;
		if (!sytue_delegados) sytue_delegados = [NSMutableSet set];
		[sytue_delegados addObject:d];
		p.delegate = d;
		[root presentViewController:p animated:YES completion:nil];
	});
}

// -2 sin exportar todavía, 0 el selector abierto, 1 elegido, -1 cancelado.
static int sytue_exportado_ios(const char *ruta) {
	NSString *r = [NSString stringWithUTF8String:ruta];
	@synchronized ([NSFileManager class]) {
		NSNumber *e = sytue_estados[r];
		return e ? e.intValue : -2;
	}
}

// Vuelve a escribir la copia de trabajo en el destino elegido, coordinado con «Archivos»
// (y con el proveedor de la nube, si lo es). 1 hecho, 0 ahora no, -1 ese destino no vale.
static int sytue_reexportar_ios(const char *ruta) {
	@autoreleasepool {
		NSString *r = [NSString stringWithUTF8String:ruta];
		NSURL *destino = nil;
		@synchronized ([NSFileManager class]) {
			destino = sytue_destinos[r];
		}
		if (!destino) return -1;
		NSData *datos = [NSData dataWithContentsOfFile:r];
		if (!datos) return 0;
		__block BOOL ok = NO;
		NSError *err = nil;
		NSFileCoordinator *c = [[NSFileCoordinator alloc] initWithFilePresenter:nil];
		[c coordinateWritingItemAtURL:destino options:NSFileCoordinatorWritingForReplacing error:&err byAccessor:^(NSURL *nuevo) {
			ok = [datos writeToURL:nuevo options:NSDataWritingAtomic error:nil];
		}];
		if (ok) return 1;
		if (err && [err.domain isEqualToString:NSCocoaErrorDomain] &&
			(err.code == NSFileNoSuchFileError || err.code == NSFileWriteNoPermissionError)) return -1;
		return 0;
	}
}
*/
import "C"

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"
)

// «Guardar como…» en iOS: UIDocumentPickerViewController para EXPORTAR. iOS no da una
// ruta donde escribir antes de que el fichero exista, así que [GuardarComo] escribe en
// una copia de trabajo de la app (en sus Documentos) y, en cuanto está, se abre el
// selector con ella; lo que se guarde después se lleva al mismo sitio mientras la app
// siga abierta (movil_guardar.go). Tras cerrarla, Guardar deja el cambio en la copia de
// trabajo, y «Guardar como» lo vuelve a llevar a donde se quiera.

var reflejosIOS = nuevosReflejos("") // en iOS el destino no sobrevive al proceso: no se recuerda

func init() {
	if GuardarEnElMovil == nil {
		GuardarEnElMovil = guardarEnIOS
	}
}

func guardarEnIOS(nombre string, extensiones []string) (string, error) {
	docs, err := Carpeta() // los Documentos de la app
	if err != nil {
		return "", err
	}
	dir := carpetaGuardados(docs)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Una carpeta por «Guardar como»: el destino aún no se conoce.
	sub, err := os.MkdirTemp(dir, "")
	if err != nil {
		return "", err
	}
	ruta := filepath.Join(sub, nombrePropuesto(filepath.Base(nombre), extensiones))
	reflejosIOS.vigilar(ruta, "exportar", false, copiarIOS)
	return ruta, nil
}

func copiarIOS(ruta, _ string) error {
	cr := C.CString(ruta)
	defer C.free(unsafe.Pointer(cr))
	switch C.sytue_exportado_ios(cr) {
	case -2: // la copia acaba de aparecer: el selector
		C.sytue_exportar_ios(cr)
		return errReintentar
	case 0:
		return errReintentar // la persona aún está eligiendo
	case -1:
		// Canceló: la copia de trabajo se queda (y el documento, anclado a ella).
		return errors.New("ficheros: exportación cancelada")
	}
	switch C.sytue_reexportar_ios(cr) {
	case 1:
		return nil
	case 0:
		return errReintentar
	}
	return errors.New("ficheros: el destino elegido ya no se puede escribir")
}
