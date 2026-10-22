// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package widgets

import (
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

func init() {
	// El AppWidgetProvider va en cada APK (android-widgets.py) y lee su carpeta privada.
	lector = func() bool { return true }
	notifyPlatform = func(string) {
		// The receiver belongs to this APK. Other apps cannot supply snapshot paths
		// or destinations, and a widget update never starts the Go runtime.
		_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
			cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.Widgets")
			if err != nil {
				return err
			}
			return jni.CallStaticVoidMethod(env, cls,
				jni.GetStaticMethodID(env, cls, "actualizar", "(Landroid/content/Context;)V"), jni.Value(ctx))
		})
	}
}
