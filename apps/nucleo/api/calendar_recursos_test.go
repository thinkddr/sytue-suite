// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// Salas y recursos: listar con franja, alta, edición, baja y reserva, con las rutas, los
// verbos y los cuerpos del servicio.
func TestLasSalasYLosRecursos(t *testing.T) {
	var hechas []string
	cuerpos := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta := r.Method + " " + r.URL.Path
		hechas = append(hechas, ruta)
		if datos, _ := io.ReadAll(r.Body); len(datos) > 0 {
			var c map[string]any
			_ = json.Unmarshal(datos, &c)
			cuerpos[ruta] = c
		}
		switch ruta {
		case "GET /api/v1/resources":
			if r.URL.Query().Get("start") != "2026-09-22T08:00:00Z" || r.URL.Query().Get("end") != "2026-09-22T09:00:00Z" {
				t.Errorf("la franja no viaja en UTC y RFC3339: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"resources":[{"id":"r1","kind":"room","name":"Sala Curie","capacity":8,"auto_accept":true,"active":true,
				"busy":[{"start":"2026-09-22T08:30:00Z","end":"2026-09-22T09:30:00Z","type":"BUSY"}]}]}`)
		case "POST /api/v1/resources", "PATCH /api/v1/resources/r1":
			_, _ = io.WriteString(w, `{"id":"r1","kind":"room","name":"Sala Curie"}`)
		case "DELETE /api/v1/resources/r1":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/resources/r1/book":
			_, _ = io.WriteString(w, `{"status":"declined","resource":{"id":"r1"}}`)
		default:
			t.Errorf("petición inesperada: %s", ruta)
		}
	}))
	defer srv.Close()
	c := NewCalendars(srv.URL, nil, nil)
	ctx := context.Background()
	desde := time.Date(2026, 9, 22, 10, 0, 0, 0, time.FixedZone("CEST", 2*3600))

	l, err := c.Resources(ctx, desde, desde.Add(time.Hour))
	if err != nil || len(l) != 1 || l[0].Capacity != 8 || !l[0].Ocupado(desde, desde.Add(time.Hour)) {
		t.Fatalf("Resources: %+v, %v", l, err)
	}
	if l[0].Ocupado(desde.Add(-time.Hour), desde.Add(30*time.Minute)) {
		t.Error("[09:00, 10:30) no cruza [10:30, 11:30) y sale ocupado")
	}
	if _, err := c.CreateResource(ctx, ResourceInput{Kind: RecursoSala, Name: "Sala Curie", Capacity: 8, AutoAccept: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.UpdateResource(ctx, "r1", ResourceInput{Kind: RecursoSala, Name: "Sala Curie", Location: "Planta 2"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteResource(ctx, "r1"); err != nil {
		t.Fatal(err)
	}
	estado, err := c.BookResource(ctx, "r1", Booking{UID: "u1@calendar.sytue.eu", Summary: "Comité", Start: desde, End: desde.Add(time.Hour), TZID: "Europe/Madrid"})
	if err != nil || estado != ReservaRechazada {
		t.Fatalf("BookResource: %q, %v", estado, err)
	}
	if alta := cuerpos["POST /api/v1/resources"]; alta["kind"] != "room" || alta["capacity"] != 8.0 || alta["auto_accept"] != true {
		t.Errorf("alta: %v", alta)
	}
	if cambio := cuerpos["PATCH /api/v1/resources/r1"]; cambio["location"] != "Planta 2" || cambio["kind"] != nil {
		t.Errorf("la edición no puede mandar la clase: %v", cambio)
	}
	if reserva := cuerpos["POST /api/v1/resources/r1/book"]; reserva["uid"] != "u1@calendar.sytue.eu" || reserva["start"] != "2026-09-22T10:00:00+02:00" || reserva["tzid"] != "Europe/Madrid" {
		t.Errorf("reserva: %v", reserva)
	}

	// Y el servicio sigue hablando así.
	manejadores := webref.Read(t, "services/calendar/internal/http/resources.go")
	for _, literal := range []string{
		`map[string]any{"resources": out}`, "Busy []domain.FreeBusyPeriod `json:\"busy\"`",
		"Kind       string   `json:\"kind\"`", "AutoAccept *bool    `json:\"auto_accept\"`",
		"UID     string `json:\"uid\"`", "TZID      string      `json:\"tzid\"`", `"status": bookingStatusLabel(res.Status)`,
		`return "accepted"`, `return "declined"`, `return "pending"`,
	} {
		if !strings.Contains(manejadores, literal) {
			t.Errorf("services/calendar/internal/http/resources.go ya no dice %s", literal)
		}
	}
	rutas := webref.Read(t, "services/calendar/internal/http/server.go")
	for _, ruta := range []string{`api.Get("/resources"`, `api.Post("/resources"`, `api.Patch("/resources/{id}"`, `api.Delete("/resources/{id}"`, `api.Post("/resources/{id}/book"`} {
		if !strings.Contains(rutas, ruta) {
			t.Errorf("services/calendar ya no monta %s", ruta)
		}
	}
}
