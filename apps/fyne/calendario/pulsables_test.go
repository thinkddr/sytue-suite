// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// TestLosBotonesDelCalendarioSePulsanComoUnBoton: el botón de vista, el día del
// mini-mes, la píldora y la celda del mes son `<button>` en la web y embeben
// `ui.Pulsable`. Cada uno tiene que llegar a SU acción con el ratón, con Intro y con
// Espacio —si el constructor olvida `AlPulsar`, no llega ninguna— y encender `:hover`.
func TestLosBotonesDelCalendarioSePulsanComoUnBoton(t *testing.T) {
	dia := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	var pulsado string
	g := &GrupoVista{Rotulos: [4]string{"Mes", "Semana", "Día", "Agenda"}, AlElegir: func(Vista) { pulsado = "vista" }}
	mm := &MiniMes{Cursor: dia, AlElegir: func(time.Time) { pulsado = "día" }}
	m := &Mes{Cursor: dia, AlCrear: func(time.Time) { pulsado = "crear" }, AlAbrir: func(Ocurrencia, fyne.CanvasObject) { pulsado = "abrir" }}

	type boton interface {
		fyne.Tappable
		fyne.Focusable
		MouseIn(*desktop.MouseEvent)
		Encima() bool
	}
	for _, c := range []struct {
		quiero string
		b      boton
	}{
		{"vista", nuevoBotonVista(g, 0)},
		{"día", nuevoDiaMini(mm, dia)},
		{"abrir", nuevaPildora(m, Ocurrencia{}, tema.Claro)},
		{"crear", nuevaCelda(m, dia, nil, 0, 0, tema.Claro)},
	} {
		for _, via := range []struct {
			como  string
			pulsa func()
		}{
			{"clic", func() { c.b.Tapped(nil) }},
			{"Intro", func() { c.b.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn}) }},
			{"Espacio", func() { c.b.TypedRune(' ') }},
		} {
			pulsado = ""
			via.pulsa()
			if pulsado != c.quiero {
				t.Errorf("con %s, «%s» llega a %q", via.como, c.quiero, pulsado)
			}
		}
		c.b.MouseIn(nil)
		if !c.b.Encima() {
			t.Errorf("«%s» no enciende `:hover` con el puntero encima", c.quiero)
		}
	}
}
