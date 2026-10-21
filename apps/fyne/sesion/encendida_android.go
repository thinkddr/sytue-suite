// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

import (
	"fyne.io/fyne/v2"
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// PantallaEncendida pide que la pantalla no se apague (o deja que se apague). En Android
// va por eu.sytue.nativo.Encendida, que lo hace en el hilo de la interfaz: el de Fyne no
// llega a la ventana (ver la clase).
func PantallaEncendida(_ fyne.App, si bool) {
	_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.Encendida")
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "poner", "(Landroid/content/Context;Z)V"), jni.Value(ctx), jni.Value(boolAJava(si)))
	})
}

func boolAJava(si bool) int {
	if si {
		return 1
	}
	return 0
}
