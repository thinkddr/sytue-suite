// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
)

// Una invitación da UN aviso a cada invitado —la primera, aunque el evento se
// reenvíe al cambiar—, con su fecha y lo que recibió, y ninguno a quien organiza, a
// quien quitaron del evento, de una invitación cancelada ni de un evento borrado.
func TestAvisosHistoricosDeInvitaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	primera := time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var cal string
		if err := tx.QueryRow(ctx, `INSERT INTO calendars (tenant, owner_sub, name) VALUES ('acme', 'ana', 'c') RETURNING id::text`).Scan(&cal); err != nil {
			return err
		}
		evento := func(uid string, borrado bool) (string, error) {
			var obj, id string
			if err := tx.QueryRow(ctx, `INSERT INTO calendar_objects (tenant, calendar_id, uid, resource_name, data)
				VALUES ('acme', $1, $2, $2 || '.ics', 'x') RETURNING id::text`, cal, uid).Scan(&obj); err != nil {
				return "", err
			}
			err := tx.QueryRow(ctx, `INSERT INTO events (tenant, calendar_id, object_id, uid, summary, organizer, dtstart, dtend, deleted_at)
				VALUES ('acme', $1, $2, $3, 'Revisión', 'mailto:ana@acme.es', $4, $4, CASE WHEN $5 THEN now() END) RETURNING id::text`,
				cal, obj, uid, primera.Add(48*time.Hour), borrado).Scan(&id)
			return id, err
		}
		vivo, err := evento("u1", false)
		if err != nil {
			return err
		}
		borrado, err := evento("u2", true)
		if err != nil {
			return err
		}
		for _, e := range []string{"Bea@acme.es", "ana@acme.es", "carla@acme.es"} { // eli ya no está
			if _, err := tx.Exec(ctx, `INSERT INTO attendees (tenant, event_id, email) VALUES ('acme', $1, $2)`, vivo, e); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO attendees (tenant, event_id, email) VALUES ('acme', $1, 'dani@acme.es')`, borrado); err != nil {
			return err
		}
		ics := func(resumen string) string {
			return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//t//ES\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:u1\r\n" +
				"DTSTAMP:20260906T080000Z\r\nDTSTART:20260908T080000Z\r\nDTEND:20260908T090000Z\r\nSUMMARY:" + resumen +
				"\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
		}
		for _, i := range []struct {
			ev, para, estado, resumen string
			horas                     int
		}{
			{vivo, "bea@acme.es", "pending", "Revisión inicial", 0},
			{vivo, "Bea@acme.es", "accepted", "Revisión", 5}, // el reenvío al cambiar el evento
			{vivo, "ana@acme.es", "pending", "Revisión", 0},  // la organizadora
			{vivo, "carla@acme.es", "cancelled", "Revisión", 0},
			{vivo, "eli@acme.es", "pending", "Revisión", 0}, // la quitaron del evento
			{borrado, "dani@acme.es", "pending", "Revisión", 0},
		} {
			if _, err := tx.Exec(ctx, `INSERT INTO invitations (tenant, event_id, uid, method, organizer, recipient, status, created_at, data)
				SELECT 'acme', id, uid, 'REQUEST', 'mailto:ana@acme.es', $2, $3, $4, $5 FROM events WHERE id = $1`,
				i.ev, i.para, i.estado, primera.Add(time.Duration(i.horas)*time.Hour), ics(i.resumen)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	avisos, err := AvisosHistoricos(ctx, h.Pool, "https://calendar.sytue.eu")
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(avisos))
	for _, a := range avisos {
		got = append(got, a.Subject)
		if !a.Fecha.Equal(primera) {
			t.Errorf("sin la fecha de la primera invitación: %+v", a)
		}
		if a.Params["title"] != "Revisión inicial" {
			t.Errorf("tiene que decir lo que recibió, no lo de ahora: %+v", a.Params)
		}
	}
	if !slices.Equal(got, []string{hubnotify.PorCorreo("bea@acme.es")}) {
		t.Fatalf("avisos %v, quiero uno a Bea", got)
	}
}
