// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package certipdf

import (
	"image"
	"image/color"
	"math"

	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

/*
Los TRAZOS de la capa de marcas: lo que en la web es la `<svg>` de encima de la hoja
—`<polyline>`, `<line>`, `<ellipse>` y `<rect>`, con `stroke-linecap="round"` y, en las
anotaciones ya puestas, `stroke-dasharray="3 2"`—.

Se pintan con `rasterx`, que ya está en `go.mod` por los iconos y es el MISMO trazador
que usa Fyne por dentro: da uniones y remates redondos, discontinuo y elipses, que
`canvas.Line` (remate recto) y `canvas.Circle` (siempre un círculo) no dan.

Todo se escribe en PUNTOS del SVG de la página, como la web (`viewBox` en puntos): el
pincel sabe cuántos píxeles de dispositivo hay por punto (`k`) y dónde cae el origen del
lienzo que pinta (`ox`, `oy`), así que el grosor y el discontinuo escalan con el zoom
igual que un `stroke-width` dentro de un `viewBox`.
*/

// pincel pinta sobre `img` con `k` píxeles por punto y el origen del lienzo en (ox, oy)
// puntos del SVG.
type pincel struct {
	img    *image.RGBA
	k      float64
	ox, oy float64
}

func nuevoPincel(w, h int, k, ox, oy float64) pincel {
	return pincel{img: image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1))), k: k, ox: ox, oy: oy}
}

func (p pincel) punto(x, y float64) fixed.Point26_6 {
	return rasterx.ToFixedP((x-p.ox)*p.k, (y-p.oy)*p.k)
}

// trazador prepara un trazo de SVG. `redondo` es `stroke-linecap="round"` y
// `stroke-linejoin="round"` (la `<polyline>` del lápiz y la `<line>`); sin él, lo de
// serie de SVG: remate recto (`butt`) y unión en inglete con límite 4, que es lo que
// llevan el `<rect>`, la `<ellipse>` y el discontinuo de las anotaciones.
func (p pincel) trazador(c color.Color, grosor float64, redondo bool, discontinuo []float64) *rasterx.Dasher {
	b := p.img.Bounds()
	d := rasterx.NewDasher(b.Dx(), b.Dy(), rasterx.NewScannerGV(b.Dx(), b.Dy(), p.img, b))
	guiones := make([]float64, 0, len(discontinuo))
	for _, g := range discontinuo {
		guiones = append(guiones, g*p.k)
	}
	remate, hueco, union := rasterx.ButtCap, rasterx.FlatGap, rasterx.Miter
	if redondo {
		remate, hueco, union = rasterx.RoundCap, rasterx.RoundGap, rasterx.Round
	}
	d.SetStroke(fixed.Int26_6(grosor*p.k*64), 4<<6, remate, remate, hueco, union, guiones, 0)
	d.SetColor(c)
	return d
}

// linea es una `<polyline>` (o una `<line>` con dos puntos).
func (p pincel) linea(puntos [][2]float64, c color.Color, grosor float64) {
	if len(puntos) < 2 {
		return
	}
	d := p.trazador(c, grosor, true, nil)
	d.Start(p.punto(puntos[0][0], puntos[0][1]))
	for _, q := range puntos[1:] {
		d.Line(p.punto(q[0], q[1]))
	}
	d.Stop(false)
	d.Draw()
}

// caja es un `<rect>`: relleno (si no es nil) y contorno (si el grosor es > 0), con el
// trazo CENTRADO en el borde, como en SVG.
func (p pincel) caja(x, y, w, h float64, relleno, contorno color.Color, grosor float64, discontinuo []float64) {
	if relleno != nil {
		b := p.img.Bounds()
		f := rasterx.NewFiller(b.Dx(), b.Dy(), rasterx.NewScannerGV(b.Dx(), b.Dy(), p.img, b))
		f.SetColor(relleno)
		rasterx.AddRect((x-p.ox)*p.k, (y-p.oy)*p.k, (x+w-p.ox)*p.k, (y+h-p.oy)*p.k, 0, f)
		f.Draw()
	}
	if contorno == nil || grosor <= 0 {
		return
	}
	d := p.trazador(contorno, grosor, false, discontinuo)
	d.Start(p.punto(x, y))
	d.Line(p.punto(x+w, y))
	d.Line(p.punto(x+w, y+h))
	d.Line(p.punto(x, y+h))
	d.Stop(true)
	d.Draw()
}

// elipse es un `<ellipse>` sin relleno inscrito en la caja.
func (p pincel) elipse(x, y, w, h float64, c color.Color, grosor float64) {
	d := p.trazador(c, grosor, false, nil)
	rasterx.AddEllipse((x+w/2-p.ox)*p.k, (y+h/2-p.oy)*p.k, w/2*p.k, h/2*p.k, 0, d)
	d.Draw()
}

// cajaDe es la caja (x, y, ancho, alto) de un arrastre, sea cual sea la esquina de la
// que salió.
func cajaDe(x0, y0, x1, y1 float64) (x, y, w, h float64) {
	return math.Min(x0, x1), math.Min(y0, y1), math.Abs(x1 - x0), math.Abs(y1 - y0)
}
