// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"image"
	"image/color"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// editorMontado es la presentación de muestra montada como en la app (el marco en su
// armazón), a 1280×800: lo mismo que retratan las capturas.
func editorMontado(t *testing.T) (*Editor, fyne.Window) {
	t.Helper()
	ctx := &editor.Contexto{Tipo: editor.Presentacion, Estado: editor.Listo, Zoom: 100, T: traductor(t), Titulo: "Resultados del trimestre"}
	e := NuevoEditor(ctx)
	e.Programar = func(time.Duration, func()) {}
	ctx.Lienzo = e
	ctx.SoloLectura = !e.Cargar(string(muestra(t)))
	marco := editor.NuevoMarco(ctx, Registro(e), func() {})
	cromo := ctx.Refrescar
	ctx.Refrescar = func() { cromo(); e.Refrescar() }
	ctx.Refrescar()
	w := test.NewTempWindow(t, container.NewStack(marco.Montado(), e.Avisos()))
	w.SetPadded(false)
	w.Resize(fyne.NewSize(1280, 800))
	return e, w
}

// TestLaMiniBarraYLaMesaSiguenAlTemaEnCaliente: con la presentación abierta, cambiar de
// tema repinta la mini-barra, la banda de lo que no se pinta, la mesa, las miniaturas y
// la cabecera del editor con el tema NUEVO, y la ventana queda igual, píxel a píxel, que
// una que nace en él.
//
// Al pasar a oscuro quedaba una fila blanca —la de la mini-barra— y la mesa clara: eran
// rectángulos con el color resuelto al componer la zona, y las miniaturas y el
// escenario guardaban la variante que les dio el editor. VISTO EN ROJO antes del arreglo,
// en las dos direcciones.
func TestLaMiniBarraYLaMesaSiguenAlTemaEnCaliente(t *testing.T) {
	cambiar := func(nombre string) {
		tema.Elegir(nombre)
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	}
	t.Cleanup(func() { cambiar("") })
	for _, c := range []struct{ desde, hasta string }{{"claro", "oscuro"}, {"oscuro", "claro"}} {
		t.Run(c.desde+"→"+c.hasta, func(t *testing.T) {
			cambiar(c.desde)
			e, w := editorMontado(t)
			cambiar(c.hasta)
			cambiado := w.Canvas().Capture()
			_, w2 := editorMontado(t)
			directo := w2.Canvas().Capture()

			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
			en := func(o fyne.CanvasObject, dx, dy float32) color.NRGBA {
				p, ok := posicionEn(w.Content(), o, fyne.Position{})
				if !ok {
					t.Fatalf("%T no está en la ventana", o)
				}
				return color.NRGBAModel.Convert(cambiado.At(int(p.X+dx), int(p.Y+dy))).(color.NRGBA) //nolint:forcetypeassert // el modelo devuelve eso
			}
			// La mesa es la pila del fondo con el desplazador del escenario encima.
			barra, mesa := e.zona.Objects[0], fyne.CanvasObject(nil)
			for _, o := range e.zona.Objects {
				if p, es := o.(*fyne.Container); es && len(p.Objects) == 2 && p.Objects[1] == e.desliz {
					mesa = p
				}
			}
			if mesa == nil {
				t.Fatal("no se encuentra la mesa en la zona del editor")
			}
			// El extremo derecho de la mini-barra, donde no hay controles, y la esquina de
			// la mesa, que el papel de la diapositiva no tapa.
			if got, quiero := en(barra, barra.Size().Width-4, 4), tema.Color("surface", v); got != quiero {
				t.Errorf("la mini-barra pinta %v tras pasar a %s; su `surface` es %v", got, c.hasta, quiero)
			}
			if got, quiero := en(mesa, 4, 4), colorLienzo(v); got != quiero {
				t.Errorf("la mesa pinta %v tras pasar a %s; la de ese tema es %v", got, c.hasta, quiero)
			}
			if n, donde := pixelesDistintos(cambiado, directo); n > 0 {
				t.Errorf("%d píxeles distintos de los de una ventana que nace en %s (el primero en %v)", n, c.hasta, donde)
			}
		})
	}
}

// posicionEn es dónde cae `o` en la ventana, buscándolo desde `raiz` por contenedores y
// renderers.
func posicionEn(raiz, o fyne.CanvasObject, base fyne.Position) (fyne.Position, bool) {
	p := base.Add(raiz.Position())
	if raiz == o {
		return p, true
	}
	var hijos []fyne.CanvasObject
	switch x := raiz.(type) {
	case *fyne.Container:
		hijos = x.Objects
	case fyne.Widget:
		hijos = test.WidgetRenderer(x).Objects()
	}
	for _, h := range hijos {
		if q, ok := posicionEn(h, o, p); ok {
			return q, true
		}
	}
	return fyne.Position{}, false
}

// pixelesDistintos cuenta los píxeles que no son iguales en las dos imágenes, y dice
// dónde está el primero.
func pixelesDistintos(a, b image.Image) (n int, primero image.Point) {
	caja := a.Bounds().Intersect(b.Bounds())
	for y := caja.Min.Y; y < caja.Max.Y; y++ {
		for x := caja.Min.X; x < caja.Max.X; x++ {
			if color.NRGBAModel.Convert(a.At(x, y)) != color.NRGBAModel.Convert(b.At(x, y)) {
				if n == 0 {
					primero = image.Pt(x, y)
				}
				n++
			}
		}
	}
	return n, primero
}
