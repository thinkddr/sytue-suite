// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

/*
#include <jni.h>
*/
import "C"

import (
	"sync"

	"fyne.io/fyne/v2"
	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// ImagenEnImagen es la del sistema en Android: la actividad encogida en una esquina
// (eu.sytue.nativo.ImagenEnImagen). La app tiene que declararla en su FyneApp.toml
// (`imagenEnImagen = "si"`), o Android no la deja entrar.
var ImagenEnImagen imagenEnImagen = pipAndroid{}

type pipAndroid struct{}

// llamar ejecuta un método estático de la clase con el contexto de la app.
func (pipAndroid) llamar(f func(env jni.Env, ctx jni.Object, cls jni.Class) error) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.ImagenEnImagen")
		if err != nil {
			return err
		}
		return f(env, ctx, cls)
	})
}

// booleano llama a un `static boolean m(Context)`; `id` lo busca, con la firma escrita
// en cada llamada para que TestLasFirmasJNICoincidenConElJava la compare con el Java.
func (p pipAndroid) booleano(id func(env jni.Env, cls jni.Class) jni.MethodID) bool {
	si := false
	_ = p.llamar(func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		var err error
		si, err = jni.CallStaticBooleanMethod(env, cls, id(env, cls), jni.Value(ctx))
		return err
	})
	return si
}

func (p pipAndroid) Disponible() bool {
	return p.booleano(func(env jni.Env, cls jni.Class) jni.MethodID {
		return jni.GetStaticMethodID(env, cls, "disponible", "(Landroid/content/Context;)Z")
	})
}

func (p pipAndroid) Activa() bool {
	return p.booleano(func(env jni.Env, cls jni.Class) jni.MethodID {
		return jni.GetStaticMethodID(env, cls, "activa", "(Landroid/content/Context;)Z")
	})
}

func (p pipAndroid) Preparar(ancho, alto int, reproduciendo bool) {
	_ = p.llamar(func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "preparar", "(Landroid/content/Context;IIZ)V"),
			jni.Value(ctx), jni.Value(ancho), jni.Value(alto), jni.Value(boolAJava(reproduciendo)))
	})
}

func (p pipAndroid) Entrar(reproduciendo bool) bool {
	si := false
	_ = p.llamar(func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		var err error
		si, err = jni.CallStaticBooleanMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "entrar", "(Landroid/content/Context;Z)Z"),
			jni.Value(ctx), jni.Value(boolAJava(reproduciendo)))
		return err
	})
	return si
}

// Java_eu_sytue_nativo_ImagenEnImagen_suceso es `ImagenEnImagen.suceso(que)`: el botón de
// la ventana (1) o que se ha cerrado (2). Llega en el hilo de Android y se lleva al de Fyne.
//
//export Java_eu_sytue_nativo_ImagenEnImagen_suceso
func Java_eu_sytue_nativo_ImagenEnImagen_suceso(env *C.JNIEnv, cls C.jclass, que C.jint) { //nolint:revive // el nombre lo pone JNI
	_, _ = env, cls // los pide la firma de JNI; cgo no admite parámetros sin nombre
	fyne.Do(func() { avisarPiP(int(que)) })
}

var (
	muPiP     sync.Mutex
	alSuceder func(int)
)

// AlSucesoPiP apunta quién atiende el botón de la ventana y su cierre. Se le llama en el
// hilo de la interfaz.
func AlSucesoPiP(f func(que int)) {
	muPiP.Lock()
	alSuceder = f
	muPiP.Unlock()
}

func avisarPiP(que int) {
	muPiP.Lock()
	f := alSuceder
	muPiP.Unlock()
	if f != nil {
		f(que)
	}
}
