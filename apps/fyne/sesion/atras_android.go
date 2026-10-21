// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

/*
#include <jni.h>
*/
import "C"

import (
	"log/slog"

	"fyne.io/fyne/v2"
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// registrarAtras pone el receptor de «atrás» de Android 13+ (eu.sytue.nativo.Atras): con
// target 36, Android 16 ya no manda la tecla y sin esto «atrás» cerraba la app.
func registrarAtras() {
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.Atras")
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "registrar", "(Landroid/content/Context;)V"), jni.Value(ctx))
	})
	if err != nil {
		slog.Warn("sesion: «atrás» no se ha podido enganchar; cerrará la app", "error", err)
	}
}

// Java_eu_sytue_nativo_Atras_pulsado es `Atras.pulsado()`: llega en el hilo de la interfaz
// de Android, y se lleva al de Fyne.
//
//export Java_eu_sytue_nativo_Atras_pulsado
func Java_eu_sytue_nativo_Atras_pulsado(env *C.JNIEnv, cls C.jclass) { //nolint:revive // el nombre lo pone JNI
	_, _ = env, cls // los pide la firma de JNI; cgo no admite parámetros sin nombre
	fyne.Do(atrasDelSistema)
}

// atrasDelSistema lleva «atrás» a la única ventana de la app (atrasEn).
func atrasDelSistema() {
	a := fyne.CurrentApp()
	if a == nil {
		return
	}
	atrasEnVentanaActual(a)
}
