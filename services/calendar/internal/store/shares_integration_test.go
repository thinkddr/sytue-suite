// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integracion de la comparticion de calendarios con ACL contra un
// PostgreSQL real (pgtest). Verifican, end-to-end sobre el store:
//   - el dueño comparte por correo → el grantee ve (CalendarAccess da el permiso);
//   - reader vs writer;
//   - sin share → ErrNotFound (deny-by-default, 404);
//   - revocar corta el acceso;
//   - «compartido conmigo» lista lo correcto (dueño + permiso), no lo propio;
//   - solo el dueño comparte/revoca;
//   - aislamiento por tenant (un share no cruza tenant).
package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

func TestCalendarSharingACL(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	const (
		ownerSub  = "owner-1"
		bobSub    = "bob-1"
		bobEmail  = "bob@sytue.eu"
		carolSub  = "carol-1"
		carolMail = "carol@sytue.eu"
	)

	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: ownerSub, Name: "trabajo", DisplayName: "Trabajo"})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}

	// El DUEÑO siempre tiene acceso de escritura sobre su calendario.
	if perm, err := st.CalendarAccess(ctx, ownerSub, "owner@sytue.eu", cal.ID); err != nil || perm != domain.PermWriter {
		t.Fatalf("dueño: perm=%q err=%v, quiero writer/nil", perm, err)
	}

	// Sin share, Bob NO tiene acceso: deny-by-default → ErrNotFound (404).
	if _, err := st.CalendarAccess(ctx, bobSub, bobEmail, cal.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("sin share: err=%v, quiero ErrNotFound", err)
	}

	// El dueño comparte con Bob como READER (por correo; el sub se casa aparte).
	if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, bobEmail, domain.PermReader); err != nil {
		t.Fatalf("CreateCalendarShare(bob reader): %v", err)
	}
	// Bob, por su CORREO del JWT (sub distinto/desconocido), ve como reader.
	if perm, err := st.CalendarAccess(ctx, "otro-sub-de-bob", bobEmail, cal.ID); err != nil || perm != domain.PermReader {
		t.Fatalf("bob por email: perm=%q err=%v, quiero reader/nil", perm, err)
	}
	// Casado case-insensitive del correo.
	if perm, err := st.CalendarAccess(ctx, "", "BOB@Sytue.EU", cal.ID); err != nil || perm != domain.PermReader {
		t.Fatalf("bob por email may/min: perm=%q err=%v, quiero reader/nil", perm, err)
	}

	// Re-compartir con Bob como WRITER: upsert (mismo destinatario) sube el permiso.
	if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, bobEmail, domain.PermWriter); err != nil {
		t.Fatalf("CreateCalendarShare(bob writer upsert): %v", err)
	}
	if perm, err := st.CalendarAccess(ctx, bobSub, bobEmail, cal.ID); err != nil || perm != domain.PermWriter {
		t.Fatalf("bob tras upsert: perm=%q err=%v, quiero writer/nil", perm, err)
	}
	// El upsert no duplicó: un solo share para Bob.
	shares, err := st.ListCalendarShares(ctx, cal.ID, ownerSub)
	if err != nil {
		t.Fatalf("ListCalendarShares: %v", err)
	}
	if len(shares) != 1 || shares[0].Permission != domain.PermWriter {
		t.Fatalf("shares tras upsert: %+v, quiero 1 writer", shares)
	}

	// Comparte con Carol como reader (segunda vía). shared-with-me de Carol.
	if _, err := st.CreateCalendarShare(ctx, cal.ID, ownerSub, carolMail, domain.PermReader); err != nil {
		t.Fatalf("CreateCalendarShare(carol): %v", err)
	}
	carolShared, err := st.ListCalendarsSharedWith(ctx, carolSub, carolMail)
	if err != nil {
		t.Fatalf("ListCalendarsSharedWith(carol): %v", err)
	}
	if len(carolShared) != 1 || carolShared[0].Calendar.ID != cal.ID ||
		carolShared[0].Permission != domain.PermReader || carolShared[0].SharedBy != ownerSub {
		t.Fatalf("carol shared-with-me: %+v", carolShared)
	}

	// El DUEÑO no ve su propio calendario en «compartido conmigo».
	if own, _ := st.ListCalendarsSharedWith(ctx, ownerSub, "owner@sytue.eu"); len(own) != 0 {
		t.Fatalf("el dueño no debe verse en shared-with-me: %+v", own)
	}

	// SOLO EL DUEÑO comparte/lista/revoca. Un no-dueño → ErrNotFound (no filtra).
	if _, err := st.CreateCalendarShare(ctx, cal.ID, bobSub, "mallory@x.com", domain.PermWriter); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-dueño comparte: err=%v, quiero ErrNotFound", err)
	}
	if _, err := st.ListCalendarShares(ctx, cal.ID, bobSub); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-dueño lista shares: err=%v, quiero ErrNotFound", err)
	}
	if err := st.RevokeCalendarShare(ctx, cal.ID, shares[0].ID, bobSub); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("no-dueño revoca: err=%v, quiero ErrNotFound", err)
	}

	// Revocar el share de Bob (por el dueño) corta su acceso.
	if err := st.RevokeCalendarShare(ctx, cal.ID, shares[0].ID, ownerSub); err != nil {
		t.Fatalf("RevokeCalendarShare(bob): %v", err)
	}
	if _, err := st.CalendarAccess(ctx, bobSub, bobEmail, cal.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tras revocar, bob sigue con acceso: err=%v, quiero ErrNotFound", err)
	}
	// Carol conserva el suyo (revocar es por share, no por calendario).
	if perm, err := st.CalendarAccess(ctx, carolSub, carolMail, cal.ID); err != nil || perm != domain.PermReader {
		t.Fatalf("carol tras revocar a bob: perm=%q err=%v", perm, err)
	}
}

// TestCalendarShareTenantIsolation comprueba que un share no cruza tenant: el
// calendario/estado de un tenant no es accesible ni compartible desde otro, aunque
// se conozca el ID (RLS + FK dentro del tenant).
func TestCalendarShareTenantIsolation(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctxA := tenantCtx("tenant-a")
	ctxB := tenantCtx("tenant-b")

	calA, err := st.CreateCalendar(ctxA, store.NewCalendar{OwnerSub: "u-a", Name: "cal-a"})
	if err != nil {
		t.Fatalf("A CreateCalendar: %v", err)
	}
	// A comparte con un correo.
	if _, err := st.CreateCalendarShare(ctxA, calA.ID, "u-a", "guest@sytue.eu", domain.PermReader); err != nil {
		t.Fatalf("A CreateCalendarShare: %v", err)
	}

	// Desde el tenant B, ese correo NO ve el calendario de A (aislamiento).
	if entries, _ := st.ListCalendarsSharedWith(ctxB, "guest-sub", "guest@sytue.eu"); len(entries) != 0 {
		t.Fatalf("aislamiento roto: B ve el compartido de A: %+v", entries)
	}
	// Y el gate desde B sobre el ID de A → ErrNotFound (RLS oculta el calendario).
	if _, err := st.CalendarAccess(ctxB, "guest-sub", "guest@sytue.eu", calA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B accede al calendario de A: err=%v", err)
	}
	// Un no-dueño del tenant B no puede compartir el calendario de A por su ID.
	if _, err := st.CreateCalendarShare(ctxB, calA.ID, "u-a", "x@y.com", domain.PermReader); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B comparte el calendario de A: err=%v", err)
	}

	// En su propio tenant, el correo compartido SÍ ve (control positivo).
	entries, err := st.ListCalendarsSharedWith(ctxA, "guest-sub", "guest@sytue.eu")
	if err != nil {
		t.Fatalf("A ListCalendarsSharedWith: %v", err)
	}
	if len(entries) != 1 || entries[0].Calendar.ID != calA.ID {
		t.Fatalf("A: el grantee no ve el compartido en su tenant: %+v", entries)
	}
}
