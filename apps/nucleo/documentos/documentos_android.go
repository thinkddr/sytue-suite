// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package documentos

import (
	"context"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// claseDocumentos es la clase propia de la APK (apps/fyne/android/java).
const claseDocumentos = "eu.sytue.nativo.Documentos"

// En Android el proveedor es de Java y habla él con Drive por WebDAV: de Go solo
// necesita los textos (servir) y saber que la credencial ha cambiado (conectar).
var hayProveedor = true

// servir le deja al proveedor los textos ya traducidos, para cuando la app no esté.
func servir(_ context.Context, _ Fuente, t Textos) error {
	return llamar("configurar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)V",
		func(env jni.Env) []jni.Value {
			return []jni.Value{jni.Value(jni.JavaString(env, t.Titulo)), jni.Value(jni.JavaString(env, t.SinApp))}
		})
}

// conectar avisa a «Archivos» de que la raíz ha cambiado: el proveedor vuelve a leer la
// credencial del almacén.
var conectar, desconectar = avisarCambio, func(c Credencial) {
	avisar("el proveedor de documentos no se ha enterado", avisarCambio(c))
}

func avisarCambio(Credencial) error {
	return llamar("cambio", "(Landroid/content/Context;)V", func(jni.Env) []jni.Value { return nil })
}

// llamar invoca un método estático de Documentos con el Context delante.
func llamar(metodo, firma string, args func(jni.Env) []jni.Value) error {
	return sistema.Java(func(env jni.Env, actx jni.Object) error {
		cls, err := sistema.CargarClase(env, actx, claseDocumentos)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls, jni.GetStaticMethodID(env, cls, metodo, firma),
			append([]jni.Value{jni.Value(actx)}, args(env)...)...)
	})
}
