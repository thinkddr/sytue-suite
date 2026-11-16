// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
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
	// Lo que la tarjeta necesita además: había ocho (se ven seis, «+2») y el calendario
	// roto deja la agenda incompleta, y lo dice.
	agenda, err := c.AgendaCompleta(context.Background(), dia(0), dia(24))
	if err != nil || agenda.Total != 8 || !agenda.Parcial || len(agenda.Eventos) != MaxEventosHoy {
		t.Fatalf("agenda = %+v, %v; quiero 6 de 8 y parcial", agenda, err)
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

// TestConNueveEventosSeVenSeisYSobranTres: RecortarHoy dice cuántos había, que es el
// «+3 más en el calendario» de la tarjeta.
func TestConNueveEventosSeVenSeisYSobranTres(t *testing.T) {
	var nueve []EventoDeHoy
	for h := 20; h > 11; h-- {
		nueve = append(nueve, EventoDeHoy{ID: fmt.Sprint(h), Inicio: time.Date(2026, 9, 21, h, 0, 0, 0, time.UTC)})
	}
	vistos, total := RecortarHoy(nueve)
	if len(vistos) != 6 || total != 9 || vistos[0].ID != "12" {
		t.Fatalf("vistos %d de %d, primero %q", len(vistos), total, vistos[0].ID)
	}
}

// TestSiNoContestaNingunCalendarioNoHayAgenda: con todos caídos no es un día vacío, es
// una agenda que no se puede enseñar.
func TestSiNoContestaNingunCalendarioNoHayAgenda(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/calendars" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"calendars": []Calendar{{ID: "a"}, {ID: "b"}}})
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	ahora := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	desde, hasta := LimitesDelDia(ahora)
	if _, err := NewCalendars(srv.URL, nil, nil).AgendaCompleta(context.Background(), desde, hasta); !errors.Is(err, ErrAgendaSinCalendarios) {
		t.Fatalf("err = %v", err)
	}
}

// TestElEstadoDeCadaEventoConRelojFijo: «Ahora» en lo que está en curso y «Siguiente»
// solo en el primero por venir; lo de todo el día y lo que empezó sin fin, nada. Es el
// `marcasDeLaAgenda` de la web con el mismo reloj.
func TestElEstadoDeCadaEventoConRelojFijo(t *testing.T) {
	ahora := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	a := func(h, m int) time.Time { return time.Date(2026, 9, 9, h, m, 0, 0, time.UTC) }
	lista := []EventoDeHoy{
		{ID: "dia", Inicio: a(0, 0), TodoElDia: true},
		{ID: "pasado", Inicio: a(9, 0), Fin: a(10, 0)},
		{ID: "sin-fin", Inicio: a(11, 0)},
		{ID: "curso", Inicio: a(11, 30), Fin: a(12, 30)},
		{ID: "justo", Inicio: a(12, 0), Fin: a(13, 0)},
		{ID: "luego", Inicio: a(15, 0), Fin: a(16, 0)},
		{ID: "despues", Inicio: a(17, 0), Fin: a(18, 0)},
	}
	quiero := []EstadoDeEvento{EventoSinEstado, EventoSinEstado, EventoSinEstado, EventoAhora, EventoAhora, EventoSiguiente, EventoSinEstado}
	got := MarcasDeLaAgenda(lista, ahora)
	for i := range lista {
		if got[i] != quiero[i] {
			t.Errorf("%s: marca %d, quiero %d", lista[i].ID, got[i], quiero[i])
		}
	}
	if lista[1].Estado(ahora) != EventoPasado || lista[6].Estado(ahora) != EventoSiguiente {
		t.Error("Estado no distingue pasado de por venir")
	}
}
