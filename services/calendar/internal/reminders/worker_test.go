// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

type dummyQuerier struct{}

func (d *dummyQuerier) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

var errNotImplemented = errors.New("not implemented")

func (d *dummyQuerier) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errNotImplemented
}

func (d *dummyQuerier) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return nil
}

func TestWorkerPublishEmitsHubNotification(t *testing.T) {
	received := make(chan hubnotify.Notification, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/notifications" {
			http.NotFound(w, r)
			return
		}
		var notif hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&notif)
		received <- notif
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	notifier := hubnotify.New(srv.URL, "test-secret")
	w := NewWorker(nil, time.Minute, notifier, "https://calendar.sytue.eu")

	evID := uuid.New()
	due := store.DueReminder{
		ID:        uuid.New(),
		Tenant:    "acme",
		OwnerSub:  "user-sub-123",
		EventID:   evID,
		UID:       "event-uid-1",
		Summary:   "Reunión de equipo",
		DTStart:   time.Now().Add(10 * time.Minute),
		TriggerAt: time.Now(),
		Action:    "DISPLAY",
	}

	q := &dummyQuerier{}
	if err := w.publish(context.Background(), q, due); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case n := <-received:
		if n.Source != "calendar" || n.Type != "calendar.reminder.due" {
			t.Errorf("tipo/origen inesperado: %+v", n)
		}
		if n.Subject != "user-sub-123" {
			t.Errorf("subject esperado 'user-sub-123', got %q", n.Subject)
		}
		if n.Tenant != "acme" {
			t.Errorf("tenant esperado 'acme', got %q", n.Tenant)
		}
		if !strings.Contains(n.Title, "Reunión de equipo") {
			t.Errorf("título no contiene resumen: %q", n.Title)
		}
		// El evento, en el host de Calendar, con el prefijo de la app y la organización
		// del recordatorio detrás de él (ADR 0056): antes era
		// `https://calendar.sytue.localhost/#event-…`, que no abre nada.
		if quiero := "https://calendar.sytue.eu/calendar/acme/events/" + evID.String(); n.URL != quiero {
			t.Errorf("enlace = %q, quiero %q", n.URL, quiero)
		}
		if n.Params["title"] != "Reunión de equipo" || n.Params["start"] == "" {
			t.Errorf("params = %+v", n.Params)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout esperando notificación en Hub")
	}
}
