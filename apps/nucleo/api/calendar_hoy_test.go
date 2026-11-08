// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestLaAgendaDeHoyEsLaDelBFF: seis como mucho, por hora de inicio, de todos los
// calendarios; un calendario que falla no tumba la agenda, y la lista de calendarios sí.
// Es la regla que comparten la tarjeta «Hoy» del Hub y su widget.
//
// Visto en ROJO a mano el 2026-09-22 (en el frontal): recortando antes de ordenar, se
// cuela el evento de las 23:00 y el test lo dice.
func TestLaAgendaDeHoyEsLaDelBFF(t *testing.T) {
	dia := func(h int) time.Time { return time.Date(2026, 9, 21, h, 0, 0, 0, time.UTC) }
	ocurrencias := func(horas ...int) map[string]any {
		out := make([]Instance, 0, len(horas))
		for _, h := range horas {
			out = append(out, Instance{Event: Event{ID: fmt.Sprint(h), Summary: fmt.Sprint(h)}, Start: dia(h), End: dia(h).Add(time.Hour)})
		}
		return map[string]any{"instances": out}
	}
	var listaFalla atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cuerpo any
		switch r.URL.Path {
		case "/api/v1/calendars":
			if listaFalla.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			cuerpo = map[string]any{"calendars": []Calendar{{ID: "a"}, {ID: "roto"}, {ID: "b"}}}
		case "/api/v1/calendars/a/instances":
			cuerpo = ocurrencias(23, 9, 10, 11)
		case "/api/v1/calendars/b/instances":
			cuerpo = ocurrencias(12, 13, 14, 8)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cuerpo)
	}))
	defer srv.Close()
	c := NewCalendars(srv.URL, nil, nil)
	got, err := c.Hoy(context.Background(), dia(0), dia(24))
	if err != nil {
		t.Fatalf("un calendario roto tumba la agenda: %v", err)
	}
	if len(got) != MaxEventosHoy || got[0].Inicio.Hour() != 8 || got[5].Inicio.Hour() != 13 {
		t.Fatalf("salen %d eventos: %+v", len(got), got)
	}
	listaFalla.Store(true)
	if _, err := c.Hoy(context.Background(), dia(0), dia(24)); err == nil {
		t.Fatal("sin la lista de calendarios la agenda no falla")
	}
}

// TestUnEventoTerminadoSeApagaYUnoDeTodoElDiaNo: lo que la tarjeta y el widget marcan
// como pasado.
func TestUnEventoTerminadoSeApagaYUnoDeTodoElDiaNo(t *testing.T) {
	ahora := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	casos := []struct {
		e    EventoDeHoy
		pasa bool
	}{
		{EventoDeHoy{Fin: ahora.Add(-time.Minute)}, true},
		{EventoDeHoy{Fin: ahora.Add(time.Minute)}, false},
		{EventoDeHoy{Fin: ahora.Add(-time.Hour), TodoElDia: true}, false},
		{EventoDeHoy{}, false},
	}
	for _, c := range casos {
		if c.e.HaTerminado(ahora) != c.pasa {
			t.Errorf("%+v: HaTerminado = %v", c.e, !c.pasa)
		}
	}
}

// TestLosLimitesDelDiaSonLocales: de la medianoche local a la siguiente, que es lo
// que hace `limitesDelDia` en la web. En UTC daría «ayer» a las 00:30 de Madrid.
func TestLosLimitesDelDiaSonLocales(t *testing.T) {
	madrid := time.FixedZone("CEST", 2*60*60)
	desde, hasta := LimitesDelDia(time.Date(2026, 9, 21, 0, 30, 0, 0, madrid))
	if desde.Day() != 21 || desde.Hour() != 0 || desde.Location() != madrid {
		t.Errorf("el día empieza en %v", desde)
	}
	if hasta.Sub(desde) != 24*time.Hour {
		t.Errorf("el día dura %v", hasta.Sub(desde))
	}
}
