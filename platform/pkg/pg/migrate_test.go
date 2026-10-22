// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"net/url"
	"testing"
)

func TestMigrationTableSurvivesProductionDirectDSN(t *testing.T) {
	t.Setenv(DirectDSNEnv, "postgres://id@postgres/id?sslmode=require")
	got, err := migrationDSN("postgres://id@pgbouncer/id", "events_schema_migrations")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "postgres" || u.Query().Get("x-migrations-table") != "events_schema_migrations" || u.Query().Get("sslmode") != "require" {
		t.Fatalf("lost independent migration ledger or direct database: %s", got)
	}
	if _, err := migrationDSN("postgres://id@pgbouncer/id", "events; DROP TABLE users"); err == nil {
		t.Fatal("accepted a non-identifier migration table")
	}
}
