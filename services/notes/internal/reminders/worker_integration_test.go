// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package reminders

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/db"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

// De punta a punta contra un Hub de mentira: lo vencido llega una vez, con el enlace a
// su nota, y vaciar otra vez no vuelve a mandarlo.
func TestElWorkerAvisaEnElHubUnaVez(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	in := domain.NotebookInput{Title: "Casa", Visibility: domain.VisPersonal}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	nb, err := st.CreateNotebook(ctx, "ada", in)
	if err != nil {
		t.Fatal(err)
	}
	nota, err := st.CreateNote(ctx, nb.ID, "ada", domain.NoteInput{Title: "Llamar al fontanero"})
	if err != nil {
		t.Fatal(err)
	}
	r := domain.ReminderInput{DueAt: time.Now().Add(-time.Minute)}
	if err := r.Normalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetReminder(ctx, nota.ID, "ada", r); err != nil {
		t.Fatal(err)
	}

	avisos := make(chan hubnotify.Notification, 4)
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n hubnotify.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		avisos <- n
		w.WriteHeader(http.StatusCreated)
	}))
	defer hub.Close()
	w := NewWorker(st, time.Minute, hubnotify.New(hub.URL, "s"), "https://notes.sytue.eu")
	w.Vaciar(context.Background())
	select {
	case n := <-avisos:
		quiero := hubnotify.EnlaceNota("https://notes.sytue.eu", "acme", nb.ID.String(), nota.ID.String())
		if n.Type != TipoAviso || n.Subject != "ada" || n.URL != quiero || n.Params["title"] != "Llamar al fontanero" {
			t.Errorf("aviso: %+v", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el recordatorio vencido no llega al Hub")
	}
	w.Vaciar(context.Background())
	select {
	case n := <-avisos:
		t.Errorf("vaciar otra vez vuelve a avisar: %+v", n)
	case <-time.After(500 * time.Millisecond):
	}
}
