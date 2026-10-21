// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package events_test

import (
	"os"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
)

func TestSharedOutboxHasIndependentLedgerWithDirectDSN(t *testing.T) {
	h := pgtest.New(t, os.DirFS("."), "migrations")
	if _, err := h.Pool.Exec(t.Context(), `UPDATE schema_migrations SET version=115`); err != nil {
		t.Fatal(err)
	}
	t.Setenv(pg.DirectDSNEnv, h.DSN)
	for range 2 {
		if err := events.Migrate("postgres://unused@pgbouncer/id"); err != nil {
			t.Fatal(err)
		}
	}
	var service, shared int
	if err := h.Pool.QueryRow(t.Context(), `SELECT version FROM schema_migrations`).Scan(&service); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.QueryRow(t.Context(), `SELECT version FROM events_schema_migrations`).Scan(&shared); err != nil {
		t.Fatal(err)
	}
	if service != 115 || shared != 2 {
		t.Fatalf("migration ledgers collided: service=%d outbox=%d", service, shared)
	}
}
