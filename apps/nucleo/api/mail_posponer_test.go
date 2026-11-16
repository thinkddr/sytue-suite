// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPosponerYSilenciarHablanConElServicio: las rutas de D-MAIL-07 con sus cuerpos, y
// los rechazos con código propio traducidos a errores que la app sabe contar.
func TestPosponerYSilenciarHablanConElServicio(t *testing.T) {
	cuerpos := map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clave := r.Method + " " + r.URL.Path
		if datos, _ := io.ReadAll(r.Body); len(datos) > 0 {
			var c map[string]any
			_ = json.Unmarshal(datos, &c)
			cuerpos[clave] = c
		}
		switch clave {
		case "GET /api/v1/snoozes":
			_, _ = io.WriteString(w, `{"snoozes":[{"thread_id":"h1","until":"2026-11-20T08:00:00Z"}]}`)
		case "PUT /api/v1/threads/h1/snooze":
			_, _ = io.WriteString(w, `{"thread_id":"h1","until":"2026-11-20T08:00:00Z"}`)
		case "PUT /api/v1/threads/h2/snooze":
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":{"code":"nothing_to_snooze","message":"Only conversations in the Inbox can be snoozed."}}`)
		case "PUT /api/v1/threads/h3/snooze":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"code":"invalid_snooze","message":"…"}}`)
		case "DELETE /api/v1/threads/h1/snooze":
			w.WriteHeader(http.StatusNoContent)
		case "PUT /api/v1/threads/h1/mute":
			_, _ = io.WriteString(w, `{"muted":true,"archived":2}`)
		case "GET /api/v1/threads/h1/messages":
			_, _ = io.WriteString(w, `{"messages":[],"muted":true,"snoozed_until":"2026-11-20T08:00:00Z"}`)
		default:
			t.Errorf("petición inesperada: %s", clave)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c, ctx := NewMail(srv.URL, nil, nil), context.Background()
	vuelve := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)

	if !c.MailPospone() {
		t.Error("el servicio pospone")
	}
	if l, err := c.Snoozes(ctx); err != nil || len(l) != 1 || l[0].ThreadID != "h1" || !l[0].Until.Equal(vuelve) {
		t.Errorf("Snoozes: %+v, %v", l, err)
	}
	if sn, err := c.SnoozeThread(ctx, "h1", vuelve.In(time.FixedZone("CET", 3600))); err != nil || !sn.Until.Equal(vuelve) {
		t.Errorf("SnoozeThread: %+v, %v", sn, err)
	}
	if got := cuerpos["PUT /api/v1/threads/h1/snooze"]["until"]; got != "2026-11-20T08:00:00Z" {
		t.Errorf("until = %v; va en UTC y RFC 3339", got)
	}
	if _, err := c.SnoozeThread(ctx, "h2", vuelve); !errors.Is(err, ErrNadaQuePosponer) {
		t.Errorf("nothing_to_snooze → %v", err)
	}
	if _, err := c.SnoozeThread(ctx, "h3", vuelve); !errors.Is(err, ErrSnoozeInvalido) {
		t.Errorf("invalid_snooze → %v", err)
	}
	if err := c.UnsnoozeThread(ctx, "h1"); err != nil {
		t.Errorf("UnsnoozeThread: %v", err)
	}
	if n, err := c.MuteThread(ctx, "h1", true); err != nil || n != 2 {
		t.Errorf("MuteThread: %d, %v", n, err)
	}
	if got := cuerpos["PUT /api/v1/threads/h1/mute"]["muted"]; got != true {
		t.Errorf("muted = %v", got)
	}
	st, err := c.ThreadState(ctx, "h1")
	if err != nil || !st.Muted || st.SnoozedUntil == nil || !st.SnoozedUntil.Equal(vuelve) {
		t.Errorf("ThreadState: %+v, %v", st, err)
	}
}

// TestLoVueltoDePospuestosSeOrdenaPorLaVuelta: el sobre trae `unsnoozed_at` y la
// fecha de orden es esa, no la de llegada; sin ella, la de llegada de siempre.
func TestLoVueltoDePospuestosSeOrdenaPorLaVuelta(t *testing.T) {
	var ms []MailMessage
	if err := json.Unmarshal([]byte(`[
		{"id":"a","received_at":"2026-03-01T08:00:00Z","unsnoozed_at":"2026-11-16T09:00:00Z"},
		{"id":"b","received_at":"2026-11-10T08:00:00Z"}]`), &ms); err != nil {
		t.Fatal(err)
	}
	vuelta := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	if ms[0].UnsnoozedAt == nil || !ms[0].FechaDeOrden().Equal(vuelta) {
		t.Errorf("lo vuelto se ordena por %v, quiero la vuelta %v", ms[0].FechaDeOrden(), vuelta)
	}
	if ms[1].UnsnoozedAt != nil || !ms[1].FechaDeOrden().Equal(ms[1].ReceivedAt) {
		t.Errorf("lo no pospuesto se ordena por %v, quiero la llegada", ms[1].FechaDeOrden())
	}
}
