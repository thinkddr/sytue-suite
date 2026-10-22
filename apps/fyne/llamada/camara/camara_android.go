// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package camara

import (
	"encoding/binary"
	"image"
	"strings"
	"sync"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// La cámara de Android: Camera2 por la clase de la APK eu.sytue.nativo.Camara
// (apps/fyne/android/java). El permiso CAMERA se pide al encenderla, no al instalar.
const claseCamara = "eu.sytue.nativo.Camara"

type fuenteAndroid struct {
	mu      sync.Mutex
	cerrada bool
}

func llamar(f func(env jni.Env, ctx jni.Object, cls jni.Class) error) error {
	return llamarClase(claseCamara, f)
}

// llamarClase ejecuta f con una clase propia de la APK cargada.
func llamarClase(clase string, f func(env jni.Env, ctx jni.Object, cls jni.Class) error) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, clase)
		if err != nil {
			return err
		}
		return f(env, ctx, cls)
	})
}

func abrir(p Pedido) (Fuente, error) {
	var r int
	err := llamar(func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		m := jni.GetStaticMethodID(env, cls, "abrir", "(Landroid/content/Context;Ljava/lang/String;III)I")
		var err error
		r, err = jni.CallStaticIntMethod(env, cls, m, jni.Value(ctx), jni.Value(jni.JavaString(env, p.ID)),
			jni.Value(p.Ancho), jni.Value(p.Alto), jni.Value(p.Fotogramas))
		return err
	})
	switch {
	case err != nil:
		return nil, err
	case r == -2:
		return nil, ErrSinPermiso
	case r != 0:
		return nil, ErrSinCamara
	}
	return &fuenteAndroid{}, nil
}

func (f *fuenteAndroid) Leer() (*image.YCbCr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cerrada {
		return nil, false
	}
	var b []byte
	_ = llamar(func(env jni.Env, _ jni.Object, cls jni.Class) error {
		m := jni.GetStaticMethodID(env, cls, "leer", "()[B")
		o, err := jni.CallStaticObjectMethod(env, cls, m)
		if err != nil || o == 0 {
			return err
		}
		b = jni.GetByteArrayElements(env, jni.ByteArray(o))
		jni.DeleteLocalRef(env, o)
		return nil
	})
	const cabecera = 20 // ancho, alto, giro del sensor, delantera y giro de la pantalla
	if len(b) < cabecera {
		return nil, false
	}
	entero := func(i int) int { return int(int32(binary.BigEndian.Uint32(b[4*i:]))) }
	w, h := entero(0), entero(1)
	if w <= 0 || h <= 0 || len(b) < cabecera+w*h*3/2 {
		return nil, false
	}
	return girar(deI420(b[cabecera:], w, h), giroPara(entero(2), entero(3) == 1, entero(4))), true
}

// Concedido dice si ya se puede usar la cámara, SIN pedir el permiso: es para esperar la
// respuesta al diálogo del sistema que abre Abrir la primera vez.
func Concedido() bool {
	si := false
	_ = llamar(func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		m := jni.GetStaticMethodID(env, cls, "concedido", "(Landroid/content/Context;)Z")
		var err error
		si, err = jni.CallStaticBooleanMethod(env, cls, m, jni.Value(ctx))
		return err
	})
	return si
}

func (f *fuenteAndroid) Cerrar() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cerrada {
		return
	}
	f.cerrada = true
	_ = llamar(func(env jni.Env, _ jni.Object, cls jni.Class) error {
		return jni.CallStaticVoidMethod(env, cls, jni.GetStaticMethodID(env, cls, "cerrar", "()V"))
	})
}

func dispositivos() []Dispositivo {
	var s string
	_ = llamar(func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		m := jni.GetStaticMethodID(env, cls, "lista", "(Landroid/content/Context;)Ljava/lang/String;")
		o, err := jni.CallStaticObjectMethod(env, cls, m, jni.Value(ctx))
		if err == nil && o != 0 {
			s = jni.GoString(env, jni.String(o))
		}
		return err
	})
	var out []Dispositivo
	for _, l := range strings.Split(s, "\n") {
		if id, nombre, ok := strings.Cut(l, "\t"); ok {
			out = append(out, Dispositivo{ID: id, Nombre: nombre})
		}
	}
	return out
}
