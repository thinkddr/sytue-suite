// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package camara

import (
	"encoding/binary"
	"image"
	"sync"
	"time"

	"git.wow.st/gmp/jni"
)

// La pantalla de Android: MediaProjection por las clases de la APK eu.sytue.nativo.Pantalla,
// PantallaPermiso (el diálogo de Android) y PantallaServicio (el servicio en primer plano
// con su aviso, que Android 14 exige).
const clasePantalla = "eu.sytue.nativo.Pantalla"

// EsperaConsentimiento es lo que se espera a que la persona conteste al diálogo.
var EsperaConsentimiento = 2 * time.Minute

type pantallaAndroid struct {
	mu      sync.Mutex
	cerrada bool
}

// PantallaDisponible indica que este destino implementa la captura; los permisos
// y el acceso al equipo se comprueban al abrirla.
const PantallaDisponible = true

func abrirPantalla() (Fuente, error) {
	err := llamarClase(clasePantalla, func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		m := jni.GetStaticMethodID(env, cls, "pedir", "(Landroid/content/Context;Ljava/lang/String;)V")
		return jni.CallStaticVoidMethod(env, cls, m, jni.Value(ctx), jni.Value(jni.JavaString(env, TextoAvisoPantalla)))
	})
	if err != nil {
		return nil, err
	}
	// La respuesta del diálogo llega sola: se espera aquí, fuera del hilo de la interfaz.
	for fin := time.Now().Add(EsperaConsentimiento); time.Now().Before(fin); time.Sleep(200 * time.Millisecond) {
		switch estadoPantalla() {
		case 1:
			return &pantallaAndroid{}, nil
		case -1:
			return nil, ErrSinPermiso
		}
	}
	return nil, ErrSinPermiso
}

func estadoPantalla() int {
	e := -1
	_ = llamarClase(clasePantalla, func(env jni.Env, _ jni.Object, cls jni.Class) error {
		var err error
		e, err = jni.CallStaticIntMethod(env, cls, jni.GetStaticMethodID(env, cls, "estado", "()I"))
		return err
	})
	return e
}

func (p *pantallaAndroid) Leer() (*image.YCbCr, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cerrada {
		return nil, false
	}
	var b []byte
	_ = llamarClase(clasePantalla, func(env jni.Env, _ jni.Object, cls jni.Class) error {
		o, err := jni.CallStaticObjectMethod(env, cls, jni.GetStaticMethodID(env, cls, "leer", "()[B"))
		if err != nil || o == 0 {
			return err
		}
		b = jni.GetByteArrayElements(env, jni.ByteArray(o))
		jni.DeleteLocalRef(env, o)
		return nil
	})
	if len(b) < 8 {
		return nil, false
	}
	w, h := int(binary.BigEndian.Uint32(b)), int(binary.BigEndian.Uint32(b[4:]))
	if w <= 0 || h <= 0 || len(b) < 8+w*h*4 {
		return nil, false
	}
	return DeRGBA(&image.RGBA{Pix: b[8 : 8+w*h*4], Stride: w * 4, Rect: image.Rect(0, 0, w, h)}), true
}

func (p *pantallaAndroid) Cerrar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cerrada {
		return
	}
	p.cerrada = true
	_ = llamarClase(clasePantalla, func(env jni.Env, ctx jni.Object, cls jni.Class) error {
		m := jni.GetStaticMethodID(env, cls, "cerrar", "(Landroid/content/Context;)V")
		return jni.CallStaticVoidMethod(env, cls, m, jni.Value(ctx))
	})
}
