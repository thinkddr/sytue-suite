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
 // An omitted agrp is a wildcard for reads, updates and deletes. Private
 // callers must supply the actual app group; never broaden a failed lookup.
 if (grupo == NULL || grupo[0] == '\0') return NULL;
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
 if (q == NULL) return (int)errSecParam;
	OSStatus st = SecItemDelete(q);
	CFRelease(q);
	return (st == errSecSuccess || st == errSecItemNotFound) ? 0 : (int)st;
}

int sytue_llavero_guardar(const char *cuenta, const char *grupo, const void *datos, int n) {
	CFMutableDictionaryRef q = sytue_consulta(cuenta, grupo);
 if (q == NULL) return (int)errSecParam;
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
 if (q == NULL) return (int)errSecParam;
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

// Public CoreFoundation metadata works on iOS as well as macOS. SecTask is not
// used to discover private groups on iOS. The signing pipeline declares the same
// TeamID.bundle group; Security.framework enforces the entitlement at each IO.
char *sytue_llavero_bundle(void) {
 CFBundleRef bundle = CFBundleGetMainBundle();
 CFStringRef identifier = bundle ? CFBundleGetIdentifier(bundle) : NULL;
 if (identifier == NULL || CFStringGetLength(identifier) > 255) return NULL;
 CFIndex size = CFStringGetMaximumSizeForEncoding(CFStringGetLength(identifier), kCFStringEncodingUTF8) + 1;
 char *value = malloc((size_t)size);
 if (value == NULL) return NULL;
 if (!CFStringGetCString(identifier, value, size, kCFStringEncodingUTF8)) { free(value); return NULL; }
 return value;
}

int sytue_llavero_existe(const char *cuenta, const char *grupo, int *existe) {
 *existe = 0;
 CFMutableDictionaryRef q = sytue_consulta(cuenta, grupo);
 if (q == NULL) return (int)errSecParam;
 CFDictionarySetValue(q, kSecReturnAttributes, kCFBooleanTrue);
 CFTypeRef attributes = NULL;
 OSStatus status = SecItemCopyMatching(q, &attributes);
 CFRelease(q);
 if (attributes != NULL) CFRelease(attributes);
 if (status == errSecItemNotFound) return 0;
 if (status != errSecSuccess) return (int)status;
 *existe = 1;
 return 0;
}

// A transfer changes only agrp, atomically. Duplicate destinations or a locked
// Keychain fail without deleting or replacing either item.
int sytue_llavero_mover(const char *cuenta, const char *origen, const char *destino) {
 if (destino == NULL || destino[0] == '\0') return (int)errSecParam;
 CFMutableDictionaryRef q = sytue_consulta(cuenta, origen);
 if (q == NULL) return (int)errSecParam;
 CFMutableDictionaryRef change = CFDictionaryCreateMutable(NULL, 0,
   &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
 CFStringRef group = CFStringCreateWithCString(NULL, destino, kCFStringEncodingUTF8);
 CFDictionarySetValue(change, kSecAttrAccessGroup, group);
 CFRelease(group);
 OSStatus status = SecItemUpdate(q, change);
 CFRelease(change);
 CFRelease(q);
 return (int)status;
}

char *sytue_llavero_esquema(void) {
 CFBundleRef bundle = CFBundleGetMainBundle();
 CFTypeRef types = bundle ? CFBundleGetValueForInfoDictionaryKey(bundle, CFSTR("CFBundleURLTypes")) : NULL;
 if (types == NULL || CFGetTypeID(types) != CFArrayGetTypeID()) return NULL;
 char *selected = NULL;
 for (CFIndex i = 0; i < CFArrayGetCount((CFArrayRef)types); i++) {
  CFTypeRef type = CFArrayGetValueAtIndex((CFArrayRef)types, i);
  if (CFGetTypeID(type) != CFDictionaryGetTypeID()) continue;
  CFTypeRef schemes = CFDictionaryGetValue((CFDictionaryRef)type, CFSTR("CFBundleURLSchemes"));
  if (schemes == NULL || CFGetTypeID(schemes) != CFArrayGetTypeID()) continue;
  for (CFIndex j = 0; j < CFArrayGetCount((CFArrayRef)schemes); j++) {
   CFTypeRef scheme = CFArrayGetValueAtIndex((CFArrayRef)schemes, j);
   if (CFGetTypeID(scheme) != CFStringGetTypeID() || !CFStringHasPrefix((CFStringRef)scheme, CFSTR("sytue-"))) continue;
   char value[128];
   if (!CFStringGetCString((CFStringRef)scheme, value, sizeof(value), kCFStringEncodingUTF8) || selected != NULL) {
    free(selected); return NULL;
   }
   selected = strdup(value);
  }
 }
 return selected;
}
