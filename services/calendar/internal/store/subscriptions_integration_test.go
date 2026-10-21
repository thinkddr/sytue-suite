// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración de las suscripciones ICS externas (migración 0005): un calendario
// de SOLO LECTURA (read_only) respaldado por un feed, cuyos VEVENT se cachean
// como objetos/eventos normales. Verifica: la puerta de solo-lectura rechaza las
// escrituras de usuario (create/update/delete), el refresco (Sync) reemplaza el
// caché y se renderiza/expande, el aislamiento por dueño, y el claim con lease +
// el marcado condicional (304 conserva el ETag).
package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/caldav"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// feedICS es un feed externo de muestra: una festividad puntual y una serie
// semanal (RRULE COUNT=6).
const feedICS = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Ext//Feed//EN\r\n" +
	"BEGIN:VEVENT\r\nUID:ny@ext\r\nDTSTART;VALUE=DATE:20260101\r\nDTEND;VALUE=DATE:20260102\r\n" +
	"SUMMARY:Año Nuevo\r\nEND:VEVENT\r\n" +
	"BEGIN:VEVENT\r\nUID:standup@ext\r\nDTSTART:20260105T090000Z\r\nDTEND:20260105T091500Z\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=MO;COUNT=6\r\nSUMMARY:Standup\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

func feedObjects(t *testing.T, ics string) []store.SubscriptionObject {
	t.Helper()
	parsed, err := caldav.DecodeICSFeed(ics, 0)
	if err != nil {
		t.Fatalf("DecodeICSFeed: %v", err)
	}
	out := make([]store.SubscriptionObject, 0, len(parsed))
	for _, po := range parsed {
		out = append(out, store.SubscriptionObject{Master: po.Master, Overrides: po.Overrides})
	}
	return out
}

// TestSubscriptionReadOnly: una suscripción es un calendario read_only; NINGUNA
// escritura de usuario (create/update/delete de evento) la modifica.
func TestSubscriptionReadOnly(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateSubscription(ctx, "festivos", store.NewSubscription{
		OwnerSub: "user-1", URL: "https://example.com/holidays.ics",
		DisplayName: "Festivos", Color: "#0F7052",
	})
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if !cal.ReadOnly || cal.Subscription == nil || cal.Subscription.URL != "https://example.com/holidays.ics" {
		t.Fatalf("calendario de suscripción mal formado: readonly=%v sub=%+v", cal.ReadOnly, cal.Subscription)
	}

	if _, _, err := st.CreateEvent(ctx, sampleEvent(cal.ID, "intruso")); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("CreateEvent en suscripción = %v, want ErrReadOnly", err)
	}
	if _, _, err := st.UpdateEvent(ctx, sampleEvent(cal.ID, "intruso"), uuid.Nil); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("UpdateEvent en suscripción = %v, want ErrReadOnly", err)
	}
	if err := st.DeleteEvent(ctx, cal.ID, "intruso", uuid.Nil); !errors.Is(err, domain.ErrReadOnly) {
		t.Errorf("DeleteEvent en suscripción = %v, want ErrReadOnly", err)
	}
}

// TestSubscriptionSyncRenders: el refresco (Sync) cachea los VEVENT del feed como
// eventos normales, consultables por rango y expandibles (recurrencia respetada);
// un segundo Sync REEMPLAZA el caché.
func TestSubscriptionSyncRenders(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateSubscription(ctx, "equipo", store.NewSubscription{
		OwnerSub: "user-1", URL: "https://example.com/team.ics", DisplayName: "Equipo",
	})
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	if err := st.SyncSubscriptionObjects(ctx, cal.ID, feedObjects(t, feedICS)); err != nil {
		t.Fatalf("SyncSubscriptionObjects: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	evs, err := st.ListEventsInRange(ctx, cal.ID, from, to)
	if err != nil {
		t.Fatalf("ListEventsInRange: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("eventos cacheados = %d, want 2 (festividad + serie)", len(evs))
	}
	insts := domain.ExpandEvents(evs, from, to)
	if len(insts) != 7 { // 1 festividad + 6 standups (COUNT=6)
		t.Fatalf("instancias expandidas = %d, want 7", len(insts))
	}

	// Re-sync con solo la festividad: el caché se REEMPLAZA (la serie desaparece).
	only := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:ny@ext\r\n" +
		"DTSTART;VALUE=DATE:20260101\r\nDTEND;VALUE=DATE:20260102\r\nSUMMARY:Año Nuevo\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	if err := st.SyncSubscriptionObjects(ctx, cal.ID, feedObjects(t, only)); err != nil {
		t.Fatalf("re-Sync: %v", err)
	}
	evs2, _ := st.ListEventsInRange(ctx, cal.ID, from, to)
	if len(evs2) != 1 {
		t.Fatalf("tras re-sync eventos = %d, want 1 (reemplazo total)", len(evs2))
	}
}

// TestSubscriptionOwnerIsolation: la suscripción de un dueño no aparece en la
// lista de otro dueño del mismo tenant.
func TestSubscriptionOwnerIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateSubscription(ctx, "festivos", store.NewSubscription{
		OwnerSub: "user-1", URL: "https://example.com/holidays.ics", DisplayName: "Festivos",
	})
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	mine, err := st.ListCalendars(ctx, "user-1")
	if err != nil {
		t.Fatalf("ListCalendars user-1: %v", err)
	}
	var found bool
	for _, c := range mine {
		if c.ID == cal.ID {
			found = true
			if c.Subscription == nil || !c.ReadOnly {
				t.Errorf("suscripción propia sin estado: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("la suscripción no aparece para su dueño")
	}

	other, err := st.ListCalendars(ctx, "user-2")
	if err != nil {
		t.Fatalf("ListCalendars user-2: %v", err)
	}
	for _, c := range other {
		if c.ID == cal.ID {
			t.Fatal("la suscripción de user-1 es visible para user-2 (fuga de aislamiento)")
		}
	}
}

// TestSubscriptionClaimAndMark: el claim arrienda la fila (otra réplica no la
// re-toma) y el marcado condicional conserva el ETag en un 304.
func TestSubscriptionClaimAndMark(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateSubscription(ctx, "festivos", store.NewSubscription{
		OwnerSub: "user-1", URL: "https://example.com/holidays.ics", DisplayName: "Festivos",
	})
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}

	// Nace con next_refresh_at=now() => se reclama una vez; el lease impide re-tomarla.
	jobs, err := st.ClaimDueSubscriptions(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimDueSubscriptions: %v", err)
	}
	if len(jobs) != 1 || jobs[0].CalendarID != cal.ID {
		t.Fatalf("claim = %+v, want 1 job para el calendario", jobs)
	}
	if again, _ := st.ClaimDueSubscriptions(ctx, 10); len(again) != 0 {
		t.Fatalf("segundo claim inmediato = %d, want 0 (arrendada)", len(again))
	}

	// Marca OK con un ETag; SubscriptionForCalendar lo refleja.
	job := jobs[0]
	if err := st.MarkSubscriptionResult(ctx, job, store.SubscriptionResult{
		Status: "ok", ETag: `"v1"`, NextRefresh: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkSubscriptionResult ok: %v", err)
	}
	after, err := st.SubscriptionForCalendar(ctx, cal.ID)
	if err != nil || after.ETag != `"v1"` {
		t.Fatalf("tras marcar ok: etag=%q err=%v, want \"v1\"", after.ETag, err)
	}

	// Marcado condicional (304): conserva el ETag previo (no cambia nada).
	if err := st.MarkSubscriptionResult(ctx, after, store.SubscriptionResult{
		Status: "not_modified", ETag: after.ETag, LastModified: after.LastModified,
		NextRefresh: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("MarkSubscriptionResult not_modified: %v", err)
	}
	final, _ := st.SubscriptionForCalendar(ctx, cal.ID)
	if final.ETag != `"v1"` {
		t.Fatalf("tras 304 etag=%q, want \"v1\" (conservado)", final.ETag)
	}
}
