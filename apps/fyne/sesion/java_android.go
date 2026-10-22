// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/driver"
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// El núcleo habla con Android por JNI —el idioma, el tema, la sesión en el Keystore— y la
// máquina de Java la tiene Fyne. Se le da aquí, en la primera línea de cada app, antes
// de que `Abrir` lea la sesión. Cada llamada va por `driver.RunNative`, que engancha el
// hilo a la máquina y entrega el contexto vigente: guardar los punteros una vez los
// dejaría viejos en cuanto Android recree la actividad.
func init() {
	sistema.EnAndroid = func(f func(vm, env, ctx uintptr) error) error {
		return driver.RunNative(func(c any) error {
			ac, ok := c.(*driver.AndroidContext)
			if !ok {
				return sistema.ErrSinJava
			}
			return f(ac.VM, ac.Env, ac.Ctx)
		})
	}
}

// prepararCasa da a Go en Android las carpetas que en un escritorio salen de $HOME: sin
// ella `os.UserCacheDir` y `os.UserConfigDir` fallan («neither $XDG_CACHE_HOME nor $HOME
// are defined», visto en el emulador), y con ellos la caché de trabajar sin conexión, el
// tema y el idioma elegidos, que no se guardaban. Son las carpetas privadas de la app
// (`getFilesDir`, `getCacheDir`), a las que no llega ninguna otra.
func prepararCasa() {
	if os.Getenv("HOME") != "" {
		return
	}
	var ficheros, cache string
	_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		ruta := func(metodo string) string {
			f, err := jni.CallObjectMethod(env, ctx,
				jni.GetMethodID(env, jni.GetObjectClass(env, ctx), metodo, "()Ljava/io/File;"))
			if err != nil || f == 0 {
				return ""
			}
			s, err := jni.CallObjectMethod(env, f,
				jni.GetMethodID(env, jni.GetObjectClass(env, f), "getAbsolutePath", "()Ljava/lang/String;"))
			if err != nil {
				return ""
			}
			return jni.GoString(env, jni.String(s))
		}
		ficheros, cache = ruta("getFilesDir"), ruta("getCacheDir")
		return nil
	})
	if ficheros == "" {
		return
	}
	_ = os.Setenv("HOME", ficheros)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(ficheros, ".config"))
	if cache != "" {
		_ = os.Setenv("XDG_CACHE_HOME", cache)
	}
}
