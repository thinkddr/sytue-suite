// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package asistente

import (
	"context"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// En Android la IA del sistema que Google abre a otras apps es Gemini Nano por la Prompt
// API de ML Kit GenAI, que habla con AICore (Pixel 9 y posteriores, Galaxy S25 y otros).
// ML Kit es Java: el puente es eu.sytue.nativo.IASistema (apps/fyne/android/java), que la
// busca por reflexión. Sin ella —el teléfono no la tiene, o la APK no lleva ML Kit— Ajustes
// lo dice y Asistir devuelve ErrSinIANativa.

const claseIA = "eu.sytue.nativo.IASistema"

// Los estados de IASistema.java.
var estadosAndroid = map[int]EstadoSistema{
	0: SistemaLista,
	1: SistemaDescargable,
	2: SistemaDescargando,
	3: SistemaNoSoportada,
	4: SistemaSinPuente,
	5: SistemaAntiguo,
	6: SistemaFallo,
}

var puente = &puenteNativo{
	nombre:    "Gemini Nano (en el teléfono)",
	consultar: consultarAndroid,
	responder: responderAndroid,
	preparar:  descargarAndroid,
}

// NombreSistema es el nombre de la IA del sistema en español: el de los registros y el
// del destino (Asistente.Destino). Ajustes la nombra en su idioma con EstadoDelSistema.
func NombreSistema() string { return puente.nombre }

// EstadoDelSistema es lo último que se sabe del modelo, sin esperar: Ajustes pinta en el
// hilo de la interfaz y dice con él por qué no hay IA del sistema. Si es viejo, se
// vuelve a preguntar por detrás.
func EstadoDelSistema() EstadoSistema { return puente.estadoSinEsperar() }

// SistemaDisponible dice si Gemini Nano está listo en este teléfono.
func SistemaDisponible(context.Context) bool { return puente.estadoAhora() == SistemaLista }

func completarSistema(ctx context.Context, _ Config, system, user string) (string, error) {
	return puente.completar(ctx, system, user)
}

func consultarAndroid() EstadoSistema {
	e := SistemaFallo
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseIA)
		if err != nil {
			return err
		}
		n, err := jni.CallStaticIntMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "estado", "(Landroid/content/Context;)I"), jni.Value(ctx))
		if err != nil {
			return err
		}
		if v, ok := estadosAndroid[n]; ok {
			e = v
		}
		return nil
	})
	if err != nil {
		// Sin la máquina de Java o sin la clase (una APK sin el puente).
		return SistemaSinPuente
	}
	return e
}

// responderAndroid no se puede cancelar: ML Kit no da cómo, y Go deja de esperar.
func responderAndroid(_ int64, system, user string) (string, error) {
	var texto string
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseIA)
		if err != nil {
			return err
		}
		o, err := jni.CallStaticObjectMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "responder", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, system)), jni.Value(jni.JavaString(env, user)))
		if err == nil && o != 0 {
			texto = jni.GoString(env, jni.String(o))
		}
		return err
	})
	return texto, err
}

func descargarAndroid() {
	_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseIA)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "descargar", "(Landroid/content/Context;)V"), jni.Value(ctx))
	})
}
