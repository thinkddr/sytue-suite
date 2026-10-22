// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// AreaFicha es un `<textarea>` escrito a mano en la web, con sus clases y no las de
// Form.tsx: la petición de Sytue AI del editor (`rows=3 rounded-sm border p-2 text-xs
// bg-surface`) y la descripción en edición de la ficha del visor (`rows=3 rounded-md
// border px-2.5 py-1.5 text-xs sm:text-sm bg-bg`). `ui.AreaTexto` es la de Form.tsx
// (`px-3 py-2 text-base`) y no tiene tallas, así que aquí va el motor de edición sin cromo
// (`ui.EditorPlano`) dentro de su caja, con su marca de agua repartida en líneas como la
// del navegador.
type AreaFicha struct {
	widget.BaseWidget

	// Editor es donde se escribe.
	Editor *ui.EditorPlano
	// AlCambiar se llama con el texto en cada pulsación.
	AlCambiar func(string)

	escalon          tema.Escalon
	filas            int
	margenX, margenY float32
	radio            float32
	fondo            string
	marca            *ui.Parrafo
	caja             *ui.Caja
	aro              *ui.Aro
}

var _ fyne.Widget = (*AreaFicha)(nil)

// NuevaAreaFicha crea el área con su letra, sus filas, su relleno, su radio y su fondo.
func NuevaAreaFicha(marcaDeAgua string, e tema.Escalon, filas int, mx, my, radio float32, fondo string) *AreaFicha {
	a := &AreaFicha{escalon: e, filas: filas, margenX: mx, margenY: my, radio: radio, fondo: fondo}
	a.ExtendBaseWidget(a)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.aro = ui.NuevoAro(radio, ui.SinHueco, v)
	a.Editor = ui.NuevoEditorPlanoMultilinea(e, func(si bool) {
		a.aro.Ver(si)
		a.aro.Pintar(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	})
	a.marca = ui.NuevoParrafo(marcaDeAgua, e, ui.Normal, "text-muted", v)
	a.Editor.AlCambiar(func(s string) {
		a.marca.Hidden = s != ""
		a.marca.Refresh()
		if a.AlCambiar != nil {
			a.AlCambiar(s)
		}
	})
	return a
}

// Texto es lo escrito.
func (a *AreaFicha) Texto() string { return a.Editor.Texto() }

// PonerTexto escribe (y esconde o enseña la marca de agua).
func (a *AreaFicha) PonerTexto(s string) {
	a.Editor.PonerTexto(s)
	a.marca.Hidden = s != ""
}

// MinSize son las filas de la caja de línea, el relleno y el borde: el alto de un
// `<textarea rows=N>`, que es N veces la línea y no N veces la letra.
func (a *AreaFicha) MinSize() fyne.Size {
	return fyne.NewSize(0, float32(a.filas)*a.escalon.Linea+2*(a.margenY+tema.Borde))
}

// CreateRenderer: el aro, la caja, la marca de agua y el editor.
func (a *AreaFicha) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.caja = ui.NuevaCaja(tema.Color(a.fondo, v), tema.Color("border", v), a.radio)
	objs := append(append(a.aro.Objetos(), a.caja.Objetos()...), a.marca, a.Editor.Objeto())
	return &areaFichaRenderer{a: a, objs: objs}
}

type areaFichaRenderer struct {
	a    *AreaFicha
	objs []fyne.CanvasObject
}

func (r *areaFichaRenderer) MinSize() fyne.Size           { return r.a.MinSize() }
func (r *areaFichaRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *areaFichaRenderer) Destroy()                     {}

func (r *areaFichaRenderer) Layout(t fyne.Size) {
	a := r.a
	a.aro.Colocar(t)
	a.caja.Redimensionar(t)
	x, y := tema.Borde+a.margenX, tema.Borde+a.margenY
	dentro := fyne.NewSize(max(t.Width-2*x, 0), max(t.Height-2*y, 0))
	a.marca.Move(fyne.NewPos(x, y))
	a.marca.Resize(fyne.NewSize(dentro.Width, ui.AltoParrafo(a.marca, dentro.Width)))
	a.Editor.Colocar(fyne.NewPos(x, y), dentro)
}

func (r *areaFichaRenderer) Refresh() {
	a := r.a
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	a.caja.Pintar(tema.Color(a.fondo, v), tema.Color("border", v))
	a.aro.Pintar(v)
	r.Layout(a.Size())
	for _, o := range r.objs {
		o.Refresh()
	}
}
