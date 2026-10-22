// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

/*
El `shadow-sm` del PAPEL (`border bg-white shadow-sm` de la `Hoja`).

No es `ui.Sombra` a propósito, y no por copiar: esa desenfoca un plano del tamaño del
panel, y aquí el panel es una página a pantalla —siete megapíxeles a ×2—, una por hoja,
que se rehace cada vez que el ajuste al ancho cambia el zoom (en cada píxel de arrastrar
el borde de la ventana). El papel no tiene esquinas redondeadas, y la sombra de un
rectángulo es SEPARABLE: la gaussiana de una caja es el producto de dos funciones de
error, una por eje, así que cada píxel se calcula directamente y sin desenfocar nada. Y
solo se pinta el anillo que asoma fuera del papel (en CSS, `box-shadow` no se pinta
debajo de la caja): cuatro tiras de seis píxeles.
*/

// margenSombraPapel es lo que la sombra asoma: 3σ de la capa más ancha (σ = 1,5) y su
// desplazamiento de 1.
const margenSombraPapel = 6

// sombraPapel son las cuatro tiras de la sombra alrededor de un papel.
type sombraPapel struct {
	tiras [4]*canvas.Raster
	capas []tema.CapaSombra
	// ancho y alto son los del papel, en px de CSS.
	ancho, alto float32
}

func nuevaSombraPapel() *sombraPapel {
	s := &sombraPapel{}
	for i := range s.tiras {
		lado := i
		s.tiras[i] = canvas.NewRaster(func(w, h int) image.Image { return s.pintar(lado, w, h) })
	}
	return s
}

func (s *sombraPapel) Objetos() []fyne.CanvasObject {
	return []fyne.CanvasObject{s.tiras[0], s.tiras[1], s.tiras[2], s.tiras[3]}
}

// Colocar las pone alrededor de un papel de `ancho`×`alto` con la esquina en (0, 0).
func (s *sombraPapel) Colocar(ancho, alto float32, v tema.Variante) {
	cambia := s.ancho != ancho || s.alto != alto || len(s.capas) == 0 || s.capas[0] != tema.Sombra(tema.SombraSM, v)[0]
	s.capas, s.ancho, s.alto = tema.Sombra(tema.SombraSM, v), ancho, alto
	for i, r := range s.cajas() {
		s.tiras[i].Move(fyne.NewPos(r[0], r[1]))
		s.tiras[i].Resize(fyne.NewSize(r[2], r[3]))
		if cambia {
			s.tiras[i].Refresh()
		}
	}
}

// cajas son arriba, abajo, izquierda y derecha: (x, y, ancho, alto) en px de CSS.
func (s *sombraPapel) cajas() [4][4]float32 {
	m := float32(margenSombraPapel)
	return [4][4]float32{
		{-m, -m, s.ancho + 2*m, m},
		{-m, s.alto, s.ancho + 2*m, m},
		{-m, 0, m, s.alto},
		{s.ancho, 0, m, s.alto},
	}
}

// AlfaSombra es la sombra en (x, y) —px de CSS, con el papel en (0, 0)-(ancho, alto)—:
// cada capa es su color por la gaussiana de la caja desplazada, y la PRIMERA va encima.
func AlfaSombra(capas []tema.CapaSombra, ancho, alto, x, y float64) color.NRGBA {
	var r, g, b, a float64 // premultiplicado
	for i := len(capas) - 1; i >= 0; i-- {
		c := capas[i]
		sigma := float64(c.Desviacion())
		cobertura := perfil(x-float64(c.DesplX), ancho, sigma) * perfil(y-float64(c.DesplY), alto, sigma)
		ca := float64(c.Color.A) / 255 * cobertura
		r = float64(c.Color.R)/255*ca + r*(1-ca)
		g = float64(c.Color.G)/255*ca + g*(1-ca)
		b = float64(c.Color.B)/255*ca + b*(1-ca)
		a = ca + a*(1-ca)
	}
	if a <= 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: uint8(r/a*255 + 0.5), G: uint8(g/a*255 + 0.5), B: uint8(b/a*255 + 0.5), A: uint8(a*255 + 0.5)}
}

// perfil es la gaussiana de desviación σ de un segmento [0, largo] en el punto t.
func perfil(t, largo, sigma float64) float64 {
	if sigma <= 0 {
		if t >= 0 && t < largo {
			return 1
		}
		return 0
	}
	k := math.Sqrt2 * sigma
	return (math.Erf(t/k) - math.Erf((t-largo)/k)) / 2
}

// pintar es el generador de una tira, en píxeles de dispositivo.
func (s *sombraPapel) pintar(lado, w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	r := s.cajas()[lado]
	if w <= 0 || h <= 0 || r[2] <= 0 || r[3] <= 0 {
		return img
	}
	kx, ky := float64(r[2])/float64(w), float64(r[3])/float64(h)
	for py := range h {
		for px := range w {
			x := float64(r[0]) + (float64(px)+0.5)*kx
			y := float64(r[1]) + (float64(py)+0.5)*ky
			img.SetNRGBA(px, py, AlfaSombra(s.capas, float64(s.ancho), float64(s.alto), x, y))
		}
	}
	return img
}
