// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Invitaciones que se materializan solas, contra PostgreSQL real.
//
// El caso que arregla: a alguien lo invitan a una organización, el correo con el
// enlace no llega —filtros, un MSA caído, una dirección mal escrita— y esa
// persona acaba entrando por su cuenta. Sin esto no pertenecía a ninguna
// organización y la suite la recibía con el asistente de alta preguntándole
// «¿para quién es Sytue?», cuando alguien ya la había dado de alta en su empresa.
package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

func TestInvitacionPendienteSeMaterializaAlEntrar(t *testing.T) {
	st, ctx := newStore(t)
	org, _ := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	quienInvita := seedUser(t, ctx, st, "jefa@acme.test")
	ahora := time.Now()

	// Invitada con rol admin, con el correo esperándola.
	invitada := seedUser(t, ctx, st, "ops@acme.test")
	if err := st.InsertInvitation(ctx, nil, "hash-viva", org.ID, "ops@acme.test",
		string(domain.RoleAdmin), quienInvita.ID, "", "", ahora.Add(48*time.Hour)); err != nil {
		t.Fatalf("InsertInvitation: %v", err)
	}

	// Entra por su cuenta: queda DENTRO, con el rol que le dieron.
	invs, err := st.ClaimPendingInvitations(ctx, nil, invitada.ID, "ops@acme.test", ahora)
	if err != nil {
		t.Fatalf("ClaimPendingInvitations: %v", err)
	}
	if len(invs) != 1 {
		t.Fatalf("%d invitaciones materializadas, quiero 1", len(invs))
	}
	m, err := st.GetMembership(ctx, org.ID, invitada.ID)
	if err != nil {
		t.Fatalf("no quedó dentro de la organización: %v", err)
	}
	if m.Role != domain.RoleAdmin {
		t.Errorf("rol %q: se perdió el que eligió quien invitó", m.Role)
	}

	// La invitación se CONSUME: si siguiera viva, el enlace del correo —que puede
	// llegar tarde— volvería a añadirla, y el histórico diría que entró dos veces.
	segunda, err := st.ClaimPendingInvitations(ctx, nil, invitada.ID, "ops@acme.test", ahora)
	if err != nil {
		t.Fatalf("segunda llamada: %v", err)
	}
	if len(segunda) != 0 {
		t.Errorf("la invitación seguía viva tras materializarse")
	}

	// Una invitación CADUCADA no vale: es el mismo plazo que el del enlace, y si
	// no lo fuera, esta vía sería una puerta trasera a las que ya expiraron.
	otro := seedUser(t, ctx, st, "tarde@acme.test")
	if err := st.InsertInvitation(ctx, nil, "hash-caducada", org.ID, "tarde@acme.test",
		string(domain.RoleMember), quienInvita.ID, "", "", ahora.Add(-time.Hour)); err != nil {
		t.Fatalf("InsertInvitation caducada: %v", err)
	}
	vencidas, err := st.ClaimPendingInvitations(ctx, nil, otro.ID, "tarde@acme.test", ahora)
	if err != nil {
		t.Fatalf("caducada: %v", err)
	}
	if len(vencidas) != 0 {
		t.Errorf("se materializó una invitación caducada")
	}
	if _, err := st.GetMembership(ctx, org.ID, otro.ID); err == nil {
		t.Error("entró en la organización con una invitación vencida")
	}

	// Y el correo de otra persona no arrastra invitaciones ajenas.
	ajeno := seedUser(t, ctx, st, "nadie@acme.test")
	ninguna, err := st.ClaimPendingInvitations(ctx, nil, ajeno.ID, "nadie@acme.test", ahora)
	if err != nil || len(ninguna) != 0 {
		t.Errorf("sin invitación se materializaron %d (%v)", len(ninguna), err)
	}
}
