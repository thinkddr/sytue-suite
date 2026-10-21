// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package escritorio

/*
#include <jni.h>
#include <stdint.h>
#include <stdlib.h>

// Si Java ha lanzado una excepción, se limpia y el aviso no sale: seguir llamando con
// una pendiente es un comportamiento indefinido de JNI. Todo va dentro de un marco local
// (PushLocalFrame) que se suelta al salir: en un hilo que ya estaba enganchado a Java las
// referencias locales no se liberan solas, y la app avisa durante días.
// sytueLibre suelta una cadena de C.CString. Está aquí para que el Go no importe
// `unsafe`: gocritic toma `import "C"` y `unsafe` por el mismo paquete dos veces.
static void sytueLibre(char *p) { free(p); }

#define SYTUE_FALLA(env) if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); (*env)->PopLocalFrame(env, NULL); return 1; }

// sytueSdk es `Build.VERSION.SDK_INT`, o -1 si Java ha fallado.
static jint sytueSdk(JNIEnv *env) {
	jclass cVersion = (*env)->FindClass(env, "android/os/Build$VERSION");
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return -1; }
	jint sdk = (*env)->GetStaticIntField(env, cVersion, (*env)->GetStaticFieldID(env, cVersion, "SDK_INT", "I"));
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return -1; }
	return sdk;
}

// sytueCanal crea el canal de avisos de la app (API 26+; crearlo otra vez no hace nada)
// y devuelve el gestor, o NULL si Java ha fallado.
static jobject sytueCanal(JNIEnv *env, jobject ctx, jint sdk, const char *canal, const char *nombre) {
	jclass cCtx = (*env)->GetObjectClass(env, ctx);
	jobject mgr = (*env)->CallObjectMethod(env, ctx,
		(*env)->GetMethodID(env, cCtx, "getSystemService", "(Ljava/lang/String;)Ljava/lang/Object;"),
		(*env)->NewStringUTF(env, "notification"));
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return NULL; }
	if (mgr == NULL || sdk < 26) {
		return mgr;
	}
	jclass cCanal = (*env)->FindClass(env, "android/app/NotificationChannel");
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return NULL; }
	jobject c = (*env)->NewObject(env, cCanal,
		(*env)->GetMethodID(env, cCanal, "<init>", "(Ljava/lang/String;Ljava/lang/CharSequence;I)V"),
		(*env)->NewStringUTF(env, canal), (*env)->NewStringUTF(env, nombre), 3); // IMPORTANCE_DEFAULT
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return NULL; }
	(*env)->CallVoidMethod(env, mgr,
		(*env)->GetMethodID(env, (*env)->GetObjectClass(env, mgr), "createNotificationChannel",
			"(Landroid/app/NotificationChannel;)V"), c);
	if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); return NULL; }
	return mgr;
}

// sytuePrepararAvisos deja la app lista para avisar ANTES del primer aviso. Son dos
// caminos al mismo diálogo de Android 13+, porque cuál vale depende del SDK objetivo de
// la APK (hoy 29, el que pone la CLI de Fyne; Google Play pide 35):
//
//   - con objetivo 32 o menos, el sistema pregunta él solo la primera vez que la app
//     crea un canal de avisos: por eso el canal se crea aquí y no con el primer aviso,
//     que se perdería mientras la pregunta está en pantalla;
//   - con objetivo 33 o más, solo pregunta si la app lo pide:
//     `activity.requestPermissions({"android.permission.POST_NOTIFICATIONS"}, n)`.
//
// Si ya está concedido no hace nada, y si se negó dos veces Android ya no vuelve a
// preguntar: la persona lo cambia en los ajustes del sistema.
//
// Devuelve 0 si el permiso está concedido (o no hace falta), 1 si se ha pedido y 2 si
// Java ha fallado.
static int sytuePrepararAvisos(uintptr_t envp, uintptr_t ctxp, const char *canal, const char *nombre) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	if ((*env)->PushLocalFrame(env, 32) != 0) {
		(*env)->ExceptionClear(env);
		return 2;
	}
	jint sdk = sytueSdk(env);
	if (sdk < 0 || sytueCanal(env, ctx, sdk, canal, nombre) == NULL) {
		(*env)->PopLocalFrame(env, NULL);
		return 2;
	}
	int r = 0;
	if (sdk >= 33) {
		jstring permiso = (*env)->NewStringUTF(env, "android.permission.POST_NOTIFICATIONS");
		jint concedido = (*env)->CallIntMethod(env, ctx,
			(*env)->GetMethodID(env, (*env)->GetObjectClass(env, ctx), "checkSelfPermission", "(Ljava/lang/String;)I"),
			permiso);
		jclass cActividad = (*env)->FindClass(env, "android/app/Activity");
		if ((*env)->ExceptionCheck(env)) {
			(*env)->ExceptionClear(env);
			r = 2;
		} else if (concedido != 0 && (*env)->IsInstanceOf(env, ctx, cActividad)) { // PERMISSION_GRANTED = 0
			jobjectArray lista = (*env)->NewObjectArray(env, 1, (*env)->FindClass(env, "java/lang/String"), permiso);
			(*env)->CallVoidMethod(env, ctx,
				(*env)->GetMethodID(env, cActividad, "requestPermissions", "([Ljava/lang/String;I)V"), lista, 7413);
			r = (*env)->ExceptionCheck(env) ? 2 : 1;
			(*env)->ExceptionClear(env);
		}
	}
	(*env)->PopLocalFrame(env, NULL);
	return r;
}

// sytueAvisarAndroid es el `sendNotification` del fork de Fyne con lo que le falta: el
// PendingIntent que, al pulsar el aviso, abre `enlace` EN ESTA app (setPackage), y el
// icono de la app en vez del de «app desconocida». Devuelve 0 si ha salido.
//
//	mgr = ctx.getSystemService("notification")
//	mgr.createNotificationChannel(new NotificationChannel(canal, nombre, IMPORTANCE_DEFAULT))  // API 26+
//	i = new Intent(ACTION_VIEW, Uri.parse(enlace)); i.setPackage(ctx.getPackageName())
//	pi = PendingIntent.getActivity(ctx, n, i, FLAG_IMMUTABLE | FLAG_UPDATE_CURRENT)
//	mgr.notify(n, new Notification.Builder(ctx, canal).setContentTitle(titulo)
//	    .setContentText(cuerpo).setSmallIcon(ctx.getApplicationInfo().icon)
//	    .setAutoCancel(true).setContentIntent(pi).build())
static int sytueAvisarAndroid(uintptr_t envp, uintptr_t ctxp, const char *canal, const char *nombre,
		const char *enlace, const char *titulo, const char *cuerpo, int n) {
	JNIEnv *env = (JNIEnv *)envp;
	jobject ctx = (jobject)ctxp;
	if ((*env)->PushLocalFrame(env, 64) != 0) {
		(*env)->ExceptionClear(env);
		return 1;
	}
	jclass cCtx = (*env)->GetObjectClass(env, ctx);
	jint sdk = sytueSdk(env);
	jobject mgr = sdk < 0 ? NULL : sytueCanal(env, ctx, sdk, canal, nombre);
	if (mgr == NULL) {
		(*env)->PopLocalFrame(env, NULL);
		return 1;
	}
	jclass cMgr = (*env)->GetObjectClass(env, mgr);

	jstring jCanal = (*env)->NewStringUTF(env, canal);
	jclass cBuilder = (*env)->FindClass(env, "android/app/Notification$Builder");
	SYTUE_FALLA(env);
	jobject b;
	if (sdk >= 26) {
		b = (*env)->NewObject(env, cBuilder,
			(*env)->GetMethodID(env, cBuilder, "<init>", "(Landroid/content/Context;Ljava/lang/String;)V"), ctx, jCanal);
	} else {
		b = (*env)->NewObject(env, cBuilder,
			(*env)->GetMethodID(env, cBuilder, "<init>", "(Landroid/content/Context;)V"), ctx);
	}
	SYTUE_FALLA(env);

	jclass cUri = (*env)->FindClass(env, "android/net/Uri");
	SYTUE_FALLA(env);
	jobject uri = (*env)->CallStaticObjectMethod(env, cUri,
		(*env)->GetStaticMethodID(env, cUri, "parse", "(Ljava/lang/String;)Landroid/net/Uri;"),
		(*env)->NewStringUTF(env, enlace));
	SYTUE_FALLA(env);
	jclass cIntent = (*env)->FindClass(env, "android/content/Intent");
	SYTUE_FALLA(env);
	jobject intent = (*env)->NewObject(env, cIntent,
		(*env)->GetMethodID(env, cIntent, "<init>", "(Ljava/lang/String;Landroid/net/Uri;)V"),
		(*env)->NewStringUTF(env, "android.intent.action.VIEW"), uri);
	SYTUE_FALLA(env);
	jobject paquete = (*env)->CallObjectMethod(env, ctx,
		(*env)->GetMethodID(env, cCtx, "getPackageName", "()Ljava/lang/String;"));
	SYTUE_FALLA(env);
	(*env)->CallObjectMethod(env, intent,
		(*env)->GetMethodID(env, cIntent, "setPackage", "(Ljava/lang/String;)Landroid/content/Intent;"), paquete);
	SYTUE_FALLA(env);
	jclass cPI = (*env)->FindClass(env, "android/app/PendingIntent");
	SYTUE_FALLA(env);
	// FLAG_IMMUTABLE (obligatorio desde Android 12) | FLAG_UPDATE_CURRENT.
	jobject pi = (*env)->CallStaticObjectMethod(env, cPI,
		(*env)->GetStaticMethodID(env, cPI, "getActivity",
			"(Landroid/content/Context;ILandroid/content/Intent;I)Landroid/app/PendingIntent;"),
		ctx, n, intent, 0x04000000 | 0x08000000);
	SYTUE_FALLA(env);

	jobject info = (*env)->CallObjectMethod(env, ctx,
		(*env)->GetMethodID(env, cCtx, "getApplicationInfo", "()Landroid/content/pm/ApplicationInfo;"));
	SYTUE_FALLA(env);
	jint icono = (*env)->GetIntField(env, info,
		(*env)->GetFieldID(env, (*env)->GetObjectClass(env, info), "icon", "I"));
	SYTUE_FALLA(env);

	const char *seq = "(Ljava/lang/CharSequence;)Landroid/app/Notification$Builder;";
	(*env)->CallObjectMethod(env, b, (*env)->GetMethodID(env, cBuilder, "setContentTitle", seq),
		(*env)->NewStringUTF(env, titulo));
	SYTUE_FALLA(env);
	(*env)->CallObjectMethod(env, b, (*env)->GetMethodID(env, cBuilder, "setContentText", seq),
		(*env)->NewStringUTF(env, cuerpo));
	SYTUE_FALLA(env);
	(*env)->CallObjectMethod(env, b,
		(*env)->GetMethodID(env, cBuilder, "setSmallIcon", "(I)Landroid/app/Notification$Builder;"), icono);
	SYTUE_FALLA(env);
	(*env)->CallObjectMethod(env, b,
		(*env)->GetMethodID(env, cBuilder, "setAutoCancel", "(Z)Landroid/app/Notification$Builder;"), JNI_TRUE);
	SYTUE_FALLA(env);
	(*env)->CallObjectMethod(env, b,
		(*env)->GetMethodID(env, cBuilder, "setContentIntent",
			"(Landroid/app/PendingIntent;)Landroid/app/Notification$Builder;"), pi);
	SYTUE_FALLA(env);
	jobject aviso = (*env)->CallObjectMethod(env, b,
		(*env)->GetMethodID(env, cBuilder, "build", "()Landroid/app/Notification;"));
	SYTUE_FALLA(env);
	(*env)->CallVoidMethod(env, mgr,
		(*env)->GetMethodID(env, cMgr, "notify", "(ILandroid/app/Notification;)V"), n, aviso);
	SYTUE_FALLA(env);
	(*env)->PopLocalFrame(env, NULL);
	return 0;
}
*/
import "C"

import (
	"errors"
	"sync/atomic"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// numeroAviso es el identificador de cada aviso, y también el `requestCode` de su
// PendingIntent: con el mismo, Android reutilizaría el enlace del anterior.
var numeroAviso atomic.Int32

// canalAvisos es el canal de Android de todos los avisos de la app.
const canalAvisos = "sytue-avisos"

// prepararAvisos crea el canal y, en Android 13+, pide POST_NOTIFICATIONS: sin él
// Android se calla los avisos sin dar ningún error.
func prepararAvisos(nombre string) error {
	if sistema.EnAndroid == nil {
		return sistema.ErrSinJava
	}
	canal, n := C.CString(canalAvisos), C.CString(nombre)
	defer C.sytueLibre(canal)
	defer C.sytueLibre(n)
	return sistema.EnAndroid(func(_, env, ctx uintptr) error {
		if C.sytuePrepararAvisos(C.uintptr_t(env), C.uintptr_t(ctx), canal, n) == 2 {
			return errors.New("escritorio: Android no ha dejado preparar los avisos")
		}
		return nil
	})
}

// avisar pone el aviso con un PendingIntent que abre su [EnlaceDeAviso] en esta app, y
// de ahí vuelve a [Pulsado] por el receptor de enlaces, como en Windows. Si Android mató
// el proceso, el mismo enlace arranca la app (`GoNativeActivity.onCreate` lo entrega) y
// el aviso lleva su destino.
func avisar(a Aviso) error {
	if a.Esquema == "" {
		return ErrNoDisponible
	}
	if sistema.EnAndroid == nil {
		return sistema.ErrSinJava
	}
	textos := []*C.char{
		C.CString(canalAvisos), C.CString(a.Nombre), C.CString(EnlaceDeAviso(a.Esquema, apuntar(a.AlPulsar), a.Destino)),
		C.CString(a.Titulo), C.CString(a.Cuerpo),
	}
	defer func() {
		for _, t := range textos {
			C.sytueLibre(t)
		}
	}()
	n := numeroAviso.Add(1)
	return sistema.EnAndroid(func(_, env, ctx uintptr) error {
		if C.sytueAvisarAndroid(C.uintptr_t(env), C.uintptr_t(ctx),
			textos[0], textos[1], textos[2], textos[3], textos[4], C.int(n)) != 0 {
			return errors.New("escritorio: Android no ha dejado poner el aviso")
		}
		return nil
	})
}

// El lanzador pone el punto él solo mientras haya avisos en la bandeja.
func insignia(string, int) error { return nil }

func hayBandeja() bool { return true }
