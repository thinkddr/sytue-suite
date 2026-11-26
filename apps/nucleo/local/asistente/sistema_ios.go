// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build ios && cgo

package asistente

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>

typedef int32_t (*sytue_ia_estado_t)(void);
typedef char *(*sytue_ia_responder_t)(const char *, const char *, int64_t, int32_t *);
typedef void (*sytue_ia_cancelar_t)(int64_t);

static void *sytue_ia_lib = NULL;

// El framework va en <App>.app/Frameworks (empaquetar.sh lo mete solo en Sytue Office).
// Si no está, NULL y Go dice que esta compilación no lleva el puente.
static void *sytue_ia_cargar(void) {
	if (sytue_ia_lib) return sytue_ia_lib;
	@autoreleasepool {
		NSString *dir = [[NSBundle mainBundle] privateFrameworksPath];
		if (!dir) return NULL;
		NSString *ruta = [dir stringByAppendingPathComponent:@"SytueIA.framework/SytueIA"];
		sytue_ia_lib = dlopen([ruta fileSystemRepresentation], RTLD_NOW | RTLD_LOCAL);
	}
	return sytue_ia_lib;
}

static int32_t sytue_ia_estado_c(void) {
	void *l = sytue_ia_cargar();
	if (!l) return -1;
	sytue_ia_estado_t f = (sytue_ia_estado_t)dlsym(l, "sytue_ia_estado");
	return f ? f() : -1;
}

static char *sytue_ia_responder_c(const char *instr, const char *texto, int64_t id, int32_t *fallo) {
	void *l = sytue_ia_cargar();
	sytue_ia_responder_t f = l ? (sytue_ia_responder_t)dlsym(l, "sytue_ia_responder") : NULL;
	if (!f) { *fallo = 3; return NULL; }
	return f(instr, texto, id, fallo);
}

static void sytue_ia_cancelar_c(int64_t id) {
	void *l = sytue_ia_cargar();
	sytue_ia_cancelar_t f = l ? (sytue_ia_cancelar_t)dlsym(l, "sytue_ia_cancelar") : NULL;
	if (f) f(id);
}
*/
import "C"

import (
	"context"
	"errors"
	"unsafe"
)

// En iOS y iPadOS 26 la IA del sistema es Foundation Models, el modelo de Apple
// Intelligence en el dispositivo. Solo se llama desde Swift: el puente es SytueIA.framework
// (apps/fyne/xcode/ia-sistema), que se abre con dlopen para que una app sin él —o un iOS
// anterior a 26— siga arrancando y Ajustes diga por qué no hay IA del sistema.

// Los estados de sytue_ia_estado (SytueIA.swift); -1 es que no hay framework.
var estadosIOS = map[C.int32_t]EstadoSistema{
	-1: SistemaSinPuente,
	0:  SistemaLista,
	1:  SistemaNoSoportada,
	2:  SistemaApagada,
	3:  SistemaDescargando,
	4:  SistemaFallo,
	5:  SistemaAntiguo,
}

var puente = &puenteNativo{
	nombre:    "Apple Intelligence (en el dispositivo)",
	consultar: consultarIOS,
	responder: responderIOS,
	cancelar:  func(id int64) { C.sytue_ia_cancelar_c(C.int64_t(id)) },
}

// NombreSistema es el nombre de la IA del sistema en español: el de los registros y el
// del destino (Asistente.Destino). Ajustes la nombra en su idioma con EstadoDelSistema.
func NombreSistema() string { return puente.nombre }

// EstadoDelSistema es lo último que se sabe del modelo, sin esperar: Ajustes pinta en el
// hilo de la interfaz y dice con él por qué no hay IA del sistema. Si es viejo, se
// vuelve a preguntar por detrás.
func EstadoDelSistema() EstadoSistema { return puente.estadoSinEsperar() }

// SistemaDisponible dice si Foundation Models está listo en este dispositivo.
func SistemaDisponible(context.Context) bool { return puente.estadoAhora() == SistemaLista }

func completarSistema(ctx context.Context, _ Config, system, user string) (string, error) {
	return puente.completar(ctx, system, user)
}

func consultarIOS() EstadoSistema {
	if e, ok := estadosIOS[C.sytue_ia_estado_c()]; ok {
		return e
	}
	return SistemaFallo
}

func responderIOS(id int64, system, user string) (string, error) {
	ci, ct := C.CString(system), C.CString(user)
	defer C.free(unsafe.Pointer(ci))
	defer C.free(unsafe.Pointer(ct))
	var fallo C.int32_t
	r := C.sytue_ia_responder_c(ci, ct, C.int64_t(id), &fallo)
	texto := ""
	if r != nil {
		texto = C.GoString(r)
		C.free(unsafe.Pointer(r))
	}
	switch fallo {
	case 0:
		return texto, nil
	case 2:
		return "", context.Canceled
	case 3:
		return "", ErrSinIANativa
	}
	return "", errors.New(texto)
}
