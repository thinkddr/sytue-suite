// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// El Keychain de iOS —y el de la variante de la Mac App Store— por SecItem. Ver
// store_apple.go: compilado con el SDK de iOS 27 y con el de macOS, sin ejecutar en un
// iPhone ni en un Mac. El nombre del fichero no lleva `_ios` a propósito: ese sufijo lo
// ataría a GOOS=ios.

//go:build ios || darwin

#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <TargetConditionals.h>
#if TARGET_OS_OSX
#include <Security/SecTask.h>
#endif
#include <stdlib.h>
#include <string.h>

// La entrada es una «generic password» de eu.sytue.one. El servicio y la cuenta la
// identifican, igual que en el llavero del escritorio.
static CFMutableDictionaryRef sytue_consulta(const char *cuenta, const char *grupo) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(
		NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(q, kSecClass, kSecClassGenericPassword);
	CFStringRef servicio = CFStringCreateWithCString(NULL, "eu.sytue.one", kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecAttrService, servicio);
	CFRelease(servicio);
	CFStringRef c = CFStringCreateWithCString(NULL, cuenta, kCFStringEncodingUTF8);
	CFDictionarySetValue(q, kSecAttrAccount, c);
	CFRelease(c);
	if (grupo != NULL && grupo[0] != '\0') {
		CFStringRef g = CFStringCreateWithCString(NULL, grupo, kCFStringEncodingUTF8);
		CFDictionarySetValue(q, kSecAttrAccessGroup, g);
		CFRelease(g);
	}
#if TARGET_OS_OSX
	// macOS (variante de la Mac App Store): el llavero de protección de datos, el mismo
	// de iOS, que es el que entiende de grupos de acceso; sin esto SecItem va al llavero
	// de ficheros, donde cada app firmada tendría su propia lista de acceso.
	CFDictionarySetValue(q, kSecUseDataProtectionKeychain, kCFBooleanTrue);
#endif
	return q;
}

int sytue_llavero_borrar(const char *cuenta, const char *grupo) {
	CFMutableDictionaryRef q = sytue_consulta(cuenta, grupo);
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return (st == errSecSuccess || st == errSecItemNotFound) ? 0 : (int)st;
}

int sytue_llavero_guardar(const char *cuenta, const char *grupo, const void *datos, int n) {
	CFMutableDictionaryRef q = sytue_consulta(cuenta, grupo);
	CFMutableDictionaryRef cambios = CFDictionaryCreateMutable(
		NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDataRef d = CFDataCreate(NULL, (const UInt8 *)datos, n);
	CFDictionarySetValue(cambios, kSecValueData, d);
	CFDictionarySetValue(cambios, kSecAttrAccessible, kSecAttrAccessibleWhenUnlockedThisDeviceOnly);
	// Actualizar conserva la entrada anterior si el sistema rechaza el guardado.
	// Borrar y añadir dejaba un hueco sin refresh y perdía la sesión ante un fallo.
	OSStatus st = SecItemUpdate(q, cambios);
	if (st == errSecItemNotFound) {
		CFDictionarySetValue(q, kSecValueData, d);
		CFDictionarySetValue(q, kSecAttrAccessible, kSecAttrAccessibleWhenUnlockedThisDeviceOnly);
		st = SecItemAdd(q, NULL);
		if (st == errSecDuplicateItem) {
			CFDictionaryRemoveValue(q, kSecValueData);
			CFDictionaryRemoveValue(q, kSecAttrAccessible);
			st = SecItemUpdate(q, cambios);
		}
	}
	CFRelease(d);
	CFRelease(cambios);
	CFRelease(q);
	return (st == errSecSuccess) ? 0 : (int)st;
}

// Sin entrada guardada devuelve 0 con *datos a NULL. Quien llama libera *datos.
int sytue_llavero_leer(const char *cuenta, const char *grupo, void **datos, int *n) {
	*datos = NULL;
	*n = 0;
	CFMutableDictionaryRef q = sytue_consulta(cuenta, grupo);
	CFDictionarySetValue(q, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef salida = NULL;
	OSStatus st = SecItemCopyMatching(q, &salida);
	CFRelease(q);
	if (st == errSecItemNotFound) {
		return 0;
	}
	if (st != errSecSuccess || salida == NULL) {
		return (int)st;
	}
	CFDataRef d = (CFDataRef)salida;
	*n = (int)CFDataGetLength(d);
	*datos = malloc((size_t)*n);
	if (*datos != NULL) {
		memcpy(*datos, CFDataGetBytePtr(d), (size_t)*n);
	}
	CFRelease(salida);
	return (*datos != NULL || *n == 0) ? 0 : (int)errSecAllocate;
}

#if TARGET_OS_OSX
// Sólo los binarios firmados con el grupo comparten el llavero de protección de
// datos. Un binario de desarrollo sin ese derecho conserva su llavero de escritorio.
int sytue_llavero_autorizado(const char *grupo) {
	SecTaskRef tarea = SecTaskCreateFromSelf(NULL);
	if (tarea == NULL) return 0;
	CFTypeRef grupos = SecTaskCopyValueForEntitlement(tarea, CFSTR("keychain-access-groups"), NULL);
	CFRelease(tarea);
	if (grupos == NULL) return 0;
	CFStringRef esperado = CFStringCreateWithCString(NULL, grupo, kCFStringEncodingUTF8);
	int autorizado = 0;
	if (CFGetTypeID(grupos) == CFArrayGetTypeID()) {
		autorizado = CFArrayContainsValue((CFArrayRef)grupos,
			CFRangeMake(0, CFArrayGetCount((CFArrayRef)grupos)), esperado);
	}
	CFRelease(esperado);
	CFRelease(grupos);
	return autorizado;
}
#endif
