// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestElIntentDeCrearSeSanea es `readCreateIntent`: título y descripción recortados,
// invitados solo válidos, en minúsculas y sin repetir, y sin `start` la hora en punto
// siguiente, con una hora de duración.
func TestElIntentDeCrearSeSanea(t *testing.T) {
	ahora := time.Date(2026, 9, 22, 10, 37, 0, 0, time.UTC)
	q := url.Values{
		"action":    {"create"},
		"title":     {strings.Repeat("x", 600)},
		"attendees": {" Ana@Sytue.eu, basura, ana@sytue.eu ,bea@sytue.eu"},
	}
	e, ok := LeerIntencion(q, ahora, time.UTC)
	if !ok {
		t.Fatal("no reconoce el intent de crear")
	}
	if len([]rune(e.Titulo)) != 500 {
		t.Errorf("el título no se recorta a 500: %d", len([]rune(e.Titulo)))
	}
	if len(e.Invitados) != 2 || e.Invitados[0] != "ana@sytue.eu" || e.Invitados[1] != "bea@sytue.eu" {
		t.Errorf("invitados %v", e.Invitados)
	}
	if !e.Inicio.Equal(time.Date(2026, 9, 22, 11, 0, 0, 0, time.UTC)) || e.Fin.Sub(e.Inicio) != time.Hour {
		t.Errorf("sin start empieza a las %v y dura %v", e.Inicio, e.Fin.Sub(e.Inicio))
	}
	q.Set("start", "2026-10-01T09:30:00Z")
	if e, _ := LeerIntencion(q, ahora, time.UTC); !e.Inicio.Equal(time.Date(2026, 10, 1, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("con start empieza a las %v", e.Inicio)
	}
	if _, ok := LeerIntencion(url.Values{"action": {"borrar"}}, ahora, time.UTC); ok {
		t.Error("una acción que no es crear se toma por intent")
	}
}

// TestElEnlaceDeUnEventoLlevaASuDia: con `all_day=1` la FECHA, sin que la zona la mueva.
func TestElEnlaceDeUnEventoLlevaASuDia(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("sin tzdata")
	}
	d, ok := FechaDelEnlace(url.Values{"date": {"2026-09-30T00:00:00Z"}, "all_day": {"1"}}, ny)
	if !ok || d.Day() != 30 {
		t.Errorf("un día completo del 30 cae el %v en Nueva York", d)
	}
	if _, ok := FechaDelEnlace(url.Values{"date": {"mañana"}}, ny); ok {
		t.Error("una fecha que no vale lleva a algún día")
	}
}

// TestElEnlaceDeUnEventoVaASuDia es `pages/events/[id].ts`: el resultado del buscador
// del Hub pide el evento y abre la vista de día en su fecha; lo que no es un UUID no
// pide nada.
func TestElEnlaceDeUnEventoVaASuDia(t *testing.T) {
	const id = "6f1c2a9e-3b4d-4e5f-8a9b-0c1d2e3f4a5b"
	for ruta, quiero := range map[string]bool{
		"/calendar/events/" + id: true, "/events/" + id: true,
		"/calendar/events/new": false, "/events/" + id + "/x": false, "/files/" + id: false, "/": false,
	} {
		if _, ok := api.LeerEvento(&url.URL{Path: ruta}); ok != quiero {
			t.Errorf("%s: ¿evento? %v", ruta, ok)
		}
	}
	pedidas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events/"+id {
			pedidas++
			_, _ = io.WriteString(w, `{"id":"`+id+`","summary":"Comité","dtstart":"2031-03-05T09:00:00Z","dtend":"2031-03-05T10:00:00Z"}`)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	test.NewTempApp(t)
	p := &Pantalla{T: traductor(t), Cursor: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC), Zona: time.UTC, Vista: VistaMes}
	a := NuevaApp(p, api.NewCalendars(srv.URL, nil, nil))
	a.lanzar = func(f func()) { f() }
	a.enUI = func(f func()) { f() }
	a.AtenderEnlace(&url.URL{Scheme: "sytue-calendar", Path: "/calendar/events/" + id})
	if pedidas != 1 || p.Vista != VistaDia || p.Cursor.Day() != 5 || p.Cursor.Month() != time.March {
		t.Errorf("pedidas %d, vista %v, día %v", pedidas, p.Vista, p.Cursor)
	}
	a.AtenderEnlace(&url.URL{Scheme: "sytue-calendar", Path: "/calendar/events/no-es-un-uuid"})
	if pedidas != 1 {
		t.Error("un id que no es UUID no se pide")
	}
}
