// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios || darwin

#import <Foundation/Foundation.h>
#import <TargetConditionals.h>
#if TARGET_OS_OSX
#import <Security/SecTask.h>
#endif
#include <stdlib.h>
#include <string.h>

char *sytue_contenedor_suite(const char *grupo, int *autorizado) {
	@autoreleasepool {
		NSString *nombre = [NSString stringWithUTF8String:grupo];
		*autorizado = 1;
#if TARGET_OS_OSX
		// No se toma una ruta supuesta ~/Library/Group Containers: sólo un grupo
		// declarado por la firma. Sin él (desarrollo, o los widgets apagados), la
		// configuración anterior de cada variante (baseSinGrupoApple).
		SecTaskRef tarea = SecTaskCreateFromSelf(NULL);
		CFTypeRef grupos = tarea ? SecTaskCopyValueForEntitlement(tarea,
			CFSTR("com.apple.security.application-groups"), NULL) : NULL;
		if (tarea) CFRelease(tarea);
		*autorizado = grupos && CFGetTypeID(grupos) == CFArrayGetTypeID() &&
			CFArrayContainsValue((CFArrayRef)grupos,
				CFRangeMake(0, CFArrayGetCount((CFArrayRef)grupos)), (__bridge CFStringRef)nombre);
		if (grupos) CFRelease(grupos);
		if (!*autorizado) return NULL;
#endif
		NSURL *ruta = [[NSFileManager defaultManager] containerURLForSecurityApplicationGroupIdentifier:nombre];
#if TARGET_OS_IPHONE
		// iOS no deja preguntar por los derechos propios: sin el grupo en la firma, la
		// carpeta es nil. Eso es «no autorizado», no un error (base_apple.go).
		if (!ruta) *autorizado = 0;
#endif
		return ruta ? strdup([ruta fileSystemRepresentation]) : NULL;
	}
}

#if TARGET_OS_IPHONE
// Aplicar al temporal vacío: un fallo nunca publica ni escribe datos sin clase A.
int sytue_fichero_proteger(const char *ruta) {
	@autoreleasepool {
		NSString *nombre = [NSString stringWithUTF8String:ruta];
		NSError *error = nil;
		BOOL protegido = [[NSFileManager defaultManager]
			setAttributes:@{NSFileProtectionKey: NSFileProtectionComplete}
			ofItemAtPath:nombre error:&error];
		return protegido ? 0 : (error ? (int)error.code : -1);
	}
}
#endif
