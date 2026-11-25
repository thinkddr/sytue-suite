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

// TestElMasDelMesDespliegaElDiaYNoCrea: el «+N más» de la celda era texto pintado y su
// pulsación caía a la celda, que creaba un evento. Ahora es un botón propio que abre la
// lista de TODOS los eventos del día, y cada fila abre su vista rápida (cerrando la
// lista). El hueco de la celda sigue creando.
func TestElMasDelMesDespliegaElDiaYNoCrea(t *testing.T) {
	p := pantallaDeMuestra(t)
	dia := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	p.Eventos = []Ocurrencia{{Instance: api.Instance{
		Event: api.Event{ID: "congreso", Summary: "Congreso", AllDay: true, DTStart: dia.AddDate(0, 0, -1)},
		Start: dia.AddDate(0, 0, -1), End: dia.AddDate(0, 0, 2),
	}, Color: AcentoPorDefecto}}
	for i := range 6 {
		inicio := dia.Add(time.Duration(9+i) * time.Hour)
		p.Eventos = append(p.Eventos, Ocurrencia{Instance: api.Instance{
			Event: api.Event{ID: fmt.Sprintf("r%d", i), Summary: fmt.Sprintf("Reunión %d", i), DTStart: inicio},
			Start: inicio, End: inicio.Add(30 * time.Minute),
		}, Color: AcentoPorDefecto})
	}
	var creado time.Time
	var abierto string
	var ancla fyne.CanvasObject
	p.AlCrearEvento = func(d time.Time) { creado = d }
	p.AlAbrirEvento = func(o Ocurrencia, a fyne.CanvasObject) { abierto, ancla = o.Event.ID, a }

	vista := p.vista()
	w := test.NewWindow(vista)
	t.Cleanup(w.Close)
	// Alta para que en la semana quepan los cuatro carriles de la web (la barra y tres
	// píldoras) y el «+N más»: con menos alto se ven menos y el número sube (CAL-08).
	w.Resize(fyne.NewSize(1024, 1000))
	m := vista.(*container.ThemeOverride).Content.(*container.Scroll).Content.(*Mes)
	r := test.WidgetRenderer(m).(*mesRenderer)
	var celda *celdaMes
	for _, c := range r.celdas {
		if c.dia.Equal(dia) {
			celda = c
		}
	}
	cr := test.WidgetRenderer(celda).(*celdaRenderer)
	if cr.mas == nil {
		t.Fatal("el día con siete eventos no enseña «+N más»")
	}
	if _, ok := fyne.CanvasObject(cr.mas).(fyne.Focusable); !ok || cr.mas.AccessibilityLabel() != "+3 más" {
		t.Fatalf("«+N más» no es un botón enfocable con su rótulo: %q", cr.mas.AccessibilityLabel())
	}

	test.Tap(cr.mas)
	if !creado.IsZero() {
		t.Fatal("pulsar «+N más» crea un evento")
	}
	var filas []*filaAgenda
	enCapas(w.Canvas(), func(o fyne.CanvasObject) {
		if f, ok := o.(*filaAgenda); ok {
			filas = append(filas, f)
		}
	})
	if len(filas) != 7 {
		t.Fatalf("la lista del día tiene %d filas, no las 7 del día", len(filas))
	}
	if filas[0].o.Event.ID != "congreso" {
		t.Errorf("el día completo no va primero: %q", filas[0].o.Event.ID)
	}

	test.Tap(filas[4])
	if abierto != filas[4].o.Event.ID || ancla != cr.mas {
		t.Fatalf("la fila abre %q anclada a %T", abierto, ancla)
	}
	if len(w.Canvas().Overlays().List()) != 0 {
		t.Error("la lista del día sigue abierta tras abrir un evento")
	}

	test.Tap(celda)
	if !creado.Equal(dia) {
		t.Error("el hueco de la celda ya no crea")
	}
}
