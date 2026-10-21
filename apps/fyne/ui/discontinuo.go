// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BordeDiscontinuo es `border-dashed`, que `canvas.Rectangle` no sabe pintar: un borde de
// Grosor px de CSS en una caja de esquinas de Radio, con el patrón de Chromium. Medido en
// Chromium (la zona de abrir y el botón de descargar de CertiPDF, la zona de soltar de
// Photos): trazos de 3 veces el grosor cada 5, empezando donde acaba la esquina, y las
// esquinas enteras —con 2 px, 6 de trazo y 4 de hueco; con 1 px, 3 y 2—.
//
// No ocupa sitio (MinSize cero) ni recibe el puntero: va en una pila, encima del fondo de
// la caja, y el contenido lleva su propio margen del grosor. Se pinta píxel a píxel con
// cobertura de 4×4, que es el antialias del navegador en las esquinas.
type BordeDiscontinuo struct {
	widget.BaseWidget
	Grosor, Radio float32
	// Token es el color, un token del tema; vacío, `border`.
	Token string
}

var _ fyne.Widget = (*BordeDiscontinuo)(nil)

// NuevoBordeDiscontinuo crea el borde.
func NuevoBordeDiscontinuo(grosor, radio float32, token string) *BordeDiscontinuo {
	b := &BordeDiscontinuo{Grosor: grosor, Radio: radio, Token: token}
	b.ExtendBaseWidget(b)
	return b
}

// EnBordeDiscontinuo dice si el punto (x, y), en px de CSS, cae en un trazo de un borde
// discontinuo de ese grosor y ese radio en una caja de w×h. Es la única cuenta: la usa
// el pintado y la comprueban los guardianes.
func EnBordeDiscontinuo(x, y, w, h, grosor, radio float64) bool {
	b, rad := grosor, radio
	// Dentro de la caja redondeada exterior y fuera de la interior.
	if !enRedondeado(x, y, 0, 0, w, h, rad) || enRedondeado(x, y, b, b, w-b, h-b, rad-b) {
		return false
	}
	trazo, periodo := 3*b, 5*b
	switch {
	case x >= rad && x <= w-rad:
		return math.Mod(x-rad, periodo) < trazo
	case y >= rad && y <= h-rad:
		return math.Mod(y-rad, periodo) < trazo
	}
	return true // las esquinas, enteras
}

// enRedondeado dice si (x, y) cae dentro del rectángulo (x0,y0)-(x1,y1) de esquinas de
// radio `rad`.
func enRedondeado(x, y, x0, y0, x1, y1, rad float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	rad = max(rad, 0)
	cx := min(max(x, x0+rad), x1-rad)
	cy := min(max(y, y0+rad), y1-rad)
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= rad*rad
}

// CreateRenderer es un ráster que pinta píxel a píxel.
func (b *BordeDiscontinuo) CreateRenderer() fyne.WidgetRenderer {
	r := &bordeDiscontinuoRenderer{b: b}
	r.raster = canvas.NewRasterWithPixels(r.pixel)
	r.Refresh()
	return r
}

type bordeDiscontinuoRenderer struct {
	b      *BordeDiscontinuo
	raster *canvas.Raster
	tinta  color.NRGBA
}

func (r *bordeDiscontinuoRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }
func (r *bordeDiscontinuoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.raster}
}
func (r *bordeDiscontinuoRenderer) Destroy()           {}
func (r *bordeDiscontinuoRenderer) Layout(t fyne.Size) { r.raster.Resize(t) }

func (r *bordeDiscontinuoRenderer) Refresh() {
	token := r.b.Token
	if token == "" {
		token = "border"
	}
	r.tinta = tema.Color(token, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r.raster.Refresh()
}

// pixel dice cuánto del píxel cae en un trazo del borde.
func (r *bordeDiscontinuoRenderer) pixel(x, y, w, h int) color.Color {
	escala := float64(w) / float64(max(r.raster.Size().Width, 1))
	g, rad := float64(r.b.Grosor), float64(r.b.Radio)
	var dentro int
	for sy := range 4 {
		for sx := range 4 {
			px := (float64(x) + (float64(sx)+0.5)/4) / escala
			py := (float64(y) + (float64(sy)+0.5)/4) / escala
			if EnBordeDiscontinuo(px, py, float64(w)/escala, float64(h)/escala, g, rad) {
				dentro++
			}
		}
	}
	c := r.tinta
	c.A = uint8(uint32(c.A) * uint32(dentro) / 16) //nolint:gosec // G115: dentro va de 0 a 16
	return c
}
