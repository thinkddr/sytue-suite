// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/event"
)

// servicioDeLaSerie es Sytue Calendar con una serie, como contesta y como guarda de
// verdad (services/calendar): /instances expande el maestro guardado en el rango pedido
// (`event.ExpandEvents`, que deja fuera sus EXDATE) y cada ocurrencia lleva su inicio;
// «solo este» → eliminar (amend-recurrence) le añade la EXDATE de la ocurrencia, y el
// PUT del maestro guarda lo que manda la pantalla: sus horas, su zona, su regla y sus
// EXDATE (la columna `exdate` del servicio sale de ahí).
type servicioDeLaSerie struct {
	mu      sync.Mutex
	maestro api.Event
	puts    int
}

func (s *servicioDeLaSerie) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/calendars":
		_, _ = io.WriteString(w, `{"calendars":[{"id":"mio","display_name":"Personal","color":"#1e50a8","is_default":true}]}`)
	case "GET /api/v1/calendars/mio/instances":
		desde, _ := time.Parse(time.RFC3339, r.URL.Query().Get("start"))
		hasta, _ := time.Parse(time.RFC3339, r.URL.Query().Get("end"))
		_ = json.NewEncoder(w).Encode(map[string]any{"instances": s.instancias(desde, hasta)})
	case "GET /api/v1/events/" + s.maestro.ID:
		_ = json.NewEncoder(w).Encode(s.maestro)
	case "POST /api/v1/calendars/mio/amend-recurrence":
		var c struct {
			UID          string    `json:"uid"`
			RecurrenceID time.Time `json:"recurrence_id"`
			Mode         string    `json:"mode"`
		}
		if json.NewDecoder(r.Body).Decode(&c) != nil || c.UID != s.maestro.UID || c.Mode != api.RecorteExcluir {
			http.Error(w, "no", http.StatusBadRequest)
			return
		}
		s.maestro.ExDate = append(s.maestro.ExDate, c.RecurrenceID.UTC())
		_, _ = io.WriteString(w, `{}`)
	case "PUT /api/v1/events/" + s.maestro.ID:
		var in api.EventInput
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "no", http.StatusBadRequest)
			return
		}
		m := &s.maestro
		m.Summary, m.DTStart, m.DTEnd, m.TZID, m.AllDay = in.Summary, in.Start, in.End, in.TZID, in.AllDay
		m.RRule, m.RDate, m.ExDate = in.RRule, in.RDate, in.ExDate
		s.puts++
		_ = json.NewEncoder(w).Encode(s.maestro)
	default:
		_, _ = io.WriteString(w, `{}`)
	}
}

// instancias son las ocurrencias del maestro guardado en [desde, hasta).
func (s *servicioDeLaSerie) instancias(desde, hasta time.Time) []api.Instance {
	m := s.maestro
	ev := event.Event{
		UID: m.UID, Summary: m.Summary, DTStart: m.DTStart, DTEnd: m.DTEnd, TZID: m.TZID,
		AllDay: m.AllDay, RRule: m.RRule, RDate: m.RDate, ExDate: m.ExDate,
	}
	expandidas := event.ExpandEvents([]event.Event{ev}, desde, hasta)
	out := make([]api.Instance, 0, len(expandidas))
	for _, i := range expandidas {
		o := m
		o.DTStart, o.DTEnd = i.Start, i.End
		out = append(out, api.Instance{Event: o, Start: i.Start, End: i.End})
	}
	return out
}

func (s *servicioDeLaSerie) guardado() (api.Event, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maestro, s.puts
}

// TestMoverLaSerieNoDevuelveLoQuitado: en la edición conectada, contra el servicio como
// guarda de verdad, se quita la tercera ocurrencia de una serie semanal («solo este» →
// eliminar) y se mueve la serie una hora más tarde con el formulario («todos»), desde la
// primera ocurrencia o desde otra: la quitada sigue quitada a la hora nueva, porque la
// EXDATE se mueve con la serie (los mismos días y hora de pared que su DTSTART). Antes
// se mandaba en la hora vieja y la tercera volvía a salir a las 11:00.
func TestMoverLaSerieNoDevuelveLoQuitado(t *testing.T) {
	madrid := cargarZona(t, "Europe/Madrid")
	semana := func(n, h int) time.Time { return time.Date(2026, 9, 1+7*n, h, 0, 0, 0, madrid) }
	for _, desde := range []int{0, 3} {
		s := &servicioDeLaSerie{maestro: api.Event{
			ID: "s1", UID: "us", CalendarID: "mio", Summary: "Seguimiento", TZID: "Europe/Madrid",
			RRule: "FREQ=WEEKLY", DTStart: semana(0, 10), DTEnd: semana(0, 11),
		}}
		a, w := appEnZona(t, s, madrid)
		inicios := func() []time.Time {
			var out []time.Time
			for _, o := range a.P.Eventos {
				if o.Event.ID == "s1" {
					out = append(out, o.Start)
				}
			}
			return out
		}
		quitada := ocurrenciaQueEmpieza(t, a, "s1", semana(2, 10))
		if err := BorrarSerie(t.Context(), a.Cal, quitada, AlcanceEste); err != nil {
			t.Fatal(err)
		}
		a.Recargar()
		if slices.ContainsFunc(inicios(), func(i time.Time) bool { return i.Equal(semana(2, 10)) }) {
			t.Fatalf("la tercera, quitada, sigue saliendo: %v", inicios())
		}

		e := a.P.EdicionDe(ocurrenciaQueEmpieza(t, a, "s1", semana(desde, 10)))
		f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
		f.d.Mostrar(w.Canvas())
		f.inicio.PonerTexto(semana(desde, 11).Format("02/01/2006, 15:04"))
		f.fin.PonerTexto(semana(desde, 12).Format("02/01/2006, 15:04"))
		f.enviar()
		confirmarAlcance(t, w, "Todos los eventos")
		m, puts := s.guardado()
		if puts != 1 || !m.DTStart.Equal(semana(0, 11)) {
			t.Fatalf("desde la semana %d: la serie no ha pasado a las 11:00 desde su primera (%d PUT): %v", desde, puts, m.DTStart.In(madrid))
		}
		if len(m.ExDate) != 1 || !m.ExDate[0].Equal(semana(2, 11)) {
			t.Errorf("desde la semana %d: la EXDATE de la tercera, a las 11:00 como la serie: %v", desde, m.ExDate)
		}
		a.Recargar()
		vistas := inicios()
		quiero := []time.Time{semana(0, 11), semana(1, 11), semana(3, 11), semana(4, 11)}
		for _, q := range quiero {
			if !slices.ContainsFunc(vistas, q.Equal) {
				t.Errorf("desde la semana %d: falta la ocurrencia de las %v en %v", desde, q, vistas)
			}
		}
		for _, v := range vistas {
			if v.Year() == 2026 && v.Month() == 9 && v.In(madrid).Day() == 15 {
				t.Errorf("desde la semana %d: la tercera, quitada, vuelve a salir a las %v", desde, v.In(madrid))
			}
		}
	}
}

// TestLaSerieNuevaDeLosSiguientesLlevaSusExdateMovidas: «este y los siguientes» con la
// tercera una hora más tarde crea la serie nueva a las 11:00 con la EXDATE que le queda
// (la quinta) también a las 11:00; antes iba a las 10:00 y la quinta volvía a salir.
func TestLaSerieNuevaDeLosSiguientesLlevaSusExdateMovidas(t *testing.T) {
	madrid := cargarZona(t, "Europe/Madrid")
	semana := func(n, h int) time.Time { return time.Date(2026, 9, 1+7*n, h, 0, 0, 0, madrid) }
	maestro := api.Event{
		ID: "s1", UID: "us", CalendarID: "mio", Summary: "Seguimiento", TZID: "Europe/Madrid", RRule: "FREQ=WEEKLY",
		DTStart: semana(0, 10), DTEnd: semana(0, 11), ExDate: []time.Time{semana(1, 10).UTC(), semana(4, 10).UTC()},
	}
	s := &servicioFiel{maestros: map[string]api.Event{"s1": maestro}, instancias: serieFiel(maestro, 0, 2, 3)}
	a, w := appEnZona(t, s, madrid)
	e := a.P.EdicionDe(ocurrenciaQueEmpieza(t, a, "s1", semana(2, 10)))
	f := a.P.nuevoFormulario(e, a.destinos(), a.accionesFormulario(e))
	f.d.Mostrar(w.Canvas())
	f.inicio.PonerTexto(semana(2, 11).Format("02/01/2006, 15:04"))
	f.fin.PonerTexto(semana(2, 12).Format("02/01/2006, 15:04"))
	f.enviar()
	confirmarAlcance(t, w, "Este y los siguientes")
	hechas, cuerpos := s.apuntadas()
	if len(hechas) != 2 || hechas[0] != "POST /api/v1/calendars/mio/events" {
		t.Fatalf("«siguientes» hizo %v", hechas)
	}
	if ini := instanteDe(t, cuerpos[0], "start"); !ini.Equal(semana(2, 11)) {
		t.Errorf("la serie nueva empieza %v", ini.In(madrid))
	}
	ex, _ := cuerpos[0]["exdate"].([]any)
	if len(ex) != 1 {
		t.Fatalf("la serie nueva, con la EXDATE de la quinta: %v", cuerpos[0]["exdate"])
	}
	if v, _ := ex[0].(string); !strings.HasPrefix(v, "2026-09-29T") || !instanteDe(t, map[string]any{"exdate": v}, "exdate").Equal(semana(4, 11)) {
		t.Errorf("la EXDATE de la quinta, a las 11:00 como la serie nueva: %v", ex[0])
	}
}
