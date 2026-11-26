// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && cgo

package carrete

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Photos -framework ImageIO -framework CoreGraphics -framework Network
#import <Foundation/Foundation.h>
#import <Photos/Photos.h>
#import <ImageIO/ImageIO.h>
#import <Network/Network.h>
#include <stdlib.h>
#include <string.h>

// Lo que PhotoKit dice del acceso, en los números de Permiso: 0 sin preguntar, 1 todo,
// 2 limitado, 3 denegado o restringido (control parental, MDM).
static int sytue_carrete_traducir(PHAuthorizationStatus s) {
	switch (s) {
	case PHAuthorizationStatusAuthorized: return 1;
	case PHAuthorizationStatusLimited: return 2;
	case PHAuthorizationStatusNotDetermined: return 0;
	default: return 3;
	}
}

// 1 si el Info.plist explica para qué se leen las fotos. Sin esa clave, iOS y macOS MATAN
// la app al pedir el permiso: pasa con un binario sin empaquetar (`go run`) o con un
// empaquetado que no lleva `fototeca = "si"`. Ahí la galería no se ofrece.
static int sytue_carrete_declarado(void) {
	@autoreleasepool {
		id v = [[NSBundle mainBundle] objectForInfoDictionaryKey:@"NSPhotoLibraryUsageDescription"];
		return [v isKindOfClass:[NSString class]] && [(NSString *)v length] > 0 ? 1 : 0;
	}
}

static int sytue_carrete_estado(void) {
	return sytue_carrete_traducir([PHPhotoLibrary authorizationStatusForAccessLevel:PHAccessLevelReadWrite]);
}

// Pide el acceso de lectura y escritura y ESPERA la respuesta. Se llama desde una
// goroutine, nunca desde el hilo principal: el diálogo lo pinta el sistema en el suyo.
static int sytue_carrete_pedir(void) {
	dispatch_semaphore_t listo = dispatch_semaphore_create(0);
	__block PHAuthorizationStatus r = PHAuthorizationStatusNotDetermined;
	[PHPhotoLibrary requestAuthorizationForAccessLevel:PHAccessLevelReadWrite handler:^(PHAuthorizationStatus s) {
		r = s;
		dispatch_semaphore_signal(listo);
	}];
	dispatch_semaphore_wait(listo, DISPATCH_TIME_FOREVER);
	return sytue_carrete_traducir(r);
}

// Con solo «añadir» (NSPhotoLibraryAddUsageDescription) basta para guardar.
static int sytue_carrete_pedir_anadir(void) {
	PHAuthorizationStatus s = [PHPhotoLibrary authorizationStatusForAccessLevel:PHAccessLevelAddOnly];
	if (s != PHAuthorizationStatusNotDetermined) {
		return sytue_carrete_traducir(s);
	}
	dispatch_semaphore_t listo = dispatch_semaphore_create(0);
	__block PHAuthorizationStatus r = PHAuthorizationStatusNotDetermined;
	[PHPhotoLibrary requestAuthorizationForAccessLevel:PHAccessLevelAddOnly handler:^(PHAuthorizationStatus st) {
		r = st;
		dispatch_semaphore_signal(listo);
	}];
	dispatch_semaphore_wait(listo, DISPATCH_TIME_FOREVER);
	return sytue_carrete_traducir(r);
}

static char *sytue_carrete_copia(NSString *s) {
	const char *u = s.UTF8String;
	return strdup(u ? u : "");
}

static NSString *sytue_carrete_limpio(NSString *s) {
	if (!s) return @"";
	s = [s stringByReplacingOccurrencesOfString:@"\t" withString:@" "];
	s = [s stringByReplacingOccurrencesOfString:@"\n" withString:@" "];
	return [s stringByReplacingOccurrencesOfString:@"\r" withString:@" "];
}

// El álbum de Sytue («Sytue Photos»), o nil si no está o no se ve (acceso limitado).
static PHAssetCollection *sytue_carrete_album(const char *titulo) {
	PHFetchOptions *o = [PHFetchOptions new];
	o.predicate = [NSPredicate predicateWithFormat:@"localizedTitle == %@", [NSString stringWithUTF8String:titulo]];
	return [PHAssetCollection fetchAssetCollectionsWithType:PHAssetCollectionTypeAlbum
	                                                subtype:PHAssetCollectionSubtypeAlbumRegular
	                                                options:o].firstObject;
}

// El recurso ORIGINAL de una foto o un vídeo: el de la cámara, sin las ediciones de
// Fotos encima.
static PHAssetResource *sytue_carrete_original(PHAsset *a) {
	BOOL video = a.mediaType == PHAssetMediaTypeVideo;
	PHAssetResourceType quiero = video ? PHAssetResourceTypeVideo : PHAssetResourceTypePhoto;
	PHAssetResourceType editado = video ? PHAssetResourceTypeFullSizeVideo : PHAssetResourceTypeFullSizePhoto;
	NSArray<PHAssetResource *> *rs = [PHAssetResource assetResourcesForAsset:a];
	for (PHAssetResource *r in rs) {
		if (r.type == quiero) return r;
	}
	for (PHAssetResource *r in rs) {
		if (r.type == editado) return r;
	}
	return rs.firstObject;
}

// Las fotos y los vídeos, del más antiguo al más reciente, uno por línea:
// «id\tnombre\t\tmilisegundos\t-1» y, en un vídeo, detrás «\tv\tduración\tancho\talto»
// (la duración en milisegundos), sin los del álbum de Sytue. El tipo va vacío (Go lo saca
// del nombre) y el tamaño no se sabe sin leer el recurso.
static char *sytue_carrete_listar(const char *propio) {
	@autoreleasepool {
		NSMutableSet<NSString *> *saltar = [NSMutableSet set];
		PHAssetCollection *album = sytue_carrete_album(propio);
		if (album) {
			PHFetchResult<PHAsset *> *suyas = [PHAsset fetchAssetsInAssetCollection:album options:nil];
			[suyas enumerateObjectsUsingBlock:^(PHAsset *a, NSUInteger i, BOOL *fin) {
				[saltar addObject:a.localIdentifier];
			}];
		}
		PHFetchOptions *o = [PHFetchOptions new];
		o.predicate = [NSPredicate predicateWithFormat:@"mediaType == %d OR mediaType == %d",
		                                               (int)PHAssetMediaTypeImage, (int)PHAssetMediaTypeVideo];
		o.sortDescriptors = @[ [NSSortDescriptor sortDescriptorWithKey:@"creationDate" ascending:YES] ];
		PHFetchResult<PHAsset *> *todas = [PHAsset fetchAssetsWithOptions:o];
		NSMutableString *s = [NSMutableString string];
		[todas enumerateObjectsUsingBlock:^(PHAsset *a, NSUInteger i, BOOL *fin) {
			@autoreleasepool {
				if ([saltar containsObject:a.localIdentifier]) return;
				PHAssetResource *r = sytue_carrete_original(a);
				NSDate *f = a.creationDate ?: a.modificationDate;
				NSString *video = @"";
				if (a.mediaType == PHAssetMediaTypeVideo) {
					video = [NSString stringWithFormat:@"\tv\t%lld\t%lu\t%lu",
					                                   (long long)(a.duration > 0 ? a.duration * 1000.0 : 0),
					                                   (unsigned long)a.pixelWidth, (unsigned long)a.pixelHeight];
				}
				[s appendFormat:@"%@\t%@\t\t%lld\t-1%@\n", a.localIdentifier,
				               sytue_carrete_limpio(r.originalFilename),
				               (long long)(f ? f.timeIntervalSince1970 * 1000.0 : 0), video];
			}
		}];
		return sytue_carrete_copia(s);
	}
}

// Escribe el original de `id` en `destino`, que NO debe existir. Lo baja de iCloud si
// hace falta (la «optimización de almacenamiento» deja en el teléfono solo una copia
// reducida). Devuelve NULL si ha ido bien, o el motivo (que libera quien llama).
static char *sytue_carrete_copiar(const char *id, const char *destino) {
	@autoreleasepool {
		PHAsset *a = [PHAsset fetchAssetsWithLocalIdentifiers:@[ [NSString stringWithUTF8String:id] ] options:nil].firstObject;
		if (!a) return strdup("la foto ya no está en la galería");
		PHAssetResource *r = sytue_carrete_original(a);
		if (!r) return strdup("la foto no tiene original");
		PHAssetResourceRequestOptions *o = [PHAssetResourceRequestOptions new];
		o.networkAccessAllowed = YES;
		dispatch_semaphore_t listo = dispatch_semaphore_create(0);
		__block NSString *motivo = nil;
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:destino]];
		[[PHAssetResourceManager defaultManager] writeDataForAssetResource:r toFile:url options:o
		                                                completionHandler:^(NSError *e) {
			if (e) motivo = e.localizedDescription ?: @"error de PhotoKit";
			dispatch_semaphore_signal(listo);
		}];
		dispatch_semaphore_wait(listo, DISPATCH_TIME_FOREVER);
		return motivo ? sytue_carrete_copia(motivo) : NULL;
	}
}

// Pasa la foto de `origen` (HEIC, WebP, AVIF, RAW) a JPEG en `destino` con ImageIO, que
// lleva con ella sus propiedades: EXIF, GPS y la orientación (los píxeles no se giran).
// Es lo que hace el iPhone al pasar fotos a un ordenador. NULL si ha ido bien.
static char *sytue_carrete_convertir(const char *origen, const char *destino) {
	@autoreleasepool {
		NSURL *de = [NSURL fileURLWithPath:[NSString stringWithUTF8String:origen]];
		NSURL *a = [NSURL fileURLWithPath:[NSString stringWithUTF8String:destino]];
		CGImageSourceRef src = CGImageSourceCreateWithURL((__bridge CFURLRef)de, NULL);
		if (!src) return strdup("no se puede abrir la foto");
		if (CGImageSourceGetCount(src) == 0) {
			CFRelease(src);
			return strdup("la foto no tiene imagen");
		}
		CGImageDestinationRef dst = CGImageDestinationCreateWithURL((__bridge CFURLRef)a, CFSTR("public.jpeg"), 1, NULL);
		if (!dst) {
			CFRelease(src);
			return strdup("no se puede escribir el JPEG");
		}
		NSDictionary *opciones = @{(__bridge NSString *)kCGImageDestinationLossyCompressionQuality: @0.92};
		CGImageDestinationAddImageFromSource(dst, src, 0, (__bridge CFDictionaryRef)opciones);
		bool ok = CGImageDestinationFinalize(dst);
		CFRelease(dst);
		CFRelease(src);
		return ok ? NULL : strdup("no se ha podido escribir el JPEG");
	}
}

// 1 si la red de ahora es «cara» (datos móviles, el punto de acceso de un iPhone) o
// está en «Modo de datos reducidos». Lo dice el primer aviso del monitor de rutas; sin
// él en dos segundos, se da por cara: no se gastan datos por no saber.
static int sytue_carrete_medida(void) {
	nw_path_monitor_t m = nw_path_monitor_create();
	dispatch_queue_t q = dispatch_queue_create("eu.sytue.carrete.red", DISPATCH_QUEUE_SERIAL);
	dispatch_semaphore_t listo = dispatch_semaphore_create(0);
	__block int medida = 1;
	nw_path_monitor_set_queue(m, q);
	nw_path_monitor_set_update_handler(m, ^(nw_path_t camino) {
		medida = (nw_path_is_expensive(camino) || nw_path_is_constrained(camino)) ? 1 : 0;
		dispatch_semaphore_signal(listo);
	});
	nw_path_monitor_start(m);
	long esperado = dispatch_semaphore_wait(listo, dispatch_time(DISPATCH_TIME_NOW, 2 * (int64_t)NSEC_PER_SEC));
	nw_path_monitor_cancel(m);
	return esperado == 0 ? medida : 1;
}

// Añade el fichero `ruta` a la fototeca con el nombre `nombre` y, si se puede, al álbum
// de Sytue. Devuelve el localIdentifier nuevo, o "" (y el motivo en *error).
static char *sytue_carrete_guardar(const char *ruta, const char *nombre, const char *propio, char **error) {
	@autoreleasepool {
		*error = NULL;
		if (sytue_carrete_estado() == 0 && sytue_carrete_pedir_anadir() == 3) {
			*error = strdup("sin permiso para añadir a Fotos");
			return strdup("");
		}
		NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:ruta]];
		NSString *n = [NSString stringWithUTF8String:nombre];
		__block NSString *nuevo = nil;
		NSError *e = nil;
		BOOL ok = [[PHPhotoLibrary sharedPhotoLibrary] performChangesAndWait:^{
			PHAssetCreationRequest *p = [PHAssetCreationRequest creationRequestForAsset];
			PHAssetResourceCreationOptions *o = [PHAssetResourceCreationOptions new];
			o.originalFilename = n;
			[p addResourceWithType:PHAssetResourceTypePhoto fileURL:url options:o];
			nuevo = p.placeholderForCreatedAsset.localIdentifier;
		} error:&e];
		if (!ok || !nuevo) {
			*error = sytue_carrete_copia(e.localizedDescription ?: @"Fotos no ha aceptado la imagen");
			return strdup("");
		}
		// El álbum, aparte: con acceso limitado o de solo añadir no se puede, y eso no
		// deshace lo guardado.
		if (sytue_carrete_estado() == 1) {
			NSString *titulo = [NSString stringWithUTF8String:propio];
			PHAssetCollection *album = sytue_carrete_album(propio);
			PHAsset *a = [PHAsset fetchAssetsWithLocalIdentifiers:@[ nuevo ] options:nil].firstObject;
			if (a) {
				[[PHPhotoLibrary sharedPhotoLibrary] performChangesAndWait:^{
					PHAssetCollectionChangeRequest *c = album
					    ? [PHAssetCollectionChangeRequest changeRequestForAssetCollection:album]
					    : [PHAssetCollectionChangeRequest creationRequestForAssetCollectionWithTitle:titulo];
					[c addAssets:@[ a ]];
				} error:nil];
			}
		}
		return sytue_carrete_copia(nuevo);
	}
}
*/
import "C"

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

// # iOS y macOS: PhotoKit
//
// La fototeca de la app Fotos, la misma API en los dos; ImageIO para pasar a JPEG y el
// monitor de rutas de Network para saber si la red es de pago. El texto con que se pide permiso
// (NSPhotoLibraryUsageDescription y NSPhotoLibraryAddUsageDescription) y, en macOS, el
// derecho `personal-information.photos-library` los pone `empaquetar.sh` SOLO en las
// apps con `fototeca = "si"`: sin la clave del Info.plist, el sistema mata la app al
// pedir el permiso.

// disponible: con la clave del Info.plist (sytue_carrete_declarado), que no cambia
// mientras corre el proceso.
func disponible() bool {
	declarado.Do(func() { conClave = C.sytue_carrete_declarado() != 0 })
	return conClave
}

var (
	declarado sync.Once
	conClave  bool
)

func estadoPermiso() Permiso { return Permiso(C.sytue_carrete_estado()) }

func pedirPermiso(ctx context.Context) (Permiso, error) {
	listo := make(chan Permiso, 1)
	go func() { listo <- Permiso(C.sytue_carrete_pedir()) }()
	select {
	case p := <-listo:
		return p, nil
	case <-ctx.Done():
		return SinPreguntar, ctx.Err()
	}
}

func cadenaPropia() *C.char { return C.CString(AlbumPropio) }

func listar(ctx context.Context) ([]Elemento, error) {
	propio := cadenaPropia()
	defer C.free(unsafe.Pointer(propio))
	s := C.sytue_carrete_listar(propio)
	defer C.free(unsafe.Pointer(s))
	return deLineas(ctx, C.GoString(s))
}

func abrir(_ context.Context, e Elemento) (string, func(), error) {
	// PhotoKit no escribe encima de un fichero que ya existe: una carpeta temporal propia
	// y el nombre original dentro, que conserva la extensión (HEIC, JPG, DNG).
	dir, err := os.MkdirTemp("", "sytue-carrete-*")
	if err != nil {
		return "", func() {}, err
	}
	limpiar := func() { _ = os.RemoveAll(dir) }
	ruta := filepath.Join(dir, limpiarNombre(e.Nombre))
	id, destino := C.CString(e.ID), C.CString(ruta)
	defer C.free(unsafe.Pointer(id))
	defer C.free(unsafe.Pointer(destino))
	if motivo := C.sytue_carrete_copiar(id, destino); motivo != nil {
		defer C.free(unsafe.Pointer(motivo))
		return "", limpiar, errors.New("carrete: " + C.GoString(motivo))
	}
	return ruta, limpiar, nil
}

func convertir(_ context.Context, ruta string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "sytue-convertida-*")
	if err != nil {
		return "", func() {}, err
	}
	limpiar := func() { _ = os.RemoveAll(dir) }
	destino := filepath.Join(dir, "foto.jpg")
	o, d := C.CString(ruta), C.CString(destino)
	defer C.free(unsafe.Pointer(o))
	defer C.free(unsafe.Pointer(d))
	if motivo := C.sytue_carrete_convertir(o, d); motivo != nil {
		defer C.free(unsafe.Pointer(motivo))
		return "", limpiar, errors.New("carrete: " + C.GoString(motivo))
	}
	return destino, limpiar, nil
}

func redMedida() bool { return C.sytue_carrete_medida() != 0 }

func guardar(_ context.Context, ruta, nombre string) (string, error) {
	r, n, propio := C.CString(ruta), C.CString(nombre), cadenaPropia()
	defer C.free(unsafe.Pointer(r))
	defer C.free(unsafe.Pointer(n))
	defer C.free(unsafe.Pointer(propio))
	var motivo *C.char
	id := C.sytue_carrete_guardar(r, n, propio, &motivo)
	defer C.free(unsafe.Pointer(id))
	if motivo != nil {
		defer C.free(unsafe.Pointer(motivo))
		return "", errors.New("carrete: " + C.GoString(motivo))
	}
	return C.GoString(id), nil
}
