// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"

	"git.sytue.eu/sytue/one/apps/fyne/agenda"
)

// TestLaPildoraDelMesLlevaLaHoraDeInicio es `EventChip`: el punto, la hora de inicio en
// la convención del idioma y el título; la hora solo desde `sm:` (640 de VENTANA), que
// en el teléfono la celda mide 50 y se comería el título.
func TestLaPildoraDelMesLlevaLaHoraDeInicio(t *testing.T) {
	m := NuevoMes(i18n.Locale("es"), time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC))
	m.Zona = time.UTC
	ini := time.Date(2026, 9, 10, 8, 5, 0, 0, time.UTC)
	m.Eventos = []Ocurrencia{{Instance: api.Instance{
		Event: api.Event{ID: "e1", Summary: "Comité", DTStart: ini, DTEnd: ini.Add(time.Hour)},
		Start: ini, End: ini.Add(time.Hour),
	}}}
	w := test.NewWindow(m)
	t.Cleanup(w.Close)
	w.SetPadded(false)

	hora := func() (string, bool) {
		t.Helper()
		p, _ := buscarObjeto(m, func(o fyne.CanvasObject) bool { _, ok := o.(*pildora); return ok }).(*pildora)
		if p == nil {
			t.Fatal("no hay píldora en la rejilla")
		}
		r := test.WidgetRenderer(p).(*pildoraRenderer)
		r.Layout(p.Size())
		return r.hora.Text, r.hora.Visible()
	}

	w.Resize(fyne.NewSize(1280, 744))
	if texto, se := hora(); texto != "08:05" || !se {
		t.Errorf("en el escritorio la píldora dice %q (visible: %v) y quiero «08:05»", texto, se)
	}
	w.Resize(fyne.NewSize(390, 744))
	if _, se := hora(); se {
		t.Error("en el teléfono la hora se come el título y no debería salir")
	}
}

// TestEnElTelefonoLaBarraDeDiaCompletoSeLee es lo que se veía en iOS, en la franja de
// día completo de la SEMANA (`nuevaBarraEvento`, vistahoraria.go), que es donde la barra
// estrecha sigue viva por debajo de 640: la columna de un día mide ~50 y la barra de
// escritorio dejaba solo «…». En estrecho baja a `px-0.5` y corta sin suspensivos.
//
// Antes vivía en el mes, junto a la píldora compacta; el mes por debajo de 640 es ahora
// el mes del teléfono (vistamescompacta.go) y esa píldora se retiró. VISTO EN ROJO con
// la rama estrecha de `barraRenderer.Layout` apagada: «Vac…».
func TestEnElTelefonoLaBarraDeDiaCompletoSeLee(t *testing.T) {
	dia := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	p := &Pantalla{
		T: traductor(t), Vista: VistaSemana, Estado: agenda.Exito, Zona: time.UTC, Ventana: 390,
		Cursor: time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
		Eventos: []Ocurrencia{{Instance: api.Instance{
			Event: api.Event{ID: "e2", Summary: "Vacaciones en la playa", AllDay: true, DTStart: dia, DTEnd: dia.AddDate(0, 0, 1)},
			Start: dia, End: dia.AddDate(0, 0, 1),
		}}},
	}
	w := test.NewWindow(p.Construir())
	t.Cleanup(w.Close)
	w.SetPadded(false)
	w.Resize(fyne.NewSize(390, 844))

	b, _ := buscarObjeto(w.Content(), func(o fyne.CanvasObject) bool { _, ok := o.(*barraEvento); return ok }).(*barraEvento)
	if b == nil {
		t.Fatal("no hay barra de día completo en la semana")
	}
	rb := test.WidgetRenderer(b).(*barraRenderer)
	rb.Layout(b.Size())
	if got := []rune(rb.texto.Text); len(got) < 6 || strings.HasSuffix(rb.texto.Text, "…") {
		t.Errorf("en el teléfono la barra enseña %q: quiero al menos seis letras y sin «…»", rb.texto.Text)
	}
}
