// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El registro de lo que hace un admin GLOBAL, contra PostgreSQL real.
//
// Lo que se prueba es que se lea entero. El registro existía desde el principio
// y estaba partido en dos mitades invisibles: la que lleva el org_id de la
// organización tocada solo se veía desde la consola de ESA organización —donde
// un admin global no entra— y la que no lo lleva (conceder alcance, elevarse)
// no se veía en ninguna parte.
package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestListPlatformAuditPage(t *testing.T) {
	st, ctx := newStore(t)

	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	admin := seedUser(t, ctx, st, "global@sytue.eu")

	audita := func(action, orgID string) {
		t.Helper()
		if err := st.Audit(ctx, nil, store.AuditEntry{
			ActorID: admin.ID, OrgID: orgID, Action: action, Target: "x",
		}); err != nil {
			t.Fatalf("Audit(%s): %v", action, err)
		}
	}
	// Las dos mitades del registro de plataforma...
	audita("platform.org.status", org.ID) // con organización
	audita("platform.pim.activated", "")  // sin ninguna
	audita("platform.admin.granted", "")
	// ...y ruido que NO es de plataforma, en la misma tabla.
	audita("session.login", org.ID)
	audita("user.suspended", org.ID)

	page, more, err := st.ListPlatformAuditPage(ctx, store.AuditFilter{}, nil, 50)
	if err != nil {
		t.Fatalf("ListPlatformAuditPage: %v", err)
	}
	if more {
		t.Errorf("more=true con cinco filas y limit 50")
	}
	if len(page) != 3 {
		t.Fatalf("%d filas; quiero las 3 de plataforma y ninguna más: %+v", len(page), page)
	}
	// Orden cronológico inverso: lo último hecho, primero.
	if page[0].Action != "platform.admin.granted" {
		t.Errorf("primera fila = %q; el registro no está en orden inverso", page[0].Action)
	}
	// La fila con organización dice CUÁL. Sin esto, «suspendida» es un uuid.
	var conOrg store.AuditRow
	for _, a := range page {
		if a.Action == "platform.org.status" {
			conOrg = a
		}
		if a.ActorEmail != "global@sytue.eu" {
			t.Errorf("fila %q sin el correo del actor: %q", a.Action, a.ActorEmail)
		}
	}
	if conOrg.OrgSlug != "acme" {
		t.Errorf("org_slug = %q, quiero acme: la fila no dice sobre quién se actuó", conOrg.OrgSlug)
	}
	// Y las que no tienen organización siguen saliendo: son justo las que no se
	// veían en ninguna consola.
	for _, a := range page {
		if a.Action == "platform.pim.activated" && a.OrgSlug != "" {
			t.Errorf("una elevación no pertenece a ninguna organización, y trae %q", a.OrgSlug)
		}
	}

	// Filtro por prefijo de acción y paginación keyset.
	solo, _, err := st.ListPlatformAuditPage(ctx, store.AuditFilter{Action: "platform.pim."}, nil, 50)
	if err != nil || len(solo) != 1 || solo[0].Action != "platform.pim.activated" {
		t.Fatalf("filtro por acción = %d filas (%v)", len(solo), err)
	}
	p1, more, err := st.ListPlatformAuditPage(ctx, store.AuditFilter{}, nil, 2)
	if err != nil || !more || len(p1) != 2 {
		t.Fatalf("página 1 = (%d, more=%v, %v)", len(p1), more, err)
	}
	last := p1[len(p1)-1]
	p2, more, err := st.ListPlatformAuditPage(ctx, store.AuditFilter{},
		&store.AuditCursor{OccurredAt: last.OccurredAt, ID: last.ID}, 2)
	if err != nil || more || len(p2) != 1 {
		t.Fatalf("página 2 = (%d, more=%v, %v)", len(p2), more, err)
	}
	if p2[0].ID == last.ID {
		t.Error("el cursor repite la última fila de la página anterior")
	}

	// Un rango de fechas que no incluye nada devuelve vacío, no todo.
	futuro := time.Now().Add(24 * time.Hour)
	vacio, _, err := st.ListPlatformAuditPage(ctx, store.AuditFilter{Since: &futuro}, nil, 50)
	if err != nil || len(vacio) != 0 {
		t.Fatalf("rango futuro = %d filas (%v)", len(vacio), err)
	}
}
