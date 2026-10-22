// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

import (
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// AltoTeclado son los píxeles que tapa ahora el teclado en pantalla desde abajo, o 0 (por
// eu.sytue.nativo.Teclado: Fyne no lo sabe).
func AltoTeclado() float32 {
	var px int
	_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.Teclado")
		if err != nil {
			return err
		}
		px, err = jni.CallStaticIntMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "alto", "(Landroid/content/Context;)I"), jni.Value(ctx))
		return err
	})
	return float32(px)
}
