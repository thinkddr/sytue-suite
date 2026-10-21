// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package camara

import "image"

// # Poner derecho lo que da la cámara de un teléfono
//
// El sensor de casi todas las cámaras de teléfono va montado tumbado (90 o 270 grados), y
// Camera2 entrega los fotogramas como los ve el sensor. Sin girarlos, con el teléfono de
// pie el directo y la videollamada salían de lado. El Java dice el giro del sensor, si es
// la delantera y cómo está girada la pantalla; aquí se decide y se gira, que se prueba
// sin teléfono.

// giroPara es cuánto hay que girar, en el sentido del reloj, lo que da un sensor montado
// a `sensor` grados cuando la pantalla está girada `pantalla` grados: la fórmula de la
// documentación de Camera2 (`CaptureRequest.JPEG_ORIENTATION`). La delantera gira al
// revés porque mira hacia quien la sostiene.
func giroPara(sensor int, delantera bool, pantalla int) int {
	if delantera {
		return ((sensor+pantalla)%360 + 360) % 360
	}
	return ((sensor-pantalla)%360 + 360) % 360
}

// girar devuelve el fotograma girado 90, 180 o 270 grados en el sentido del reloj; con
// cualquier otro valor, el mismo.
func girar(img *image.YCbCr, grados int) *image.YCbCr {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	var out *image.YCbCr
	switch grados {
	case 90, 270:
		out = I420(h, w)
	case 180:
		out = I420(w, h)
	default:
		return img
	}
	plano(img.Y, img.YStride, out.Y, out.YStride, w, h, grados)
	plano(img.Cb, img.CStride, out.Cb, out.CStride, w/2, h/2, grados)
	plano(img.Cr, img.CStride, out.Cr, out.CStride, w/2, h/2, grados)
	return out
}

// plano gira un plano de w×h con sus pasos de fila.
func plano(s []byte, ps int, d []byte, pd, w, h, grados int) {
	for y := range h {
		for x := range w {
			var dx, dy int
			switch grados {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			default: // 270
				dx, dy = y, w-1-x
			}
			d[dy*pd+dx] = s[y*ps+x]
		}
	}
}
