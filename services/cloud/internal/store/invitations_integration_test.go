// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	clouddb "git.sytue.eu/sytue/one/services/cloud/db"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// TestMemberInvitationLifecycle fija las propiedades que dependen de PostgreSQL:
// unicidad case-insensitive, aislamiento RLS, consumo de un solo uso y alta de la
// membresía en la misma transacción.
func TestMemberInvitationLifecycle(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	acme := tenantCtx("acme")
	globex := tenantCtx("globex")
	expires := time.Now().UTC().Add(24 * time.Hour)

	first, err := st.UpsertMemberInvitation(acme, "Person@Example.eu", domain.RoleMember, "owner-acme", expires)
	if err != nil {
		t.Fatalf("UpsertMemberInvitation(acme): %v", err)
	}
	resend, err := st.UpsertMemberInvitation(acme, "person@example.eu", domain.RoleAdmin, "owner-acme", expires.Add(time.Hour))
	if err != nil {
		t.Fatalf("reenviar invitación: %v", err)
	}
	if resend.ID != first.ID || resend.Role != domain.RoleAdmin {
		t.Fatalf("el reenvío debe actualizar la misma fila: first=%s resend=%s role=%q", first.ID, resend.ID, resend.Role)
	}

	if _, err := st.UpsertMemberInvitation(globex, "person@example.eu", domain.RoleBilling, "owner-globex", expires); err != nil {
		t.Fatalf("UpsertMemberInvitation(globex): %v", err)
	}
	acmePending, err := st.ListMemberInvitations(acme)
	if err != nil {
		t.Fatalf("ListMemberInvitations(acme): %v", err)
	}
	if len(acmePending) != 1 || acmePending[0].Role != domain.RoleAdmin {
		t.Fatalf("RLS/upsert acme incorrecto: %#v", acmePending)
	}
	globexPending, err := st.ListMemberInvitations(globex)
	if err != nil {
		t.Fatalf("ListMemberInvitations(globex): %v", err)
	}
	if len(globexPending) != 1 || globexPending[0].Role != domain.RoleBilling {
		t.Fatalf("RLS globex incorrecto: %#v", globexPending)
	}

	membership, accepted, err := st.AcceptMemberInvitation(acme, "subject-1", "PERSON@example.eu")
	if err != nil {
		t.Fatalf("AcceptMemberInvitation: %v", err)
	}
	if !accepted || membership.Sub != "subject-1" || membership.Role != domain.RoleAdmin {
		t.Fatalf("membresía materializada incorrecta: accepted=%v membership=%#v", accepted, membership)
	}
	if pending, listErr := st.ListMemberInvitations(acme); listErr != nil || len(pending) != 0 {
		t.Fatalf("la invitación consumida debe desaparecer: pending=%#v err=%v", pending, listErr)
	}
	if role, roleErr := st.GetMembershipRole(acme, "subject-1"); roleErr != nil || role != domain.RoleAdmin {
		t.Fatalf("membresía no persistida: role=%q err=%v", role, roleErr)
	}
	if _, acceptedAgain, acceptErr := st.AcceptMemberInvitation(acme, "subject-1", "person@example.eu"); acceptErr != nil || acceptedAgain {
		t.Fatalf("la invitación debe ser de un solo uso: accepted=%v err=%v", acceptedAgain, acceptErr)
	}
	if pending, listErr := st.ListMemberInvitations(globex); listErr != nil || len(pending) != 1 {
		t.Fatalf("aceptar en acme no debe tocar globex: pending=%#v err=%v", pending, listErr)
	}
}

func TestExpiredMemberInvitationCannotBeAccepted(t *testing.T) {
	h := pgtest.New(t, clouddb.Migrations, clouddb.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	if _, err := st.UpsertMemberInvitation(ctx, "late@example.eu", domain.RoleMember, "owner", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("UpsertMemberInvitation: %v", err)
	}
	if pending, err := st.ListMemberInvitations(ctx); err != nil || len(pending) != 0 {
		t.Fatalf("una invitación caducada no debe listarse: pending=%#v err=%v", pending, err)
	}
	if _, accepted, err := st.AcceptMemberInvitation(ctx, "late-sub", "late@example.eu"); err != nil || accepted {
		t.Fatalf("una invitación caducada no debe aceptarse: accepted=%v err=%v", accepted, err)
	}
}
