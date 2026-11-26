// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// graphSeries es un Graph de mentira con lo que hace falta para las zonas, las
// ocurrencias de una serie y las respuestas: eventos con su zona de Windows, las
// instancias de una serie (sin la cancelada), PATCH y DELETE de una instancia, y
// accept/decline/tentativelyAccept.
type graphSeries struct {
	mu         sync.Mutex
	base       string
	eventos    map[string]map[string]any
	instancias map[string]map[string]any // de m1
	ronda      int
	hechos     []string // «MÉTODO ruta» de lo que cambia algo
	cuerpos    map[string]map[string]any
}

func (g *graphSeries) etag(ev map[string]any) {
	g.ronda++
	ev["@odata.etag"] = "W/\"" + itoa(g.ronda) + "\""
}

func (g *graphSeries) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/v1.0")
	var cuerpo map[string]any
	if r.Method == http.MethodPost || r.Method == http.MethodPatch {
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		g.hechos = append(g.hechos, r.Method+" "+ruta)
		g.cuerpos[r.Method+" "+ruta] = cuerpo
	}
	if r.Method == http.MethodDelete {
		g.hechos = append(g.hechos, r.Method+" "+ruta)
	}
	switch {
	case ruta == "/me":
		escribir(map[string]any{"mail": "Ana@Example.org"})
	case ruta == "/me/calendars":
		escribir(map[string]any{"value": []any{map[string]any{"id": "cal1", "name": "Calendario", "canEdit": true}}})
	case strings.HasSuffix(ruta, "/calendarView/delta"):
		vs := make([]any, 0, 3)
		if r.URL.Query().Get("ronda") == "" {
			vs = append(vs, g.eventos["u1"], g.eventos["u2"], map[string]any{"id": "m1_x", "type": "occurrence", "seriesMasterId": "m1"})
		}
		escribir(map[string]any{"value": vs, "@odata.deltaLink": g.base + "/v1.0/me/calendars/cal1/calendarView/delta?ronda=2"})
	case ruta == "/me/events/m1/instances":
		vs := make([]any, 0, len(g.instancias))
		for _, k := range slices.Sorted(maps.Keys(g.instancias)) {
			vs = append(vs, g.instancias[k])
		}
		escribir(map[string]any{"value": vs})
	case strings.HasPrefix(ruta, "/me/events/"):
		resto := strings.TrimPrefix(ruta, "/me/events/")
		id, accion, _ := strings.Cut(resto, "/")
		if inst := g.instancias[id]; inst != nil {
			switch r.Method {
			case http.MethodDelete:
				delete(g.instancias, id)
				g.etag(g.eventos["m1"])
				w.WriteHeader(http.StatusNoContent)
			case http.MethodPatch:
				for k, v := range cuerpo {
					inst[k] = v
				}
				inst["type"] = "exception"
				g.etag(g.eventos["m1"])
				escribir(inst)
			default:
				escribir(inst)
			}
			return
		}
		ev := g.eventos[id]
		if ev == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if m := r.Header.Get("If-Match"); m != "" && m != ev["@odata.etag"] {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		switch {
		case accion != "" && r.Method == http.MethodPost:
			ev["responseStatus"] = map[string]any{"response": map[string]string{"accept": "accepted", "decline": "declined", "tentativelyAccept": "tentativelyAccepted"}[accion]}
			g.etag(ev)
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPatch:
			if ev["isOrganizer"] == false {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			for k, v := range cuerpo {
				ev[k] = v
			}
			g.etag(ev)
			escribir(ev)
		default:
			escribir(ev)
		}
	default:
		http.NotFound(w, r)
	}
}

func instanciaGraph(id, original, inicio, fin, asunto, tipo string) map[string]any {
	return map[string]any{
		"id": id, "type": tipo, "seriesMasterId": "m1", "subject": asunto, "originalStart": original,
		"start": map[string]any{"dateTime": inicio, "timeZone": "UTC"}, "end": map[string]any{"dateTime": fin, "timeZone": "UTC"},
	}
}

// Graph: las horas en la zona del evento (bajan con su nombre IANA, suben en ella); las
// ocurrencias canceladas de una serie bajan como EXDATE y las quitadas aquí se borran
// allí; una excepción hecha aquí cambia su ocurrencia; y quien no organiza contesta con
// accept, sin tocar el evento.
func TestGraphZonasOcurrenciasYRespuestas(t *testing.T) {
	g := &graphSeries{eventos: map[string]map[string]any{}, instancias: map[string]map[string]any{}, cuerpos: map[string]map[string]any{}}
	g.eventos["u1"] = map[string]any{
		"id": "u1", "@odata.etag": "W/\"u\"", "type": "singleInstance", "iCalUId": "uid-u1", "subject": "Dentista",
		"isOrganizer": true, "originalStartTimeZone": "Romance Standard Time", "originalEndTimeZone": "Romance Standard Time",
		"start": map[string]any{"dateTime": "2026-12-01T09:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T10:00:00.0000000", "timeZone": "UTC"},
	}
	g.eventos["m1"] = map[string]any{
		"id": "m1", "@odata.etag": "W/\"m\"", "type": "seriesMaster", "iCalUId": "uid-m1", "subject": "Reunión",
		"isOrganizer": true, "originalStartTimeZone": "Romance Standard Time",
		"start": map[string]any{"dateTime": "2026-12-01T09:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T10:00:00.0000000", "timeZone": "UTC"},
		"recurrence": map[string]any{
			"pattern": map[string]any{"type": "weekly", "interval": 1, "daysOfWeek": []any{"tuesday"}},
			"range":   map[string]any{"type": "numbered", "startDate": "2026-12-01", "numberOfOccurrences": 5, "recurrenceTimeZone": "Romance Standard Time"},
		},
	}
	g.eventos["u2"] = map[string]any{
		"id": "u2", "@odata.etag": "W/\"p\"", "type": "singleInstance", "iCalUId": "uid-u2", "subject": "Comité",
		"isOrganizer": false, "responseStatus": map[string]any{"response": "notResponded"},
		"organizer": map[string]any{"emailAddress": map[string]any{"address": "pedro@example.org", "name": "Pedro"}},
		"attendees": []any{
			map[string]any{"emailAddress": map[string]any{"address": "ana@example.org"}, "status": map[string]any{"response": "none"}},
			map[string]any{"emailAddress": map[string]any{"address": "luis@example.org"}, "status": map[string]any{"response": "accepted"}},
		},
		"start": map[string]any{"dateTime": "2026-12-03T15:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-03T16:00:00.0000000", "timeZone": "UTC"},
	}
	g.instancias["m1_1201"] = instanciaGraph("m1_1201", "2026-12-01T09:00:00Z", "2026-12-01T09:00:00.0000000", "2026-12-01T10:00:00.0000000", "Reunión", "occurrence")
	g.instancias["m1_1208"] = instanciaGraph("m1_1208", "2026-12-08T09:00:00Z", "2026-12-08T10:00:00.0000000", "2026-12-08T11:00:00.0000000", "Reunión (otra sala)", "exception")
	g.instancias["m1_1215"] = instanciaGraph("m1_1215", "2026-12-15T09:00:00Z", "2026-12-15T09:00:00.0000000", "2026-12-15T10:00:00.0000000", "Reunión", "occurrence")
	// La del 22 se canceló en Outlook: ya no está entre las instancias.
	g.instancias["m1_1229"] = instanciaGraph("m1_1229", "2026-12-29T09:00:00Z", "2026-12-29T09:00:00.0000000", "2026-12-29T10:00:00.0000000", "Reunión", "occurrence")
	srv := httptest.NewTLSServer(g)
	defer srv.Close()
	g.base = srv.URL
	ahora := func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

	dir := t.TempDir()
	repo := abrirCalendario(t, dir)
	s := &agenda.Sincronizador{
		Remoto: agenda.GraphCalendarioDe(srv.URL+"/v1.0", srv.Client(), ahora), Local: agenda.LocalCalendario{Repo: repo},
		Estado: filepath.Join(dir, "m.json"), Ventana: agenda.VentanaCalendario, Ahora: ahora,
	}
	sinc := func() agenda.Resultado {
		t.Helper()
		res, err := s.Sincronizar(t.Context())
		if err != nil || len(res.Errores) > 0 {
			t.Fatalf("%v %v", err, res.Errores)
		}
		return res
	}
	if res := sinc(); res.Bajados != 3 {
		t.Fatalf("bajada: %+v", res)
	}
	cal := s.Colecciones()[0].ID
	objs, _ := repo.ObjetosCrudos(t.Context(), cal)
	por := map[string]string{}
	ids := map[string]string{}
	for _, o := range objs {
		por[o.UID], ids[o.UID] = o.Datos, o.ID
	}
	if !strings.Contains(por["uid-u1"], "DTSTART;TZID=Europe/Paris:20261201T100000") || !strings.Contains(por["uid-u1"], "DTEND;TZID=Europe/Paris:20261201T110000") {
		t.Fatalf("zona: %q", por["uid-u1"])
	}
	m1 := por["uid-m1"]
	if !strings.Contains(m1, "DTSTART;TZID=Europe/Paris:20261201T100000") || !strings.Contains(m1, "EXDATE:20261222T090000Z") ||
		strings.Count(m1, "EXDATE") != 1 || !strings.Contains(m1, "RECURRENCE-ID;TZID=Europe/Paris:20261208T100000") {
		t.Fatalf("serie: %q", m1)
	}
	inst, _ := repo.Instances(t.Context(), cal, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	reuniones := 0
	for _, i := range inst {
		if strings.HasPrefix(i.Event.Summary, "Reunión") {
			reuniones++
		}
	}
	if reuniones != 4 {
		t.Fatalf("ocurrencias de la serie: %d", reuniones)
	}

	// Aquí: el dentista cambia de título; en la serie se quita el 29 y se mueve el 15 a
	// las once; y Ana acepta el comité.
	poner := func(uid, datos string) {
		t.Helper()
		if _, err := repo.PonerObjetoCrudo(t.Context(), cal, ids[uid], "", datos); err != nil {
			t.Fatal(err)
		}
	}
	poner("uid-u1", strings.Replace(por["uid-u1"], "SUMMARY:Dentista", "SUMMARY:Dentista (Dr. Ruiz)", 1))
	serie := strings.Replace(m1, "EXDATE:20261222T090000Z\r\n", "EXDATE:20261222T090000Z,20261229T090000Z\r\n", 1)
	serie = strings.Replace(serie, "END:VCALENDAR", "BEGIN:VEVENT\r\nUID:uid-m1\r\nDTSTAMP:20261124T120000Z\r\nRECURRENCE-ID;TZID=Europe/Paris:20261215T100000\r\n"+
		"DTSTART;TZID=Europe/Paris:20261215T110000\r\nDTEND;TZID=Europe/Paris:20261215T120000\r\nSUMMARY:Reunión (tarde)\r\nEND:VEVENT\r\nEND:VCALENDAR", 1)
	poner("uid-m1", serie)
	poner("uid-u2", strings.Replace(por["uid-u2"], "ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ana@example.org", "ATTENDEE;PARTSTAT=ACCEPTED:mailto:ana@example.org", 1))
	if !strings.Contains(por["uid-u2"], "ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ana@example.org") {
		t.Fatalf("invitación: %q", por["uid-u2"])
	}
	if res := sinc(); res.Subidos != 3 || res.Pendientes != 0 {
		t.Fatalf("subida: %+v %v", res, g.hechos)
	}
	// El dentista sube en su zona.
	inicio, _ := g.cuerpos["PATCH /me/events/u1"]["start"].(map[string]any)
	if inicio["timeZone"] != "Europe/Paris" || inicio["dateTime"] != "2026-12-01T10:00:00" {
		t.Fatalf("hora subida: %v", g.cuerpos["PATCH /me/events/u1"])
	}
	rec, _ := json.Marshal(g.cuerpos["PATCH /me/events/m1"]["recurrence"])
	if !strings.Contains(string(rec), `"recurrenceTimeZone":"Europe/Paris"`) || !strings.Contains(string(rec), `"startDate":"2026-12-01"`) {
		t.Fatalf("regla subida: %s", rec)
	}
	// La serie: el 29 borrado (Graph lo cancela) y el 15 movido; el 22 ya no estaba.
	if !slices.Contains(g.hechos, "DELETE /me/events/m1_1229") || slices.Contains(g.hechos, "PATCH /me/events/m1_1208") {
		t.Fatalf("ocurrencias: %v", g.hechos)
	}
	quince := g.cuerpos["PATCH /me/events/m1_1215"]
	if quince == nil || quince["subject"] != "Reunión (tarde)" || quince["start"].(map[string]any)["dateTime"] != "2026-12-15T11:00:00" {
		t.Fatalf("excepción subida: %v", quince)
	}
	// El comité: accept con respuesta al organizador, y ni un PATCH del evento.
	if r := g.cuerpos["POST /me/events/u2/accept"]; r == nil || r["sendResponse"] != true || slices.Contains(g.hechos, "PATCH /me/events/u2") {
		t.Fatalf("respuesta: %v %v", g.hechos, r)
	}
	// Otra vuelta: nada que subir.
	n := len(g.hechos)
	if res := sinc(); res.Subidos != 0 || len(g.hechos) != n {
		t.Fatalf("en vacío: %+v %v", res, g.hechos[n:])
	}
}
