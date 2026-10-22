// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

func TestMatrixRevocationFencesActivationAfterCommit(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	if err := events.Migrate(h.DSN); err != nil {
		t.Fatal(err)
	}
	st, ctx := store.New(h.Pool), context.Background()
	user := seedUser(t, ctx, st, "matrix@acme.example")
	unavailable := errors.New("encrypted transport unavailable")
	var observed []store.MatrixAccessChange
	st.SetExternalAccessGuard(store.ExternalAccessGuard{Apply: func(_ context.Context, c store.MatrixAccessChange) error {
		observed = append(observed, c)
		return unavailable
	}})
	if _, err := st.SetUserStatus(ctx, nil, user.ID, domain.StatusSuspended); !errors.Is(err, unavailable) {
		t.Fatalf("confirmed a suspension without revoking Matrix: %v", err)
	}
	current, err := st.GetUserByID(ctx, user.ID)
	if err != nil || !current.Active() {
		t.Fatalf("failed revocation changed ID: %+v, %v", current, err)
	}
	firstVersion := observed[0].Version
	if firstVersion <= 0 || observed[0].Tenant != "" || observed[0].Status != "suspended" {
		t.Fatalf("invalid global revocation: %+v", observed[0])
	}
	st.SetExternalAccessGuard(store.ExternalAccessGuard{Apply: func(_ context.Context, c store.MatrixAccessChange) error {
		observed = append(observed, c)
		return nil
	}})
	if _, err := st.SetUserStatus(ctx, nil, user.ID, domain.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if observed[1].Version <= firstVersion {
		t.Fatal("rollback reused a Matrix fence version")
	}
	// The worker cannot observe an activation from a rolled-back transaction.
	rollback := errors.New("rollback")
	if err := st.InTx(ctx, func(q pg.Querier) error {
		if _, err := st.SetUserStatus(ctx, q, user.ID, domain.StatusActive); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var pending int
	if err := st.DB().QueryRow(ctx, `SELECT count(*) FROM events_outbox`).Scan(&pending); err != nil || pending != 0 {
		t.Fatalf("activation escaped before commit: %d, %v", pending, err)
	}
	if _, err := st.SetUserStatus(ctx, nil, user.ID, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 {
		t.Fatal("activation called Matrix synchronously before its commit")
	}
	var version int64
	if err := st.DB().QueryRow(ctx, `SELECT (payload->>'version')::bigint FROM events_outbox
		WHERE type=$1 AND payload->>'subject'=$2`, store.MatrixAccessActive, user.ID).Scan(&version); err != nil || version <= observed[1].Version {
		t.Fatalf("missing committed activation with a newer fence: %d, %v", version, err)
	}
}

func TestMatrixOrganizationRemovalFailsClosed(t *testing.T) {
	st, ctx := newStore(t)
	user := seedUser(t, ctx, st, "matrix-member@acme.example")
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: "matrix-acme", Name: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: org.ID, UserID: user.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	unavailable := errors.New("encrypted transport unavailable")
	st.SetExternalAccessGuard(store.ExternalAccessGuard{Apply: func(_ context.Context, c store.MatrixAccessChange) error {
		if c.Subject != user.ID || c.Tenant != org.Slug || c.Status != "removed" || c.Version <= 0 {
			t.Errorf("incorrect removal scope: %+v", c)
		}
		return unavailable
	}})
	if err := st.RemoveMembership(ctx, nil, org.ID, user.ID); !errors.Is(err, unavailable) {
		t.Fatal("organization removal ignored a failed Matrix revocation")
	}
	if _, err := st.GetMembership(ctx, org.ID, user.ID); err != nil {
		t.Fatalf("organization membership changed despite failed revocation: %v", err)
	}
}
