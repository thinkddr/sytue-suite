// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// TestGroupCRUDMembershipAndTenancy cubre el CRUD de grupos, la membresía
// (alta/baja, duplicado, cascada al borrar) y —lo crítico para el review— el
// AISLAMIENTO por-tenant: un group_id o la vista de otra org NUNCA liga ni ve los
// grupos/miembros ajenos, y GroupsForUser (forma del claim JWT) solo devuelve los
// nombres del tenant consultado.
func TestGroupCRUDMembershipAndTenancy(t *testing.T) {
	st, ctx := newStore(t)

	acme, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg(acme): %v", err)
	}
	otra, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "otra", Name: "Otra"})
	if err != nil {
		t.Fatalf("CreateOrg(otra): %v", err)
	}
	ada := seedUser(t, ctx, st, "ada@acme.eu")
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: acme.ID, UserID: ada.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership(ada@acme): %v", err)
	}

	// --- CRUD ---
	g, err := st.CreateGroup(ctx, nil, acme.ID, "eng", "Ingeniería")
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if g.Name != "eng" || g.Description != "Ingeniería" || g.OrgID != acme.ID {
		t.Fatalf("CreateGroup devolvió %+v", g)
	}
	// Nombre único por-tenant ⇒ conflicto.
	if _, err := st.CreateGroup(ctx, nil, acme.ID, "eng", ""); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CreateGroup duplicado: err=%v; quería ErrConflict", err)
	}
	// Mismo nombre en OTRA org sí se permite (unicidad es por-tenant).
	if _, err := st.CreateGroup(ctx, nil, otra.ID, "eng", ""); err != nil {
		t.Fatalf("CreateGroup(otra,eng): %v", err)
	}

	list, err := st.ListGroups(ctx, acme.ID)
	if err != nil || len(list) != 1 || list[0].MemberCount != 0 {
		t.Fatalf("ListGroups(acme) = %+v, err=%v; quería 1 grupo con 0 miembros", list, err)
	}

	// --- Membresía ---
	if err := st.AddGroupMember(ctx, nil, acme.ID, g.ID, ada.ID); err != nil {
		t.Fatalf("AddGroupMember: %v", err)
	}
	// Duplicado ⇒ conflicto.
	if err := st.AddGroupMember(ctx, nil, acme.ID, g.ID, ada.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("AddGroupMember duplicado: err=%v; quería ErrConflict", err)
	}
	members, err := st.ListGroupMembers(ctx, acme.ID, g.ID)
	if err != nil || len(members) != 1 || members[0].UserID != ada.ID {
		t.Fatalf("ListGroupMembers = %+v, err=%v", members, err)
	}
	// GroupsForUser (forma del claim): nombres del tenant.
	if names, err := st.GroupsForUser(ctx, acme.ID, ada.ID); err != nil || len(names) != 1 || names[0] != "eng" {
		t.Fatalf("GroupsForUser(acme) = %v, err=%v; quería [eng]", names, err)
	}

	// --- Aislamiento por-tenant ---
	// El grupo de acme no existe desde la vista de otra.
	if _, err := st.GetGroup(ctx, otra.ID, g.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetGroup(otra, grupo-de-acme): err=%v; quería ErrNotFound", err)
	}
	// No se puede ligar un miembro al grupo de acme usando el org_id de otra.
	if err := st.AddGroupMember(ctx, nil, otra.ID, g.ID, ada.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AddGroupMember(otra, grupo-de-acme): err=%v; quería ErrNotFound", err)
	}
	// El claim del tenant otra no incluye grupos de acme.
	if names, err := st.GroupsForUser(ctx, otra.ID, ada.ID); err != nil || len(names) != 0 {
		t.Fatalf("GroupsForUser(otra) = %v, err=%v; quería vacío", names, err)
	}
	// La lista de miembros vista desde otra no expone los de acme.
	if m, err := st.ListGroupMembers(ctx, otra.ID, g.ID); err != nil || len(m) != 0 {
		t.Fatalf("ListGroupMembers(otra, grupo-de-acme) = %+v, err=%v; quería vacío", m, err)
	}
	// Quitar miembro desde otra no toca la membresía real (idempotente, org-guarded).
	if err := st.RemoveGroupMember(ctx, nil, otra.ID, g.ID, ada.ID); err != nil {
		t.Fatalf("RemoveGroupMember(otra): %v", err)
	}
	if m, _ := st.ListGroupMembers(ctx, acme.ID, g.ID); len(m) != 1 {
		t.Fatal("RemoveGroupMember cross-tenant NO debía quitar al miembro real")
	}

	// --- Quitar la membresía de la org purga las pertenencias a grupos ---
	// (group_members no cae en cascada al quitar la membresía; RemoveMembership la
	// purga para no dejar authz huérfana que reconcedería el claim al re-invitar).
	if err := st.RemoveMembership(ctx, nil, acme.ID, ada.ID); err != nil {
		t.Fatalf("RemoveMembership: %v", err)
	}
	if names, err := st.GroupsForUser(ctx, acme.ID, ada.ID); err != nil || len(names) != 0 {
		t.Fatalf("tras RemoveMembership, GroupsForUser = %v, err=%v; quería vacío", names, err)
	}
	if m, err := st.ListGroupMembers(ctx, acme.ID, g.ID); err != nil || len(m) != 0 {
		t.Fatalf("tras RemoveMembership, ListGroupMembers = %+v, err=%v; quería vacío", m, err)
	}
	// Re-ligar para el resto del test (update/delete).
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: acme.ID, UserID: ada.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("re-UpsertMembership: %v", err)
	}
	if err := st.AddGroupMember(ctx, nil, acme.ID, g.ID, ada.ID); err != nil {
		t.Fatalf("re-AddGroupMember: %v", err)
	}

	// --- Update ---
	upd, err := st.UpdateGroup(ctx, nil, acme.ID, g.ID, "engineering", "")
	if err != nil || upd.Name != "engineering" || upd.Description != "" {
		t.Fatalf("UpdateGroup = %+v, err=%v", upd, err)
	}
	// Update desde otra org no encuentra el grupo.
	if _, err := st.UpdateGroup(ctx, nil, otra.ID, g.ID, "x", ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UpdateGroup(otra): err=%v; quería ErrNotFound", err)
	}

	// --- Delete + cascada ---
	ok, err := st.DeleteGroup(ctx, nil, acme.ID, g.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteGroup: ok=%v err=%v", ok, err)
	}
	// La membresía cayó en cascada (FK ON DELETE CASCADE): el claim queda vacío.
	if names, _ := st.GroupsForUser(ctx, acme.ID, ada.ID); len(names) != 0 {
		t.Fatalf("tras borrar el grupo, GroupsForUser = %v; quería vacío", names)
	}
	// Borrar un grupo inexistente en la org ⇒ false (no error).
	if ok, err := st.DeleteGroup(ctx, nil, acme.ID, g.ID); err != nil || ok {
		t.Fatalf("DeleteGroup(2ª vez): ok=%v err=%v; quería (false,nil)", ok, err)
	}
}
