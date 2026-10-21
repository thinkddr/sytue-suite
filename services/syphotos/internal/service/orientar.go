package service

import (
	"image"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// orientar deja la imagen «derecha» según la orientación EXIF: primero el
// espejo y luego el giro, que es como la tabla de domain.Orientacion define
// cada valor. Las de identidad devuelven la misma imagen sin copiar.
func orientar(src image.Image, o domain.Orientacion) image.Image {
	t := o.Transformacion()
	if t.Grados == 0 && !t.EspejoH {
		return src
	}
	out := src
	if t.EspejoH {
		out = espejar(out, true)
	}
	return girar(out, t.Grados)
}

// espejar refleja en horizontal (izquierda↔derecha) o en vertical.
func espejar(src image.Image, horizontal bool) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if horizontal {
				m.Set(x, y, src.At(b.Min.X+w-1-x, b.Min.Y+y))
			} else {
				m.Set(x, y, src.At(b.Min.X+x, b.Min.Y+h-1-y))
			}
		}
	}
	return m
}

// girar rota en sentido horario 90, 180 o 270 grados; 0 devuelve la misma imagen.
func girar(src image.Image, grados int) image.Image {
	grados = ((grados % 360) + 360) % 360
	if grados == 0 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	var dst *image.NRGBA
	if grados == 90 || grados == 270 {
		dst = image.NewNRGBA(image.Rect(0, 0, h, w))
	} else {
		dst = image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	for y := range h {
		for x := range w {
			c := src.At(b.Min.X+x, b.Min.Y+y)
			switch grados {
			case 90:
				dst.Set(h-1-y, x, c)
			case 180:
				dst.Set(w-1-x, h-1-y, c)
			case 270:
				dst.Set(y, w-1-x, c)
			}
		}
	}
	return dst
}
