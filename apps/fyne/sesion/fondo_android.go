// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sesion

/*
#include <jni.h>
extern JavaVM* sytue_fondo_vm(JNIEnv* env);
extern JNIEnv* sytue_fondo_adjuntar(JavaVM* vm, int* adjuntado);
extern void sytue_fondo_soltar(JavaVM* vm);
extern int sytue_fondo_limpiar(JNIEnv* env);
*/
import "C"

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// La entrada JNI del refresco sin ventana (fondo_sinventana.go) y el relevo de One
// (relevo.go). La llama eu.sytue.nativo.RefrescoWidgets desde un hilo de trabajo, con la
// biblioteca recién cargada por System.loadLibrary: los `init` han corrido y el `main`
// no. Probado en la JVM del equipo (una .so c-shared con esta misma forma, cargada sin
// actividad y llamada desde un hilo de trabajo); en un teléfono, no: ver el runbook.

// errExcepcionJava: lo que se llamó en Java lanzó y nadie la recogió.
var errExcepcionJava = errors.New("sesion: excepción de Java sin recoger")

var javaSinVentana sync.Once

// Java_eu_sytue_nativo_RefrescoWidgets_refrescar es `RefrescoWidgets.refrescar(Context,
// String paquete, String producto, String zona, int desfase)`: devuelve el Resultado.
//
//export Java_eu_sytue_nativo_RefrescoWidgets_refrescar
func Java_eu_sytue_nativo_RefrescoWidgets_refrescar(env *C.JNIEnv, _ C.jclass, contexto C.jobject,
	paquete, producto, zona C.jstring, desfase C.jint) (r C.jint) {
	defer func() {
		if recover() != nil {
			r = C.jint(Fallo)
		}
	}()
	e := jni.EnvFor(uintptr(unsafe.Pointer(env)))
	o := SinVentana{
		Bundle:   jni.GoString(e, jni.String(paquete)),
		Producto: jni.GoString(e, jni.String(producto)),
		Zona:     zonaDe(jni.GoString(e, jni.String(zona)), int(desfase)),
	}
	if suiteViva.Load() == nil {
		vm := C.sytue_fondo_vm(env)
		if vm == nil {
			return C.jint(Fallo)
		}
		// Una vez por proceso: la referencia global al contexto de la APLICACIÓN (no
		// el de este JobService, que muere al acabar) vive lo que el proceso.
		javaSinVentana.Do(func() {
			global := jni.NewGlobalRef(e, jni.Object(contexto))
			sistema.EnAndroid = conRespaldo(sistema.EnAndroid, javaPropia(vm, uintptr(global)))
		})
		prepararCasa() // HOME y XDG: sin ellos ni ajustes ni el snapshot tienen carpeta
	}
	ctx, parar := context.WithTimeout(context.Background(), widgets.PlazoWidget+5*time.Second)
	defer parar()
	return C.jint(RefrescarSinVentana(ctx, o))
}

// javaPropia ejecuta f en un hilo enganchado a `vm`, con el contexto `ctx`: lo que hace
// RunOnJVM en el toolkit (internal/driver/mobile/mobileinit), que aquí no se puede usar
// porque su máquina solo la pone la actividad y el paquete es interno del fork.
func javaPropia(vm *C.JavaVM, ctx uintptr) ejecutorJava {
	return func(f func(vm, env, ctx uintptr) error) error {
		errc := make(chan error, 1)
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			var adjuntado C.int
			env := C.sytue_fondo_adjuntar(vm, &adjuntado)
			if env == nil {
				errc <- sistema.ErrSinJava
				return
			}
			if adjuntado != 0 {
				defer C.sytue_fondo_soltar(vm)
			}
			err := f(uintptr(unsafe.Pointer(vm)), uintptr(unsafe.Pointer(env)), ctx)
			if C.sytue_fondo_limpiar(env) != 0 && err == nil {
				err = errExcepcionJava
			}
			errc <- err
		}()
		return <-errc
	}
}

func init() {
	// One releva la señal de cambios a la app cerrada (relevo.go). Sin datos: solo el
	// paquete, que RelevoWidgets.avisar comprueba contra la lista cerrada y su firma.
	relevarWidget = func(paquete string) {
		_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
			cls, err := sistema.CargarClase(env, ctx, "eu.sytue.nativo.RelevoWidgets")
			if err != nil {
				return err
			}
			_, err = jni.CallStaticBooleanMethod(env, cls,
				jni.GetStaticMethodID(env, cls, "avisar", "(Landroid/content/Context;Ljava/lang/String;)Z"),
				jni.Value(ctx), jni.Value(jni.JavaString(env, paquete)))
			return err
		})
	}
}
