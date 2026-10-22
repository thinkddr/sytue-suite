// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de recursos reservables (salas, equipos) contra un
// PostgreSQL real (pgtest). Verifican, end-to-end sobre el store:
//   - alta de recurso + su calendario de reservas (backing calendar);
//   - reservar una sala LIBRE → aceptada; una OCUPADA → rechazada (anti doble-reserva);
//   - dos reservas SIMULTÁNEAS de la misma franja → solo una gana (bloqueo atómico);
//   - free/busy del recurso vía disponibilidad en ventana (sin N+1);
//   - reprogramar la propia reserva (mismo evento) no choca consigo misma; liberar;
//   - editar/borrar solo el gestor (o admin); aislamiento por tenant;
//   - delegación de agenda: un share 'manage' concede escritura (CanWrite).
package store_test

import (
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func booking(uid string, start time.Time, dur time.Duration) store.ResourceBooking {
	return store.ResourceBooking{
		SourceUID: uid, Summary: "Reunión", Organizer: "org@sytue.eu",
		DTStart: start, DTEnd: start.Add(dur), TZID: "UTC",
	}
}

func TestResourceBookingLifecycle(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	room, err := st.CreateResource(ctx, store.NewResource{
		Kind: domain.ResourceRoom, Name: "Sala Marie Curie", Capacity: 8,
		Location: "P2", Email: "curie@sytue.eu", AutoAccept: true, CreatedBy: "mgr-1",
	})
	if err != nil {
		t.Fatalf("CreateResource: %v", err)
	}
	if room.CalendarID == uuid.Nil {
		t.Fatalf("el recurso debe tener un calendario de reservas: %+v", room)
	}

	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)

	// Sala LIBRE → aceptada.
	got, err := st.BookResource(ctx, room.ID, booking("evt-1", base, time.Hour))
	if err != nil {
		t.Fatalf("BookResource(libre): %v", err)
	}
	if got.Status != domain.PartAccepted {
		t.Fatalf("sala libre: status=%q, quiero accepted", got.Status)
	}

	// Sala OCUPADA (solape 09:30–10:30) → rechazada, con el periodo en conflicto.
	dec, err := st.BookResource(ctx, room.ID, booking("evt-2", base.Add(30*time.Minute), time.Hour))
	if err != nil {
		t.Fatalf("BookResource(ocupada): %v", err)
	}
	if dec.Status != domain.PartDeclined || dec.Conflict == nil {
		t.Fatalf("sala ocupada: status=%q conflict=%v, quiero declined+conflict", dec.Status, dec.Conflict)
	}

	// Franja contigua (10:00–11:00) → libre → aceptada.
	if r, err := st.BookResource(ctx, room.ID, booking("evt-3", base.Add(time.Hour), time.Hour)); err != nil || r.Status != domain.PartAccepted {
		t.Fatalf("franja contigua: status=%q err=%v, quiero accepted", r.Status, err)
	}

	// Disponibilidad en ventana (free/busy del recurso, sin N+1): dos periodos ocupados.
	avail, err := st.ListResources(ctx, base.Add(-time.Hour), base.Add(4*time.Hour), true)
	if err != nil {
		t.Fatalf("ListResources(availability): %v", err)
	}
	if len(avail) != 1 || avail[0].Resource.ID != room.ID {
		t.Fatalf("disponibilidad: %+v", avail)
	}
	if len(avail[0].Busy) == 0 {
		t.Fatalf("el recurso reservado debe reportar periodos ocupados")
	}

	// Reprogramar la MISMA reserva (evt-1) a otra hora libre no choca consigo misma.
	if r, err := st.BookResource(ctx, room.ID, booking("evt-1", base.Add(6*time.Hour), time.Hour)); err != nil || r.Status != domain.PartAccepted {
		t.Fatalf("reprogramar propia reserva: status=%q err=%v, quiero accepted", r.Status, err)
	}

	// Liberar la reserva evt-3: quien reservó (org@sytue.eu) puede.
	if err := st.ReleaseResourceBooking(ctx, room.ID, "evt-3", "someone", "org@sytue.eu", false); err != nil {
		t.Fatalf("ReleaseResourceBooking: %v", err)
	}
	// Ahora esa franja (10:00–11:00) vuelve a estar libre → aceptada.
	if r, err := st.BookResource(ctx, room.ID, booking("evt-4", base.Add(time.Hour), time.Hour)); err != nil || r.Status != domain.PartAccepted {
		t.Fatalf("tras liberar: status=%q err=%v, quiero accepted", r.Status, err)
	}
}

// TestResourceConcurrentBookingsSingleWinner es la prueba nuclear del anti
// doble-reserva ATÓMICO: dos reservas SIMULTÁNEAS de la misma sala y franja compiten;
// exactamente una debe ganar (aceptada) y la otra rechazarse. El bloqueo de fila del
// recurso (FOR UPDATE) las serializa.
func TestResourceConcurrentBookingsSingleWinner(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	room, err := st.CreateResource(ctx, store.NewResource{
		Kind: domain.ResourceRoom, Name: "Sala Turing", AutoAccept: true, CreatedBy: "mgr-1",
	})
	if err != nil {
		t.Fatalf("CreateResource: %v", err)
	}
	slot := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)

	const n = 8
	var wg sync.WaitGroup
	results := make([]store.BookingResult, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // arrancan a la vez
			// UID distinto por reserva: son eventos distintos que compiten por la MISMA
			// franja (si compartieran UID sería reprogramar la misma reserva, no competir).
			results[i], errs[i] = st.BookResource(ctx, room.ID,
				booking("evt-concurrent-"+strconv.Itoa(i), slot, time.Hour))
		}(i)
	}
	close(start)
	wg.Wait()

	accepted := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: BookResource error: %v", i, errs[i])
		}
		switch results[i].Status {
		case domain.PartAccepted:
			accepted++
		case domain.PartDeclined:
		default:
			t.Fatalf("goroutine %d: status inesperado %q", i, results[i].Status)
		}
	}
	if accepted != 1 {
		t.Fatalf("reservas simultáneas: %d aceptadas, quiero EXACTAMENTE 1 (anti doble-reserva)", accepted)
	}
}

// TestResourceManagerGate: editar/borrar solo el gestor (managers/created_by) o un
// admin del tenant; un extraño → ErrNotManager.
func TestResourceManagerGate(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	room, err := st.CreateResource(ctx, store.NewResource{
		Kind: domain.ResourceRoom, Name: "Sala A", AutoAccept: true,
		CreatedBy: "mgr-1", Managers: []string{"mgr-2"},
	})
	if err != nil {
		t.Fatalf("CreateResource: %v", err)
	}

	newName := "Sala A (renombrada)"
	// Un extraño (no gestor, no admin) no puede editar.
	if _, err := st.UpdateResource(ctx, room.ID, "stranger", false, store.ResourcePatch{Name: &newName}); !errors.Is(err, domain.ErrNotManager) {
		t.Fatalf("extraño edita: err=%v, quiero ErrNotManager", err)
	}
	// El gestor listado (mgr-2) sí.
	if _, err := st.UpdateResource(ctx, room.ID, "mgr-2", false, store.ResourcePatch{Name: &newName}); err != nil {
		t.Fatalf("gestor edita: %v", err)
	}
	// Un admin (isAdmin=true) también, aunque no sea gestor.
	active := false
	if _, err := st.UpdateResource(ctx, room.ID, "admin-x", true, store.ResourcePatch{Active: &active}); err != nil {
		t.Fatalf("admin edita: %v", err)
	}
	// Un extraño no puede borrar; el creador sí.
	if err := st.DeleteResource(ctx, room.ID, "stranger", false); !errors.Is(err, domain.ErrNotManager) {
		t.Fatalf("extraño borra: err=%v, quiero ErrNotManager", err)
	}
	if err := st.DeleteResource(ctx, room.ID, "mgr-1", false); err != nil {
		t.Fatalf("creador borra: %v", err)
	}
	// Tras borrar, no aparece en el listado.
	if list, _ := st.ListResources(ctx, time.Time{}, time.Time{}, false); len(list) != 0 {
		t.Fatalf("recurso borrado sigue listándose: %+v", list)
	}
}

// TestResourceTenantIsolation: un recurso de un tenant no es visible/reservable desde
// otro, aunque se conozca su ID (RLS).
func TestResourceTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	room, err := st.CreateResource(ctxA, store.NewResource{Kind: domain.ResourceRoom, Name: "Sala A", AutoAccept: true, CreatedBy: "u-a"})
	if err != nil {
		t.Fatalf("A CreateResource: %v", err)
	}
	// B no ve el recurso de A por su ID.
	if _, err := st.GetResource(ctxB, room.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B ve el recurso de A: err=%v", err)
	}
	// B no puede reservar el recurso de A por su ID.
	if _, err := st.BookResource(ctxB, room.ID, booking("evt-x", time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), time.Hour)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B reserva el recurso de A: err=%v", err)
	}
	// B no lista los recursos de A.
	if list, _ := st.ListResources(ctxB, time.Time{}, time.Time{}, false); len(list) != 0 {
		t.Fatalf("aislamiento roto: B lista recursos de A: %+v", list)
	}
}

// TestDelegationManageGrantsWrite: la delegación de agenda es un share 'manage' que
// concede ESCRITURA (como writer) en el calendario del dueño; un reader/sin share no.
func TestDelegationManageGrantsWrite(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "boss-1", Name: "agenda", DisplayName: "Agenda"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}
	// El dueño delega en su asistente (por correo) con permiso 'manage'.
	if _, err := st.CreateCalendarShare(ctx, cal.ID, "boss-1", "assistant@sytue.eu", domain.PermManage); err != nil {
		t.Fatalf("CreateCalendarShare(manage): %v", err)
	}
	// El asistente, por su correo del JWT, obtiene permiso manage (que es escritura).
	perm, err := st.CalendarAccess(ctx, "assistant-sub", "assistant@sytue.eu", cal.ID)
	if err != nil || perm != domain.PermManage {
		t.Fatalf("delegado: perm=%q err=%v, quiero manage", perm, err)
	}
	if !domain.CanWrite(perm) {
		t.Fatalf("manage debe conceder escritura (delegación de agenda)")
	}
	// Aparece en «compartido conmigo» con permiso manage.
	shared, err := st.ListCalendarsSharedWith(ctx, "assistant-sub", "assistant@sytue.eu")
	if err != nil || len(shared) != 1 || shared[0].Permission != domain.PermManage {
		t.Fatalf("delegación en shared-with-me: %+v err=%v", shared, err)
	}
}
