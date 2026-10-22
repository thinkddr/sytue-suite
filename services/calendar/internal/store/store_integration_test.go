// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion de Sytue Calendar contra un PostgreSQL real (efimero,
// via platform/pkg/pgtest sobre testcontainers-go). Detras del build-tag
// `integration`: no corren en el build/CI por defecto (sin Docker). Verifican:
//   - que las migraciones del servicio aplican (esquema + RLS),
//   - CRUD basico del store (calendario + evento),
//   - AISLAMIENTO por tenant (RLS): datos de un tenant no visibles para otro.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// tenantCtx devuelve un contexto con el tenant fijado (lo consume RLS).
func tenantCtx(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// sampleEvent construye un EventInput valido con un UID dado.
func sampleEvent(calendarID uuid.UUID, uid string) domain.EventInput {
	start := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	return domain.EventInput{
		CalendarID: calendarID,
		UID:        uid,
		Summary:    "Reunion de equipo",
		Location:   "Sala Sytue",
		DTStart:    start,
		DTEnd:      start.Add(time.Hour),
		ICalData:   "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n",
	}
}

// TestMigrationsApply verifica que las migraciones embebidas aplican limpiamente
// contra un PostgreSQL real (el harness llama pg.MigrateUp en New; aqui ademas
// comprobamos que las tablas clave existen y RLS esta habilitada/forzada).
func TestMigrationsApply(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()

	for _, tbl := range []string{"calendars", "calendar_objects", "events", "attendees", "invitations"} {
		var relrowsecurity, relforcerowsecurity bool
		err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity
			 FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&relrowsecurity, &relforcerowsecurity)
		if err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !relrowsecurity || !relforcerowsecurity {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, relrowsecurity, relforcerowsecurity)
		}
	}
}

// TestCalendarEventCRUD ejercita el ciclo completo crear/leer/borrar de un
// calendario y un evento, a traves del store (que fija app.tenant por RLS).
func TestCalendarEventCRUD(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// Crear calendario.
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{
		OwnerSub:    "user-1",
		Name:        "trabajo",
		DisplayName: "Trabajo",
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	if cal.ID == uuid.Nil || cal.Name != "trabajo" {
		t.Fatalf("calendario inesperado: %+v", cal)
	}

	// Leer calendario.
	got, err := st.GetCalendar(ctx, cal.ID)
	if err != nil {
		t.Fatalf("GetCalendar: %v", err)
	}
	if got.ID != cal.ID {
		t.Fatalf("GetCalendar devolvio id %s, esperado %s", got.ID, cal.ID)
	}

	// Crear evento en el calendario.
	ev, obj, err := st.CreateEvent(ctx, sampleEvent(cal.ID, "evt-uid-1"))
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if ev.UID != "evt-uid-1" || obj.UID != "evt-uid-1" {
		t.Fatalf("evento/objeto con UID inesperado: ev=%q obj=%q", ev.UID, obj.UID)
	}

	// Leer evento.
	gotEv, err := st.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if gotEv.Summary != "Reunion de equipo" {
		t.Fatalf("GetEvent summary inesperado: %q", gotEv.Summary)
	}

	// El recurso CalDAV existe por UID.
	if _, err := st.GetObjectByUID(ctx, cal.ID, "evt-uid-1"); err != nil {
		t.Fatalf("GetObjectByUID: %v", err)
	}

	// Borrar evento (soft) y confirmar que ya no se lee.
	if err := st.DeleteEvent(ctx, cal.ID, "evt-uid-1", uuid.Nil); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	if _, err := st.GetEvent(ctx, ev.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetEvent tras borrar: esperaba ErrNotFound, obtuve %v", err)
	}

	// Borrar calendario y confirmar.
	if err := st.DeleteCalendar(ctx, cal.ID); err != nil {
		t.Fatalf("DeleteCalendar: %v", err)
	}
	if _, err := st.GetCalendar(ctx, cal.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetCalendar tras borrar: esperaba ErrNotFound, obtuve %v", err)
	}
}

// TestEventColorRoundTrip verifica que el color por evento persiste en la columna
// consultable y vuelve intacto por GetEvent (proyeccion REST/instancias). El
// round-trip CalDAV completo (verbatim) lo cubre el test de caldav; aqui se
// comprueba la columna.
func TestEventColorRoundTrip(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "user-1", Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	in := sampleEvent(cal.ID, "evt-color-1")
	in.Color = "#6B3E9E"
	ev, _, err := st.CreateEvent(ctx, in)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	if ev.Color != "#6B3E9E" {
		t.Fatalf("CreateEvent color = %q, want #6B3E9E", ev.Color)
	}
	got, err := st.GetEvent(ctx, ev.ID)
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.Color != "#6B3E9E" {
		t.Errorf("GetEvent color = %q, want #6B3E9E (columna)", got.Color)
	}
}

// TestRespondInvitationRecipientOnly verifica que SOLO el destinatario (por su
// email o su sub del JWT) puede responder una invitacion: otro usuario del
// mismo tenant recibe ErrNotFound y la invitacion sigue pendiente (sin PARTSTAT
// falsificado ni REPLY suplantado).
func TestRespondInvitationRecipientOnly(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "organizer-1", Name: "trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	in := sampleEvent(cal.ID, "evt-invite-1")
	in.Attendees = []domain.Attendee{{Email: "bob@sytue.eu"}}
	ev, _, err := st.CreateEvent(ctx, in)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	inv, err := st.CreateInvitation(ctx, store.NewInvitation{
		EventID: &ev.ID, UID: ev.UID, Method: domain.MethodRequest,
		Organizer: "organizer@sytue.eu", Recipient: "bob@sytue.eu",
	})
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	// Mallory (otro usuario del tenant, con el UUID de la invitacion) NO puede
	// responder por Bob: ErrNotFound, sin filtrar existencia.
	if _, err := st.RespondInvitation(ctx, inv.ID, domain.InvDeclined, []string{"mallory@sytue.eu"}, "mallory-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-destinatario responde: err=%v, quiero ErrNotFound", err)
	}
	// Una identidad vacia tampoco casa (deny-by-default).
	if _, err := st.RespondInvitation(ctx, inv.ID, domain.InvAccepted, nil, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("identidad vacia responde: err=%v, quiero ErrNotFound", err)
	}
	// Sigue pendiente: el intento ajeno no muto nada.
	if got, err := st.GetInvitation(ctx, inv.ID); err != nil || got.Status != domain.InvPending {
		t.Fatalf("tras intento ajeno: status=%v err=%v, quiero pending", got.Status, err)
	}

	// Bob SI responde, casando su email del JWT case-insensitive.
	resp, err := st.RespondInvitation(ctx, inv.ID, domain.InvAccepted, []string{"BOB@Sytue.EU"}, "bob-1")
	if err != nil || resp.Status != domain.InvAccepted {
		t.Fatalf("destinatario responde: status=%v err=%v, quiero accepted", resp.Status, err)
	}
	// Y su PARTSTAT quedo reflejado en el evento.
	gotEv, err := st.GetEvent(ctx, ev.ID)
	if err != nil || len(gotEv.Attendees) != 1 || gotEv.Attendees[0].PartStat != domain.PartAccepted {
		t.Fatalf("partstat tras responder: %+v err=%v, quiero ACCEPTED", gotEv.Attendees, err)
	}
}

// TestTenantIsolation es la prueba nuclear de RLS (CONVENTIONS §0.7/§10): los
// datos de un tenant NO son visibles ni borrables desde otro tenant, aunque se
// conozca el ID de la fila. Se comparte el mismo contenedor entre subfases.
func TestTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)

	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	// Tenant A crea un calendario y un evento.
	calA, err := st.CreateCalendar(ctxA, store.NewCalendar{OwnerSub: "u-a", Name: "personal-a"})
	if err != nil {
		t.Fatalf("A CreateCalendar: %v", err)
	}
	evA, _, err := st.CreateEvent(ctxA, sampleEvent(calA.ID, "evt-a"))
	if err != nil {
		t.Fatalf("A CreateEvent: %v", err)
	}

	// Tenant B NO puede ver el calendario de A por su ID (RLS lo oculta).
	if _, err := st.GetCalendar(ctxB, calA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el calendario de A (err=%v)", err)
	}
	// Tenant B NO puede ver el evento de A por su ID.
	if _, err := st.GetEvent(ctxB, evA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el evento de A (err=%v)", err)
	}
	// Tenant B NO puede borrar el evento de A.
	if err := st.DeleteEventByID(ctxB, evA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B borra el evento de A (err=%v)", err)
	}

	// A sigue viendo lo suyo intacto.
	if _, err := st.GetEvent(ctxA, evA.ID); err != nil {
		t.Fatalf("A perdio su propio evento: %v", err)
	}

	// B crea su propio calendario con el mismo owner/name; no colisiona con A
	// (aislamiento a nivel de fila por tenant en el indice unico).
	calB, err := st.CreateCalendar(ctxB, store.NewCalendar{OwnerSub: "u-a", Name: "personal-a"})
	if err != nil {
		t.Fatalf("B CreateCalendar: %v", err)
	}
	if calB.ID == calA.ID {
		t.Fatalf("B reutilizo el ID de A: %s", calB.ID)
	}

	// Cada tenant solo lista sus propios calendarios.
	listA, err := st.ListCalendars(ctxA, "u-a")
	if err != nil {
		t.Fatalf("A ListCalendars: %v", err)
	}
	if len(listA) != 1 || listA[0].ID != calA.ID {
		t.Fatalf("A ve calendarios ajenos: %+v", listA)
	}
	listB, err := st.ListCalendars(ctxB, "u-a")
	if err != nil {
		t.Fatalf("B ListCalendars: %v", err)
	}
	if len(listB) != 1 || listB[0].ID != calB.ID {
		t.Fatalf("B ve calendarios ajenos: %+v", listB)
	}
}
