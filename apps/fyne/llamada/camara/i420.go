// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package camara

import "image"

// deI420 copia un I420 empaquetado (Y, luego U, luego V, sin relleno) a una imagen
// nueva: la fuente reutiliza su buffer en la siguiente lectura.
func deI420(b []byte, ancho, alto int) *image.YCbCr {
	img := I420(ancho, alto)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := range h {
		copy(img.Y[y*img.YStride:y*img.YStride+w], b[y*ancho:])
	}
	cw, ch, cs := w/2, h/2, ancho/2
	u, v := b[ancho*alto:], b[ancho*alto+cs*(alto/2):]
	for y := range ch {
		copy(img.Cb[y*img.CStride:y*img.CStride+cw], u[y*cs:])
		copy(img.Cr[y*img.CStride:y*img.CStride+cw], v[y*cs:])
	}
	return img
}

// DeYUYV pasa un fotograma YUYV (4:2:2 entrelazado, lo que dan casi todas las webcams
// USB por V4L2 y Media Foundation) a I420, promediando el croma de cada dos filas.
func DeYUYV(b []byte, ancho, alto int) *image.YCbCr {
	img := I420(ancho, alto)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	fila := ancho * 2
	for y := range h {
		for x := range w {
			img.Y[y*img.YStride+x] = b[y*fila+2*x]
		}
	}
	for y := range h / 2 {
		a, c := b[2*y*fila:], b[(2*y+1)*fila:]
		for x := range w / 2 {
			i := 4 * x
			img.Cb[y*img.CStride+x] = satura((int(a[i+1]) + int(c[i+1]) + 1) / 2)
			img.Cr[y*img.CStride+x] = satura((int(a[i+3]) + int(c[i+3]) + 1) / 2)
		}
	}
	return img
}

// DeNV12 pasa un NV12 (Y y luego CbCr entrelazado; lo que da Media Foundation y
// Camera2 casi siempre) a I420.
func DeNV12(b []byte, ancho, alto int) *image.YCbCr {
	img := I420(ancho, alto)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	for y := range h {
		copy(img.Y[y*img.YStride:y*img.YStride+w], b[y*ancho:])
	}
	uv := b[ancho*alto:]
	for y := range h / 2 {
		for x := range w / 2 {
			img.Cb[y*img.CStride+x] = uv[y*ancho+2*x]
			img.Cr[y*img.CStride+x] = uv[y*ancho+2*x+1]
		}
	}
	return img
}

// DeRGBA pasa una imagen RGBA (la pantalla) a I420 con los coeficientes BT.601 de rango
// limitado, que es lo que VP8 y el navegador suponen.
func DeRGBA(img *image.RGBA) *image.YCbCr {
	b := img.Rect
	out := I420(b.Dx(), b.Dy())
	w, h := out.Rect.Dx(), out.Rect.Dy()
	for y := range h {
		for x := range w {
			p := img.Pix[y*img.Stride+4*x:]
			r, g, bl := int(p[0]), int(p[1]), int(p[2])
			out.Y[y*out.YStride+x] = satura((66*r+129*g+25*bl+128)>>8 + 16)
		}
	}
	for y := range h / 2 {
		for x := range w / 2 {
			var r, g, bl int
			for _, d := range [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				p := img.Pix[(2*y+d[1])*img.Stride+4*(2*x+d[0]):]
				r, g, bl = r+int(p[0]), g+int(p[1]), bl+int(p[2])
			}
			r, g, bl = r/4, g/4, bl/4
			out.Cb[y*out.CStride+x] = satura((-38*r-74*g+112*bl+128)>>8 + 128)
			out.Cr[y*out.CStride+x] = satura((112*r-94*g-18*bl+128)>>8 + 128)
		}
	}
	return out
}

// Escalar reduce un fotograma al tamaño pedido conservando la proporción (cabe dentro),
// por vecino más cercano. No amplía: un fotograma que ya cabe sale tal cual.
//
// ponytail: vecino más cercano, sin filtro. Para una tesela de videollamada a 640×360 no
// se nota; si se ven dientes en líneas finas, un promedio de 2×2 en cada paso.
func Escalar(img *image.YCbCr, ancho, alto int) *image.YCbCr {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= ancho && h <= alto {
		return img
	}
	f := min(float64(ancho)/float64(w), float64(alto)/float64(h))
	out := I420(int(float64(w)*f), int(float64(h)*f))
	ow, oh := out.Rect.Dx(), out.Rect.Dy()
	for y := range oh {
		sy := y * h / oh
		for x := range ow {
			out.Y[y*out.YStride+x] = img.Y[sy*img.YStride+x*w/ow]
		}
	}
	for y := range oh / 2 {
		sy := y * (h / 2) / (oh / 2)
		for x := range ow / 2 {
			sx := x * (w / 2) / (ow / 2)
			out.Cb[y*out.CStride+x] = img.Cb[sy*img.CStride+sx]
			out.Cr[y*out.CStride+x] = img.Cr[sy*img.CStride+sx]
		}
	}
	return out
}

// A420 pasa una imagen YCbCr de cualquier submuestreo (el JPEG de una webcam en MJPEG
// suele ser 4:2:2) a 4:2:0, tomando el croma de la fila par.
func A420(img *image.YCbCr) *image.YCbCr {
	if img.SubsampleRatio == image.YCbCrSubsampleRatio420 && img.Rect.Min == (image.Point{}) {
		return img
	}
	b := img.Rect
	out := I420(b.Dx(), b.Dy())
	w, h := out.Rect.Dx(), out.Rect.Dy()
	for y := range h {
		copy(out.Y[y*out.YStride:y*out.YStride+w], img.Y[img.YOffset(b.Min.X, b.Min.Y+y):])
	}
	for y := range h / 2 {
		for x := range w / 2 {
			i := img.COffset(b.Min.X+2*x, b.Min.Y+2*y)
			out.Cb[y*out.CStride+x], out.Cr[y*out.CStride+x] = img.Cb[i], img.Cr[i]
		}
	}
	return out
}

// satura recorta a un byte: con estos coeficientes nunca sale de rango, pero el
// compilador no lo sabe y un redondeo no debe dar la vuelta a 0.
func satura(v int) uint8 { return uint8(max(0, min(255, v))) } //nolint:gosec // G115: recortado a [0, 255]
