// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package ficheros

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// Las clases propias de la APK (apps/fyne/android/java).
const (
	claseImprimir = "eu.sytue.nativo.Imprimir"
	claseCaptura  = "eu.sytue.nativo.Captura"
)

// imprimirMovil abre el diálogo de impresión de Android con el PDF de `ruta`: las
// impresoras del teléfono y «Guardar como PDF». Solo PDF, que es lo único que la suite
// imprime (CertiPDF y los tres editores exportan antes).
func imprimirMovil(ruta string) error {
	if !strings.EqualFold(filepath.Ext(ruta), ".pdf") {
		return ErrNoDisponible
	}
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseImprimir)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "pdf", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, filepath.Base(ruta))), jni.Value(jni.JavaString(env, ruta)))
	})
}

// abrirMovil abre el fichero con el visor asociado del sistema en Android.
func abrirMovil(ruta string) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseCaptura)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "compartir", "(Landroid/content/Context;Ljava/lang/String;Z)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, ruta)), jni.Value(1))
	})
}

// revelarMovil abre la hoja de compartir del sistema (share sheet) con el fichero.
func revelarMovil(ruta string) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseCaptura)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "compartir", "(Landroid/content/Context;Ljava/lang/String;Z)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, ruta)), jni.Value(0))
	})
}

// numeroFoto hace único el nombre de cada foto; el proveedor solo sirve los que
// empiezan por `sytue-captura-`.
var numeroFoto atomic.Int64

// fotografiar espera a la cámara en su rutina: si `ctx` se cancela antes, se deja de
// esperar y la foto, si llega, se queda en la caché, que el sistema limpia.
func fotografiar(ctx context.Context) (string, error) {
	nombre := fmt.Sprintf("sytue-captura-%d-%d.jpg", time.Now().UnixNano(), numeroFoto.Add(1))
	type resultado struct {
		ruta string
		err  error
	}
	listo := make(chan resultado, 1)
	go func() {
		var ruta string
		err := sistema.Java(func(env jni.Env, actx jni.Object) error {
			cls, err := sistema.CargarClase(env, actx, claseCaptura)
			if err != nil {
				return err
			}
			o, err := jni.CallStaticObjectMethod(env, cls,
				jni.GetStaticMethodID(env, cls, "fotografiar", "(Landroid/content/Context;Ljava/lang/String;)Ljava/lang/String;"),
				jni.Value(actx), jni.Value(jni.JavaString(env, nombre)))
			if err == nil && o != 0 {
				ruta = jni.GoString(env, jni.String(o))
			}
			return err
		})
		listo <- resultado{ruta, err}
	}()
	select {
	case r := <-listo:
		return r.ruta, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func leerLocalSeguro(ruta string) ([]byte, error) {
	limpia := strings.TrimPrefix(ruta, "file://")
	return os.ReadFile(limpia)
}
