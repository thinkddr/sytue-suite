// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package carrete

import (
	"context"
	"errors"
	"os"
	"time"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// # Android: MediaStore
//
// La clase propia de la APK `eu.sytue.nativo.Carrete` (apps/fyne/android/java) hace el
// trabajo; aquí solo se la llama y se traduce. Los permisos que pide los declara
// `empaquetar.sh` SOLO en las apps con `fototeca = "si"` en su FyneApp.toml.

const claseCarrete = "eu.sytue.nativo.Carrete"

func disponible() bool { return true }

func llamar(f func(env jni.Env, cls jni.Class, ctx jni.Object) error) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseCarrete)
		if err != nil {
			return err
		}
		return f(env, cls, ctx)
	})
}

// permisoJava es Carrete.permiso: 1 todas, 2 las elegidas, 0 ninguna.
func permisoJava() (int, error) {
	var n int
	err := llamar(func(env jni.Env, cls jni.Class, ctx jni.Object) (err error) {
		n, err = jni.CallStaticIntMethod(env, cls, jni.GetStaticMethodID(env, cls, "permiso", "(Landroid/content/Context;)I"), jni.Value(ctx))
		return err
	})
	return n, err
}

// pedirJava es Carrete.pedir: enseña el diálogo, si hay actividad.
func pedirJava() bool {
	var si bool
	_ = llamar(func(env jni.Env, cls jni.Class, ctx jni.Object) (err error) {
		si, err = jni.CallStaticBooleanMethod(env, cls, jni.GetStaticMethodID(env, cls, "pedir", "(Landroid/content/Context;)Z"), jni.Value(ctx))
		return err
	})
	return si
}

// conFocoJava es Carrete.conFoco: con el diálogo de permisos delante, false.
func conFocoJava() bool {
	si := true
	_ = llamar(func(env jni.Env, cls jni.Class, ctx jni.Object) (err error) {
		si, err = jni.CallStaticBooleanMethod(env, cls, jni.GetStaticMethodID(env, cls, "conFoco", "(Landroid/content/Context;)Z"), jni.Value(ctx))
		return err
	})
	return si
}

func cadena(env jni.Env, o jni.Object) string {
	if o == 0 {
		return ""
	}
	return jni.GoString(env, jni.String(o))
}

func estadoPermiso() Permiso {
	switch n, err := permisoJava(); {
	case err != nil:
		return NoDisponible
	case n == 1:
		return Concedido
	case n == 2:
		return Limitado
	default:
		return SinPreguntar
	}
}

// pedirPermiso enseña el diálogo y lo espera mirando el permiso cada medio segundo: una
// NativeActivity no recibe `onRequestPermissionsResult`. Se sabe que el diálogo se cerró
// sin conceder porque la actividad recupera el foco; y si nunca lo pierde, es que Android
// ya no enseña el diálogo («no volver a preguntar»).
func pedirPermiso(ctx context.Context) (Permiso, error) {
	if !pedirJava() {
		return Denegado, nil
	}
	fuera, dentro := false, 0
	for n := 0; ; n++ {
		select {
		case <-ctx.Done():
			return estadoPermiso(), ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		if p := estadoPermiso(); p.Lee() {
			return p, nil
		}
		switch foco := conFocoJava(); {
		case !foco:
			fuera, dentro = true, 0
		case fuera:
			dentro++
			if dentro >= 3 {
				return Denegado, nil
			}
		case n >= 6: // tres segundos sin diálogo
			return Denegado, nil
		}
	}
}

func listar(ctx context.Context) ([]Elemento, error) {
	var texto string
	err := llamar(func(env jni.Env, cls jni.Class, actx jni.Object) error {
		o, err := jni.CallStaticObjectMethod(env, cls, jni.GetStaticMethodID(env, cls, "listar", "(Landroid/content/Context;)Ljava/lang/String;"), jni.Value(actx))
		texto = cadena(env, o)
		return err
	})
	if err != nil {
		return nil, err
	}
	return deLineas(ctx, texto)
}

func abrir(_ context.Context, e Elemento) (string, func(), error) {
	f, err := os.CreateTemp("", "sytue-carrete-*")
	if err != nil {
		return "", func() {}, err
	}
	ruta := f.Name()
	_ = f.Close()
	limpiar := func() { _ = os.Remove(ruta) }
	var motivo string
	err = llamar(func(env jni.Env, cls jni.Class, actx jni.Object) error {
		m := jni.GetStaticMethodID(env, cls, "copiar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;")
		o, err := jni.CallStaticObjectMethod(env, cls, m,
			jni.Value(actx), jni.Value(jni.JavaString(env, e.ID)), jni.Value(jni.JavaString(env, ruta)))
		motivo = cadena(env, o)
		return err
	})
	if err == nil && motivo != "" {
		err = errors.New("carrete: " + motivo)
	}
	return ruta, limpiar, err
}

func convertir(_ context.Context, ruta string) (string, func(), error) {
	f, err := os.CreateTemp("", "sytue-convertida-*.jpg")
	if err != nil {
		return "", func() {}, err
	}
	destino := f.Name()
	_ = f.Close()
	limpiar := func() { _ = os.Remove(destino) }
	var motivo string
	err = llamar(func(env jni.Env, cls jni.Class, _ jni.Object) error {
		m := jni.GetStaticMethodID(env, cls, "convertir", "(Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;")
		o, err := jni.CallStaticObjectMethod(env, cls, m, jni.Value(jni.JavaString(env, ruta)), jni.Value(jni.JavaString(env, destino)))
		motivo = cadena(env, o)
		return err
	})
	if err == nil && motivo != "" {
		err = errors.New("carrete: " + motivo)
	}
	return destino, limpiar, err
}

// redMedida es Carrete.medida: sin poder preguntarlo, se da por medida (no se gastan
// datos por un fallo de la pregunta).
func redMedida() bool {
	medida := true
	_ = llamar(func(env jni.Env, cls jni.Class, ctx jni.Object) (err error) {
		medida, err = jni.CallStaticBooleanMethod(env, cls, jni.GetStaticMethodID(env, cls, "medida", "(Landroid/content/Context;)Z"), jni.Value(ctx))
		return err
	})
	return medida
}

func guardar(_ context.Context, ruta, nombre string) (string, error) {
	var id string
	err := llamar(func(env jni.Env, cls jni.Class, actx jni.Object) error {
		m := jni.GetStaticMethodID(env, cls, "guardar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;")
		o, err := jni.CallStaticObjectMethod(env, cls, m,
			jni.Value(actx), jni.Value(jni.JavaString(env, ruta)), jni.Value(jni.JavaString(env, nombre)),
			jni.Value(jni.JavaString(env, TipoDe(nombre))))
		id = cadena(env, o)
		return err
	})
	if err == nil && id == "" {
		err = ErrNoDisponible // API < 29, o MediaStore dijo que no: a descargas
	}
	return id, err
}
