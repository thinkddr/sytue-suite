// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package escritorio

import (
	"errors"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// claseAvisos es la clase propia de la APK (apps/fyne/android/java).
const claseAvisos = "eu.sytue.nativo.Avisos"

func mantenerAvisos(titulo, texto string) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseAvisos)
		if err != nil {
			return err
		}
		ok, err := jni.CallStaticBooleanMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "empezar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)Z"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, titulo)), jni.Value(jni.JavaString(env, texto)))
		if err == nil && !ok {
			err = errors.New("escritorio: Android no deja arrancar el servicio de avisos con la app detrás")
		}
		return err
	})
}

func soltarAvisos() error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseAvisos)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "parar", "(Landroid/content/Context;)V"), jni.Value(ctx))
	})
}
