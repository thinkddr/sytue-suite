// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// El CN del organizador se guarda (migración 0028) y vuelve por la API: el de la sesión
// al crear por REST, el del iCalendar al escribir por CalDAV, y el que ya tenía al
// editar. Es lo que deja a las apps nativas nombrar al organizador como la web.
//
// VISTO EN ROJO (2026-11-25): sin la columna, GetEvent y ListInstances devuelven
// `organizer_name` vacío en los tres casos.
func TestElCNDelOrganizadorSeGuardaYVuelve(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour}, st, nil)
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}),
		authn.Claims{Subject: ownerSub, Email: ownerEmail, Name: "Olga Dueña"})
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}

	// Por REST: el nombre de la sesión, que manda sobre lo que pida el cuerpo.
	in := nuevoEvento(cal.ID, "uid-rest")
	in.OrganizerName = "Otro nombre"
	ev, err := svc.CreateEvent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if ev.OrganizerName != "Olga Dueña" {
		t.Errorf("CreateEvent devuelve CN %q", ev.OrganizerName)
	}
	leido, err := svc.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatal(err)
	}
	if leido.OrganizerName != "Olga Dueña" {
		t.Errorf("GetEvent devuelve CN %q", leido.OrganizerName)
	}

	// Editar conserva el que tenía, aunque el cuerpo traiga otro.
	in.Summary, in.OrganizerName = "reunión movida", "Cambiado"
	editado, err := svc.UpdateEvent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if editado.OrganizerName != "Olga Dueña" {
		t.Errorf("UpdateEvent cambia el CN a %q", editado.OrganizerName)
	}

	// Por CalDAV: el CN del iCalendar, aunque sea de otra persona.
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//prueba//ES\r\nBEGIN:VEVENT\r\n" +
		"UID:uid-caldav\r\nDTSTAMP:20260701T080000Z\r\nDTSTART:20260701T120000Z\r\nDTEND:20260701T130000Z\r\n" +
		"SUMMARY:Con fuera\r\nORGANIZER;CN=\"Ada Lovelace\":mailto:ada@fuera.example\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if _, err := svc.PutObject(ctx, cal.ID, ics, uuid.Nil, true); err != nil {
		t.Fatal(err)
	}
	desde := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	insts, err := svc.ListInstances(ctx, cal.ID, desde, desde.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	cn := map[string]string{}
	for _, i := range insts {
		cn[i.Event.UID] = i.Event.OrganizerName
	}
	if cn["uid-caldav"] != "Ada Lovelace" || cn["uid-rest"] != "Olga Dueña" {
		t.Errorf("ListInstances: %v", cn)
	}
}
