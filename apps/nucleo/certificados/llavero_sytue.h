/* SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * El lado C del Llavero (llavero_darwin.go): Security.framework en macOS e iOS.
 */
#ifndef SYTUE_LLAVERO_H
#define SYTUE_LLAVERO_H
#include <stdlib.h>
#include <string.h>
#include <TargetConditionals.h>
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <CommonCrypto/CommonDigest.h>

static void sytue_soltar_cadena(char *c) { free(c); }
static const void *sytue_datos(CFDataRef d) { return CFDataGetBytePtr(d); }
static CFDataRef sytue_elemento(CFArrayRef a, CFIndex i) { return (CFDataRef)CFArrayGetValueAtIndex(a, i); }

// sytue_consulta es la de SecItemCopyMatching para `clase`, en el llavero de `ruta` si
// viene (solo macOS: las pruebas usan uno temporal) o en los de la persona.
static CFMutableDictionaryRef sytue_consulta(const char *ruta, CFTypeRef clase) {
	CFMutableDictionaryRef q = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(q, kSecClass, clase);
	CFDictionarySetValue(q, kSecMatchLimit, kSecMatchLimitAll);
	CFDictionarySetValue(q, kSecReturnRef, kCFBooleanTrue);
#if TARGET_OS_OSX
	if (ruta) {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		SecKeychainRef kc = NULL;
		if (SecKeychainOpen(ruta, &kc) == errSecSuccess) {
			CFArrayRef lista = CFArrayCreate(NULL, (const void **)&kc, 1, &kCFTypeArrayCallBacks);
			CFDictionarySetValue(q, kSecMatchSearchList, lista);
			CFRelease(lista);
			CFRelease(kc);
		}
#pragma clang diagnostic pop
	}
#endif
	return q;
}

// sytue_der es el DER de un certificado o de la identidad que lo lleva.
static CFDataRef sytue_der(CFTypeRef x, int identidad) {
	SecCertificateRef c = NULL;
	if (identidad) {
		if (SecIdentityCopyCertificate((SecIdentityRef)x, &c) != errSecSuccess) return NULL;
	} else {
		c = (SecCertificateRef)CFRetain(x);
	}
	CFDataRef d = SecCertificateCopyData(c);
	CFRelease(c);
	return d;
}

// sytue_llavero_lista son los DER de las identidades (1) o de los certificados (0), o
// NULL si no hay ninguno.
static CFArrayRef sytue_llavero_lista(const char *ruta, int identidades) {
	CFMutableDictionaryRef q = sytue_consulta(ruta, identidades ? kSecClassIdentity : kSecClassCertificate);
	CFTypeRef res = NULL;
	OSStatus st = SecItemCopyMatching(q, &res);
	CFRelease(q);
	if (st != errSecSuccess || !res) return NULL;
	CFMutableArrayRef out = CFArrayCreateMutable(NULL, 0, &kCFTypeArrayCallBacks);
	CFArrayRef todos = (CFArrayRef)res;
	for (CFIndex i = 0; i < CFArrayGetCount(todos); i++) {
		CFDataRef d = sytue_der(CFArrayGetValueAtIndex(todos, i), identidades);
		if (d) {
			CFArrayAppendValue(out, d);
			CFRelease(d);
		}
	}
	CFRelease(res);
	return out;
}

// sytue_llavero_firmar busca la identidad cuyo certificado tiene esa huella SHA-256 y
// le pide la firma de `resumen`. 0 = firmada; 1 = no está; 2 = el sistema dijo que no
// (en `estado`).
static int sytue_llavero_firmar(const char *ruta, const void *huella, const void *resumen, long n,
		int ecdsa, CFDataRef *firma, OSStatus *estado) {
	CFMutableDictionaryRef q = sytue_consulta(ruta, kSecClassIdentity);
	CFTypeRef res = NULL;
	*estado = SecItemCopyMatching(q, &res);
	CFRelease(q);
	if (*estado != errSecSuccess || !res) return 1;
	CFArrayRef todas = (CFArrayRef)res;
	SecIdentityRef elegida = NULL;
	for (CFIndex i = 0; i < CFArrayGetCount(todas) && !elegida; i++) {
		SecIdentityRef id = (SecIdentityRef)CFArrayGetValueAtIndex(todas, i);
		CFDataRef d = sytue_der(id, 1);
		if (!d) continue;
		unsigned char h[CC_SHA256_DIGEST_LENGTH];
		CC_SHA256(CFDataGetBytePtr(d), (CC_LONG)CFDataGetLength(d), h);
		CFRelease(d);
		if (memcmp(h, huella, sizeof h) == 0) elegida = (SecIdentityRef)CFRetain(id);
	}
	CFRelease(res);
	if (!elegida) return 1;
	SecKeyRef clave = NULL;
	*estado = SecIdentityCopyPrivateKey(elegida, &clave);
	CFRelease(elegida);
	if (*estado != errSecSuccess || !clave) return 2;
	SecKeyAlgorithm alg = ecdsa ? kSecKeyAlgorithmECDSASignatureDigestX962SHA256
	                            : kSecKeyAlgorithmRSASignatureDigestPKCS1v15SHA256;
	CFDataRef datos = CFDataCreate(NULL, resumen, n);
	CFErrorRef err = NULL;
	*firma = SecKeyCreateSignature(clave, alg, datos, &err);
	CFRelease(datos);
	CFRelease(clave);
	if (!*firma) {
		*estado = err ? (OSStatus)CFErrorGetCode(err) : -1;
		if (err) CFRelease(err);
		return 2;
	}
	return 0;
}
#endif
