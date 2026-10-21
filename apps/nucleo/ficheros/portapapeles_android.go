// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package ficheros

import (
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// copiarImagen copia la imagen al portapapeles de Android usando Captura.copiarImagen.
func copiarImagen(ruta string) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseCaptura)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "copiarImagen", "(Landroid/content/Context;Ljava/lang/String;)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, ruta)))
	})
}
