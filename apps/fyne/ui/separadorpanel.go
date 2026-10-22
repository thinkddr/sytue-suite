// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// separadorPanel es el separador vertical arrastrable entre el panel de lista
// y el detalle (paridad con ResizablePanel.tsx).
type separadorPanel struct {
	pulsable
	a *Armazon

	linea *canvas.Rectangle
	fondo *canvas.Rectangle

	arrastrando bool
}

var (
	_ fyne.Widget         = (*separadorPanel)(nil)
	_ fyne.Draggable      = (*separadorPanel)(nil)
	_ fyne.DoubleTappable = (*separadorPanel)(nil)
	_ desktop.Cursorable  = (*separadorPanel)(nil)
	_ desktop.Keyable     = (*separadorPanel)(nil)
)

func nuevoSeparadorPanel(a *Armazon) *separadorPanel {
	s := &separadorPanel{a: a}
	s.Nombre = "Separador de panel"
	s.ExtendBaseWidget(s)
	return s
}

func (s *separadorPanel) Cursor() desktop.Cursor {
	return desktop.HResizeCursor
}

func (s *separadorPanel) Dragged(e *fyne.DragEvent) {
	if s.a == nil || s.a.Panel == nil {
		return
	}
	s.arrastrando = true
	actual := s.a.anchoPanelActual() + e.Dragged.DX
	s.a.fijarAnchoPanel(actual)
	s.Refresh()
}

func (s *separadorPanel) DragEnd() {
	s.arrastrando = false
	if s.a != nil {
		s.a.guardarAnchoPreferencia()
	}
	s.Refresh()
}

func (s *separadorPanel) DoubleTapped(*fyne.PointEvent) {
	if s.a == nil {
		return
	}
	s.a.restablecerAnchoPanel()
	s.Refresh()
}

func (s *separadorPanel) TypedKey(e *fyne.KeyEvent) {
	if s.a == nil {
		s.pulsable.TypedKey(e)
		return
	}
	paso := float32(24)
	switch e.Name {
	case fyne.KeyLeft:
		s.a.fijarAnchoPanel(s.a.anchoPanelActual() - paso)
		s.a.guardarAnchoPreferencia()
		s.Refresh()
	case fyne.KeyRight:
		s.a.fijarAnchoPanel(s.a.anchoPanelActual() + paso)
		s.a.guardarAnchoPreferencia()
		s.Refresh()
	case fyne.KeyHome:
		s.a.fijarAnchoPanel(AnchoPanelMinimo)
		s.a.guardarAnchoPreferencia()
		s.Refresh()
	case fyne.KeyEnd:
		s.a.fijarAnchoPanel(s.a.anchoMaximoPanel())
		s.a.guardarAnchoPreferencia()
		s.Refresh()
	default:
		s.pulsable.TypedKey(e)
	}
}

func (s *separadorPanel) KeyDown(e *fyne.KeyEvent) {}
func (s *separadorPanel) KeyUp(e *fyne.KeyEvent)   {}

func (s *separadorPanel) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	s.fondo = canvas.NewRectangle(color.Transparent)
	s.linea = canvas.NewRectangle(tema.Color("border", v))
	r := &separadorPanelRenderer{
		s:     s,
		fondo: s.fondo,
		linea: s.linea,
	}
	r.Refresh()
	return r
}

type separadorPanelRenderer struct {
	s     *separadorPanel
	fondo *canvas.Rectangle
	linea *canvas.Rectangle
}

func (r *separadorPanelRenderer) MinSize() fyne.Size {
	return fyne.NewSize(AnchoSeparadorPanel, 0)
}

func (r *separadorPanelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.linea}
}

func (r *separadorPanelRenderer) Destroy() {}

func (r *separadorPanelRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	anchoLinea := float32(1)
	x := (t.Width - anchoLinea) / 2
	r.linea.Move(fyne.NewPos(x, 0))
	r.linea.Resize(fyne.NewSize(anchoLinea, t.Height))
}

func (r *separadorPanelRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	activo := r.s.Encima() || r.s.Enfocado() || r.s.arrastrando
	if activo {
		r.linea.FillColor = tema.Color("primary", v)
		r.fondo.FillColor = conAlfa(tema.Color("primary", v), 0.15)
	} else {
		r.linea.FillColor = tema.Color("border", v)
		r.fondo.FillColor = color.Transparent
	}
	r.linea.Refresh()
	r.fondo.Refresh()
}

func conAlfa(c color.NRGBA, f float32) color.NRGBA {
	c.A = uint8(float32(c.A) * f)
	return c
}
