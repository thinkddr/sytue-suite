// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Integración de PIM (mig 0020) contra Postgres real: retrocompat de la asignación
// directa, activación sin aprobación (con expiración perezosa), flujo de aprobación
// (pending→approved→active), no-elegibilidad y desactivación. Gated tras `integration`
// (fuera del build/CI por defecto), como el resto del harness de store.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// seedPIMOrg crea org + usuario miembro + rol personalizado «Help Desk» (users.manage).
func seedPIMOrg(t *testing.T, st *store.Store, ctx context.Context) (org domain.Organization, user domain.User, role domain.CustomRole) {
	t.Helper()
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "pim", Name: "PIM Co"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	user = seedUser(t, ctx, st, "eve@sytue.eu")
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleMember}); err != nil {
		t.Fatalf("UpsertMembership: %v", err)
	}
	role, err = st.CreateCustomRole(ctx, nil, org.ID, "Help Desk", []string{"users.manage"})
	if err != nil {
		t.Fatalf("CreateCustomRole: %v", err)
	}
	return org, user, role
}

// effective calcula los perms efectivos como buildTokens: direct ∪ activaciones activas.
func effective(t *testing.T, st *store.Store, ctx context.Context, orgID, userID string, now time.Time) []string {
	t.Helper()
	direct, err := st.PermissionsForUserInOrg(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("PermissionsForUserInOrg: %v", err)
	}
	acts, err := st.ActivationPermsForUser(ctx, orgID, userID, now)
	if err != nil {
		t.Fatalf("ActivationPermsForUser: %v", err)
	}
	return domain.EffectivePerms(direct, acts, now)
}

func has(perms []string, want string) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

// TestPIMActivationLifecycle cubre elegibilidad→activación sin aprobación, la expiración
// perezosa (los perms desaparecen pasado expires_at) y la desactivación.
func TestPIMActivationLifecycle(t *testing.T) {
	st, ctx := newStore(t)
	org, user, role := seedPIMOrg(t, st, ctx)
	now := time.Now().UTC()

	// Elegible pero SIN activar ⇒ sin perms (la elegibilidad no concede).
	if _, err := st.CreateEligibility(ctx, org.ID, user.ID, role.ID, "", false, 60); err != nil {
		t.Fatalf("CreateEligibility: %v", err)
	}
	if p := effective(t, st, ctx, org.ID, user.ID, now); has(p, "users.manage") {
		t.Fatalf("elegible-no-activado no debe conceder: %v", p)
	}

	// Activar (sin aprobación) por 30 min ⇒ active ⇒ perms presentes.
	act, err := st.CreateActivation(ctx, org.ID, user.ID, role.ID, "", "cubrir turno", 30, false, now)
	if err != nil {
		t.Fatalf("CreateActivation: %v", err)
	}
	if act.Status != domain.ActivationActive {
		t.Fatalf("status = %q, quiero active", act.Status)
	}
	if p := effective(t, st, ctx, org.ID, user.ID, now); !has(p, "users.manage") {
		t.Fatalf("activado debe conceder: %v", p)
	}

	// Pasado expires_at ⇒ perms desaparecen (expiración perezosa evaluada contra now).
	if p := effective(t, st, ctx, org.ID, user.ID, now.Add(31*time.Minute)); has(p, "users.manage") {
		t.Fatalf("expirado no debe conceder: %v", p)
	}

	// Desactivar ⇒ revoked ⇒ perms desaparecen incluso dentro de la ventana.
	ok, err := st.DeactivateActivation(ctx, org.ID, user.ID, act.ID)
	if err != nil || !ok {
		t.Fatalf("DeactivateActivation: ok=%v err=%v", ok, err)
	}
	if p := effective(t, st, ctx, org.ID, user.ID, now); has(p, "users.manage") {
		t.Fatalf("revocado no debe conceder: %v", p)
	}
}

// TestPIMApprovalFlow cubre pending→approved→active y el rechazo.
func TestPIMApprovalFlow(t *testing.T) {
	st, ctx := newStore(t)
	org, user, role := seedPIMOrg(t, st, ctx)
	approver := seedUser(t, ctx, st, "boss@sytue.eu")
	now := time.Now().UTC()

	if _, err := st.CreateEligibility(ctx, org.ID, user.ID, role.ID, "", true, 60); err != nil {
		t.Fatalf("CreateEligibility(approval): %v", err)
	}
	// Requiere aprobación ⇒ pending ⇒ sin perms.
	act, err := st.CreateActivation(ctx, org.ID, user.ID, role.ID, "", "acceso puntual", 30, true, now)
	if err != nil {
		t.Fatalf("CreateActivation: %v", err)
	}
	if act.Status != domain.ActivationPending {
		t.Fatalf("status = %q, quiero pending", act.Status)
	}
	if p := effective(t, st, ctx, org.ID, user.ID, now); has(p, "users.manage") {
		t.Fatalf("pending no debe conceder: %v", p)
	}

	// Aprobar ⇒ active con ventana ⇒ perms presentes.
	appr, err := st.ApproveActivation(ctx, org.ID, act.ID, approver.ID, now)
	if err != nil {
		t.Fatalf("ApproveActivation: %v", err)
	}
	if appr.Status != domain.ActivationActive || appr.ExpiresAt == nil {
		t.Fatalf("aprobada: status=%q expires=%v", appr.Status, appr.ExpiresAt)
	}
	if p := effective(t, st, ctx, org.ID, user.ID, now); !has(p, "users.manage") {
		t.Fatalf("aprobada debe conceder: %v", p)
	}
	// Re-aprobar una ya activa ⇒ ErrNotFound (solo transiciona desde pending).
	if _, err := st.ApproveActivation(ctx, org.ID, act.ID, approver.ID, now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("re-aprobar = %v, quiero ErrNotFound", err)
	}

	// Denegar otra solicitud pendiente ⇒ denied ⇒ sin perms.
	act2, err := st.CreateActivation(ctx, org.ID, user.ID, role.ID, "", "otra", 15, true, now)
	if err != nil {
		t.Fatalf("CreateActivation#2: %v", err)
	}
	ok, err := st.DenyActivation(ctx, org.ID, act2.ID, approver.ID)
	if err != nil || !ok {
		t.Fatalf("DenyActivation: ok=%v err=%v", ok, err)
	}
}

// TestPIMBackwardCompatAndIsolation cubre la retrocompat de la asignación DIRECTA (sigue
// concediendo sin PIM) y la no-elegibilidad (no se puede activar un rol sin elegibilidad).
func TestPIMBackwardCompatAndIsolation(t *testing.T) {
	st, ctx := newStore(t)
	org, user, role := seedPIMOrg(t, st, ctx)
	now := time.Now().UTC()

	// Asignación DIRECTA (mig 0019) ⇒ concede sin ninguna activación PIM.
	if err := st.AssignCustomRole(ctx, nil, org.ID, role.ID, user.ID); err != nil {
		t.Fatalf("AssignCustomRole: %v", err)
	}
	if p := effective(t, st, ctx, org.ID, user.ID, now); !has(p, "users.manage") {
		t.Fatalf("asignación directa debe conceder (retrocompat): %v", p)
	}

	// No-elegibilidad: sin fila de elegibilidad, GetEligibilityForUser ⇒ ErrNotFound
	// (el handler lo traduce a 403: no puedes activar un rol para el que no eres elegible).
	if _, err := st.GetEligibilityForUser(ctx, org.ID, user.ID, role.ID, ""); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("GetEligibilityForUser sin elegibilidad = %v, quiero ErrNotFound", err)
	}
}
