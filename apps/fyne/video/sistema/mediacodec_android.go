// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sistema

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
	"git.sytue.eu/sytue/one/apps/fyne/video"
	puente "git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// # MediaCodec (Android)
//
// En Android el descodificador del sistema es `MediaExtractor` + `MediaCodec`, por el puente
// Java de la APK (`eu.sytue.nativo.VideoSistema`, apps/fyne/android/java): cada fotograma
// llega en I420 ya derecho (con el giro de la pista, el de los vídeos grabados en vertical) y
// el sonido en PCM mono de 16 bits a llamada.Muestreo, cada cosa con su instante. Buscar es
// `seekTo` al punto de acceso anterior y descartar en Java lo de antes. Si el teléfono no sabe
// (no tiene descodificador para el formato, o no da ni un fotograma), abrir falla y se sigue
// como sin descodificador: lo que no es WebM se abre con el reproductor del sistema.
//
// Cada llamada va por `sistema.Java` (un hilo enganchado a la máquina, y suelto al acabar),
// una por muestra; los fotogramas pasan como byte[] y se leen sin otra copia.

const claseVideo = "eu.sytue.nativo.VideoSistema"

// Registrar registra MediaCodec en el reproductor y dice que lo está. Que un fichero lo lea
// se sabe al abrirlo.
func Registrar() bool {
	video.RegistrarSistema(AbrirMediaCodec)
	return true
}

func llamarVideo(f func(env jni.Env, cls jni.Class) error) error {
	return puente.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := puente.CargarClase(env, ctx, claseVideo)
		if err != nil {
			return err
		}
		return f(env, cls)
	})
}

// mediaCodec es un fichero abierto con MediaCodec: cumple video.Sistema.
type mediaCodec struct {
	mu   sync.Mutex
	id   int // el número de VideoSistema; 0, cerrado
	info video.InfoSistema
}

var _ video.Sistema = (*mediaCodec)(nil)

// AbrirMediaCodec abre ruta con MediaExtractor + MediaCodec. Ya trae descodificado el primer
// fotograma: un error es que este teléfono no lo sabe reproducir.
func AbrirMediaCodec(ruta string) (video.Sistema, error) {
	if _, err := os.Stat(ruta); err != nil {
		return nil, err
	}
	id := 0
	var info string
	err := llamarVideo(func(env jni.Env, cls jni.Class) error {
		var err error
		id, err = jni.CallStaticIntMethod(env, cls, jni.GetStaticMethodID(env, cls, "abrir", "(Ljava/lang/String;I)I"),
			jni.Value(jni.JavaString(env, ruta)), jni.Value(llamada.Muestreo))
		if err != nil || id <= 0 {
			return err
		}
		o, err := jni.CallStaticObjectMethod(env, cls, jni.GetStaticMethodID(env, cls, "info", "(I)Ljava/lang/String;"), jni.Value(id))
		if err == nil && o != 0 {
			info = jni.GoString(env, jni.String(o))
		}
		return err
	})
	if err != nil {
		if id > 0 {
			_ = cerrarEnJava(id)
		}
		return nil, fmt.Errorf("sistema: MediaCodec: %w", err)
	}
	if id <= 0 {
		return nil, errorDeAbrir(id)
	}
	in, err := infoDeJava(info)
	if err != nil {
		_ = cerrarEnJava(id)
		return nil, err
	}
	return &mediaCodec{id: id, info: in}, nil
}

// Info es lo que dijo MediaCodec al abrir (el tamaño, ya girado).
func (m *mediaCodec) Info() video.InfoSistema { return m.info }

// Siguiente es la imagen o el trozo de sonido que va antes.
func (m *mediaCodec) Siguiente() (video.Muestra, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.id == 0 {
		return video.Muestra{}, io.EOF
	}
	var b []byte
	fin := false
	err := llamarVideo(func(env jni.Env, cls jni.Class) error {
		o, err := jni.CallStaticObjectMethod(env, cls, jni.GetStaticMethodID(env, cls, "siguiente", "(I)[B"), jni.Value(m.id)) //nolint:gosec // G115: un número > 0 de VideoSistema, un jint
		if err != nil {
			return err
		}
		if o == 0 {
			fin = true
			return nil
		}
		b = jni.GetByteArrayElements(env, jni.ByteArray(o))
		jni.DeleteLocalRef(env, o)
		return nil
	})
	switch {
	case err != nil:
		return video.Muestra{}, fmt.Errorf("sistema: MediaCodec: %w", err)
	case fin:
		return video.Muestra{}, io.EOF
	}
	return muestraDeJava(b)
}

// Buscar salta a t: lo de antes no llega.
func (m *mediaCodec) Buscar(t time.Duration) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.id == 0 {
		return 0, errors.New("sistema: MediaCodec cerrado")
	}
	if d := m.info.Duracion; d > 0 && t > d {
		t = d
	}
	ms := min(max(t, 0).Milliseconds(), math.MaxInt32)
	r := -1
	err := llamarVideo(func(env jni.Env, cls jni.Class) error {
		var err error
		r, err = jni.CallStaticIntMethod(env, cls, jni.GetStaticMethodID(env, cls, "buscar", "(II)I"), jni.Value(m.id), jni.Value(ms)) //nolint:gosec // G115: jint > 0 y ms acotado a MaxInt32
		return err
	})
	switch {
	case err != nil:
		return 0, fmt.Errorf("sistema: MediaCodec: %w", err)
	case r < 0:
		return 0, errors.New("sistema: MediaCodec no pudo buscar")
	}
	return time.Duration(r) * time.Millisecond, nil
}

// Close suelta el códec y los extractores.
func (m *mediaCodec) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.id == 0 {
		return nil
	}
	id := m.id
	m.id = 0
	return cerrarEnJava(id)
}

func cerrarEnJava(id int) error {
	return llamarVideo(func(env jni.Env, cls jni.Class) error {
		return jni.CallStaticVoidMethod(env, cls, jni.GetStaticMethodID(env, cls, "cerrar", "(I)V"), jni.Value(id)) //nolint:gosec // G115: un número > 0 de VideoSistema, un jint
	})
}
