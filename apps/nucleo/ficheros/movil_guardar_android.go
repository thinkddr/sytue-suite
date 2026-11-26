// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package ficheros

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// «Guardar como…» en Android: ACTION_CREATE_DOCUMENT (eu.sytue.nativo.GuardarDocumento),
// donde la persona elige carpeta y nombre en el teléfono, la tarjeta o la nube que tenga.
// El permiso sobre el documento es PERSISTENTE, así que el reflejo se recuerda y sigue
// llevando cada Guardar a ese documento también tras cerrar la app (movil_guardar.go).

const claseGuardar = "eu.sytue.nativo.GuardarDocumento"

var (
	reflejosAndroid       *reflejos
	reflejosAndroidUnaVez sync.Once
)

func init() {
	if GuardarEnElMovil == nil {
		GuardarEnElMovil = guardarEnAndroid
	}
	go restaurarReflejosAndroid()
}

// baseAndroid es la carpeta privada de la app: la de `getFilesDir`, que el arranque
// nativo de Fyne deja en FILESDIR (y `sesion` en HOME).
func baseAndroid() (string, error) {
	if d := os.Getenv("FILESDIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("HOME"); d != "" {
		return d, nil
	}
	return "", fmt.Errorf("%w: sin carpeta de la app", ErrNoDisponible)
}

func losReflejosAndroid() (*reflejos, error) {
	base, err := baseAndroid()
	if err != nil {
		return nil, err
	}
	reflejosAndroidUnaVez.Do(func() { reflejosAndroid = nuevosReflejos(base) })
	return reflejosAndroid, nil
}

func guardarEnAndroid(nombre string, extensiones []string) (string, error) {
	r, err := losReflejosAndroid()
	if err != nil {
		return "", err
	}
	var crudo string
	err = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseGuardar)
		if err != nil {
			return err
		}
		o, err := jni.CallStaticObjectMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "elegir", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, nombre)), jni.Value(jni.JavaString(env, tipoMovil(nombre))))
		if err == nil && o != 0 {
			crudo = jni.GoString(env, jni.String(o))
		}
		return err
	})
	if err != nil {
		// Sin la máquina de Java o sin la clase: que la app guarde en su carpeta, como
		// sin selector.
		return "", fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	switch crudo {
	case "":
		return "", nil // cancelado
	case "!":
		return "", ErrNoDisponible // sin actividad a la que pegar el selector
	}
	uri, visible, _ := strings.Cut(crudo, "\n")
	if !strings.HasPrefix(uri, "content://") {
		return "", errors.New("ficheros: el selector de Android no devolvió un documento")
	}
	if strings.TrimSpace(visible) == "" {
		visible = nombre
	}
	base, _ := baseAndroid()
	ruta := copiaDeTrabajo(base, visible, uri, extensiones)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o700); err != nil {
		return "", err
	}
	r.vigilar(ruta, uri, true, copiarAndroid)
	return ruta, nil
}

// copiarAndroid lleva la copia de trabajo al documento.
func copiarAndroid(ruta, uri string) error {
	r := 0
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseGuardar)
		if err != nil {
			return err
		}
		r, err = jni.CallStaticIntMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "copiar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)I"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, ruta)), jni.Value(jni.JavaString(env, uri)))
		return err
	})
	switch {
	case errors.Is(err, sistema.ErrSinJava):
		return errReintentar // la app sin actividad: luego
	case err != nil:
		return err
	case r == 1:
		return errReintentar
	case r != 0:
		return errors.New("ficheros: el documento elegido ya no se puede escribir")
	}
	return nil
}

// restaurarReflejosAndroid vuelve a poner en marcha los reflejos de un arranque anterior
// cuando la máquina de Java ya está (el toolkit la da al crear la actividad). Una app que
// nunca guardó así (todas las de la edición conectada) no llega a tocar Java.
func restaurarReflejosAndroid() {
	var r *reflejos
	for i := 0; i < 60 && r == nil; i++ {
		if r, _ = losReflejosAndroid(); r == nil {
			time.Sleep(time.Second)
		}
	}
	if r == nil {
		return
	}
	if _, err := os.Stat(r.fichero); err != nil {
		return
	}
	for i := 0; i < 60; i++ {
		if sistema.EnAndroid != nil && sistema.Java(func(jni.Env, jni.Object) error { return nil }) == nil {
			break
		}
		time.Sleep(time.Second)
	}
	for ruta, uri := range r.recordados() {
		r.vigilar(ruta, uri, true, copiarAndroid)
	}
}
