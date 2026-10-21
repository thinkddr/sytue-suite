// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sistema

import (
	"errors"

	"git.wow.st/gmp/jni"
)

// # La puerta a Java, sin toolkit
//
// El núcleo habla con Android por JNI —idioma, tema, el Keystore de la sesión— y para
// eso necesita la máquina de Java y un contexto de la app. Los tiene quien arranca la
// actividad, que es el toolkit: antes `gioui.org/app.JavaVM()` y `AppContext()`, que solo
// rellenaba la actividad de Gio y en una APK de Fyne valían cero. Ahora los pide
// `apps/fyne/sesion` con `driver.RunNative` y los deja aquí, así que `apps/nucleo` no
// importa ningún toolkit (ADR 0048).

// EnAndroid ejecuta f con la máquina de Java, el entorno JNI del hilo que llama —ya
// enganchado a la máquina— y el contexto de la app. Lo pone quien tiene el toolkit al
// arrancar, antes de leer la sesión; hasta entonces es nil.
var EnAndroid func(f func(vm, env, ctx uintptr) error) error

// ErrSinJava es que todavía nadie ha puesto [EnAndroid].
var ErrSinJava = errors.New("sistema: el toolkit no ha dado todavía la máquina de Java")

// Java es [EnAndroid] con los tipos de `jni`.
func Java(f func(env jni.Env, ctx jni.Object) error) error {
	if EnAndroid == nil {
		return ErrSinJava
	}
	return EnAndroid(func(_, env, ctx uintptr) error { return f(jni.EnvFor(env), jni.Object(ctx)) })
}

// CargarClase trae una clase con el cargador de la app: el de `FindClass` en un hilo
// que no es el de Java solo ve las del sistema, y el de la app ve las dos.
func CargarClase(env jni.Env, ctx jni.Object, nombre string) (jni.Class, error) {
	return jni.LoadClass(env, jni.ClassLoaderFor(env, ctx), nombre)
}
