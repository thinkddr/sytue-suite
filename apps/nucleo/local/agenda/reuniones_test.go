// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateCalendar "git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// googleConSala es un Google Calendar de mentira que crea la sala de Meet de un evento si
// se le pide con `conferenceData.createRequest` y `conferenceDataVersion=1`, una vez por
// `requestId`, y pone su enlace en `hangoutLink` (como Google).
type googleConSala struct {
	mu         sync.Mutex
	eventos    map[string]map[string]any
	version    int
	cambios    map[string]int
	salas      int
	pedidas    map[string]bool
	subidas    []string // «MÉTODO consulta» de los POST y PATCH
	sinVersion []string // los que pidieron sala sin conferenceDataVersion
}

func (g *googleConSala) poner(ev map[string]any) {
	g.version++
	ev["etag"] = `"v` + itoa(g.version) + `"`
	g.eventos[ev["id"].(string)] = ev
	g.cambios[ev["id"].(string)] = g.version
}

func (g *googleConSala) sala(r *http.Request, cuerpo, ev map[string]any) {
	cd, _ := cuerpo["conferenceData"].(map[string]any)
	delete(cuerpo, "conferenceData")
	cr, _ := cd["createRequest"].(map[string]any)
	if cr == nil {
		return
	}
	if r.URL.Query().Get("conferenceDataVersion") != "1" {
		g.sinVersion = append(g.sinVersion, r.Method)
		return
	}
	clave, _ := cr["conferenceSolutionKey"].(map[string]any)
	rid, _ := cr["requestId"].(string)
	if clave["type"] != "hangoutsMeet" || rid == "" || g.pedidas[rid] {
		return
	}
	g.pedidas[rid] = true
	g.salas++
	enlace := "https://meet.google.com/sala-" + itoa(g.salas)
	ev["hangoutLink"] = enlace
	ev["conferenceData"] = map[string]any{
		"entryPoints":   []any{map[string]any{"entryPointType": "video", "uri": enlace}},
		"createRequest": map[string]any{"requestId": rid, "status": map[string]any{"statusCode": "success"}},
	}
}

func (g *googleConSala) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
	switch {
	case ruta == "/users/me/calendarList":
		escribir(map[string]any{"items": []map[string]any{{"id": "ana@gmail.com", "summary": "Ana", "accessRole": "owner", "primary": true}}})
	case strings.HasSuffix(ruta, "/events") && r.Method == http.MethodGet:
		desde := 0
		if t := r.URL.Query().Get("syncToken"); t != "" {
			for _, c := range t[1:] {
				desde = desde*10 + int(c-'0')
			}
		}
		var items []any
		for id, ev := range g.eventos {
			if g.cambios[id] > desde {
				items = append(items, ev)
			}
		}
		escribir(map[string]any{"items": items, "nextSyncToken": "v" + itoa(g.version)})
	case strings.HasSuffix(ruta, "/events") && r.Method == http.MethodPost:
		g.subidas = append(g.subidas, "POST "+r.URL.RawQuery)
		var ev map[string]any
		_ = json.NewDecoder(r.Body).Decode(&ev)
		ev["id"] = "g" + itoa(len(g.eventos)+1)
		ev["status"] = "confirmed"
		g.sala(r, ev, ev)
		g.poner(ev)
		escribir(ev)
	case strings.Contains(ruta, "/events/"):
		id := ruta[strings.LastIndex(ruta, "/")+1:]
		ev := g.eventos[id]
		if ev == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			escribir(ev)
		case http.MethodPatch:
			if m := r.Header.Get("If-Match"); m != "" && m != ev["etag"] {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			g.subidas = append(g.subidas, "PATCH "+r.URL.RawQuery)
			var cambio map[string]any
			_ = json.NewDecoder(r.Body).Decode(&cambio)
			g.sala(r, cambio, ev)
			for k, v := range cambio {
				ev[k] = v
			}
			g.poner(ev)
			escribir(ev)
		}
	default:
		http.NotFound(w, r)
	}
}

// eventoLocal crea un evento en cal con el repositorio (como el formulario) y, si pide no
// es "", lo marca para que su proveedor le cree la reunión (como Calendar al guardar).
func eventoLocal(t *testing.T, repo *privateCalendar.Repository, cal, titulo, pide string) api.Event {
	t.Helper()
	inicio := time.Now().AddDate(0, 0, 2).UTC().Truncate(time.Hour)
	ev, err := repo.CreateEvent(t.Context(), cal, api.EventInput{Summary: titulo, Start: inicio, End: inicio.Add(time.Hour), TZID: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if pide != "" {
		if err := repo.PonerExtension(t.Context(), ev.ID, agenda.PropReunion, pide); err != nil {
			t.Fatal(err)
		}
	}
	return ev
}

// A-13: «Añadir Google Meet» en un evento de un calendario de Google. La subida pide la
// sala con `conferenceData.createRequest` y `conferenceDataVersion=1`, y el evento vuelve
// en la misma vuelta con su enlace (CONFERENCE) y sin la marca; la vuelta siguiente no
// sube nada. Un evento ya subido la pide con su PATCH; uno que no la pide sube como
// siempre.
func TestGoogleMeetDesdeElFormulario(t *testing.T) {
	g := &googleConSala{eventos: map[string]map[string]any{}, cambios: map[string]int{}, pedidas: map[string]bool{}}
	srv := httptest.NewTLSServer(g)
	defer srv.Close()
	dir := t.TempDir()
	repo := abrirCalendario(t, dir)
	s := &agenda.Sincronizador{
		Remoto: agenda.GoogleCalendarDe(srv.URL+"/calendar/v3", srv.Client()), Local: agenda.LocalCalendario{Repo: repo},
		Estado: filepath.Join(dir, "g.json"), Ventana: agenda.VentanaCalendario,
	}
	sinc := func() agenda.Resultado {
		t.Helper()
		res, err := s.Sincronizar(t.Context())
		if err != nil || len(res.Errores) > 0 {
			t.Fatalf("%v %v", err, res.Errores)
		}
		return res
	}
	sinc()
	cal := s.Colecciones()[0].ID

	conMeet := eventoLocal(t, repo, cal, "Revisión", agenda.ReunionMeet)
	sinMeet := eventoLocal(t, repo, cal, "Comida", "")
	if res := sinc(); res.Subidos != 2 || res.Devueltos != 1 {
		t.Fatalf("subida: %+v", res)
	}
	if g.salas != 1 || len(g.sinVersion) != 0 {
		t.Fatalf("salas creadas: %d (sin versión: %v)", g.salas, g.sinVersion)
	}
	conVersion := 0
	for _, x := range g.subidas {
		if strings.Contains(x, "conferenceDataVersion=1") {
			conVersion++
		}
		if !strings.Contains(x, "sendUpdates=all") {
			t.Errorf("sin sendUpdates: %s", x)
		}
	}
	if conVersion != 1 {
		t.Fatalf("solo el que pide la sala lleva conferenceDataVersion=1: %v", g.subidas)
	}
	ev, err := repo.GetEvent(t.Context(), conMeet.ID)
	if err != nil || ev.ConferenceURL != "https://meet.google.com/sala-1" {
		t.Fatalf("el enlace vuelve al evento de aquí: %+v %v", ev.ConferenceURL, err)
	}
	if ev.UID != conMeet.UID {
		t.Fatalf("el UID de aquí se queda: %q ≠ %q", ev.UID, conMeet.UID)
	}
	lista, _ := repo.ObjetosCrudos(t.Context(), cal)
	for _, o := range lista {
		if strings.Contains(o.Datos, agenda.PropReunion) {
			t.Fatalf("la marca no se queda: %s", o.Datos)
		}
	}
	if ev, _ := repo.GetEvent(t.Context(), sinMeet.ID); ev.ConferenceURL != "" {
		t.Fatalf("el que no la pidió no tiene sala: %q", ev.ConferenceURL)
	}
	// La vuelta siguiente no sube nada (lo que baja de lo subido trae el mismo enlace).
	if res := sinc(); res.Subidos != 0 {
		t.Fatalf("la vuelta siguiente no sube nada: %+v", res)
	}
	if ev, _ := repo.GetEvent(t.Context(), conMeet.ID); ev.ConferenceURL != "https://meet.google.com/sala-1" || g.salas != 1 {
		t.Fatalf("tras bajar: %q, salas %d", ev.ConferenceURL, g.salas)
	}

	// Ya en Google, se pide después: el PATCH la pide.
	if err := repo.PonerExtension(t.Context(), sinMeet.ID, agenda.PropReunion, agenda.ReunionMeet); err != nil {
		t.Fatal(err)
	}
	if res := sinc(); res.Subidos != 1 || g.salas != 2 || !strings.HasPrefix(g.subidas[len(g.subidas)-1], "PATCH") {
		t.Fatalf("en un evento ya subido: %+v %d %v", res, g.salas, g.subidas)
	}
	if ev, _ := repo.GetEvent(t.Context(), sinMeet.ID); ev.ConferenceURL != "https://meet.google.com/sala-2" {
		t.Fatalf("enlace del segundo: %q", ev.ConferenceURL)
	}
}

// graphConTeams es un Graph de mentira que crea la reunión de Teams de un evento que se
// sube con `isOnlineMeeting` y `onlineMeetingProvider: teamsForBusiness`, y no guarda el
// UID de aquí (Exchange pone el suyo).
type graphConTeams struct {
	mu       sync.Mutex
	eventos  map[string]map[string]any
	ronda    int
	reunion  int
	cuerpos  []map[string]any
	base     string
	sinTeams []string
}

func (g *graphConTeams) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/v1.0")
	etag := func(ev map[string]any) { g.ronda++; ev["@odata.etag"] = `W/"` + itoa(g.ronda) + `"` }
	teams := func(cuerpo, ev map[string]any) {
		if si, _ := cuerpo["isOnlineMeeting"].(bool); si {
			if cuerpo["onlineMeetingProvider"] != "teamsForBusiness" {
				g.sinTeams = append(g.sinTeams, r.Method)
				return
			}
			g.reunion++
			ev["isOnlineMeeting"] = true
			ev["onlineMeeting"] = map[string]any{"joinUrl": "https://teams.microsoft.com/l/meetup-join/reunion-" + itoa(g.reunion)}
		}
	}
	switch {
	case ruta == "/me":
		escribir(map[string]any{"mail": "ana@example.org"})
	case ruta == "/me/calendars":
		escribir(map[string]any{"value": []any{map[string]any{"id": "cal1", "name": "Calendario", "canEdit": true}}})
	case strings.HasSuffix(ruta, "/calendarView/delta"):
		vs := make([]any, 0, len(g.eventos))
		for _, ev := range g.eventos {
			vs = append(vs, ev)
		}
		escribir(map[string]any{"value": vs, "@odata.deltaLink": g.base + "/v1.0/me/calendars/cal1/calendarView/delta?ronda=" + itoa(g.ronda)})
	case ruta == "/me/calendars/cal1/events" && r.Method == http.MethodPost:
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		g.cuerpos = append(g.cuerpos, cuerpo)
		ev := map[string]any{
			"id": "m" + itoa(len(g.eventos)+1), "iCalUId": "040000008200E00074C5B7101A82E008" + itoa(len(g.eventos)+1),
			"type": "singleInstance", "isOrganizer": true, "subject": cuerpo["subject"], "start": cuerpo["start"], "end": cuerpo["end"],
			"organizer": map[string]any{"emailAddress": map[string]any{"address": "ana@example.org", "name": "Ana"}},
		}
		teams(cuerpo, ev)
		etag(ev)
		g.eventos[ev["id"].(string)] = ev
		escribir(ev)
	case strings.HasPrefix(ruta, "/me/events/"):
		id := strings.TrimPrefix(ruta, "/me/events/")
		ev := g.eventos[id]
		if ev == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodGet:
			escribir(ev)
		case http.MethodPatch:
			if m := r.Header.Get("If-Match"); m != "" && m != ev["@odata.etag"] {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			var cuerpo map[string]any
			_ = json.NewDecoder(r.Body).Decode(&cuerpo)
			g.cuerpos = append(g.cuerpos, cuerpo)
			for _, k := range []string{"subject", "start", "end"} {
				if v, ok := cuerpo[k]; ok {
					ev[k] = v
				}
			}
			teams(cuerpo, ev)
			etag(ev)
			escribir(ev)
		}
	default:
		http.NotFound(w, r)
	}
}

// A-13: «Reunión de Teams» en un evento de un calendario de Microsoft. La subida lo pide
// con `isOnlineMeeting` y `onlineMeetingProvider: teamsForBusiness`, y el evento vuelve con
// el enlace de Teams (CONFERENCE), con el UID de aquí y sin la marca. El que no la pide no
// lleva esos campos.
func TestReunionDeTeamsDesdeElFormulario(t *testing.T) {
	g := &graphConTeams{eventos: map[string]map[string]any{}}
	srv := httptest.NewTLSServer(g)
	defer srv.Close()
	g.base = srv.URL
	dir := t.TempDir()
	repo := abrirCalendario(t, dir)
	s := &agenda.Sincronizador{
		Remoto: agenda.GraphCalendarioDe(srv.URL+"/v1.0", srv.Client(), nil), Local: agenda.LocalCalendario{Repo: repo},
		Estado: filepath.Join(dir, "m.json"), Ventana: agenda.VentanaCalendario,
	}
	sinc := func() agenda.Resultado {
		t.Helper()
		res, err := s.Sincronizar(t.Context())
		if err != nil || len(res.Errores) > 0 {
			t.Fatalf("%v %v", err, res.Errores)
		}
		return res
	}
	sinc()
	cal := s.Colecciones()[0].ID
	conTeams := eventoLocal(t, repo, cal, "Revisión", agenda.ReunionTeams)
	sinTeams := eventoLocal(t, repo, cal, "Comida", "")
	if res := sinc(); res.Subidos != 2 || res.Devueltos != 1 || g.reunion != 1 || len(g.sinTeams) != 0 {
		t.Fatalf("subida: %+v, reuniones %d, %v", res, g.reunion, g.sinTeams)
	}
	pedidas := 0
	for _, c := range g.cuerpos {
		if c["isOnlineMeeting"] == true {
			pedidas++
		} else if _, ok := c["onlineMeetingProvider"]; ok {
			t.Fatalf("el que no la pide no lleva el proveedor: %v", c)
		}
	}
	if pedidas != 1 {
		t.Fatalf("cuerpos: %v", g.cuerpos)
	}
	ev, err := repo.GetEvent(t.Context(), conTeams.ID)
	if err != nil || ev.ConferenceURL != "https://teams.microsoft.com/l/meetup-join/reunion-1" || ev.UID != conTeams.UID {
		t.Fatalf("el enlace de Teams vuelve al evento de aquí con su UID: %q %q %v", ev.ConferenceURL, ev.UID, err)
	}
	if ev, _ := repo.GetEvent(t.Context(), sinTeams.ID); ev.ConferenceURL != "" {
		t.Fatalf("el que no la pidió no tiene reunión: %q", ev.ConferenceURL)
	}
	lista, _ := repo.ObjetosCrudos(t.Context(), cal)
	for _, o := range lista {
		if strings.Contains(o.Datos, agenda.PropReunion) {
			t.Fatalf("la marca no se queda: %s", o.Datos)
		}
	}
	if res := sinc(); res.Subidos != 0 {
		t.Fatalf("la vuelta siguiente no sube nada: %+v", res)
	}
}
