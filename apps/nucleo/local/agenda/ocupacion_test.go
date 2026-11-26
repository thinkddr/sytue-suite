// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package agenda_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cuentas/dav/davprueba"
	"git.sytue.eu/sytue/one/apps/nucleo/local/agenda"
	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
	"git.sytue.eu/sytue/one/apps/nucleo/local/secretos"
)

// La cuenta CalDAV da la dirección de la persona (el ORGANIZER de lo que se crea aquí)
// en la sincronización, y sin red se sigue sabiendo; la ocupación de los invitados se
// pregunta por su buzón de salida (A-13), y una libreta nueva se crea en la cuenta
// (MKCOL) y baja con la sincronización siguiente.
func TestCuentaDAVDireccionOcupacionYLibretaNueva(t *testing.T) {
	srv := davprueba.Nuevo()
	defer srv.Close()
	srv.Planifica = true
	srv.Usuario = "ana" // un usuario que no es un correo: la dirección la da el servidor
	srv.Correo = "ana.garcia@example.org"
	srv.Crear(davprueba.CasaCal+"personal/", "Personal", false)
	srv.Crear(davprueba.CasaCard+"libreta/", "Libreta", false)
	ajustes := t.TempDir()
	r := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), secretos.SoloFichero(filepath.Join(ajustes, "secretos")), nil)
	r.HTTP = func(red.Proposito) *http.Client { return srv.Client() }
	c, err := r.AnadirDAV(t.Context(), agenda.Alta{Rotulo: "Nextcloud", Usuario: srv.Usuario, Clave: srv.Clave, Calendario: srv.URL + "/cal/", Contactos: srv.URL + "/card/"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Direccion(c.ID) != "" {
		t.Fatal("una dirección inventada antes de sincronizar")
	}
	cal := abrirCalendario(t, t.TempDir())
	dir := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Calendario, agenda.LocalCalendario{Repo: cal}, dir, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	if d := r.Direccion(c.ID); d != "ana.garcia@example.org" {
		t.Fatalf("dirección: %q", d)
	}
	gs := r.Grupos(agenda.Calendario, agenda.LocalCalendario{Repo: cal}, dir)
	if len(gs) != 1 || len(gs[0].Colecciones) != 1 || !gs[0].Colecciones[0].Planifica {
		t.Fatalf("planifica: %+v", gs)
	}
	// Otra apertura (sin red): la dirección está en el estado.
	r2 := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), secretos.SoloFichero(filepath.Join(ajustes, "secretos")), nil)
	r2.HTTP = func(red.Proposito) *http.Client { return srv.Client() }
	r2.Grupos(agenda.Calendario, agenda.LocalCalendario{Repo: cal}, dir)
	srv.Caido = true
	if d := r2.Direccion(c.ID); d != "ana.garcia@example.org" {
		t.Fatalf("dirección sin red: %q", d)
	}
	srv.Caido = false

	dia := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	srv.Ocupado("luis@example.org", dia.Add(10*time.Hour), dia.Add(11*time.Hour))
	m, err := r.Ocupacion(t.Context(), c.ID, []string{"Luis@example.org", "nadie@example.org", "luis@example.org", "no-es-un-correo"}, dia, dia.Add(24*time.Hour))
	if err != nil || len(m) != 1 || len(m["luis@example.org"]) != 1 {
		t.Fatalf("ocupación: %+v %v", m, err)
	}
	if q := srv.Preguntas[len(srv.Preguntas)-1]; !strings.Contains(q, "ORGANIZER:mailto:ana.garcia@example.org") || strings.Count(q, "ATTENDEE") != 2 {
		t.Fatalf("pregunta: %q", q)
	}
	// Sin buzón de salida, el proveedor no la da.
	srv.SinBuzon = true
	r3 := agenda.AbrirEn(filepath.Join(ajustes, "agenda.json"), secretos.SoloFichero(filepath.Join(ajustes, "secretos")), nil)
	r3.HTTP = r.HTTP
	if _, err := r3.Ocupacion(t.Context(), c.ID, []string{"luis@example.org"}, dia, dia.Add(time.Hour)); !errors.Is(err, agenda.ErrNoSoportado) {
		t.Fatalf("sin buzón: %v", err)
	}

	// Libretas: se crean en la cuenta y bajan.
	if !r.PuedeCrear(c, agenda.Contactos) {
		t.Fatal("CardDAV sabe crear libretas")
	}
	if err := r.CrearLibreta(t.Context(), c.ID, "Proveedores"); err != nil {
		t.Fatal(err)
	}
	if ls := srv.Libretas(); len(ls) != 2 {
		t.Fatalf("en el servidor: %v", ls)
	}
	con := abrirContactos(t, t.TempDir())
	dirC := filepath.Join(t.TempDir(), "agenda")
	if _, errs := r.SincronizarTodo(t.Context(), agenda.Contactos, agenda.LocalContactos{Repo: con}, dirC, ""); len(errs) > 0 {
		t.Fatal(errs)
	}
	libretas := r.Grupos(agenda.Contactos, agenda.LocalContactos{Repo: con}, dirC)[0].Colecciones
	nombres := make([]string, 0, len(libretas))
	for _, x := range libretas {
		nombres = append(nombres, x.Nombre)
	}
	if !slices.Equal(nombres, []string{"Libreta", "Proveedores"}) {
		t.Fatalf("libretas: %v", nombres)
	}
}

// Google (`freeBusy`) y Microsoft (`getSchedule`) dan la ocupación de los invitados que
// conocen; a quien no, no se le inventa nada. Las dos cuentas dan su dirección.
func TestOcupacionDeGoogleYMicrosoft(t *testing.T) {
	var mu sync.Mutex
	var cuerpos []map[string]any
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var c map[string]any
		_ = json.NewDecoder(r.Body).Decode(&c)
		cuerpos = append(cuerpos, c)
		switch r.URL.Path {
		case "/calendar/v3/users/me/calendarList":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{map[string]any{"id": "Ana@gmail.com", "summary": "Ana", "accessRole": "owner", "primary": true}}})
		case "/calendar/v3/freeBusy":
			_ = json.NewEncoder(w).Encode(map[string]any{"calendars": map[string]any{
				"luis@example.org":  map[string]any{"busy": []any{map[string]any{"start": "2026-12-01T10:00:00Z", "end": "2026-12-01T11:30:00Z"}}},
				"nadie@example.org": map[string]any{"errors": []any{map[string]any{"domain": "global", "reason": "notFound"}}},
			}})
		case "/v1.0/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"mail": "Ana@Contoso.com"})
		case "/v1.0/me/calendar/getSchedule":
			if r.Header.Get("Prefer") != `outlook.timezone="UTC"` {
				http.Error(w, "sin zona", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": []any{
				map[string]any{"scheduleId": "luis@example.org", "scheduleItems": []any{
					map[string]any{"status": "busy", "start": map[string]any{"dateTime": "2026-12-01T09:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T10:00:00.0000000", "timeZone": "UTC"}},
					map[string]any{"status": "tentative", "start": map[string]any{"dateTime": "2026-12-01T12:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T12:30:00.0000000", "timeZone": "UTC"}},
					map[string]any{"status": "free", "start": map[string]any{"dateTime": "2026-12-01T13:00:00.0000000", "timeZone": "UTC"}, "end": map[string]any{"dateTime": "2026-12-01T14:00:00.0000000", "timeZone": "UTC"}},
				}},
				map[string]any{"scheduleId": "nadie@example.org", "error": map[string]any{"message": "no existe", "responseCode": "ErrorMailRecipientNotFound"}},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dia := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	quienes := []string{"luis@example.org", "nadie@example.org"}

	g := agenda.GoogleCalendarDe(srv.URL+"/calendar/v3", srv.Client())
	if _, err := g.Colecciones(t.Context()); err != nil {
		t.Fatal(err)
	}
	if d, _ := g.(agenda.Direccionado).Direccion(t.Context()); d != "ana@gmail.com" {
		t.Fatalf("dirección de Google: %q", d)
	}
	m, err := g.(agenda.Ocupacion).Ocupacion(t.Context(), "", quienes, dia, dia.Add(24*time.Hour))
	if err != nil || len(m) != 1 || len(m["luis@example.org"]) != 1 || m["luis@example.org"][0].Hasta.Sub(m["luis@example.org"][0].Desde) != 90*time.Minute {
		t.Fatalf("Google: %+v %v", m, err)
	}
	items, _ := json.Marshal(cuerpos[len(cuerpos)-1]["items"])
	if !strings.Contains(string(items), `{"id":"luis@example.org"}`) || cuerpos[len(cuerpos)-1]["timeMin"] != "2026-12-01T00:00:00Z" {
		t.Fatalf("pregunta a Google: %v", cuerpos[len(cuerpos)-1])
	}

	ms := agenda.GraphCalendarioDe(srv.URL+"/v1.0", srv.Client(), nil)
	if d, _ := ms.(agenda.Direccionado).Direccion(t.Context()); d != "ana@contoso.com" {
		t.Fatalf("dirección de Microsoft: %q", d)
	}
	m, err = ms.(agenda.Ocupacion).Ocupacion(t.Context(), "", quienes, dia, dia.Add(24*time.Hour))
	if err != nil || len(m) != 1 || len(m["luis@example.org"]) != 2 || m["luis@example.org"][0].Desde.Hour() != 9 || !m["luis@example.org"][1].Provisional {
		t.Fatalf("Microsoft: %+v %v", m, err)
	}
}
