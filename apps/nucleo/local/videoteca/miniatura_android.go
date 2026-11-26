// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package videoteca

import (
	"context"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// La miniatura del sistema en Android (CA-06): `MediaMetadataRetriever.getFrameAtTime` (o
// getScaledFrameAtTime desde la API 27), a un segundo si el vídeo dura más de tres, ya
// derecho y en JPEG, por el puente Java de la APK (`eu.sytue.nativo.VideoSistema.miniatura`,
// apps/fyne/android/java). Sin la máquina de Java o sin fotograma, no hay miniatura.

const claseVideo = "eu.sytue.nativo.VideoSistema"

// miniaturaAndroid es la miniatura de abs, que cabe en TamanoMiniatura, en JPEG.
func miniaturaAndroid(ctx context.Context, abs string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var datos []byte
	err := sistema.Java(func(env jni.Env, app jni.Object) error {
		cls, err := sistema.CargarClase(env, app, claseVideo)
		if err != nil {
			return err
		}
		o, err := jni.CallStaticObjectMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "miniatura", "(Ljava/lang/String;I)[B"),
			jni.Value(jni.JavaString(env, abs)), jni.Value(TamanoMiniatura))
		if err != nil || o == 0 {
			return err
		}
		datos = jni.GetByteArrayElements(env, jni.ByteArray(o))
		jni.DeleteLocalRef(env, o)
		return nil
	})
	if err != nil || len(datos) == 0 {
		return nil, ErrSinMiniatura
	}
	return datos, nil
}
