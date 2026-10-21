// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integracion de la cola de recordatorios: al crear un evento con VALARM, el
// store precalcula su disparo en calendar_reminders (camino de escritura), y el
// worker lo reclama cuando vence (ClaimDueReminders, cross-tenant via
// app.maintenance). Verifica migracion 0002 + RLS + idempotencia (fired_at).
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// eventWithAlarm construye un EventInput con un VALARM DISPLAY 15 min antes.
func eventWithAlarm(calendarID uuid.UUID, uid string, start time.Time) domain.EventInput {
	in := sampleEvent(calendarID, uid)
	in.DTStart = start
	in.DTEnd = start.Add(time.Hour)
	in.ICalData = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\n" +
		"DTSTART:" + start.UTC().Format("20060102T150405Z") + "\r\n" +
		"DTEND:" + start.Add(time.Hour).UTC().Format("20060102T150405Z") + "\r\n" +
		"SUMMARY:Con recordatorio\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	return in
}

func TestReminderQueue(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "user-1", Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}

	// Evento dentro de 2h => su recordatorio (-15m) dispara dentro de 1h45m.
	start := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	if _, _, err := st.CreateEvent(ctx, eventWithAlarm(cal.ID, "ev-alarm-1", start)); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	var claimed []store.DueReminder
	record := func(_ context.Context, _ pg.Querier, r store.DueReminder) error {
		claimed = append(claimed, r)
		return nil
	}

	// Aún no vencido: no se reclama nada.
	if n, err := st.ClaimDueReminders(ctx, time.Now(), 50, record); err != nil || n != 0 {
		t.Fatalf("reclamo prematuro: n=%d err=%v (esperaba 0)", n, err)
	}

	// Ya vencido (now = inicio del evento, > trigger): se reclama exactamente 1.
	n, err := st.ClaimDueReminders(ctx, start, 50, record)
	if err != nil {
		t.Fatalf("ClaimDueReminders: %v", err)
	}
	if n != 1 || len(claimed) != 1 {
		t.Fatalf("reclamados n=%d (registrados %d), esperaba 1", n, len(claimed))
	}
	if claimed[0].OwnerSub != "user-1" || claimed[0].UID != "ev-alarm-1" || claimed[0].Action != "DISPLAY" {
		t.Fatalf("recordatorio inesperado: %+v", claimed[0])
	}

	// Idempotencia: ya disparado, no se vuelve a reclamar.
	if n, err := st.ClaimDueReminders(ctx, start, 50, record); err != nil || n != 0 {
		t.Fatalf("reclamo duplicado: n=%d err=%v (esperaba 0)", n, err)
	}
}

func TestReminderRemovedOnDelete(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "user-2", Name: "personal", DisplayName: "Personal"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	start := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second)
	if _, _, err := st.CreateEvent(ctx, eventWithAlarm(cal.ID, "ev-alarm-2", start)); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}

	// Borrado (soft) del evento: su recordatorio pendiente desaparece de la cola.
	if err := st.DeleteEvent(ctx, cal.ID, "ev-alarm-2", uuid.Nil); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	noop := func(context.Context, pg.Querier, store.DueReminder) error { return nil }
	if n, err := st.ClaimDueReminders(ctx, start, 50, noop); err != nil || n != 0 {
		t.Fatalf("tras borrar el evento no debe quedar aviso: n=%d err=%v", n, err)
	}
}

// TestDueRemindersInApp verifica la SELECCIÓN de recordatorios vencidos para la
// entrega in-app: solo los del usuario, vencidos (trigger_at<=now) y no
// descartados; keyset (trigger_at, id) DESC; y que descartar los oculta.
func TestDueRemindersInApp(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "user-1", Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	// Calendario de OTRO usuario del mismo tenant: sus avisos no deben mezclarse.
	other, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "user-2", Name: "otro", DisplayName: "Otro"})
	if err != nil {
		t.Fatalf("CreateCalendar other: %v", err)
	}

	// El store solo encola disparos FUTUROS al escribir; se crean con inicio futuro
	// (VALARM -15m aún por vencer) y luego se consulta con un `now` avanzado para
	// que estén vencidos. `base` = ahora real (los inserts exigen trigger > ahora).
	base := time.Now().UTC().Truncate(time.Second)
	mk := func(c uuid.UUID, uid string, startAt time.Time) {
		if _, _, err := st.CreateEvent(ctx, eventWithAlarm(c, uid, startAt)); err != nil {
			t.Fatalf("CreateEvent %s: %v", uid, err)
		}
	}
	mk(cal.ID, "due-a", base.Add(90*time.Minute))  // trigger base+75m
	mk(cal.ID, "due-b", base.Add(120*time.Minute)) // trigger base+105m (más reciente)
	mk(cal.ID, "future", base.Add(10*time.Hour))   // trigger base+9h45m: no vence aún
	mk(other.ID, "other-user", base.Add(90*time.Minute))

	// A las +4h: due-a y due-b están vencidos; `future` no; other-user es de user-2.
	now := base.Add(4 * time.Hour)
	// user-1: exactamente 2 vencidos, del más reciente (due-b) al más antiguo.
	due, err := st.ListDueReminders(ctx, "user-1", now, nil, 50)
	if err != nil {
		t.Fatalf("ListDueReminders: %v", err)
	}
	if len(due) != 2 {
		t.Fatalf("vencidos de user-1 = %d, want 2 (%+v)", len(due), due)
	}
	if due[0].UID != "due-b" || due[1].UID != "due-a" {
		t.Errorf("orden keyset DESC roto: %s, %s", due[0].UID, due[1].UID)
	}

	// Keyset: página de 1 + continuación trae el siguiente y luego nada.
	page1, _ := st.ListDueReminders(ctx, "user-1", now, nil, 1)
	if len(page1) != 1 || page1[0].UID != "due-b" {
		t.Fatalf("página 1 = %+v", page1)
	}
	cur := &store.ReminderCursor{TriggerAt: page1[0].TriggerAt, ID: page1[0].ID}
	page2, _ := st.ListDueReminders(ctx, "user-1", now, cur, 1)
	if len(page2) != 1 || page2[0].UID != "due-a" {
		t.Fatalf("página 2 = %+v", page2)
	}

	// Descartar due-b lo oculta; queda solo due-a. Idempotente.
	if err := st.DismissReminder(ctx, "user-1", due[0].ID); err != nil {
		t.Fatalf("DismissReminder: %v", err)
	}
	if err := st.DismissReminder(ctx, "user-1", due[0].ID); err != nil {
		t.Fatalf("DismissReminder (idempotente): %v", err)
	}
	after, _ := st.ListDueReminders(ctx, "user-1", now, nil, 50)
	if len(after) != 1 || after[0].UID != "due-a" {
		t.Fatalf("tras descartar due-b queda %+v, want solo due-a", after)
	}

	// Descartar un id ajeno/inexistente para el usuario => ErrNotFound.
	if err := st.DismissReminder(ctx, "user-1", uuid.New()); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("dismiss de id inexistente = %v, want ErrNotFound", err)
	}
}
