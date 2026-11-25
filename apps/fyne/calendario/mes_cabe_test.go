// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// CAL-08: a 1280×800 el mes no cabía. `Mes.MinSize` daba a las seis semanas el alto de
// la celda más llena —cuatro carriles y el «+N más», 142 por semana—, la sexta quedaba
// debajo desplazando la vista y el «+N más» no saltaba nunca. Ahora las seis semanas
// reparten el alto de la vista y cada día enseña lo que cabe en el suyo; el resto va a
// su «+N más», sin pisar la semana de abajo. Con más alto, se ven más.
//
// VISTO EN ROJO con el `MinSize` y el reparto de antes: el mes mide 885 en una vista de
// 691 y la sexta semana solo se ve desplazando.
func TestElMesCabeEnLaVentanaYElMasSalta(t *testing.T) {
	p := pantallaDeMuestra(t)
	p.Eventos = nil
	for dia := 1; dia <= 30; dia++ {
		for k := range 5 {
			inicio := time.Date(2026, 9, dia, 8+k, 0, 0, 0, time.UTC)
			p.Eventos = append(p.Eventos, Ocurrencia{Instance: api.Instance{
				Event: api.Event{ID: fmt.Sprintf("d%d-%d", dia, k), Summary: "Reunión", DTStart: inicio},
				Start: inicio, End: inicio.Add(30 * time.Minute),
			}, Color: AcentoPorDefecto})
		}
	}
	// Y un congreso de tres días, que es una barra encima de las píldoras.
	congreso := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	p.Eventos = append(p.Eventos, Ocurrencia{Instance: api.Instance{
		Event: api.Event{ID: "congreso", Summary: "Congreso", AllDay: true, DTStart: congreso},
		Start: congreso, End: congreso.AddDate(0, 0, 3),
	}, Color: AcentoPorDefecto})

	vista := p.vista()
	w := test.NewWindow(vista)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	s := vista.(*container.ThemeOverride).Content.(*container.Scroll)
	m := s.Content.(*Mes)
	r := test.WidgetRenderer(m).(*mesRenderer)

	// pildorasDel es cuántas píldoras pinta el 9 de septiembre (cinco eventos), y
	// comprueba de paso que ninguna celda se sale de su semana.
	pildorasDel := func(que string) int {
		t.Helper()
		n := -1
		for _, c := range r.celdas {
			cr := test.WidgetRenderer(c).(*celdaRenderer)
			for _, pl := range cr.pildoras {
				if pl.Position().Y+pl.Size().Height > c.Size().Height {
					t.Errorf("%s: una píldora del %d invade la semana de abajo", que, c.dia.Day())
				}
			}
			if cr.mas != nil && cr.mas.Position().Y+cr.mas.Size().Height > c.Size().Height {
				t.Errorf("%s: el «+N más» del %d invade la semana de abajo", que, c.dia.Day())
			}
			if c.dia.Month() == time.September && c.dia.Day() == 9 {
				n = len(cr.pildoras)
				if want := c.ocultas; n+want != 5 || (want > 0) != (cr.mas != nil) {
					t.Errorf("%s: el 9 pinta %d y esconde %d de 5, con «+N más» %v", que, n, want, cr.mas != nil)
				}
			}
		}
		for semana, barras := range r.barras {
			c := r.celdas[semana*7]
			for _, b := range barras {
				if b.Position().Y+b.Size().Height > c.Position().Y+c.Size().Height {
					t.Errorf("%s: una barra invade la semana de abajo", que)
				}
			}
		}
		return n
	}

	// La vista del mes a 1280×800: el ancho sin el menú y el alto sin la cabecera.
	w.Resize(fyne.NewSize(anchoCont, altoUtil-altoCab))
	if m.Size().Height > s.Size().Height+0.5 || s.Offset.Y != 0 {
		t.Fatalf("a 1280×800 el mes mide %v en una vista de %v: no cabe", m.Size().Height, s.Size().Height)
	}
	if sem := AltoSemana(m.Size().Height); sem < AltoMinimoSemana {
		t.Fatalf("cada semana mide %v, menos que su mínimo %v", sem, AltoMinimoSemana)
	}
	bajo := pildorasDel("a 800")
	if bajo < 1 || bajo >= 5 {
		t.Fatalf("a 800 el día con cinco eventos pinta %d píldoras; tocaba alguna y el «+N más»", bajo)
	}

	// Con la ventana más alta, la semana gana carriles y el «+N más» baja.
	w.Resize(fyne.NewSize(anchoCont, 1100))
	if alto := pildorasDel("a 1100"); alto <= bajo {
		t.Errorf("con más alto el día pinta %d píldoras, no más que las %d de antes", alto, bajo)
	}

	// Muy bajo, el mes no se aplasta: conserva su mínimo y desplaza la vista.
	w.Resize(fyne.NewSize(anchoCont, 300))
	if m.Size().Height < m.MinSize().Height || m.MinSize().Height <= s.Size().Height {
		t.Errorf("a 300 de alto el mes mide %v (mínimo %v) en %v", m.Size().Height, m.MinSize().Height, s.Size().Height)
	}
	pildorasDel("a 300")
}
