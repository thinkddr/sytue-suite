// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

// Tests de integracion del store de SCIM saliente contra un PostgreSQL real y
// efimero (platform/pkg/pgtest), detras del build-tag `integration`. Verifican que
// scim_map y sync_runs tienen RLS ENABLE+FORCE y que el aislamiento por org es
// deny-by-default: una org NO ve ni toca el mapeo ni el historial de otra.
package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestScimTablesRLS(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()
	for _, tbl := range []string{"scim_map", "sync_runs"} {
		var enable, force bool
		if err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&enable, &force); err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !enable || !force {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, enable, force)
		}
	}
}

func TestScimMapOrgIsolation(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "scim")

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")

	instA, err := st.CreateInstance(ctxA, "scim", "SaaS de A", nil, nil)
	if err != nil {
		t.Fatalf("A CreateInstance: %v", err)
	}
	entry := domain.ScimMapEntry{Kind: domain.ScimUser, SytueID: "u1", RemoteID: "srv-1", Hash: "h1"}
	if err := st.UpsertScimMap(ctxA, instA.ID, entry); err != nil {
		t.Fatalf("A UpsertScimMap: %v", err)
	}

	// A ve su mapeo.
	got, err := st.LoadScimMap(ctxA, instA.ID)
	if err != nil {
		t.Fatalf("A LoadScimMap: %v", err)
	}
	if len(got) != 1 || got[0].RemoteID != "srv-1" {
		t.Fatalf("A no ve su propio mapeo: %+v", got)
	}

	// B NO ve el mapeo de A (RLS deny-by-default).
	gotB, err := st.LoadScimMap(ctxB, instA.ID)
	if err != nil {
		t.Fatalf("B LoadScimMap: %v", err)
	}
	if len(gotB) != 0 {
		t.Fatalf("aislamiento roto: B ve %d filas de scim_map de A", len(gotB))
	}
	// Ni puede borrarlo (0 filas afectadas ⇒ ErrNotFound; la fila de A es invisible).
	if err := st.DeleteScimMap(ctxB, instA.ID, domain.ScimUser, "u1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B borra el mapeo de A (err=%v)", err)
	}
	// A conserva su fila intacta.
	if got, _ := st.LoadScimMap(ctxA, instA.ID); len(got) != 1 {
		t.Fatalf("la fila de A no debería haber cambiado: %+v", got)
	}
}

func TestSyncRunsOrgIsolation(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	seedTemplate(t, h, "scim")

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")

	instA, err := st.CreateInstance(ctxA, "scim", "SaaS de A", nil, nil)
	if err != nil {
		t.Fatalf("A CreateInstance: %v", err)
	}
	runID, err := st.StartSyncRun(ctxA, instA.ID)
	if err != nil {
		t.Fatalf("A StartSyncRun: %v", err)
	}
	if err := st.FinishSyncRun(ctxA, runID, domain.SyncCounts{Created: 2}, "ok"); err != nil {
		t.Fatalf("A FinishSyncRun: %v", err)
	}

	// A ve su run.
	runs, err := st.ListSyncRuns(ctxA, instA.ID, 50)
	if err != nil {
		t.Fatalf("A ListSyncRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "ok" || runs[0].Counts.Created != 2 {
		t.Fatalf("A no ve su run correctamente: %+v", runs)
	}

	// B NO ve el historial de A (RLS).
	runsB, err := st.ListSyncRuns(ctxB, instA.ID, 50)
	if err != nil {
		t.Fatalf("B ListSyncRuns: %v", err)
	}
	if len(runsB) != 0 {
		t.Fatalf("aislamiento roto: B ve %d sync_runs de A", len(runsB))
	}
}
