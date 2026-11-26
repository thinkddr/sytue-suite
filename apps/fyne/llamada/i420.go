// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package llamada

import (
	"image"

	"github.com/thesyncim/govpx"
)

// Aparte de video.go porque no es de la llamada: solo usa el descodificador VP9 en Go, y
// el reproductor de vídeo (`apps/fyne/video`) lo necesita también en la edición local
// (`edicionlocal`), donde la llamada no se compila.

// ImagenI420 copia un fotograma del descodificador a una imagen de Go. Copia porque el
// descodificador reutiliza sus planos en el siguiente.
//
// ponytail: una imagen nueva por fotograma (1,4 MB a 720p, ~40 MB/s a 30 fps). El
// recolector lo lleva sin despeinarse; si una rejilla grande se nota, dos búferes por
// pista y turnarlos.
func ImagenI420(f govpx.Image) *image.YCbCr {
	img := image.NewYCbCr(image.Rect(0, 0, f.Width, f.Height), image.YCbCrSubsampleRatio420)
	copiarPlano(img.Y, img.YStride, f.Y, f.YStride, f.Width, f.Height)
	cw, ch := (f.Width+1)/2, (f.Height+1)/2
	copiarPlano(img.Cb, img.CStride, f.U, f.UStride, cw, ch)
	copiarPlano(img.Cr, img.CStride, f.V, f.VStride, cw, ch)
	return img
}

func copiarPlano(dst []byte, dstStride int, src []byte, srcStride, ancho, alto int) {
	for y := range alto {
		copy(dst[y*dstStride:y*dstStride+ancho], src[y*srcStride:y*srcStride+ancho])
	}
}
