// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

var margenXAvisosDialogo = tema.Espacio(5) // px-5 de Toast.tsx dentro de Modal

// dialogoEncima es el último diálogo abierto, también bajo un menú suyo.
func (p *PilaAvisos) dialogoEncima() *Dialogo {
	c := lienzoDe(p)
	if c == nil {
		return nil
	}
	capas := c.Overlays().List()
	for i := len(capas) - 1; i >= 0; i-- {
		if cp, ok := capas[i].(*capa); ok {
			if d, ok := cp.panel.(*Dialogo); ok {
				return d
			}
		}
	}
	return nil
}

func (d *Dialogo) pilaAvisos() *PilaAvisos {
	if d.capa == nil {
		return nil
	}
	c := lienzoDe(d.capa)
	if c == nil {
		return nil
	}
	p := pilaDe(c)
	if p == nil || p.dialogoEncima() != d {
		return nil
	}
	return p
}

func altoAvisos(avisos []*Aviso, ancho float32) float32 {
	alto := float32(0)
	for i, a := range avisos {
		if i > 0 {
			alto += HuecoPilaAvisos
		}
		alto += MedidaAviso(a, ancho).Height
	}
	return alto
}

// La banda reserva espacio en el flujo y desplaza sólo los avisos que no caben.
func (d *Dialogo) altoAvisos(ancho float32, lienzo fyne.Size) float32 {
	p := d.pilaAvisos()
	if p == nil {
		return 0
	}
	alto := altoAvisos(p.instantanea(), ancho+2*(margenXDialogo-margenXAvisosDialogo))
	if alto == 0 {
		return 0
	}
	return min(alto+margenYCuerpo, min(lienzo.Height/4, 10*tema.Rem))
}

type avisosDialogo struct {
	widget.BaseWidget
	d     *Dialogo
	ancho float32
}

func nuevoAvisosDialogo(d *Dialogo) *avisosDialogo {
	a := &avisosDialogo{d: d}
	a.ExtendBaseWidget(a)
	return a
}

func (a *avisosDialogo) CreateRenderer() fyne.WidgetRenderer { return &avisosDialogoRenderer{a: a} }

type avisosDialogoRenderer struct{ a *avisosDialogo }

func (r *avisosDialogoRenderer) MinSize() fyne.Size {
	p := r.a.d.pilaAvisos()
	if p == nil {
		return fyne.Size{}
	}
	return fyne.NewSize(0, altoAvisos(p.instantanea(), r.a.ancho))
}

func (r *avisosDialogoRenderer) Objects() []fyne.CanvasObject {
	if p := r.a.d.pilaAvisos(); p != nil {
		return p.objetosAvisos()
	}
	return nil
}

func (r *avisosDialogoRenderer) Layout(t fyne.Size) {
	p := r.a.d.pilaAvisos()
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	y := float32(0)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, a := range p.avisos {
		colocarAviso(a, t.Width, fyne.NewPos(0, y), v)
		y += MedidaAviso(a, t.Width).Height + HuecoPilaAvisos
	}
}

func (r *avisosDialogoRenderer) Refresh() { r.Layout(r.a.Size()) }
func (r *avisosDialogoRenderer) Destroy() {}
