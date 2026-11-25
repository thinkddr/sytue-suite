// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// TestAnyadirFotosEsElMasFlotante: «Añadir fotos» es el «+» flotante de abajo a la derecha
// en la galería, también en escritorio (producto, 2026-11-25; el guardián web es
// `boton-flotante.test.ts`): la barra de la galería ya no lo lleva, y en los álbumes no
// está. Pulsarlo abre el selector de ficheros.
//
// VISTO EN ROJO con el botón relleno de la barra de antes y con la capa a la vista en los
// álbumes.
func TestAnyadirFotosEsElMasFlotante(t *testing.T) {
	a, w := appDePrueba(t, &servicioFalso{fotos: fotosDelServicio(12), porPagina: 100})
	elegido := make(chan struct{}, 1)
	a.Elegir = func() ([]string, error) { elegido <- struct{}{}; return nil, nil }
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	_ = w.Canvas().Capture()
	rotulo := a.v.Traduce("gallery.add")
	if !a.capaAnyadir.Visible() || !a.anyadir.Visible() || a.anyadir.Nombre != rotulo {
		t.Fatal("en la galería a 1280 no está el «+» flotante de «Añadir fotos»")
	}
	margen := ui.MargenFlotanteEn(1280)
	if pos := a.anyadir.Position(); pos.X+a.anyadir.Size().Width != 1280-margen {
		t.Errorf("el «+» está en %v y va en la esquina de abajo a la derecha", pos)
	}
	if hayBoton(a.scroll.Content, rotulo) {
		t.Error("la barra de la galería sigue con «Añadir fotos»: crear es el «+» flotante")
	}
	test.Tap(a.anyadir)
	select {
	case <-elegido:
	case <-time.After(2 * time.Second):
		t.Error("el «+» no abre el selector de ficheros")
	}

	a.Ir(RutaAlbumes)
	if a.capaAnyadir.Visible() {
		t.Error("en los álbumes el «+» de «Añadir fotos» sigue a la vista")
	}
}

// hayBoton dice si bajo `o` hay un `ui.Boton` con ese rótulo.
func hayBoton(o fyne.CanvasObject, texto string) bool {
	if b, es := o.(*ui.Boton); es && b.Texto == texto {
		return true
	}
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			if hayBoton(h, texto) {
				return true
			}
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			if hayBoton(h, texto) {
				return true
			}
		}
	}
	return false
}
