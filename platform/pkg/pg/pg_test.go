// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pg

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestConnectEmptyDSN(t *testing.T) {
	if _, err := Connect(context.Background(), Config{DSN: ""}); err == nil {
		t.Fatal("esperaba error con DSN vacío")
	}
}

func TestNormalizeDSN(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@h/db":   "pgx5://u:p@h/db",
		"postgresql://u:p@h/db": "pgx5://u:p@h/db",
		"pgx5://u:p@h/db":       "pgx5://u:p@h/db",
		"weird://x":             "weird://x",
	}
	for in, want := range cases {
		if got := normalizeDSN(in); got != want {
			t.Errorf("normalizeDSN(%q) = %q, want %q", in, got, want)
		}
	}
}

// recordingQuerier implementa Querier capturando la última sentencia Exec.
type recordingQuerier struct {
	called   bool
	lastSQL  string
	lastArgs []any
}

func (q *recordingQuerier) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q.called = true
	q.lastSQL = sql
	q.lastArgs = args
	return pgconn.CommandTag{}, nil
}

func (q *recordingQuerier) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil //nolint:nilnil // stub de test: nunca se consume el resultado.
}

func (q *recordingQuerier) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func TestSetTenantValidation(t *testing.T) {
	q := &recordingQuerier{}
	if err := SetTenant(context.Background(), q, "BAD SLUG"); err == nil {
		t.Fatal("esperaba error por slug inválido")
	}
	if q.called {
		t.Fatal("no debe ejecutar SQL con slug inválido")
	}
}

func TestSetTenantValid(t *testing.T) {
	q := &recordingQuerier{}
	if err := SetTenant(context.Background(), q, "acme"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !q.called || len(q.lastArgs) != 1 || q.lastArgs[0] != "acme" {
		t.Fatalf("set_config mal invocado: sql=%q args=%v", q.lastSQL, q.lastArgs)
	}
}
