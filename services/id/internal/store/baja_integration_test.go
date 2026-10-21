// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Baja y restauración de una cuenta (mig 0092), contra PostgreSQL real.
package store_test

import (
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// TestBajaYRestauracionDeCuenta vigila las cuatro cosas por las que esto
// existe: que la baja deja rastro, que restaurar devuelve los DATOS y no el
// ACCESO, que la ventana caduca de verdad, y que reactivar sigue sin poder
// resucitar a nadie por la puerta de al lado (la verja anti-resurrección).
func TestBajaYRestauracionDeCuenta(t *testing.T) {
	st, ctx := newStore(t)
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	jefa := seedUser(t, ctx, st, "jefa@acme.test")
	ana := seedUser(t, ctx, st, "ana@acme.test")
	for _, m := range []domain.Membership{
		{OrgID: org.ID, UserID: jefa.ID, Role: domain.RoleOwner},
		{OrgID: org.ID, UserID: ana.ID, Role: domain.RoleMember},
	} {
		if err := st.UpsertMembership(ctx, nil, m); err != nil {
			t.Fatalf("UpsertMembership: %v", err)
		}
	}

	// Una cuenta viva no se puede restaurar: no hay nada que deshacer.
	if err := st.RestoreUser(ctx, nil, ana.ID, time.Now().Add(-time.Hour)); !errors.Is(err, store.ErrNoRecuperable) {
		t.Fatalf("restaurar una cuenta viva: %v, quiero ErrNoRecuperable", err)
	}

	// Baja, con motivo y con quién la dio.
	ok, err := st.CloseUser(ctx, nil, ana.ID, jefa.ID, "fin de contrato")
	if err != nil || !ok {
		t.Fatalf("CloseUser: %v ok=%v", err, ok)
	}
	// Repetirla no reescribe nada y no es un error.
	if ok, err := st.CloseUser(ctx, nil, ana.ID, jefa.ID, "otra vez"); err != nil || ok {
		t.Fatalf("una baja repetida: %v ok=%v, quiero ok=false sin error", err, ok)
	}

	cerradas, err := st.ClosedUsersInOrg(ctx, org.ID, time.Now().Add(-24*time.Hour))
	if err != nil || len(cerradas) != 1 {
		t.Fatalf("ClosedUsersInOrg: %+v %v", cerradas, err)
	}
	if cerradas[0].Reason != "fin de contrato" || cerradas[0].ClosedBy != "jefa@acme.test" {
		t.Errorf("la baja no dejó rastro legible: %+v", cerradas[0])
	}

	// LA VERJA ANTI-RESURRECCIÓN SIGUE EN PIE: reactivar no revive a nadie.
	if reactivada, err := st.SetUserStatus(ctx, nil, ana.ID, domain.StatusActive); err != nil || reactivada {
		t.Fatalf("SetUserStatus no puede resucitar una baja: %v reactivada=%v", err, reactivada)
	}

	// Fuera de la ventana: no se restaura. (La ventana se pasa como marca, así
	// que se prueba con una del futuro en vez de esperar treinta días.)
	if err := st.RestoreUser(ctx, nil, ana.ID, time.Now().Add(time.Hour)); !errors.Is(err, store.ErrNoRecuperable) {
		t.Fatalf("una baja fuera de ventana: %v, quiero ErrNoRecuperable", err)
	}

	// Dentro de la ventana: vuelve, y vuelve SUSPENDIDA.
	if err := st.RestoreUser(ctx, nil, ana.ID, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatalf("RestoreUser: %v", err)
	}
	vuelta, err := st.GetUserByID(ctx, ana.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if vuelta.Status != domain.StatusSuspended {
		t.Errorf("restaurada como %q: restaurar devuelve los datos, no el acceso", vuelta.Status)
	}
	// Y ya no figura como baja pendiente.
	if cs, _ := st.ClosedUsersInOrg(ctx, org.ID, time.Now().Add(-24*time.Hour)); len(cs) != 0 {
		t.Errorf("sigue listada como baja recuperable: %+v", cs)
	}
	// Ahora sí se puede reactivar, que es el segundo gesto.
	if reactivada, err := st.SetUserStatus(ctx, nil, ana.ID, domain.StatusActive); err != nil || !reactivada {
		t.Fatalf("tras restaurar, reactivar tiene que funcionar: %v %v", err, reactivada)
	}
}
