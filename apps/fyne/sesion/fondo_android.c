// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// La puerta a Java del refresco sin ventana (fondo_android.go): sin actividad, el
// toolkit no ha dado la máquina de Java, y se toma de la entrada JNI.

#include <jni.h>
#include <stddef.h>

JavaVM* sytue_fondo_vm(JNIEnv* env) {
	JavaVM* vm = NULL;
	return (*env)->GetJavaVM(env, &vm) == JNI_OK ? vm : NULL;
}

// sytue_fondo_adjuntar da el entorno JNI de este hilo, enganchándolo si hace falta
// (*adjuntado = 1: hay que soltarlo al acabar).
JNIEnv* sytue_fondo_adjuntar(JavaVM* vm, int* adjuntado) {
	JNIEnv* env = NULL;
	*adjuntado = 0;
	switch ((*vm)->GetEnv(vm, (void**)&env, JNI_VERSION_1_6)) {
	case JNI_OK:
		return env;
	case JNI_EDETACHED:
		if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) {
			return NULL;
		}
		*adjuntado = 1;
		return env;
	default:
		return NULL;
	}
}

void sytue_fondo_soltar(JavaVM* vm) { (*vm)->DetachCurrentThread(vm); }

// sytue_fondo_limpiar quita la excepción pendiente, si la hay, y dice si la había.
int sytue_fondo_limpiar(JNIEnv* env) {
	if ((*env)->ExceptionCheck(env)) {
		(*env)->ExceptionClear(env);
		return 1;
	}
	return 0;
}
