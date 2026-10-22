// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/config"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/service"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// COM-07. Las invitaciones por correo salen del remitente de avisos de la plataforma:
// quien organiza es quien escribe (no lo que diga el cuerpo), y hay tope de invitados
// por evento y de correos a fuera por persona y hora.
func TestLasInvitacionesPorCorreoTienenOrganizadorRealYTopes(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(config.Config{MaxRangeWindow: 32 * 24 * time.Hour, PublicURL: "https://calendar.sytue.eu"}, st, nil)
	var enviados atomic.Int64
	ics := make(chan string, 256)
	svc.SetCorreo(func(_ context.Context, _, _, _ string, cuerpo []byte, _ string) error {
		enviados.Add(1)
		select {
		case ics <- string(cuerpo):
		default:
		}
		return nil
	})

	ctx := ctxDe(ownerSub, ownerEmail)
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	de := func(n int) []domain.Attendee {
		out := make([]domain.Attendee, n)
		for i := range out {
			out[i] = domain.Attendee{Email: fmt.Sprintf("p%d@gmail.example", i)}
		}
		return out
	}

	// El organizador no lo elige el cuerpo.
	in := nuevoEvento(cal.ID, "com07-1")
	in.Organizer = "director@otra-empresa.example"
	in.Attendees = de(1)
	in.Send = true
	ev, err := svc.CreateEvent(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(strings.TrimPrefix(strings.ToLower(ev.Organizer), "mailto:"), ownerEmail) {
		t.Fatalf("organizador = %q; quiero el de la sesión %q", ev.Organizer, ownerEmail)
	}
	select {
	case c := <-ics:
		if strings.Contains(c, "otra-empresa") {
			t.Fatalf("la invitación sale en nombre de otro:\n%s", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no ha salido la invitación")
	}

	// Demasiados invitados para un evento: no se guarda ni se reparte.
	in = nuevoEvento(cal.ID, "com07-2")
	in.Attendees = de(service.MaxInvitados + 1)
	in.Send = true
	if _, err := svc.CreateEvent(ctx, in); !errors.Is(err, domain.ErrDemasiadosInvitados) {
		t.Fatalf("con %d invitados: err=%v; quiero ErrDemasiadosInvitados", len(in.Attendees), err)
	}

	// El cupo de correos a fuera de la hora: 1 ya gastado + 99 caben; uno más, no.
	in = nuevoEvento(cal.ID, "com07-3")
	in.Attendees = de(99)
	in.Send = true
	if _, err := svc.CreateEvent(ctx, in); err != nil {
		t.Fatalf("99 invitados dentro del cupo: %v", err)
	}
	in = nuevoEvento(cal.ID, "com07-4")
	in.Attendees = de(1)
	in.Send = true
	if _, err := svc.CreateEvent(ctx, in); !errors.Is(err, domain.ErrCupoInvitaciones) {
		t.Fatalf("pasado el cupo de la hora: err=%v; quiero ErrCupoInvitaciones", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for enviados.Load() < 100 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	if n := enviados.Load(); n != 100 {
		t.Fatalf("correos enviados = %d; quiero 100", n)
	}
}
