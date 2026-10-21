// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"hash/fnv"
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
)

// TestRecorridoConSesion pulsa todo lo que se ve con los datos del servicio falso y
// cierra con Escape cada modal que se abre (ver `ui/recorrido`).
func TestRecorridoConSesion(t *testing.T) {
	a, w := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(30), porPagina: 10})
	a.Arrancar()
	recorrido.Comprobar(t, w, 400)
}

// escenasPlay: con SYTUE_CAPTURAS_PLAY las miniaturas son paisajes pintados en vez de
// «JPEG», que el visor no sabe enseñar: una galería de cuadros grises no es una captura.
var escenasPlay = os.Getenv("SYTUE_CAPTURAS_PLAY") != ""

// escena es un paisaje sencillo —cielo en degradado, sol y dos lomas— con el tono sacado
// de la ruta, para que cada foto sea distinta y la misma foto salga siempre igual.
func escena(ruta string, ancho, alto int) image.Image {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ruta))
	n := h.Sum32()
	tono := float64(n%360) / 360
	m := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	solX, solY := float64(ancho)*(0.2+float64(n%60)/100), float64(alto)*0.3
	for y := range alto {
		for x := range ancho {
			fy := float64(y) / float64(alto)
			c := hsl(tono, 0.55, 0.78-0.25*fy)
			if math.Hypot(float64(x)-solX, float64(y)-solY) < float64(alto)/10 {
				c = color.RGBA{255, 244, 214, 255}
			}
			lejos := 0.62 + 0.06*math.Sin(float64(x)/float64(ancho)*5+float64(n%7))
			cerca := 0.78 + 0.05*math.Sin(float64(x)/float64(ancho)*3+float64(n%11))
			switch {
			case fy > cerca:
				c = hsl(math.Mod(tono+0.3, 1), 0.45, 0.30)
			case fy > lejos:
				c = hsl(math.Mod(tono+0.35, 1), 0.35, 0.45)
			}
			m.Set(x, y, c)
		}
	}
	return m
}

func hsl(h, s, l float64) color.RGBA {
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * math.Min(l, 1-l)
		return uint8(255 * (l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))))
	}
	return color.RGBA{f(0), f(8), f(4), 255}
}
