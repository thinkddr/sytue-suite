// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package escritorio

import (
	"context"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// claseBiometria es la clase propia de la APK (apps/fyne/android/java).
const claseBiometria = "eu.sytue.nativo.Biometria"

// biometria pregunta a BiometricManager. Con huella o cara, el nombre es el que da el
// teléfono en su idioma; si no lo da (Android 11), o si solo hay bloqueo del teléfono,
// se ofrece como «la contraseña del equipo».
func biometria() (string, bool) {
	var (
		hay    int
		nombre string
	)
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseBiometria)
		if err != nil {
			return err
		}
		hay, err = jni.CallStaticIntMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "disponible", "(Landroid/content/Context;)I"), jni.Value(ctx))
		if err != nil || hay != 1 {
			return err
		}
		o, err := jni.CallStaticObjectMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "nombre", "(Landroid/content/Context;)Ljava/lang/String;"), jni.Value(ctx))
		if err != nil || o == 0 {
			return err
		}
		nombre = jni.GoString(env, jni.String(o))
		return nil
	})
	switch {
	case err != nil || hay == 0:
		return "", false
	case hay == 1 && nombre != "":
		return nombre, false
	}
	return "", true
}

// verificar enseña BiometricPrompt y espera. El diálogo sigue en pantalla si `ctx` se
// cancela antes: Android no deja cerrarlo desde fuera sin su CancellationSignal, y
// quien cancela ya no espera la respuesta.
func verificar(ctx context.Context, motivo string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	listo := make(chan error, 1)
	go func() {
		codigo := -1
		err := sistema.Java(func(env jni.Env, actx jni.Object) error {
			cls, err := sistema.CargarClase(env, actx, claseBiometria)
			if err != nil {
				return err
			}
			if d, err := jni.CallStaticIntMethod(env, cls,
				jni.GetStaticMethodID(env, cls, "disponible", "(Landroid/content/Context;)I"), jni.Value(actx)); err != nil || d == 0 {
				return ErrNoDisponible
			}
			// Un fallo de Java aquí, con el diálogo ya pedido, NO es «no disponible»: el
			// bloqueo de la sesión dejaría pasar. Se queda en codigo -1: no verificado.
			// OJO: con una excepción la llamada devuelve 0, que es «verificado».
			if r, err := jni.CallStaticIntMethod(env, cls,
				jni.GetStaticMethodID(env, cls, "verificar", "(Landroid/content/Context;Ljava/lang/String;)I"),
				jni.Value(actx), jni.Value(jni.JavaString(env, motivo))); err == nil {
				codigo = r
			}
			return nil
		})
		listo <- respuestaBiometricPrompt(err, codigo)
	}()
	select {
	case err := <-listo:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
