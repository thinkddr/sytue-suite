// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// La reunión suelta de un evento la abren sus invitados (decidido por el dueño el
// 2026-10-16): Rooms pregunta aquí si quien llama organiza el evento de ese UID, figura
// como asistente o es destinatario de su invitación (de la organización o de fuera).
func TestEsInvitadoDeEvento(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "ana-sub", Name: "trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	in := sampleEvent(cal.ID, "evt-reunion")
	in.Organizer = "mailto:ana@acme.example"
	in.Attendees = []domain.Attendee{{Email: "bob@acme.example"}}
	ev, _, err := st.CreateEvent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateInvitation(ctx, store.NewInvitation{
		EventID: &ev.ID, UID: ev.UID, Method: domain.MethodRequest,
		Organizer: "ana@acme.example", Recipient: "carla@fuera.example",
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		quien  string
		sub    string
		dirs   []string
		quiero bool
	}{
		{"la organizadora", "ana-sub", []string{"ana@acme.example"}, true},
		{"un asistente", "bob-sub", []string{"bob@acme.example"}, true},
		{"una invitada de fuera", "carla-sub", []string{"carla@fuera.example"}, true},
		{"alguien sin invitación", "eve-sub", []string{"eve@acme.example"}, false},
	} {
		si, err := st.EsInvitadoDeEvento(ctx, "evt-reunion", c.sub, c.dirs)
		if err != nil || si != c.quiero {
			t.Errorf("%s: %v (%v), quiero %v", c.quien, si, err, c.quiero)
		}
	}
	if si, _ := st.EsInvitadoDeEvento(ctx, "otro-uid", "bob-sub", []string{"bob@acme.example"}); si {
		t.Error("el asistente de un evento entra en el de otro")
	}
}
