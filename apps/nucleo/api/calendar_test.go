// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

func TestCalendario(t *testing.T) {
	var cuerpo, sustituido map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/calendars":
			_, _ = io.WriteString(w, `{"calendars":[{"id":"c1","name":"personal","display_name":"Personal","is_default":true}]}`)
		case "GET /api/v1/calendars/c1/instances":
			if r.URL.Query().Get("start") != "2026-08-31T22:00:00Z" || r.URL.Query().Get("end") != "2026-10-12T22:00:00Z" {
				t.Errorf("el rango no viaja en UTC y RFC3339: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"instances":[{"event":{"id":"e1","summary":"Cierre","all_day":true},"start":"2026-09-30T00:00:00Z","end":"2026-10-01T00:00:00Z"}]}`)
		case "POST /api/v1/calendars/c1/events":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &cuerpo)
			_, _ = io.WriteString(w, `{"id":"e2","summary":"Gestoría","dtstart":"2026-09-21T08:00:00Z","dtend":"2026-09-21T09:00:00Z"}`)
		case "PUT /api/v1/events/e1":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &sustituido)
			_, _ = io.WriteString(w, `{"id":"e1-bis","summary":"Gestoría (nueva hora)"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := NewCalendars(srv.URL, nil, nil)
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}

	calendarios, err := c.List(context.Background())
	if err != nil || len(calendarios) != 1 || calendarios[0].Title() != "Personal" {
		t.Fatalf("calendarios: %+v, %v", calendarios, err)
	}
	desde := time.Date(2026, 9, 1, 0, 0, 0, 0, madrid)
	ocurrencias, err := c.Instances(context.Background(), "c1", desde, desde.AddDate(0, 0, 42))
	if err != nil || len(ocurrencias) != 1 {
		t.Fatalf("ocurrencias: %+v, %v", ocurrencias, err)
	}
	// Un día completo es flotante: el 30 es el 30 también en Nueva York, donde las
	// 00:00 UTC son todavía la tarde del 29.
	nuevaYork, _ := time.LoadLocation("America/New_York")
	if d := ocurrencias[0].Day(nuevaYork); d.Day() != 30 || d.Month() != time.September {
		t.Errorf("el día completo del 30 cae el %s en Nueva York", d.Format(time.DateOnly))
	}

	inicio := time.Date(2026, 9, 21, 10, 0, 0, 0, madrid)
	ev, err := c.CreateEvent(context.Background(), "c1", EventInput{Summary: "Gestoría", Start: inicio, End: inicio.Add(time.Hour), TZID: "Europe/Madrid"})
	if err != nil || ev.DTStart.IsZero() {
		t.Fatalf("alta: %+v, %v", ev, err)
	}
	// El servicio PIDE start/end; con dtstart/dtend crearía un evento en el año 1.
	if cuerpo["start"] == nil || cuerpo["end"] == nil || cuerpo["dtstart"] != nil {
		t.Errorf("cuerpo del alta: %v", cuerpo)
	}

	// El PUT sustituye: cambiar el título parte del evento ENTERO, o se llevaría por delante
	// la descripción, la videollamada y los invitados, que ningún formulario de Gio enseña.
	guardado := Event{
		ID: "e1", UID: "u1@calendar.sytue.eu", Summary: "Gestoría", Description: "Llevar el modelo 303", Organizer: "ada@sytue.eu",
		DTStart: inicio, DTEnd: inicio.Add(time.Hour), TZID: "Europe/Madrid", Status: "CONFIRMED", Transparency: "OPAQUE",
		ConferenceURL: "https://rooms.sytue.eu/r/abc", Color: "#16734d",
		Attendees: []Attendee{{Email: "grace@sytue.eu", CommonName: "Grace", Role: "REQ-PARTICIPANT", PartStat: "ACCEPTED", RSVP: true}},
	}
	in := guardado.Input()
	in.Summary = "Gestoría (nueva hora)"
	nuevo, err := c.UpdateEvent(context.Background(), "e1", in)
	if err != nil || nuevo.ID != "e1-bis" {
		t.Fatalf("sustituir: %+v, %v", nuevo, err)
	}
	for _, campo := range []string{"start", "end", "description", "organizer", "tzid", "status", "transp", "conference_url", "color", "attendees"} {
		if sustituido[campo] == nil {
			t.Errorf("el PUT no lleva %q: el servicio lo dejaría vacío. Cuerpo: %v", campo, sustituido)
		}
	}
	invitados, _ := sustituido["attendees"].([]any)
	if len(invitados) != 1 || invitados[0].(map[string]any)["partstat"] != "ACCEPTED" {
		t.Errorf("los invitados no viajan con su respuesta: %v", sustituido["attendees"])
	}
	if sustituido["dtstart"] != nil || sustituido["uid"] != nil || sustituido["summary"] != "Gestoría (nueva hora)" {
		t.Errorf("cuerpo del PUT: %v", sustituido)
	}
	if (Event{RRule: "FREQ=WEEKLY"}).Recurring() != true || (Event{RecurrenceID: &inicio}).Recurring() != true || guardado.Recurring() {
		t.Error("Recurring no distingue una serie, o su excepción, de un evento suelto")
	}
}

func TestInvitaciones(t *testing.T) {
	var respuesta map[string]any
	var pedidos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos = append(pedidos, r.Method+" "+r.URL.Path)
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/invitations":
			if r.URL.Query().Get("pending") != "true" {
				t.Errorf("la bandeja no pide solo las pendientes: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"invitations":[
				{"id":"i1","event_id":"e1","uid":"u1","method":"REQUEST","organizer":"MAILTO:ada@sytue.eu","recipient":"grace@sytue.eu","status":"pending","created_at":"2026-09-20T10:00:00Z"},
				{"id":"i2","event_id":"e-ido","uid":"u2","method":"REQUEST","recipient":"grace@sytue.eu","status":"pending","created_at":"2026-09-20T11:00:00Z"},
				{"id":"i3","uid":"u3","method":"REQUEST","recipient":"grace@sytue.eu","status":"pending","created_at":"2026-09-20T12:00:00Z"}]}`)
		case "GET /api/v1/events/e1":
			_, _ = io.WriteString(w, `{"id":"e1","summary":"Repaso del trimestre","dtstart":"2026-09-25T08:00:00Z","dtend":"2026-09-25T09:00:00Z"}`)
		case "GET /api/v1/events/e-ido":
			w.WriteHeader(http.StatusNotFound) // el evento ya no se puede leer: la fila sigue
		case "GET /api/v1/reminders/due":
			_, _ = io.WriteString(w, `{"reminders":[{"id":"r1","event_id":"e1","uid":"u1","summary":"Comité de obra","dtstart":"2026-09-25T08:00:00Z","trigger_at":"2026-09-25T07:50:00Z","action":"DISPLAY"}],"next_cursor":""}`)
		case "POST /api/v1/reminders/r1/dismiss":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/invitations/i1/respond":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &respuesta)
			_, _ = io.WriteString(w, `{"id":"i1","uid":"u1","method":"REQUEST","recipient":"grace@sytue.eu","status":"accepted","created_at":"2026-09-20T10:00:00Z"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewCalendars(srv.URL, nil, nil), context.Background()

	invitaciones, err := c.Invitations(ctx, true)
	if err != nil || len(invitaciones) != 3 {
		t.Fatalf("bandeja: %+v, %v", invitaciones, err)
	}
	// El servicio manda el sobre; el título y la hora los pone el evento. Y quien invita se
	// llama «ada@sytue.eu»: el ORGANIZER de iCalendar es una URI y el «mailto:» sobra.
	if invitaciones[0].Summary != "Repaso del trimestre" || invitaciones[0].Start.IsZero() || invitaciones[0].Organizer != "ada@sytue.eu" {
		t.Errorf("la invitación no se enriquece con su evento: %+v", invitaciones[0])
	}
	// Un evento ilegible, o una invitación sin evento, no tumban la bandeja.
	if invitaciones[1].Summary != "" || invitaciones[2].Summary != "" {
		t.Errorf("enriquecido de más: %+v", invitaciones[1:])
	}
	// Sin event_id NO se pide ningún evento: pedir /events/ sería un 400 por cada fila.
	if slices.Contains(pedidos, "GET /api/v1/events/") {
		t.Errorf("se ha pedido un evento sin id: %v", pedidos)
	}

	recordatorios, err := c.Reminders(ctx)
	if err != nil || len(recordatorios) != 1 || recordatorios[0].Summary != "Comité de obra" || recordatorios[0].DTStart.IsZero() {
		t.Fatalf("recordatorios: %+v, %v", recordatorios, err)
	}
	if err := c.DismissReminder(ctx, "r1"); err != nil {
		t.Fatalf("descartar: %v", err)
	}

	inv, err := c.RespondInvitation(ctx, "i1", InvitacionAceptada)
	if err != nil || inv.Status != "accepted" {
		t.Fatalf("responder: %+v, %v", inv, err)
	}
	if respuesta["status"] != "accepted" {
		t.Errorf("el cuerpo de la respuesta no lleva `status`: %v", respuesta)
	}
}

// TestCompartidosConmigo: el permiso decide si se escribe, y el servicio NO lo dice con
// `read_only` (ese es de las suscripciones ICS). Un «reader» que saliera editable dejaría
// arrastrar un evento ajeno y el servicio contestaría 403 al soltarlo.
func TestCompartidosConmigo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/calendars/shared-with-me" {
			t.Errorf("petición inesperada: %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"calendars":[`+
			`{"id":"a","display_name":"Equipo","permission":"reader","shared_by":"ada@sytue.eu","is_default":true},`+
			`{"id":"b","display_name":"Guardias","permission":"writer","shared_by":"grace@sytue.eu"}]}`)
	}))
	defer srv.Close()
	cals, err := NewCalendars(srv.URL, nil, nil).SharedWithMe(context.Background())
	if err != nil || len(cals) != 2 {
		t.Fatalf("compartidos: %+v, %v", cals, err)
	}
	if !cals[0].ReadOnly || cals[1].ReadOnly {
		t.Errorf("reader tiene que salir de solo lectura y writer no: %+v", cals)
	}
	if cals[0].IsDefault {
		t.Error("el «por defecto» de un compartido es del dueño, no de quien lo recibe")
	}
	if cals[0].SharedBy != "ada@sytue.eu" || cals[1].Permission != PermisoEditar {
		t.Errorf("se pierden el dueño o el permiso: %+v", cals)
	}
}

func TestLosCamposDeCalendarSonLosDelServicio(t *testing.T) {
	dominio := webref.Read(t, "services/calendar/internal/domain/domain.go")
	for _, campo := range []string{
		`json:"dtstart"`, `json:"dtend"`, `json:"all_day"`, `json:"calendar_id"`, `json:"display_name"`, `json:"read_only"`, `json:"is_default"`, `json:"rrule,omitempty"`,
		`json:"classification"`, `json:"transp"`, `json:"rdate,omitempty"`, `json:"exdate,omitempty"`, `json:"recurrence_id,omitempty"`, `json:"attendees,omitempty"`,
		`json:"common_name,omitempty"`, `json:"partstat"`, `json:"cutype,omitempty"`, `json:"rsvp"`,
		// La invitación de la bandeja de planificación (iTIP).
		`json:"event_id,omitempty"`, `json:"method"`, `json:"organizer,omitempty"`, `json:"recipient"`, `json:"status"`,
		`InvAccepted  InvitationStatus = "accepted"`, `InvTentative InvitationStatus = "tentative"`, `InvDeclined  InvitationStatus = "declined"`,
	} {
		if !strings.Contains(dominio, campo) {
			t.Errorf("services/calendar ya no declara %s: revisar internal/api/calendar.go", campo)
		}
	}
	// «Compartidos conmigo» manda el permiso y el dueño con estos nombres.
	if compartir := webref.Read(t, "services/calendar/internal/http/shares.go"); !strings.Contains(compartir, "`json:\"permission\"`") ||
		!strings.Contains(compartir, "`json:\"shared_by\"`") {
		t.Error("services/calendar ya no manda permission/shared_by en shared-with-me: revisar SharedWithMe")
	}
	if permisos := webref.Read(t, "services/calendar/internal/domain/share.go"); !strings.Contains(permisos, `PermReader SharePermission = "reader"`) ||
		!strings.Contains(permisos, `PermWriter SharePermission = "writer"`) {
		t.Error("services/calendar cambió los nombres de los permisos: revisar PermisoLeer y PermisoEditar")
	}
	manejadores := webref.Read(t, "services/calendar/internal/http/handlers.go")
	for _, literal := range []string{
		"`json:\"start\"`", "`json:\"end\"`", `"instances": insts`, `"calendars": cals`, `q.Get("start"), q.Get("end")`,
		// Lo que EventInput manda en el PUT, con los nombres con que el servicio lo lee…
		"`json:\"organizer\"`", "`json:\"status\"`", "`json:\"classification\"`", "`json:\"transp\"`", "`json:\"color\"`", "`json:\"conference_url\"`",
		"`json:\"rrule\"`", "`json:\"rdate\"`", "`json:\"exdate\"`", "`json:\"attendees\"`", "`json:\"common_name\"`", "`json:\"partstat\"`", "`json:\"cutype\"`", "`json:\"rsvp\"`",
		// …y por qué UpdateEvent no cambia de calendario ni manda el UID: los pone el servicio.
		`req.toInput(cur.CalendarID)`, `in.UID = cur.UID`,
		// La bandeja: solo pendientes con ?pending=true, y la respuesta va en `status`.
		`Query().Get("pending") == "true"`, `"invitations": invs`, "Status string `json:\"status\"`",
		// Los recordatorios del evento: en el cuerpo, y en la respuesta del evento suelto.
		"Reminders *[]int `json:\"reminders\"`", "Reminders []int `json:\"reminders\"`", `eventConRecordatorios{Event: ev, Reminders: rec}`,
		// El recordatorio vencido, con los nombres de reminderDTO.
		"EventID   uuid.UUID `json:\"event_id\"`", "Summary   string    `json:\"summary\"`",
		"DTStart   time.Time `json:\"dtstart\"`", "TriggerAt time.Time `json:\"trigger_at\"`", `"reminders": out`,
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/calendar/internal/http/handlers.go ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/calendar/internal/http/server.go")
	for _, ruta := range []string{
		`"/calendars"`, `"/calendars/{id}/instances"`, `"/calendars/{id}/events"`, `api.Delete("/events/{id}"`, `api.Put("/events/{id}"`,
		`api.Get("/events/{id}"`, `api.Get("/invitations"`, `api.Post("/invitations/{id}/respond"`,
		`api.Get("/reminders/due"`, `api.Post("/reminders/{id}/dismiss"`, `api.Get("/calendars/shared-with-me"`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/calendar ya no monta %s", ruta)
		}
	}
	// El PUT borra la fila MAESTRA y la inserta de nuevo: por eso el evento vuelve con otro
	// id, y por eso sustituir la excepción de una serie (recurrence_id) pisaría la serie.
	if almacen := webref.Read(t, "services/calendar/internal/store/events.go"); !strings.Contains(almacen, "DELETE FROM events WHERE tenant=$1 AND object_id=$2 AND recurrence_id IS NULL") {
		t.Error("services/calendar ya no sustituye la fila maestra en el PUT: revisar UpdateEvent y quién puede editarse en apps/calendar")
	}
	// Los días completos son flotantes en UTC: Instance.Day depende de ello.
	if zona := webref.Read(t, "services/calendar/internal/domain/tz.go"); !strings.Contains(zona, "if allDay {\n\t\treturn time.UTC") {
		t.Error("services/calendar ya no guarda los días completos en UTC: revisar Instance.Day")
	}
}

// TestLoQueFaltabaDeCalendar: buscar, contestar desde el evento, recortar una serie,
// refrescar una suscripción y exportarla, con las rutas y los cuerpos del servicio.
func TestLoQueFaltabaDeCalendar(t *testing.T) {
	var partstat, recorte map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/events/search":
			if r.URL.Query().Get("q") != "comité" || r.URL.Query().Get("limit") != "20" || r.URL.Query().Get("cursor") != "k1" {
				t.Errorf("la búsqueda no lleva q, limit y cursor: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"events":[{"id":"e1","summary":"Comité","rrule":"FREQ=WEEKLY"}],"next_cursor":"k2"}`)
		case "PUT /api/v1/events/e1/attendees/partstat":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &partstat)
			_, _ = io.WriteString(w, `{}`)
		case "POST /api/v1/calendars/c1/amend-recurrence":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &recorte)
			_, _ = io.WriteString(w, `{}`)
		case "POST /api/v1/calendars/c1/refresh":
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/calendars/c1/export.ics":
			_, _ = io.WriteString(w, "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
		case "GET /api/v1/freebusy/suggest":
			q := r.URL.Query()
			if q.Get("duration") != "45" || q.Get("tz") != "Europe/Madrid" || q.Get("weekdays") != "true" ||
				q.Get("work_start") != "09:00" || q.Get("work_end") != "17:00" || q.Get("start") != "2026-09-21T00:00:00Z" {
				t.Errorf("buscar hueco pidió %v", q)
			}
			_, _ = io.WriteString(w, `{"slots":[{"start":"2026-09-21T07:00:00Z","end":"2026-09-21T09:00:00Z"}]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := NewCalendars(srv.URL, nil, nil)
	ctx := context.Background()

	b, err := c.SearchEventsPage(ctx, "comité", 20, "k1")
	if err != nil || len(b.Events) != 1 || b.Siguiente != "k2" || !b.Events[0].Recurring() {
		t.Fatalf("búsqueda: %+v, %v", b, err)
	}
	if err := c.SetPartStat(ctx, "e1", AsistenciaQuiza); err != nil || partstat["partstat"] != "TENTATIVE" {
		t.Errorf("partstat: %v, %v", partstat, err)
	}
	inst := time.Date(2026, 9, 21, 8, 0, 0, 0, time.UTC)
	if err := c.AmendRecurrence(ctx, "c1", "u1", inst, RecorteHasta); err != nil ||
		recorte["uid"] != "u1" || recorte["mode"] != "until" || recorte["recurrence_id"] != "2026-09-21T08:00:00Z" {
		t.Errorf("recorte: %v, %v", recorte, err)
	}
	if err := c.RefreshSubscription(ctx, "c1"); err != nil {
		t.Errorf("refrescar: %v", err)
	}
	if ics, err := c.ExportICS(ctx, "c1"); err != nil || !strings.HasPrefix(string(ics), "BEGIN:VCALENDAR") {
		t.Errorf("exportar: %q, %v", ics, err)
	}
	huecos, err := c.SuggestSlots(ctx, inst.Add(-8*time.Hour), inst.Add(7*24*time.Hour), 45, "Europe/Madrid", "09:00", "17:00", true)
	if err != nil || len(huecos) != 1 || huecos[0].End.Sub(huecos[0].Start) != 2*time.Hour {
		t.Errorf("buscar hueco: %+v, %v", huecos, err)
	}
	// Y que el servicio las siga montando con esos nombres.
	rutas := webref.Read(t, "services/calendar/internal/http/server.go")
	for _, ruta := range []string{
		`"/events/search"`, `"/events/{id}/attendees/partstat"`, `"/calendars/{id}/amend-recurrence"`,
		`"/calendars/{id}/refresh"`, `"/calendars/{id}/export.ics"`, `"/freebusy/suggest"`,
	} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/calendar ya no monta %s", ruta)
		}
	}
	if m := webref.Read(t, "services/calendar/internal/http/handlers.go"); !strings.Contains(m, `"next_cursor": next`) ||
		!strings.Contains(m, "Mode         string    `json:\"mode\"`") {
		t.Error("services/calendar cambió next_cursor o el recorte de series")
	}
}

// TestCompartirUnCalendario: listar, compartir y dejar de compartir, con los nombres del
// servicio.
func TestCompartirUnCalendario(t *testing.T) {
	var alta map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/calendars/c1/shares":
			_, _ = io.WriteString(w, `{"shares":[{"id":"s1","grantee_email":"ana@sytue.eu","permission":"writer"}]}`)
		case "POST /api/v1/calendars/c1/shares":
			datos, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(datos, &alta)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"s2"}`)
		case "DELETE /api/v1/calendars/c1/shares/s1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := NewCalendars(srv.URL, nil, nil)
	ctx := context.Background()
	lista, err := c.Shares(ctx, "c1")
	if err != nil || len(lista) != 1 || lista[0].Correo != "ana@sytue.eu" || lista[0].Permission != PermisoEditar {
		t.Fatalf("lista: %+v, %v", lista, err)
	}
	if err := c.Share(ctx, "c1", "bea@sytue.eu", PermisoLlevar); err != nil || alta["email"] != "bea@sytue.eu" || alta["permission"] != "manage" {
		t.Errorf("compartir: %v, %v", alta, err)
	}
	if err := c.RevokeShare(ctx, "c1", "s1"); err != nil {
		t.Errorf("revocar: %v", err)
	}
	if s := webref.Read(t, "services/calendar/internal/domain/share.go"); !strings.Contains(s, "`json:\"grantee_email,omitempty\"`") ||
		!strings.Contains(s, `PermManage SharePermission = "manage"`) {
		t.Error("services/calendar cambió cómo nombra a quién se comparte o el permiso de llevar la agenda")
	}
}

// Los recordatorios del evento: GetEvent los trae, Input los arrastra a la edición, y en
// la petición nil NO viaja (el servicio conserva los que hubiera) mientras que una lista
// vacía sí (los quita).
func TestLosRecordatoriosDelEvento(t *testing.T) {
	var cuerpos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/events/e1":
			_, _ = io.WriteString(w, `{"id":"e1","summary":"Cierre","reminders":[10,1440]}`)
		case "PUT /api/v1/events/e1":
			datos, _ := io.ReadAll(r.Body)
			cuerpos = append(cuerpos, string(datos))
			_, _ = io.WriteString(w, `{"id":"e1-bis"}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := NewCalendars(srv.URL, nil, nil)
	ev, err := c.GetEvent(context.Background(), "e1")
	if err != nil || ev.Reminders == nil || !slices.Equal(*ev.Reminders, []int{10, 1440}) {
		t.Fatalf("GetEvent: %+v, %v", ev.Reminders, err)
	}
	in := ev.Input()
	if in.Reminders == nil || !slices.Equal(*in.Reminders, []int{10, 1440}) {
		t.Fatalf("Input no arrastra los recordatorios: %v", in.Reminders)
	}
	for _, r := range []*[]int{nil, {}, {5}} {
		in.Reminders = r
		if _, err := c.UpdateEvent(context.Background(), "e1", in); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(cuerpos[0], "reminders") || !strings.Contains(cuerpos[1], `"reminders":[]`) || !strings.Contains(cuerpos[2], `"reminders":[5]`) {
		t.Fatalf("los recordatorios no viajan como pide el servicio:\n%s", strings.Join(cuerpos, "\n"))
	}
	// En un listado no vienen: nil, que es «no se sabe», no «ninguno».
	var suelto Event
	if err := json.Unmarshal([]byte(`{"id":"e2"}`), &suelto); err != nil || suelto.Reminders != nil {
		t.Fatalf("un evento sin recordatorios en el JSON: %v %v", suelto.Reminders, err)
	}
}
