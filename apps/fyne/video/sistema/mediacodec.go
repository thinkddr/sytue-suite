// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package sistema

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/video"
)

// # Lo que da MediaCodec (Android)
//
// El descodificador de Android es Java (`eu.sytue.nativo.VideoSistema`, en
// apps/fyne/android/java) y lo llama mediacodec_android.go por JNI. Aquí está cómo se lee lo
// que devuelve, fuera del fichero de Android para probarlo en cualquier sistema
// (mediacodec_test.go, que también ata estas constantes a las del Java).

// Lo que devuelve VideoSistema.abrir cuando no puede (NO_LEE, SIN_PISTAS, SIN_CODEC, FALLO).
const (
	javaNoLee     = -1
	javaSinPistas = -2
	javaSinCodec  = -3
	javaFallo     = -4
)

// Cada muestra de VideoSistema.siguiente: una cabecera de 20 bytes en orden de red (tipo,
// instante en µs y dos enteros: ancho y alto de la imagen, o cuántas muestras de sonido y
// 0) y detrás la imagen en I420 sin relleno o el PCM mono de 16 bits en orden de red.
const (
	javaImagen     = 1
	javaSonido     = 2
	javaCabecera   = 20
	javaLadoMaximo = 16384
)

// ErrSinDescodificador: el teléfono no tiene descodificador para el formato del fichero.
var ErrSinDescodificador = errors.New("sistema: este teléfono no tiene descodificador para este formato")

// errorDeAbrir traduce lo que devuelve VideoSistema.abrir cuando no puede.
func errorDeAbrir(codigo int) error {
	switch codigo {
	case javaNoLee:
		return errors.New("sistema: Android no lee este fichero")
	case javaSinPistas:
		return errors.New("sistema: el fichero no trae vídeo ni sonido")
	case javaSinCodec:
		return ErrSinDescodificador
	case javaFallo:
		return errors.New("sistema: el descodificador de Android no dio ningún fotograma")
	}
	return fmt.Errorf("sistema: MediaCodec no abrió el fichero (%d)", codigo)
}

// infoDeJava lee «duración en µs\tancho\talto\tvídeo\tsonido» de VideoSistema.info.
func infoDeJava(s string) (video.InfoSistema, error) {
	p := strings.Split(s, "\t")
	if len(p) != 5 {
		return video.InfoSistema{}, fmt.Errorf("sistema: MediaCodec dice algo ilegible del fichero: %q", s)
	}
	var n [5]int64
	for i, x := range p {
		v, err := strconv.ParseInt(x, 10, 64)
		if err != nil {
			return video.InfoSistema{}, fmt.Errorf("sistema: MediaCodec dice algo ilegible del fichero: %q", s)
		}
		n[i] = v
	}
	in := video.InfoSistema{
		Duracion: time.Duration(n[0]) * time.Microsecond,
		Ancho:    int(n[1]),
		Alto:     int(n[2]),
		ConVideo: n[3] == 1,
		ConAudio: n[4] == 1,
	}
	switch {
	case !in.ConVideo && !in.ConAudio:
		return in, errors.New("sistema: MediaCodec no da ni imagen ni sonido")
	case in.ConVideo && (in.Ancho <= 0 || in.Alto <= 0 || in.Ancho > javaLadoMaximo || in.Alto > javaLadoMaximo):
		return in, fmt.Errorf("sistema: MediaCodec dice un tamaño imposible: %d×%d", in.Ancho, in.Alto)
	}
	if in.Duracion < 0 {
		in.Duracion = 0
	}
	return in, nil
}

// muestraDeJava lee una muestra de VideoSistema.siguiente. La imagen usa los bytes tal cual
// (sin copiarlos otra vez): son ya una copia de Go del byte[] de Java.
func muestraDeJava(b []byte) (video.Muestra, error) {
	if len(b) < javaCabecera {
		return video.Muestra{}, fmt.Errorf("sistema: muestra de MediaCodec corta (%d bytes)", len(b))
	}
	entero := func(i int) int { return int(int32(binary.BigEndian.Uint32(b[i:]))) } //nolint:gosec // G115: los int de Java son con signo
	tipo := entero(0)
	t := time.Duration(int64(binary.BigEndian.Uint64(b[4:]))) * time.Microsecond //nolint:gosec // G115: el long de Java es con signo
	a, c := entero(12), entero(16)
	d := b[javaCabecera:]
	switch tipo {
	case javaImagen:
		if a <= 0 || c <= 0 || a > javaLadoMaximo || c > javaLadoMaximo {
			return video.Muestra{}, fmt.Errorf("sistema: fotograma de MediaCodec de %d×%d", a, c)
		}
		cw, ch := (a+1)/2, (c+1)/2
		ny, nc := a*c, cw*ch
		if len(d) != ny+2*nc {
			return video.Muestra{}, fmt.Errorf("sistema: fotograma de MediaCodec de %d bytes para %d×%d", len(d), a, c)
		}
		return video.Muestra{Tiempo: t, Imagen: &image.YCbCr{
			Y: d[:ny:ny], Cb: d[ny : ny+nc : ny+nc], Cr: d[ny+nc : ny+2*nc : ny+2*nc],
			YStride: a, CStride: cw, SubsampleRatio: image.YCbCrSubsampleRatio420, Rect: image.Rect(0, 0, a, c),
		}}, nil
	case javaSonido:
		if a < 0 || len(d) != 2*a {
			return video.Muestra{}, fmt.Errorf("sistema: sonido de MediaCodec de %d bytes para %d muestras", len(d), a)
		}
		pcm := make([]int16, a)
		for i := range pcm {
			pcm[i] = int16(binary.BigEndian.Uint16(d[2*i:])) //nolint:gosec // G115: PCM de 16 bits con signo
		}
		return video.Muestra{Tiempo: t, PCM: pcm}, nil
	}
	return video.Muestra{}, fmt.Errorf("sistema: muestra de MediaCodec de tipo %d", tipo)
}
