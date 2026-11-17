// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

import (
	"log/slog"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// programarTareaEnFondo es TareaFondo.programar: encola (o quita) los dos trabajos de la
// tarea —uno por fotos y vídeos nuevos en la galería y uno periódico— con la red pedida.
func programarTareaEnFondo(producto string, activa, conDatos bool) {
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.TareaFondo")
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls, jni.GetStaticMethodID(env, cls, "programar", "(Landroid/content/Context;Ljava/lang/String;ZZ)V"), jni.Value(ctx), jni.Value(jni.JavaString(env, producto)), jni.Value(booleano(activa)), jni.Value(booleano(conDatos)))
	})
	if err != nil {
		slog.Info("sesion: la tarea en segundo plano no se ha podido programar", "error", err)
	}
}

func booleano(b bool) int {
	if b {
		return 1
	}
	return 0
}
