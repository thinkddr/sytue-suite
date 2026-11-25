// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package docus

import (
	"image"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/editor"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui/recorrido"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// documentoConComentarios es un documento abierto a 1280×800 con el panel de
// comentarios a la vista, como lo deja quien lo abre en la app.
func documentoConComentarios(t *testing.T) fyne.Window {
	t.Helper()
	s := &servicioDocs{rol: "editor", html: "<h1>Propuesta</h1><p>Esta propuesta.</p>"}
	previa := appDePrueba(t, s)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Docs())
	if err != nil {
		t.Fatal(err)
	}
	a := NuevaApp(tr.T, "es", previa.Svc)
	a.lanzar, a.programar, a.EnVivo = previa.lanzar, previa.programar, previa.EnVivo
	a.portada.Lanzar = a.lanzar
	w := test.NewTempWindow(t, a.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	a.Arrancar()
	a.AbrirID("d1", "")
	a.abierto.ctx.AbrirPanel(editor.PanelComentarios)
	fyne.DoAndWait(func() {})
	return w
}

// TestLosComentariosYLaGaleriaSiguenAlTemaEnCaliente: con el panel de comentarios
// abierto, cambiar de tema repinta las tarjetas de los hilos y la galería de estilos de
// la cinta con el tema NUEVO: el panel queda igual, píxel a píxel, que el de una ventana
// que nace en él, y los rótulos de la galería llevan las tintas del tema.
//
// Las tarjetas se componían con las tintas resueltas y nada las volvía a componer, y
// las fichas de estilo solo repintaban su caja: tras pasar a oscuro quedaban tarjetas
// blancas y rótulos oscuros sobre la cinta oscura. VISTO EN ROJO antes del arreglo.
func TestLosComentariosYLaGaleriaSiguenAlTemaEnCaliente(t *testing.T) {
	cambiar := func(nombre string) {
		tema.Elegir(nombre)
		fyne.CurrentApp().Settings().SetTheme(tema.Sytue{})
	}
	t.Cleanup(func() { cambiar("") })
	for _, c := range []struct{ desde, hasta string }{{"claro", "oscuro"}, {"oscuro", "claro"}} {
		t.Run(c.desde+"→"+c.hasta, func(t *testing.T) {
			cambiar(c.desde)
			w := documentoConComentarios(t)
			cambiar(c.hasta)
			cambiado := w.Canvas().Capture()
			w2 := documentoConComentarios(t)
			directo := w2.Canvas().Capture()
			v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())

			paneles := recorrido.Buscar(w.Content(), func(o fyne.CanvasObject) bool { _, es := o.(*editor.PanelLateral); return es })
			if len(paneles) != 1 {
				t.Fatalf("hay %d paneles laterales a la vista y tiene que haber uno", len(paneles))
			}
			p, ok := posicionEn(w.Content(), paneles[0], fyne.Position{})
			if !ok {
				t.Fatal("el panel no está en la ventana")
			}
			caja := image.Rect(int(p.X), int(p.Y), int(p.X+paneles[0].Size().Width), int(p.Y+paneles[0].Size().Height))
			if n, donde := pixelesDistintos(cambiado, directo, caja); n > 0 {
				t.Errorf("el panel de comentarios tiene %d píxeles distintos de los de una ventana que nace en %s (el primero en %v)",
					n, c.hasta, donde)
			}

			fichas := recorrido.Buscar(w.Content(), func(o fyne.CanvasObject) bool { _, es := o.(*fichaEstilo); return es })
			if len(fichas) == 0 {
				t.Fatal("no hay galería de estilos en la cinta")
			}
			for _, o := range fichas {
				f := o.(*fichaEstilo)                        //nolint:forcetypeassert // lo acaba de buscar
				r := test.WidgetRenderer(f).(*fichaRenderer) //nolint:forcetypeassert // su renderer
				if quiero := tema.Color("text-subtle", v); r.nombre.Color != quiero {
					t.Errorf("el nombre de «%s» pinta %v tras pasar a %s; su `text-subtle` es %v", f.nombre, r.nombre.Color, c.hasta, quiero)
				}
				if quiero := tema.Color(f.e.token, v); r.muestra.Color != quiero {
					t.Errorf("la muestra de «%s» pinta %v tras pasar a %s; su `%s` es %v", f.nombre, r.muestra.Color, c.hasta, f.e.token, quiero)
				}
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

// pixelesDistintos cuenta los píxeles de `caja` que no son iguales en las dos imágenes,
// y dice dónde está el primero.
func pixelesDistintos(a, b image.Image, caja image.Rectangle) (n int, primero image.Point) {
	caja = caja.Intersect(a.Bounds()).Intersect(b.Bounds())
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
