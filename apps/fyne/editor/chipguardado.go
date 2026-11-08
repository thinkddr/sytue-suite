// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// ChipGuardado es el chip «Guardado hace 2 min» de la cabecera, que se pulsa
// (`CHIP_GUARDADO` de EditorHeader.tsx): abre el historial de versiones y, con el
// guardado en error, reintenta ya. Sin red se queda inerte, que en la web es texto.
//
// `h-7 px-1.5 gap-1.5 rounded-sm text-xs`, el punto de `h-1.5`, `hover:bg-bg-subtle`
// y `aria-expanded:bg-selected` mientras el historial está abierto.
type ChipGuardado struct {
	activable

	Texto string
	// Token es el color del punto: `success`, `warning` o `danger`.
	Token string

	menu *ui.Menu
}

var _ fyne.Widget = (*ChipGuardado)(nil)

// AccessibilityRole es `button`.
func (c *ChipGuardado) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// mostrarMenu abre el historial colgado del chip, que se pinta abierto mientras dure.
func (c *ChipGuardado) mostrarMenu(m *ui.Menu) {
	c.menu = m
	m.Mostrar(c)
	c.Refresh()
}

// MedidaChipGuardado es lo que ocupa con ese texto: el punto, el hueco, el texto y el
// `px-1.5` a cada lado; 28 de alto, y el suelo táctil con el dedo.
func MedidaChipGuardado(texto string) fyne.Size {
	return fyne.NewSize(2*tema.Espacio(1.5)+LadoPuntoGuardado+HuecoChip+ui.Medir(texto, tema.TextoXS, ui.Normal).Width,
		ui.SueloTactil(tema.Espacio(7)))
}

// MinSize es la medida con el texto de ahora.
func (c *ChipGuardado) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return MedidaChipGuardado(c.Texto)
}

// CreateRenderer compone el fondo, el anillo, el punto y el texto.
func (c *ChipGuardado) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &chipGuardadoRenderer{
		c:     c,
		fondo: canvas.NewRectangle(tema.Color("surface", v)),
		aro:   ui.NuevoAro(tema.RadioSM, ui.SinHueco, v),
		punto: canvas.NewCircle(tema.Color(c.Token, v)),
		texto: ui.Texto(c.Texto, tema.TextoXS, ui.Normal, tema.Color("text-muted", v)),
	}
	r.fondo.CornerRadius = tema.RadioSM
	r.objetos = r.aro.Antes(r.fondo, r.punto, r.texto)
	r.Refresh()
	return r
}

type chipGuardadoRenderer struct {
	c       *ChipGuardado
	fondo   *canvas.Rectangle
	aro     *ui.Aro
	punto   *canvas.Circle
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *chipGuardadoRenderer) MinSize() fyne.Size           { return r.c.MinSize() }
func (r *chipGuardadoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *chipGuardadoRenderer) Destroy()                     {}

// Layout coloca el punto y el texto; si el chip mide menos que su texto (un móvil, donde
// cede antes que el título), el texto se corta con elipsis.
func (r *chipGuardadoRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.aro.Colocar(t)
	x := tema.Espacio(1.5)
	r.punto.Move(fyne.NewPos(x, (t.Height-LadoPuntoGuardado)/2))
	r.punto.Resize(fyne.NewSize(LadoPuntoGuardado, LadoPuntoGuardado))
	x += LadoPuntoGuardado + HuecoChip
	r.texto.Text = r.c.Texto
	if cabe := t.Width - x - tema.Espacio(1.5); cabe > 0 {
		ui.Truncar(r.texto, r.c.Texto, tema.TextoXS, ui.Normal, cabe)
	}
	ui.ColocarTexto(r.texto, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
}

func (r *chipGuardadoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := tema.Color("surface", v), tema.Color("text-muted", v)
	switch {
	case r.c.menu != nil && r.c.menu.Abierto():
		fondo, tinta = tema.Color("selected", v), tema.Color("text", v)
	case r.c.Encima() && !r.c.Inerte:
		fondo, tinta = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	r.fondo.FillColor = fondo
	r.punto.FillColor = tema.Color(r.c.Token, v)
	r.texto.Color = tinta
	r.aro.Pintar(v)
	r.aro.Ver(r.c.Enfocado())
	r.Layout(r.c.Size())
	for _, o := range []fyne.CanvasObject{r.fondo, r.punto, r.texto} {
		o.Refresh()
	}
}
