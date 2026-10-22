// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/canvas"
)

type objetoZoom struct {
	*canvas.Rectangle
	refrescos int
}

func (o *objetoZoom) Refresh() { o.refrescos++; o.Rectangle.Refresh() }

func TestElZoomInvalidaElLienzoSoloCuandoCambia(t *testing.T) {
	for _, tipo := range []Tipo{Documento, Presentacion} {
		o := &objetoZoom{Rectangle: canvas.NewRectangle(color.Transparent)}
		c := &Contexto{Tipo: tipo, Zoom: 100, Estado: Listo, Lienzo: &lienzoFijo{o}}
		NuevoMarco(c, Genericas(), nil)
		antes := o.refrescos
		c.PonerZoom(50)
		if o.refrescos != antes+1 {
			t.Errorf("%v: cambiar zoom refresca el lienzo%d veces", tipo, o.refrescos-antes)
		}
		c.PonerZoom(50)
		c.Refrescar() // el mismo camino que usa una edición o un cambio del chip de guardado
		if o.refrescos != antes+1 {
			t.Errorf("%v: recomponer sin cambiar zoom invalida el lienzo%d veces", tipo, o.refrescos-antes)
		}
	}
}
