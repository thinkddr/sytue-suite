// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestDynamicGroups cubre el motor materializado end-to-end contra Postgres real:
// recómputo (quien cumple entra, quien no queda fuera), reasignación al cambiar la regla,
// reevaluación al cambiar el atributo de un usuario, que GroupsForUser (forma del claim
// `groups`) incluye la membresía dinámica, el aislamiento por tenant, y que un grupo
// dinámico RECHAZA la asignación manual.
func TestDynamicGroups(t *testing.T) {
	st, ctx := newStore(t)

	acme, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	otra, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatalf("CreateOrg(otra): %v", err)
	}

	// Tres usuarios de acme con distintos departamentos + uno de otra org (aislamiento).
	mk := func(email, dept, loc string) domain.User {
		u, err := st.CreateUser(ctx, nil, domain.User{Email: email, EmailVerified: true, Name: email, Department: dept, Location: loc})
		if err != nil {
			t.Fatalf("CreateUser(%s): %v", email, err)
		}
		if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: acme.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
			t.Fatalf("UpsertMembership(%s): %v", email, err)
		}
		return u
	}
	sales1 := mk("ada@acme.eu", "Sales", "Madrid")
	sales2 := mk("bob@acme.eu", "Sales", "Lisboa")
	eng1 := mk("cid@acme.eu", "Engineering", "Madrid")

	// Un usuario homónimo en otra org con department Sales: NUNCA debe entrar en los
	// grupos dinámicos de acme (el recómputo solo evalúa miembros de la org).
	intruso, err := st.CreateUser(ctx, nil, domain.User{Email: "eve@otra.eu", Department: "Sales"})
	if err != nil {
		t.Fatalf("CreateUser(intruso): %v", err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: otra.ID, UserID: intruso.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership(intruso): %v", err)
	}

	// --- Grupo dinámico: department = Sales ---
	g, err := st.CreateGroup(ctx, nil, acme.ID, "sales-team", "Equipo de ventas")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	ruleSales := domain.Rule{Match: "all", Conditions: []domain.Condition{{Attribute: domain.AttrDepartment, Op: domain.OpEquals, Value: "Sales"}}}
	if _, err := st.SetGroupRule(ctx, nil, acme.ID, g.ID, ruleSales); err != nil {
		t.Fatalf("SetGroupRule: %v", err)
	}
	res, err := st.RecomputeDynamicGroup(ctx, nil, acme.ID, g.ID, ruleSales)
	if err != nil || res.Total != 2 || res.Added != 2 || res.Removed != 0 {
		t.Fatalf("RecomputeDynamicGroup = %+v, err=%v; quería 2 altas (ada, bob)", res, err)
	}

	// El grupo cargado es dinámico y trae la regla.
	got, err := st.GetGroup(ctx, acme.ID, g.ID)
	if err != nil || !got.Dynamic() || got.MembershipRule == nil {
		t.Fatalf("GetGroup dinámico = %+v, err=%v", got, err)
	}

	// Quien cumple está; quien no, fuera. Aislamiento: el intruso de otra org no entra.
	assertMembers := func(want ...string) {
		t.Helper()
		ms, err := st.ListGroupMembers(ctx, acme.ID, g.ID)
		if err != nil {
			t.Fatalf("ListGroupMembers: %v", err)
		}
		have := map[string]bool{}
		for _, m := range ms {
			have[m.UserID] = true
		}
		if len(have) != len(want) {
			t.Fatalf("miembros = %d %+v; quería %d", len(have), ms, len(want))
		}
		for _, id := range want {
			if !have[id] {
				t.Fatalf("falta el miembro %s en %+v", id, ms)
			}
		}
	}
	assertMembers(sales1.ID, sales2.ID)

	// GroupsForUser (forma EXACTA del claim `groups`) incluye la membresía dinámica.
	if names, err := st.GroupsForUser(ctx, acme.ID, sales1.ID); err != nil || len(names) != 1 || names[0] != "sales-team" {
		t.Fatalf("GroupsForUser(ada) = %v, err=%v; quería [sales-team]", names, err)
	}
	if names, _ := st.GroupsForUser(ctx, acme.ID, eng1.ID); len(names) != 0 {
		t.Fatalf("GroupsForUser(cid, no-sales) = %v; quería vacío", names)
	}

	// --- Cambiar la REGLA reasigna la membresía ---
	ruleMadrid := domain.Rule{Match: "all", Conditions: []domain.Condition{{Attribute: domain.AttrLocation, Op: domain.OpEquals, Value: "Madrid"}}}
	if _, err := st.SetGroupRule(ctx, nil, acme.ID, g.ID, ruleMadrid); err != nil {
		t.Fatalf("SetGroupRule(madrid): %v", err)
	}
	// El recálculo escribe solo la DIFERENCIA: entra cid (Madrid), sale bob (Lisboa).
	if r, err := st.RecomputeDynamicGroup(ctx, nil, acme.ID, g.ID, ruleMadrid); err != nil ||
		r.Total != 2 || r.Added != 1 || r.Removed != 1 {
		t.Fatalf("Recompute(madrid) = %+v, err=%v; quería total 2 (ada, cid), +1 -1", r, err)
	}
	assertMembers(sales1.ID, eng1.ID) // ada (Madrid) y cid (Madrid); bob (Lisboa) sale

	// --- Cambiar el ATRIBUTO de un usuario actualiza sus grupos dinámicos ---
	// bob pasa a Madrid ⇒ debe ENTRAR; cid pasa a Lisboa ⇒ debe SALIR.
	if _, err := st.SetUserAttributes(ctx, nil, sales2.ID, "Sales", "", "Madrid"); err != nil {
		t.Fatalf("SetUserAttributes(bob): %v", err)
	}
	if err := st.ReevaluateUserDynamicGroups(ctx, nil, acme.ID, sales2.ID); err != nil {
		t.Fatalf("ReevaluateUserDynamicGroups(bob): %v", err)
	}
	if _, err := st.SetUserAttributes(ctx, nil, eng1.ID, "Engineering", "", "Lisboa"); err != nil {
		t.Fatalf("SetUserAttributes(cid): %v", err)
	}
	if err := st.ReevaluateUserDynamicGroups(ctx, nil, acme.ID, eng1.ID); err != nil {
		t.Fatalf("ReevaluateUserDynamicGroups(cid): %v", err)
	}
	assertMembers(sales1.ID, sales2.ID) // ada y bob (Madrid); cid (Lisboa) fuera

	// --- Un grupo dinámico RECHAZA la asignación manual (gate en el store) ---
	if err := st.AddGroupMember(ctx, nil, acme.ID, g.ID, eng1.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AddGroupMember en grupo dinámico = %v; quería ErrNotFound (rechazo)", err)
	}
	assertMembers(sales1.ID, sales2.ID) // cid sigue fuera: el alta manual no tuvo efecto

	// --- Preview no persiste y cuenta bien ---
	sample, total, err := st.PreviewDynamicMembers(ctx, acme.ID, ruleSales, 10)
	if err != nil || total != 2 || len(sample) != 2 {
		t.Fatalf("PreviewDynamicMembers = (%d, %+v), err=%v; quería total 2", total, sample, err)
	}
	assertMembers(sales1.ID, sales2.ID) // el preview no cambió la membresía real
}
