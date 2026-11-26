// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
)

// Un Google Calendar de mentira: lista de calendarios, eventos con syncToken (410 con
// uno que no conoce), alta con iCalUID, PATCH con If-Match y sendUpdates=all.
type googleFalso struct {
	mu       sync.Mutex
	eventos  map[string]map[string]any // id → evento
	version  int
	cambios  map[string]int // id → versión en que cambió
	peticion []string
}

func (g *googleFalso) poner(ev map[string]any) {
	g.version++
	ev["etag"] = `"` + time.Now().Format("150405.000000000") + `"`
	g.eventos[ev["id"].(string)] = ev
	g.cambios[ev["id"].(string)] = g.version
}

func (g *googleFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.peticion = append(g.peticion, r.Method+" "+r.URL.String())
	if r.Header.Get("Authorization") != "Bearer token-de-prueba" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/calendar/v3")
	switch {
	case ruta == "/users/me/calendarList":
		escribir(map[string]any{"items": []map[string]any{
			{"id": "ana@gmail.com", "summary": "Ana", "backgroundColor": "#9fe1e7", "accessRole": "owner"},
			{"id": "es.spain#holiday", "summary": "Festivos", "accessRole": "reader"},
		}})
	case strings.HasSuffix(ruta, "/events") && r.Method == http.MethodGet:
		if strings.Contains(ruta, "holiday") {
			escribir(map[string]any{"items": []any{}, "nextSyncToken": "f"})
			return
		}
		q := r.URL.Query()
		desde := 0
		if t := q.Get("syncToken"); t != "" {
			if !strings.HasPrefix(t, "v") {
				w.WriteHeader(http.StatusGone)
				return
			}
			_, _ = io.WriteString(io.Discard, t)
			for _, c := range t[1:] {
				desde = desde*10 + int(c-'0')
			}
		}
		var items []any
		for id, ev := range g.eventos {
			if q.Get("iCalUID") != "" {
				if ev["iCalUID"] == q.Get("iCalUID") {
					items = append(items, ev)
				}
				continue
			}
			if g.cambios[id] > desde {
				items = append(items, ev)
			}
		}
		escribir(map[string]any{"items": items, "nextSyncToken": "v" + itoa(g.version)})
	case strings.HasSuffix(ruta, "/events") && r.Method == http.MethodPost:
		if r.URL.Query().Get("sendUpdates") != "all" {
			http.Error(w, "sin sendUpdates", http.StatusBadRequest)
			return
		}
		var ev map[string]any
		_ = json.NewDecoder(r.Body).Decode(&ev)
		ev["id"] = "nuevo" + itoa(g.version)
		g.poner(ev)
		escribir(ev)
	case strings.Contains(ruta, "/events/"):
		id := ruta[strings.LastIndex(ruta, "/")+1:]
		ev := g.eventos[id]
		if ev == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if m := r.Header.Get("If-Match"); m != "" && m != ev["etag"] {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		switch r.Method {
		case http.MethodGet:
			escribir(ev)
		case http.MethodPatch:
			var cambio map[string]any
			_ = json.NewDecoder(r.Body).Decode(&cambio)
			for k, v := range cambio {
				ev[k] = v
			}
			g.poner(ev)
			escribir(ev)
		case http.MethodDelete:
			ev["status"] = "cancelled"
			g.poner(ev)
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		http.NotFound(w, r)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestGoogleCalendar(t *testing.T) {
	g := &googleFalso{eventos: map[string]map[string]any{}, cambios: map[string]int{}}
	manana := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Hour)
	g.poner(map[string]any{
		"id": "e1", "iCalUID": "e1@google.com", "status": "confirmed", "summary": "Dentista",
		"start":     map[string]any{"dateTime": manana.Format(time.RFC3339), "timeZone": "Europe/Madrid"},
		"end":       map[string]any{"dateTime": manana.Add(time.Hour).Format(time.RFC3339), "timeZone": "Europe/Madrid"},
		"attendees": []any{map[string]any{"email": "luis@example.org", "responseStatus": "accepted"}},
	})
	g.poner(map[string]any{
		"id": "s1", "iCalUID": "s1@google.com", "status": "confirmed", "summary": "Yoga",
		"start": map[string]any{"date": manana.Format(time.DateOnly)}, "end": map[string]any{"date": manana.AddDate(0, 0, 1).Format(time.DateOnly)},
		"recurrence": []any{"RRULE:FREQ=WEEKLY;COUNT=4"},
	})
	g.poner(map[string]any{
		"id": "s1_x", "iCalUID": "s1@google.com", "recurringEventId": "s1", "status": "cancelled",
		"originalStartTime": map[string]any{"date": manana.AddDate(0, 0, 7).Format(time.DateOnly)},
	})
	srv := httptest.NewTLSServer(g)
	defer srv.Close()

	dir := t.TempDir()
	repo := abrirCalendario(t, dir)
	s := &agenda.Sincronizador{
		Remoto: agenda.GoogleCalendarDe(srv.URL+"/calendar/v3", srv.Client()), Local: agenda.LocalCalendario{Repo: repo},
		Estado: filepath.Join(dir, "g.json"), Ventana: agenda.VentanaCalendario, MarcaCopia: "(copia)",
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
	var cal string
	for _, c := range s.Colecciones() {
		if c.Nombre == "Ana" {
			cal = c.ID
		}
		if c.Nombre == "Festivos" && !c.SoloLectura {
			t.Fatal("el de festivos no es de solo lectura")
		}
	}
	objs, _ := repo.ObjetosCrudos(t.Context(), cal)
	por := map[string]string{}
	for _, o := range objs {
		por[o.UID] = o.Datos
	}
	if !strings.Contains(por["e1@google.com"], "Dentista") || !strings.Contains(por["e1@google.com"], "TZID=Europe/Madrid") || !strings.Contains(por["e1@google.com"], "PARTSTAT=ACCEPTED") {
		t.Fatalf("evento: %q", por["e1@google.com"])
	}
	if !strings.Contains(por["s1@google.com"], "RRULE:FREQ=WEEKLY;COUNT=4") || !strings.Contains(por["s1@google.com"], "EXDATE;VALUE=DATE") {
		t.Fatalf("serie: %q", por["s1@google.com"])
	}
	inst, _ := repo.Instances(t.Context(), cal, manana.Add(-time.Hour), manana.AddDate(0, 1, 0))
	if len(inst) != 1+3 {
		t.Fatalf("ocurrencias: %d", len(inst))
	}

	// Cambiado aquí: PATCH con If-Match. Nuevo aquí: POST con su UID.
	for _, o := range objs {
		if o.UID == "e1@google.com" {
			nuevo := strings.Replace(o.Datos, "SUMMARY:Dentista", "SUMMARY:Dentista (Dr. Ruiz)", 1)
			if _, err := repo.PonerObjetoCrudo(t.Context(), cal, o.ID, "", nuevo); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := repo.PonerObjetoCrudo(t.Context(), cal, "", "", ics("local-uid", "Cena", manana.Add(5*time.Hour), "")); err != nil {
		t.Fatal(err)
	}
	if res := sinc(); res.Subidos != 2 {
		t.Fatalf("subida: %+v %v", res, g.peticion)
	}
	if g.eventos["e1"]["summary"] != "Dentista (Dr. Ruiz)" {
		t.Fatalf("no se cambió en Google: %v", g.eventos["e1"])
	}
	creado := false
	for _, ev := range g.eventos {
		if ev["iCalUID"] == "local-uid" && ev["summary"] == "Cena" {
			creado = true
		}
	}
	if !creado {
		t.Fatal("no se creó en Google con su UID")
	}
	patch := false
	for _, p := range g.peticion {
		if strings.HasPrefix(p, "PATCH") && strings.Contains(p, "sendUpdates=all") {
			patch = true
		}
	}
	if !patch {
		t.Fatalf("sin PATCH con invitaciones: %v", g.peticion)
	}

	// Borrado en Google: se borra aquí.
	g.mu.Lock()
	g.eventos["e1"]["status"] = "cancelled"
	g.poner(g.eventos["e1"])
	g.mu.Unlock()
	sinc()
	objs, _ = repo.ObjetosCrudos(t.Context(), cal)
	for _, o := range objs {
		if o.UID == "e1@google.com" {
			t.Fatal("no se borró aquí")
		}
	}
}

// Un Graph de mentira: calendarios, calendarView/delta con deltaLink, la serie por su
// maestro y sus instancias, alta y PATCH con If-Match; contactos con delta.
type graphFalso struct {
	mu        sync.Mutex
	base      string
	eventos   map[string]map[string]any
	contactos map[string]map[string]any
	ronda     int
	peticion  []string
}

func (g *graphFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.peticion = append(g.peticion, r.Method+" "+r.URL.String())
	escribir := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	ruta := strings.TrimPrefix(r.URL.Path, "/v1.0")
	switch {
	case ruta == "/me/calendars":
		escribir(map[string]any{"value": []any{map[string]any{"id": "cal1", "name": "Calendario", "canEdit": true, "hexColor": "#ff8c00"}}})
	case strings.HasSuffix(ruta, "/calendarView/delta"):
		var vs []any
		if r.URL.Query().Get("ronda") == "" {
			for _, ev := range g.eventos {
				if ev["type"] == "seriesMaster" {
					vs = append(vs, map[string]any{"id": "occ1", "type": "occurrence", "seriesMasterId": ev["id"]})
					continue
				}
				vs = append(vs, ev)
			}
		} else {
			vs = append(vs, map[string]any{"id": "u1", "@removed": map[string]any{"reason": "deleted"}})
		}
		escribir(map[string]any{"value": vs, "@odata.deltaLink": g.base + "/v1.0/me/calendars/cal1/calendarView/delta?ronda=2"})
	case strings.HasPrefix(ruta, "/me/events/") && strings.HasSuffix(ruta, "/instances"):
		dias := []string{"01", "15", "22", "29"}
		vs := make([]any, 0, 1+len(dias))
		vs = append(vs, map[string]any{
			"id": "exc1", "type": "exception", "seriesMasterId": "m1", "subject": "Reunión (otra sala)",
			"originalStart": "2026-12-08T09:00:00Z", "start": map[string]any{"dateTime": "2026-12-08T11:00:00.0000000", "timeZone": "UTC"},
			"end": map[string]any{"dateTime": "2026-12-08T12:00:00.0000000", "timeZone": "UTC"},
		})
		for _, d := range dias {
			vs = append(vs, map[string]any{
				"id": "occ" + d, "type": "occurrence", "seriesMasterId": "m1", "subject": "Reunión",
				"start": map[string]any{"dateTime": "2026-12-" + d + "T09:00:00.0000000", "timeZone": "UTC"},
				"end":   map[string]any{"dateTime": "2026-12-" + d + "T10:00:00.0000000", "timeZone": "UTC"},
			})
		}
		escribir(map[string]any{"value": vs})
	case strings.HasPrefix(ruta, "/me/events/"):
		id := strings.TrimPrefix(ruta, "/me/events/")
		ev := g.eventos[id]
		if ev == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if m := r.Header.Get("If-Match"); m != "" && m != ev["@odata.etag"] {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if r.Method == http.MethodPatch {
			var c map[string]any
			_ = json.NewDecoder(r.Body).Decode(&c)
			for k, v := range c {
				ev[k] = v
			}
			g.ronda++
			ev["@odata.etag"] = "W/\"" + itoa(g.ronda) + "\""
		}
		escribir(ev)
	case ruta == "/me/calendars/cal1/events" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		g.ronda++
		c["id"], c["@odata.etag"] = "n"+itoa(g.ronda), "W/\"n\""
		g.eventos[c["id"].(string)] = c
		escribir(c)
	case ruta == "/me/contactFolders":
		escribir(map[string]any{"value": []any{}})
	case ruta == "/me/contacts/delta":
		var vs []any
		for _, c := range g.contactos {
			vs = append(vs, c)
		}
		escribir(map[string]any{"value": vs, "@odata.deltaLink": g.base + "/v1.0/me/contacts/delta?token=1"})
	case ruta == "/me/contacts" && r.Method == http.MethodPost:
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		g.ronda++
		c["id"], c["@odata.etag"] = "k"+itoa(g.ronda), "W/\"k\""
		g.contactos[c["id"].(string)] = c
		escribir(c)
	default:
		http.NotFound(w, r)
	}
}

func TestGraphCalendarioYContactos(t *testing.T) {
	g := &graphFalso{eventos: map[string]map[string]any{}, contactos: map[string]map[string]any{}}
	g.eventos["u1"] = map[string]any{
		"id": "u1", "@odata.etag": "W/\"1\"", "type": "singleInstance", "iCalUId": "uid-u1", "subject": "Revisión",
		"body":  map[string]any{"contentType": "html", "content": "<p>Traer <b>el informe</b></p>"},
		"start": map[string]any{"dateTime": "2026-12-01T09:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T10:00:00.0000000", "timeZone": "UTC"},
		"attendees": []any{map[string]any{"emailAddress": map[string]any{"address": "luis@example.org", "name": "Luis"}, "status": map[string]any{"response": "tentativelyAccepted"}}},
	}
	g.eventos["m1"] = map[string]any{
		"id": "m1", "@odata.etag": "W/\"2\"", "type": "seriesMaster", "iCalUId": "uid-m1", "subject": "Reunión",
		"start": map[string]any{"dateTime": "2026-12-01T09:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T10:00:00.0000000", "timeZone": "UTC"},
		"recurrence": map[string]any{
			"pattern": map[string]any{"type": "weekly", "interval": 1, "daysOfWeek": []any{"tuesday"}},
			"range":   map[string]any{"type": "numbered", "startDate": "2026-12-01", "numberOfOccurrences": 5},
		},
	}
	g.contactos["c1"] = map[string]any{
		"id": "c1", "@odata.etag": "W/\"c\"", "displayName": "Marta Ruiz", "givenName": "Marta", "surname": "Ruiz",
		"emailAddresses": []any{map[string]any{"address": "marta@example.org"}}, "mobilePhone": "+34600111222",
	}
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
	res, err := s.Sincronizar(t.Context())
	if err != nil || len(res.Errores) > 0 || res.Bajados != 2 {
		t.Fatalf("bajada: %+v %v %v", res, err, g.peticion)
	}
	cal := s.Colecciones()[0].ID
	objs, _ := repo.ObjetosCrudos(t.Context(), cal)
	por := map[string]string{}
	for _, o := range objs {
		por[o.UID] = o.Datos
	}
	if !strings.Contains(por["uid-u1"], "Traer el informe") || !strings.Contains(por["uid-u1"], "PARTSTAT=TENTATIVE") {
		t.Fatalf("suelto: %q", por["uid-u1"])
	}
	if !strings.Contains(por["uid-m1"], "RRULE:FREQ=WEEKLY;BYDAY=TU;COUNT=5") || !strings.Contains(por["uid-m1"], "RECURRENCE-ID:20261208T090000Z") ||
		strings.Contains(por["uid-m1"], "EXDATE") {
		t.Fatalf("serie: %q", por["uid-m1"])
	}
	// Cambio aquí: PATCH con If-Match y la regla de vuelta en el formato de Graph.
	for _, o := range objs {
		if o.UID == "uid-m1" {
			nuevo := strings.Replace(o.Datos, "SUMMARY:Reunión\r\n", "SUMMARY:Reunión semanal\r\n", 1)
			if _, err := repo.PonerObjetoCrudo(t.Context(), cal, o.ID, "", nuevo); err != nil {
				t.Fatal(err)
			}
		}
	}
	if res, err := s.Sincronizar(t.Context()); err != nil || res.Subidos != 1 {
		t.Fatalf("subida: %+v %v", res, err)
	}
	m1 := g.eventos["m1"]
	rec, _ := json.Marshal(m1["recurrence"])
	if m1["subject"] != "Reunión semanal" || !strings.Contains(string(rec), `"weekly"`) || !strings.Contains(string(rec), `"numbered"`) {
		t.Fatalf("en Graph: %v %s", m1["subject"], rec)
	}
	// La ronda siguiente del delta borra u1.
	if _, err := s.Sincronizar(t.Context()); err != nil {
		t.Fatal(err)
	}
	objs, _ = repo.ObjetosCrudos(t.Context(), cal)
	for _, o := range objs {
		if o.UID == "uid-u1" {
			t.Fatal("no se borró aquí")
		}
	}

	// Contactos: el de Graph baja; el de aquí sube; el UID de aquí se conserva.
	con := abrirContactos(t, t.TempDir())
	sc := &agenda.Sincronizador{Remoto: agenda.GraphContactosDe(srv.URL+"/v1.0", srv.Client()), Local: agenda.LocalContactos{Repo: con}, Estado: filepath.Join(dir, "k.json")}
	if res, err := sc.Sincronizar(t.Context()); err != nil || res.Bajados != 1 {
		t.Fatalf("contactos: %+v %v", res, err)
	}
	libro := sc.Colecciones()[0].ID
	ts, _ := con.TarjetasCrudas(t.Context(), libro)
	if len(ts) != 1 || !strings.Contains(ts[0].Datos, "Marta Ruiz") || !strings.Contains(ts[0].Datos, "+34600111222") {
		t.Fatalf("tarjeta: %+v", ts)
	}
	if _, err := con.PonerTarjetaCruda(t.Context(), libro, "", "", vcf("propio", "Pablo")); err != nil {
		t.Fatal(err)
	}
	if res, err := sc.Sincronizar(t.Context()); err != nil || res.Subidos != 1 {
		t.Fatalf("subida contactos: %+v %v", res, err)
	}
	// El delta devuelve el creado con el id de Graph: aquí sigue siendo el mismo
	// contacto, con su UID.
	if _, err := sc.Sincronizar(t.Context()); err != nil {
		t.Fatal(err)
	}
	ts, _ = con.TarjetasCrudas(t.Context(), libro)
	uids := map[string]bool{}
	for _, x := range ts {
		uids[x.UID] = true
	}
	if len(ts) != 2 || !uids["propio"] {
		t.Fatalf("tras la vuelta: %+v", ts)
	}
}

func TestRecurrenciaGraph(t *testing.T) {
	if r := agenda.RRULEDeGraph("relativeMonthly", 2, []string{"monday"}, "last", "noEnd", 0); r != "FREQ=MONTHLY;BYDAY=MO;BYSETPOS=-1;INTERVAL=2" {
		t.Fatalf("relativeMonthly: %q", r)
	}
	inicio := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	for _, r := range []string{"FREQ=DAILY;COUNT=3", "FREQ=WEEKLY;BYDAY=MO,WE;INTERVAL=2", "FREQ=MONTHLY;BYDAY=-1FR", "FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=7"} {
		if _, err := agenda.GraphDeRRULE(r, inicio); err != nil {
			t.Fatalf("%s: %v", r, err)
		}
	}
	// «BYDAY=M» no es un día: antes, cortarle los dos últimos caracteres era un pánico.
	for _, r := range []string{"FREQ=HOURLY", "FREQ=WEEKLY;BYDAY=M"} {
		if _, err := agenda.GraphDeRRULE(r, inicio); err == nil {
			t.Fatalf("aceptó una regla que Graph no guarda: %s", r)
		}
	}
}

// Google Contacts: el UID de aquí viaja en clientData; un contacto borrado allí se va.
func TestGooglePeople(t *testing.T) {
	var mu sync.Mutex
	personas := map[string]map[string]any{
		"people/c1": {
			"resourceName": "people/c1", "etag": "e1", "names": []any{map[string]any{"displayName": "Eva Gil", "givenName": "Eva", "familyName": "Gil"}},
			"emailAddresses": []any{map[string]any{"value": "eva@example.org", "type": "work"}}, "birthdays": []any{map[string]any{"date": map[string]any{"month": 4, "day": 9}}},
		},
	}
	var creada map[string]any
	ronda := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/people/me/connections":
			ronda++
			var cs []any
			if r.URL.Query().Get("syncToken") == "" {
				for _, p := range personas {
					cs = append(cs, p)
				}
			} else if creada != nil {
				cs = append(cs, creada, map[string]any{"resourceName": "people/c1", "metadata": map[string]any{"deleted": true}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"connections": cs, "nextSyncToken": "s" + itoa(ronda)})
		case "/v1/people:createContact":
			_ = json.NewDecoder(r.Body).Decode(&creada)
			creada["resourceName"], creada["etag"] = "people/n1", "en"
			_ = json.NewEncoder(w).Encode(creada)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	con := abrirContactos(t, t.TempDir())
	sc := &agenda.Sincronizador{Remoto: agenda.GooglePeopleDe(srv.URL+"/v1", srv.Client()), Local: agenda.LocalContactos{Repo: con}, Estado: filepath.Join(t.TempDir(), "p.json")}
	if res, err := sc.Sincronizar(t.Context()); err != nil || res.Bajados != 1 {
		t.Fatalf("bajada: %+v %v", res, err)
	}
	libro := sc.Colecciones()[0].ID
	ts, _ := con.TarjetasCrudas(t.Context(), libro)
	if len(ts) != 1 || !strings.Contains(ts[0].Datos, "Eva Gil") || !strings.Contains(ts[0].Datos, "--0409") && !strings.Contains(ts[0].Datos, "--04-09") {
		t.Fatalf("tarjeta: %+v", ts)
	}
	if _, err := con.PonerTarjetaCruda(t.Context(), libro, "", "", vcf("mio", "Iker")); err != nil {
		t.Fatal(err)
	}
	if res, err := sc.Sincronizar(t.Context()); err != nil || res.Subidos != 1 {
		t.Fatalf("subida: %+v %v", res, err)
	}
	cd, _ := json.Marshal(creada["clientData"])
	if !strings.Contains(string(cd), `"mio"`) {
		t.Fatalf("sin el UID de aquí: %s", cd)
	}
	if _, err := sc.Sincronizar(t.Context()); err != nil {
		t.Fatal(err)
	}
	ts, _ = con.TarjetasCrudas(t.Context(), libro)
	if len(ts) != 1 || ts[0].UID != "mio" {
		t.Fatalf("tras borrar allí: %+v", ts)
	}
}
