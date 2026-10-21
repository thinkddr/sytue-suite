// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/calendar/db"
	"git.sytue.eu/sytue/one/services/calendar/internal/domain"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

// El permiso de «Pedir acceso de edición» en la base: pedirlo dos veces avisa una;
// solo el DUEÑO ve y resuelve; solo a quien lo pidió; conceder pasa a Bob de lector
// a editor y «compartido conmigo» deja de decir que lo tiene pedido.
func TestPedirYConcederEdicionDeUnCalendario(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	cal, err := st.CreateCalendar(ctx, store.NewCalendar{OwnerSub: "owner-1", Name: "trabajo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateCalendarShare(ctx, cal.ID, "owner-1", "bob@sytue.eu", domain.PermReader); err != nil {
		t.Fatal(err)
	}
	p := peticiones.Peticion{Sujeto: "bob-1", Nombre: "Bob", Correo: "bob@sytue.eu"}
	nueva, dueno, err := st.PedirEdicion(ctx, cal.ID, p)
	if err != nil || !nueva || dueno != "owner-1" {
		t.Fatalf("pedir: nueva=%v dueño=%q err=%v", nueva, dueno, err)
	}
	if nueva, _, _ := st.PedirEdicion(ctx, cal.ID, p); nueva {
		t.Error("pedirlo dos veces cuenta como nueva: el dueño recibiría dos avisos")
	}
	if sc, _ := st.ListCalendarsSharedWith(ctx, "bob-1", "bob@sytue.eu"); len(sc) != 1 || !sc[0].EditRequested {
		t.Errorf("«compartido conmigo» no dice que Bob lo tiene pedido: %+v", sc)
	}
	if _, err := st.PeticionesEdicion(ctx, cal.ID, "bob-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("el lector ve las peticiones del dueño: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, cal.ID, "bob-1", "bob-1", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("el lector se concede a sí mismo: %v", err)
	}
	if err := st.ResolverPeticionEdicion(ctx, cal.ID, "owner-1", "nadie", true); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("concedió a quien no lo pidió: %v", err)
	}
	if ps, _ := st.PeticionesEdicion(tenantCtx("globex"), cal.ID, "owner-1"); len(ps) != 0 {
		t.Errorf("otra organización ve las peticiones: %v", ps)
	}
	if err := st.ResolverPeticionEdicion(ctx, cal.ID, "owner-1", "bob-1", true); err != nil {
		t.Fatal(err)
	}
	if perm, err := st.CalendarAccess(ctx, "bob-1", "bob@sytue.eu", cal.ID); err != nil || perm != domain.PermWriter {
		t.Errorf("tras conceder, Bob es %q (%v) y tenía que ser writer", perm, err)
	}
	if sc, _ := st.ListCalendarsSharedWith(ctx, "bob-1", "bob@sytue.eu"); len(sc) != 1 || sc[0].EditRequested {
		t.Errorf("tras conceder sigue pedida: %+v", sc)
	}
}
