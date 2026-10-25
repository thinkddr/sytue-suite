// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package ui

import (
	"log/slog"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

var avisoExclusionGestos atomic.Bool

// actualizarExclusionGestos sólo comunica disponibilidad. Java calcula la región
// en píxeles con los insets y la densidad actuales de la ventana nativa.
func actualizarExclusionGestos(c fyne.Canvas, disponible bool) {
	valor := 0
	if c != nil && disponible {
		valor = 1
	}
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.Gestos")
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "actualizar", "(Landroid/content/Context;Z)V"), jni.Value(ctx), jni.Value(valor))
	})
	if err != nil {
		if !avisoExclusionGestos.Swap(true) {
			slog.Warn("ui: no se pudo reservar el borde del menú; sigue disponible su botón", "error", err)
		}
		return
	}
	avisoExclusionGestos.Store(false)
}

// FocusLost llega antes de que Fyne cambie Canvas.Focused. Recalcular en el
// siguiente turno evita conservar el editor que acaba de perder el foco.
func sincronizarExclusionTrasFoco(c fyne.Canvas) {
	if c != nil {
		fyne.Do(func() { sincronizarExclusionMenuMovil(c) })
	}
}
