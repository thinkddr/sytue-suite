// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cast

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las dos posturas de un teléfono de 360 de ancho (el de la web a 360 px): de pie y
// tumbado.
var posturasMovil = []struct {
	nombre      string
	ancho, alto float32
}{
	{"vertical", 360, 760},
	{"horizontal", 760, 360},
}

// desbordado busca algo visible que acabe más allá del borde derecho de la pantalla (con
// medio punto de tolerancia por el redondeo) y lo describe; "" si todo cabe. Mira la
// posición ABSOLUTA de cada pieza ya colocada: el MinSize de una página no sirve, porque
// el marco la declara de ancho cero y se come el desbordamiento.
func desbordado(raiz fyne.CanvasObject, ancho float32) string {
	d := fyne.CurrentApp().Driver()
	var mirar func(o fyne.CanvasObject) string
	mirar = func(o fyne.CanvasObject) string {
		if o == nil || !o.Visible() {
			return ""
		}
		if o.Size().Width > 0 {
			if fin := d.AbsolutePositionForObject(o).X + o.Size().Width; fin > ancho+0.5 {
				if tx, ok := o.(*canvas.Text); ok {
					return fmt.Sprintf("el texto %q acaba en x=%.1f", tx.Text, fin)
				}
				return fmt.Sprintf("%T acaba en x=%.1f", o, fin)
			}
		}
		var hijos []fyne.CanvasObject
		switch x := o.(type) {
		case *fyne.Container:
			hijos = x.Objects
		case fyne.Widget:
			hijos = test.WidgetRenderer(x).Objects()
		}
		for _, h := range hijos {
			if f := mirar(h); f != "" {
				return f
			}
		}
		return ""
	}
	return mirar(raiz)
}

// TestLasPantallasEnElMovil pinta cada pantalla, en cada estado, en las dos posturas de
// un teléfono, y comprueba que NADA pide más ancho que la pantalla: en el teléfono la
// página solo se desplaza en vertical, así que lo que no cabe a lo ancho se corta.
//
// Con SYTUE_CAPTURA=/ruta deja las capturas para compararlas con la web a 360 px.
func TestLasPantallasEnElMovil(t *testing.T) {
	conVariante(t, tema.Claro)
	pantallas := []string{"inicio", "explorar", "canal", "ver", "directo", "subir", "gestionar", "emitir"}
	for _, pos := range posturasMovil {
		for _, p := range pantallas {
			for _, e := range estadosDePrueba {
				if e.e == Vacia && !pantallasConVacia[p] {
					continue
				}
				if e.e != Lista && p == "directo" {
					continue
				}
				v := vistaDePrueba(t, tema.Claro)
				v.Ventana, v.Movil = pos.ancho, true
				v.Estado = e.e
				v.AlReintentar, v.AlEntrar = func() {}, func() {}
				s := servicioCompleto()
				v.Teca = tecaConPosters(s.mios, s.org, s.directos, s.comp)
				o, sec := pantallaDe(t, p, v)
				w := test.NewWindow(enMarco(v, sec, o))
				w.SetPadded(false)
				w.Resize(fyne.NewSize(pos.ancho, pos.alto))
				if v.Estado == Lista {
					_ = w.Canvas().Capture()
					v.Teca.Esperar()
					o.Refresh()
				}
				img := w.Canvas().Capture()
				guardar(t, fmt.Sprintf("movil-%s-%s-%s", pos.nombre, p, e.nombre), img)
				if fuera := desbordado(w.Content(), pos.ancho); fuera != "" {
					t.Errorf("%s/%s/%s: se sale por la derecha de una pantalla de %v: %s", pos.nombre, p, e.nombre, pos.ancho, fuera)
				}
				w.Close()
			}
		}
	}
}
