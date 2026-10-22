// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package photos

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Enlace es el `<a>` de texto de estas páginas: «← Todos los álbumes», «← Volver a la
// galería» (`text-text-muted hover:text-text text-sm`) y los subrayados de la ficha y de
// las sugerencias (`underline`). No es el de Hub —ese va en `primary-text` y
// semi-negrita—: la paridad manda sobre la reutilización.
type Enlace struct {
	ui.Pulsable

	Texto string
	// Subrayado es la clase `underline`: siempre, no solo al pasar por encima. Y
	// SubrayadoEncima, `hover:underline`: solo con el puntero encima.
	Subrayado, SubrayadoEncima bool
	// Escalon y Peso son su letra; el cero es `text-sm` normal, la de casi todos. El
	// «Ver en la ficha» del rótulo del visor es `text-xs font-medium`, y el «Ver
	// ajustes →» de la tarjeta de Sytue AI hereda su `text-xs`.
	Escalon tema.Escalon
	Peso    ui.Peso
	// Tinta es el color en reposo; vacío, `text-muted`. Al pasar por encima va a `text`
	// (`hover:text-text`), salvo que Tinta ya sea otra cosa.
	Tinta string
}

var _ fyne.Widget = (*Enlace)(nil)

// NuevoEnlace crea el enlace de texto.
func NuevoEnlace(texto string, alPulsar func()) *Enlace {
	e := &Enlace{Texto: texto}
	e.Nombre, e.AlPulsar = texto, alPulsar
	e.ExtendBaseWidget(e)
	return e
}

// AccessibilityRole: en la web es un `<a>`.
func (e *Enlace) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleLink }

// Con pone la letra del enlace y lo devuelve, para encadenar.
func (e *Enlace) Con(esc tema.Escalon, p ui.Peso) *Enlace {
	e.Escalon, e.Peso = esc, p
	return e
}

func (e *Enlace) escalon() tema.Escalon {
	if e.Escalon.Tam == 0 {
		return tema.TextoSM
	}
	return e.Escalon
}

// Medida es lo que ocupa con su letra: su ancho y la caja de línea. Es la única cuenta:
// la usan MinSize, Layout y quien reparte una fila con el enlace dentro.
func (e *Enlace) Medida() fyne.Size { return ui.Medir(e.Texto, e.escalon(), e.Peso) }

func (e *Enlace) tinta() string {
	switch {
	case e.Tinta != "":
		return e.Tinta
	case e.Encima():
		return "text"
	}
	return "text-muted"
}

// CreateRenderer compone el rótulo, el subrayado y el aro de foco.
func (e *Enlace) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &enlaceRenderer{
		e:       e,
		aro:     ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		texto:   ui.Texto(e.Texto, e.escalon(), e.Peso, tema.Color(e.tinta(), v)),
		subraya: canvas.NewRectangle(tema.Color(e.tinta(), v)),
	}
	r.objetos = append(r.aro.Objetos(), r.texto, r.subraya)
	r.Refresh()
	return r
}

type enlaceRenderer struct {
	e       *Enlace
	aro     *ui.Aro
	texto   *canvas.Text
	subraya *canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *enlaceRenderer) MinSize() fyne.Size           { return r.e.Medida() }
func (r *enlaceRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *enlaceRenderer) Destroy()                     {}

// Layout centra el rótulo y pone el subrayado a un píxel bajo la línea base, que es
// donde lo deja el navegador con `text-underline-offset: auto` en Figtree.
func (r *enlaceRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	esc := r.e.escalon()
	y := (t.Height - esc.Linea) / 2
	ui.ColocarTexto(r.texto, esc, 0, y)
	ancho := r.e.Medida().Width
	r.subraya.Move(fyne.NewPos(0, y+(esc.Linea+esc.Tam)/2))
	r.subraya.Resize(fyne.NewSize(ancho, tema.Borde))
}

func (r *enlaceRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := tema.Color(r.e.tinta(), v)
	r.texto.Text, r.texto.Color = r.e.Texto, c
	r.texto.TextSize, r.texto.FontSource = r.e.escalon().Tam, r.e.Peso.Cara()
	r.texto.Refresh()
	r.subraya.FillColor = c
	if r.e.Subrayado || (r.e.SubrayadoEncima && r.e.Encima()) {
		r.subraya.Show()
	} else {
		r.subraya.Hide()
	}
	r.subraya.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.e.Enfocado())
	r.Layout(r.e.Size())
}
