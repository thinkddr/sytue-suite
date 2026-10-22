// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package altavoz

/*
#include <jni.h>
#include <stdint.h>

// sytuePedirMicro es, en Java:
//
//	if (ctx.checkSelfPermission("android.permission.RECORD_AUDIO") != GRANTED)
//	    ((Activity) ctx).requestPermissions(new String[]{"android.permission.RECORD_AUDIO"}, 7414);
//
// Devuelve 0 si ya estaba concedido, 1 si se ha pedido y 2 si Java ha fallado.
static int sytuePedirMicro(uintptr_t envp, uintptr_t ctxp) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	if ((*env)->PushLocalFrame(env, 16) != 0) { (*env)->ExceptionClear(env); return 2; }
	jstring permiso = (*env)->NewStringUTF(env, "android.permission.RECORD_AUDIO");
	jint concedido = (*env)->CallIntMethod(env, ctx,
		(*env)->GetMethodID(env, (*env)->GetObjectClass(env, ctx), "checkSelfPermission", "(Ljava/lang/String;)I"),
		permiso);
	int r = 0;
	jclass cActividad = (*env)->FindClass(env, "android/app/Activity");
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionClear(env);
		r = 2;
	} else if (concedido != 0 && (*env)->IsInstanceOf(env, ctx, cActividad)) {
		jobjectArray lista = (*env)->NewObjectArray(env, 1, (*env)->FindClass(env, "java/lang/String"), permiso);
		(*env)->CallVoidMethod(env, ctx,
			(*env)->GetMethodID(env, cActividad, "requestPermissions", "([Ljava/lang/String;I)V"), lista, 7414);
		r = (*env)->ExceptionCheck(env) ? 2 : 1;
		(*env)->ExceptionClear(env);
	}
	(*env)->PopLocalFrame(env, NULL);
	return r;
}
*/
import "C"

import (
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// PedirMicro pide RECORD_AUDIO en el momento de usarlo —al abrir la antesala—, no al
// instalar: sin él AAudio graba silencio sin dar error. La respuesta es asíncrona; si se
// niega, se entra solo a escuchar, como en la web.
func PedirMicro() error {
	if sistema.EnAndroid == nil {
		return sistema.ErrSinJava
	}
	return sistema.EnAndroid(func(_, env, ctx uintptr) error {
		if C.sytuePedirMicro(C.uintptr_t(env), C.uintptr_t(ctx)) == 2 {
			return errors.New("altavoz: Android no ha dejado pedir el micrófono")
		}
		return nil
	})
}
