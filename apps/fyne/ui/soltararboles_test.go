// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// contadorDestroy es un widget que cuenta las veces que se destruye su renderer y las
// que se crea.
type contadorDestroy struct {
	widget.BaseWidget
	hijo             fyne.CanvasObject
	creados, tirados int
}

func nuevoContadorDestroy(hijo fyne.CanvasObject) *contadorDestroy {
	c := &contadorDestroy{hijo: hijo}
	c.ExtendBaseWidget(c)
	return c
}

func (c *contadorDestroy) CreateRenderer() fyne.WidgetRenderer {
	c.creados++
	return &rendererContador{WidgetRenderer: widget.NewSimpleRenderer(c.hijo), c: c}
}

type rendererContador struct {
	fyne.WidgetRenderer
	c *contadorDestroy
}

func (r *rendererContador) Destroy() { r.c.tirados++ }

// TestSoltarArbolesTiraLoDescolgadoYDejaLoConservado: lo descolgado se suelta en el
// acto, también lo que cuelga de sus renderers; lo que está en el árbol nuevo o se
// conserva aparte, no. VISTO EN ROJO con `SoltarArboles` vacía: la caché los guardaba
// hasta caducar.
func TestSoltarArbolesTiraLoDescolgadoYDejaLoConservado(t *testing.T) {
	dentro := nuevoContadorDestroy(canvas.NewRectangle(color.Black))
	viejo := nuevoContadorDestroy(container.NewStack(dentro))
	reutilizado := nuevoContadorDestroy(canvas.NewRectangle(color.White))
	guardado := nuevoContadorDestroy(canvas.NewRectangle(color.White))
	raiz := container.NewVBox(viejo, reutilizado, guardado)
	w := test.NewWindow(raiz)
	defer w.Close()
	w.Resize(fyne.NewSize(100, 100))
	w.Canvas().Capture()

	raiz.Objects = []fyne.CanvasObject{nuevoContadorDestroy(canvas.NewRectangle(color.Black)), reutilizado}
	raiz.Refresh()
	SoltarArboles([]fyne.CanvasObject{viejo, reutilizado, guardado, nil}, raiz, guardado)

	if viejo.tirados != 1 || dentro.tirados != 1 {
		t.Errorf("lo descolgado no se ha soltado: %d y %d renderers tirados", viejo.tirados, dentro.tirados)
	}
	if reutilizado.tirados != 0 || guardado.tirados != 0 {
		t.Error("se ha soltado lo que sigue en el árbol o se conserva aparte")
	}
	// Vuelto a enseñar, el soltado recibe otro renderer.
	raiz.Objects = append(raiz.Objects, viejo)
	raiz.Refresh()
	w.Canvas().Capture()
	if viejo.creados != 2 {
		t.Errorf("vuelto al árbol, el renderer se ha creado %d veces", viejo.creados)
	}
	SoltarArboles(nil)
}
