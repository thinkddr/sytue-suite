// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sistema

import "git.wow.st/gmp/jni"

// Modo noche de `android.content.res.Configuration`. Son los valores del SO, no nuestros:
// `uiMode` empaqueta el tipo de interfaz (teléfono, coche, televisión) y el modo noche en
// el mismo entero, y hay que quedarse con los bits del segundo.
const (
	uiModeNightMask = 0x30
	uiModeNightYes  = 0x20
)

// delSistema pregunta a Android en qué idioma habla su dueño:
//
//	Locale.getDefault().toLanguageTag()   // "es-ES", "pt-BR", "zh-Hans-CN"
//
// `getDefault` devuelve el idioma efectivo de la app, que desde Android 13 puede ser uno
// que la persona haya elegido SOLO para ella en los ajustes del sistema (`LocaleManager`);
// si no ha elegido ninguno, el del teléfono. Las dos respuestas son la correcta.
//
// Son clases del PROPIO Android: no hay `.jar` nuestro que compilar ni permiso que
// declarar. Y si algo falla —una excepción, un contexto que aún no está— se devuelve ""
// y `Idioma` sigue con el entorno: quedarse sin idioma no puede impedir que la app
// arranque.
func delSistema() string {
	var etiqueta string
	_ = Java(func(env jni.Env, ctx jni.Object) error {
		claseLocale, err := CargarClase(env, ctx, "java/util/Locale")
		if err != nil {
			return err
		}
		actual, err := jni.CallStaticObjectMethod(env, claseLocale,
			jni.GetStaticMethodID(env, claseLocale, "getDefault", "()Ljava/util/Locale;"))
		if err != nil {
			return err
		}
		tag, err := jni.CallObjectMethod(env, actual,
			jni.GetMethodID(env, claseLocale, "toLanguageTag", "()Ljava/lang/String;"))
		if err != nil {
			return err
		}
		etiqueta = jni.GoString(env, jni.String(tag))
		return nil
	})
	return etiqueta
}

// temaOscuro lee el modo noche de la configuración del sistema:
//
//	(context.getResources().getConfiguration().uiMode & UI_MODE_NIGHT_MASK) == UI_MODE_NIGHT_YES
//
// `uiMode` es un CAMPO, no un método, así que se lee con `GetFieldID`/`GetIntField`. Es
// la misma pregunta que `prefers-color-scheme: dark` en la web, que es de donde salen los
// dos temas de `internal/theme`.
//
// Se lee UNA VEZ, al arrancar. Cambiar de tema con la app abierta llega como una
// configuración nueva que nadie mira aquí; lo que Fyne sabe del tema del sistema lo
// pinta él (`tema.De`), y esto se vuelve a leer al siguiente arranque.
func temaOscuro() (oscuro, sabido bool) {
	err := Java(func(env jni.Env, contexto jni.Object) error {
		claseContexto := jni.GetObjectClass(env, contexto)
		recursos, err := jni.CallObjectMethod(env, contexto,
			jni.GetMethodID(env, claseContexto, "getResources", "()Landroid/content/res/Resources;"))
		if err != nil {
			return err
		}
		configuracion, err := jni.CallObjectMethod(env, recursos,
			jni.GetMethodID(env, jni.GetObjectClass(env, recursos), "getConfiguration", "()Landroid/content/res/Configuration;"))
		if err != nil {
			return err
		}
		modo := jni.GetIntField(env, configuracion,
			jni.GetFieldID(env, jni.GetObjectClass(env, configuracion), "uiMode", "I"))
		oscuro = modo&uiModeNightMask == uiModeNightYes
		return nil
	})
	return oscuro, err == nil
}
