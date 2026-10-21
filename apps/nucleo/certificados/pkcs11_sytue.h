/* SPDX-License-Identifier: Apache-2.0
 * Copyright (c) 2026 Daniel Delgado Ramos
 *
 * El lado C del almacén PKCS#11 (pkcs11.go). Vive en su cabecera y no en el preámbulo
 * del .go para que los linters de Go no lo lean como comentario.
 */
#ifndef SYTUE_PKCS11_H
#define SYTUE_PKCS11_H
#include <stdlib.h>
#include <string.h>

// PKCS#11 v2.40 (OASIS), declarado aquí con lo justo en vez de traer una biblioteca: son
// una docena de llamadas por una tabla de punteros, y el orden de esa tabla es el del
// estándar. En Windows las estructuras de Cryptoki van empaquetadas a 1 byte y
// CK_ULONG es de 32 bits (LLP64): así lo compilan los módulos de allí.
#ifdef _WIN32
#include <windows.h>
#pragma pack(push, cryptoki, 1)
#else
#include <dlfcn.h>
#endif

typedef unsigned long CK_ULONG;
typedef unsigned char CK_BYTE;
typedef CK_ULONG CK_RV;

typedef struct { CK_BYTE major, minor; } CK_VERSION;
typedef struct { CK_ULONG type; void *pValue; CK_ULONG ulValueLen; } CK_ATTRIBUTE;
typedef struct { CK_ULONG mechanism; void *pParameter; CK_ULONG ulParameterLen; } CK_MECHANISM;
typedef struct {
	CK_BYTE label[32], manufacturerID[32], model[16], serialNumber[16];
	CK_ULONG flags, ulMaxSessionCount, ulSessionCount, ulMaxRwSessionCount, ulRwSessionCount;
	CK_ULONG ulMaxPinLen, ulMinPinLen, ulTotalPublicMemory, ulFreePublicMemory;
	CK_ULONG ulTotalPrivateMemory, ulFreePrivateMemory;
	CK_VERSION hardwareVersion, firmwareVersion;
	CK_BYTE utcTime[16];
} CK_TOKEN_INFO;
typedef struct {
	void *CreateMutex, *DestroyMutex, *LockMutex, *UnlockMutex;
	CK_ULONG flags;
	void *pReserved;
} CK_C_INITIALIZE_ARGS;

// La tabla de funciones hasta C_Sign, que es la última que se usa: las 24 que vienen
// detrás no se leen, y no hace falta declararlas.
typedef struct {
	CK_VERSION version;
	CK_RV (*C_Initialize)(void *);
	CK_RV (*C_Finalize)(void *);
	void *C_GetInfo, *C_GetFunctionList;
	CK_RV (*C_GetSlotList)(CK_BYTE, CK_ULONG *, CK_ULONG *);
	void *C_GetSlotInfo;
	CK_RV (*C_GetTokenInfo)(CK_ULONG, CK_TOKEN_INFO *);
	void *C_GetMechanismList, *C_GetMechanismInfo, *C_InitToken, *C_InitPIN, *C_SetPIN;
	CK_RV (*C_OpenSession)(CK_ULONG, CK_ULONG, void *, void *, CK_ULONG *);
	CK_RV (*C_CloseSession)(CK_ULONG);
	void *C_CloseAllSessions, *C_GetSessionInfo, *C_GetOperationState, *C_SetOperationState;
	CK_RV (*C_Login)(CK_ULONG, CK_ULONG, CK_BYTE *, CK_ULONG);
	CK_RV (*C_Logout)(CK_ULONG);
	void *C_CreateObject, *C_CopyObject, *C_DestroyObject, *C_GetObjectSize;
	CK_RV (*C_GetAttributeValue)(CK_ULONG, CK_ULONG, CK_ATTRIBUTE *, CK_ULONG);
	void *C_SetAttributeValue;
	CK_RV (*C_FindObjectsInit)(CK_ULONG, CK_ATTRIBUTE *, CK_ULONG);
	CK_RV (*C_FindObjects)(CK_ULONG, CK_ULONG *, CK_ULONG, CK_ULONG *);
	CK_RV (*C_FindObjectsFinal)(CK_ULONG);
	void *C_EncryptInit, *C_Encrypt, *C_EncryptUpdate, *C_EncryptFinal;
	void *C_DecryptInit, *C_Decrypt, *C_DecryptUpdate, *C_DecryptFinal;
	void *C_DigestInit, *C_Digest, *C_DigestUpdate, *C_DigestKey, *C_DigestFinal;
	CK_RV (*C_SignInit)(CK_ULONG, CK_MECHANISM *, CK_ULONG);
	CK_RV (*C_Sign)(CK_ULONG, CK_BYTE *, CK_ULONG, CK_BYTE *, CK_ULONG *);
} sytue_p11;

#ifdef _WIN32
#pragma pack(pop, cryptoki)
#endif

// sytue_p11_abrir carga el módulo y devuelve su tabla, o NULL.
static sytue_p11 *sytue_p11_abrir(const char *ruta) {
	CK_RV (*lista)(sytue_p11 **) = NULL;
#ifdef _WIN32
	HMODULE h = LoadLibraryA(ruta);
	if (!h) return NULL;
	lista = (CK_RV (*)(sytue_p11 **))GetProcAddress(h, "C_GetFunctionList");
#else
	void *h = dlopen(ruta, RTLD_NOW | RTLD_LOCAL);
	if (!h) return NULL;
	lista = (CK_RV (*)(sytue_p11 **))dlsym(h, "C_GetFunctionList");
#endif
	sytue_p11 *f = NULL;
	if (!lista || lista(&f) != 0) return NULL;
	return f;
}

static CK_RV sytue_p11_iniciar(sytue_p11 *f) {
	CK_C_INITIALIZE_ARGS a;
	memset(&a, 0, sizeof a);
	a.flags = 0x2; // CKF_OS_LOCKING_OK
	return f->C_Initialize(&a);
}
static CK_RV sytue_p11_acabar(sytue_p11 *f) { return f->C_Finalize(NULL); }
static CK_RV sytue_p11_ranuras(sytue_p11 *f, CK_ULONG *r, CK_ULONG *n) { return f->C_GetSlotList(1, r, n); }
static CK_RV sytue_p11_ficha(sytue_p11 *f, CK_ULONG s, CK_TOKEN_INFO *i) { return f->C_GetTokenInfo(s, i); }
static CK_ULONG sytue_p11_banderas(CK_TOKEN_INFO *i) { return i->flags; }
static CK_RV sytue_p11_sesion(sytue_p11 *f, CK_ULONG s, CK_ULONG *h) { return f->C_OpenSession(s, 0x4, NULL, (void *)0, h); }
static CK_RV sytue_p11_cerrar(sytue_p11 *f, CK_ULONG h) { return f->C_CloseSession(h); }
static CK_RV sytue_p11_entrar(sytue_p11 *f, CK_ULONG h, void *pin, CK_ULONG n) { return f->C_Login(h, 1, (CK_BYTE *)pin, n); }
static char *sytue_p11_etiqueta(CK_TOKEN_INFO *i) { return (char *)i->label; }
static void sytue_p11_soltar_cadena(char *c) { free(c); }
static CK_RV sytue_p11_entrar_c(sytue_p11 *f, CK_ULONG h, char *pin, CK_ULONG n) { return f->C_Login(h, 1, (CK_BYTE *)pin, n); }
// sytue_p11_borrar_cadena borra el PIN y lo suelta.
static void sytue_p11_borrar_cadena(char *p, size_t n) { memset(p, 0, n); free(p); }
static CK_RV sytue_p11_salir(sytue_p11 *f, CK_ULONG h) { return f->C_Logout(h); }

// sytue_p11_buscar deja en `o` hasta `max` objetos de la clase `clase` (y con ese
// CKA_ID, si `id` no es NULL).
static CK_RV sytue_p11_buscar(sytue_p11 *f, CK_ULONG h, CK_ULONG clase, void *id, CK_ULONG nid,
		CK_ULONG *o, CK_ULONG max, CK_ULONG *n) {
	CK_ATTRIBUTE t[2];
	t[0].type = 0x0; t[0].pValue = &clase; t[0].ulValueLen = sizeof clase; // CKA_CLASS
	t[1].type = 0x102; t[1].pValue = id; t[1].ulValueLen = nid;           // CKA_ID
	CK_RV rv = f->C_FindObjectsInit(h, t, id ? 2 : 1);
	if (rv) return rv;
	rv = f->C_FindObjects(h, o, max, n);
	f->C_FindObjectsFinal(h);
	return rv;
}

// sytue_p11_atributo lee un atributo en dos vueltas: su tamaño y luego su valor. El
// búfer lo suelta quien llama con free.
static CK_RV sytue_p11_atributo(sytue_p11 *f, CK_ULONG h, CK_ULONG o, CK_ULONG tipo, void **v, CK_ULONG *n) {
	CK_ATTRIBUTE a;
	a.type = tipo; a.pValue = NULL; a.ulValueLen = 0;
	CK_RV rv = f->C_GetAttributeValue(h, o, &a, 1);
	if (rv) return rv;
	if (a.ulValueLen == (CK_ULONG)-1) return 0x12; // CKR_ATTRIBUTE_TYPE_INVALID
	a.pValue = malloc(a.ulValueLen ? a.ulValueLen : 1);
	rv = f->C_GetAttributeValue(h, o, &a, 1);
	if (rv) { free(a.pValue); return rv; }
	*v = a.pValue; *n = a.ulValueLen;
	return 0;
}

// sytue_p11_firmar firma `d` con la clave `k` y el mecanismo `m`: primero pregunta el
// tamaño y luego firma, como pide el estándar.
static CK_RV sytue_p11_firmar(sytue_p11 *f, CK_ULONG h, CK_ULONG k, CK_ULONG m,
		void *d, CK_ULONG nd, void *s, CK_ULONG *ns) {
	CK_MECHANISM mec;
	mec.mechanism = m; mec.pParameter = NULL; mec.ulParameterLen = 0;
	CK_RV rv = f->C_SignInit(h, &mec, k);
	if (rv) return rv;
	return f->C_Sign(h, (CK_BYTE *)d, nd, (CK_BYTE *)s, ns);
}

static void sytue_p11_soltar(void *p) { free(p); }
// sytue_p11_nulo es un puntero de C vacío: con él el Go no nombra `unsafe`.
static void *sytue_p11_nulo(void) { return NULL; }

#endif
