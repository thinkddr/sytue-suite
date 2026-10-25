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

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// En horizontal se recorre el mes, no se aplastan seis semanas en la poca altura
// restante. Se pulsa tanto el último día como un evento de la sexta semana.
func TestElMesConservaLasSeisSemanasAlGirar(t *testing.T) {
	t.Cleanup(ui.TactilDePrueba(true))
	for _, porDia := range []int{1, 5} {
		t.Run(fmt.Sprintf("eventos-por-dia-%d", porDia), func(t *testing.T) {
			p := pantallaDeMuestra(t)
			p.Eventos = nil
			dias := Dias(p.Cursor, time.UTC)
			for semana := range 6 {
				inicio := dias[semana*7].Add(10 * time.Hour)
				p.Eventos = append(p.Eventos, Ocurrencia{Instance: api.Instance{
					Event: api.Event{ID: fmt.Sprintf("s%d-b", semana), Summary: "Entrega de proyecto", AllDay: true, DTStart: dias[semana*7]},
					Start: dias[semana*7], End: dias[semana*7].AddDate(0, 0, 3),
				}, Color: AcentoPorDefecto})
				for i := range porDia {
					p.Eventos = append(p.Eventos, Ocurrencia{Instance: api.Instance{
						Event: api.Event{ID: fmt.Sprintf("s%d-e%d", semana, i), Summary: "Revisión del proyecto", DTStart: inicio},
						Start: inicio, End: inicio.Add(time.Hour),
					}, Color: AcentoPorDefecto})
				}
			}
			var creado time.Time
			abierto := ""
			p.AlCrearEvento = func(d time.Time) { creado = d }
			p.AlAbrirEvento = func(o Ocurrencia, _ fyne.CanvasObject) { abierto = o.Event.ID }
			vista := p.vista().(*container.ThemeOverride)
			s := vista.Content.(*container.Scroll)
			m := s.Content.(*Mes)
			for _, ventana := range []fyne.Size{
				{Width: 320, Height: 700},
				{Width: 568, Height: 320},
				{Width: 320, Height: 700},
				{Width: 844, Height: 390},
				{Width: 1024, Height: 768},
				{Width: 768, Height: 1024},
			} {
				// Reserva para el marco y las barras, además del viewport del mes.
				vista.Resize(fyne.NewSize(ventana.Width, ventana.Height-120))
				r := test.WidgetRenderer(m).(*mesRenderer)
				if len(r.celdas) != 42 || m.Size().Height < m.MinSize().Height {
					t.Fatalf("a %v se han comprimido las seis semanas: %v", ventana, m.Size())
				}
				for _, c := range r.celdas {
					if c.Size().Height < 44 {
						t.Errorf("a %v un día se reduce a %v", ventana, c.Size())
					}
					cr := test.WidgetRenderer(c).(*celdaRenderer)
					for _, evento := range cr.pildoras {
						if fin := evento.Position().Y + evento.Size().Height; fin > c.Size().Height {
							t.Errorf("a %v un evento invade la siguiente semana: fin=%g, día=%v", ventana, fin, c.Size())
						}
					}
					if cr.mas != nil && cr.mas.Position().Y+cr.mas.Size().Height > c.Size().Height {
						t.Errorf("a %v el rótulo de eventos ocultos invade la semana siguiente", ventana)
					}
				}
				for semana, barras := range r.barras {
					c := r.celdas[semana*7]
					for _, b := range barras {
						if b.Position().Y+b.Size().Height > c.Position().Y+c.Size().Height {
							t.Errorf("a %v una barra invade la semana siguiente", ventana)
						}
					}
				}
				s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -1e6)})
				ultimo := r.celdas[41]
				if y := ultimo.Position().Y - s.Offset.Y; y < 0 || y+ultimo.Size().Height > s.Size().Height+0.01 {
					t.Fatalf("a %v no se alcanza la sexta semana: y=%g, scroll=%v", ventana, y, s.Size())
				}
				test.Tap(ultimo)
				if !creado.Equal(dias[41]) {
					t.Fatal("el último día perdió su acción al girar")
				}
				evento := test.WidgetRenderer(r.celdas[35]).(*celdaRenderer).pildoras[0]
				test.Tap(evento)
				if abierto != "s5-e0" {
					t.Fatal("no se puede abrir el evento de la sexta semana")
				}
				s.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, 1e6)})
				if s.Offset.Y != 0 {
					t.Fatal("el gesto no devuelve el mes al principio")
				}
			}
		})
	}
}
