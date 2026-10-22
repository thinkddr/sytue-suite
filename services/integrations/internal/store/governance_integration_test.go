// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

// Tests de integracion de la gobernanza L5 contra un PostgreSQL real y efimero
// (platform/pkg/pgtest). Detras del build-tag `integration`. Verifican:
//   - RLS ENABLE+FORCE en policies/policy_decisions,
//   - AISLAMIENTO por org: org A no ve/edita/borra las reglas ni las decisiones de B,
//   - retencion: PrunePolicyDecisions borra las decisiones anteriores al corte.
package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

func TestGovernanceMigrationsApplyRLS(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	ctx := context.Background()
	for _, tbl := range []string{"policies", "policy_decisions"} {
		var enable, force bool
		err := h.Pool.QueryRow(ctx,
			`SELECT relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname = $1 AND relkind = 'r'`, tbl).
			Scan(&enable, &force)
		if err != nil {
			t.Fatalf("tabla %q no encontrada tras migrar: %v", tbl, err)
		}
		if !enable || !force {
			t.Fatalf("tabla %q sin RLS ENABLE+FORCE (enable=%v force=%v)", tbl, enable, force)
		}
	}
}

func TestPolicyOrgIsolation(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")

	polA, err := st.CreatePolicy(ctxA, "bloquear salesforce",
		json.RawMessage(`{"api_family":"salesforce"}`), domain.PolicyBlock, true)
	if err != nil {
		t.Fatalf("A CreatePolicy: %v", err)
	}

	// B no ve la regla de A ni por listado admin…
	listB, err := st.ListPolicies(ctxB)
	if err != nil {
		t.Fatalf("B ListPolicies: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("aislamiento roto: B ve %d reglas de A", len(listB))
	}
	// …ni por la carga del motor (que fija el tenant desde el orgID).
	loadB, err := st.LoadPolicies(context.Background(), "org-b")
	if err != nil {
		t.Fatalf("LoadPolicies(org-b): %v", err)
	}
	if len(loadB) != 0 {
		t.Fatalf("aislamiento roto: el motor carga %d reglas de A para B", len(loadB))
	}
	// …ni puede editarla o borrarla.
	if _, err := st.UpdatePolicy(ctxB, polA.ID, "secuestrada", json.RawMessage(`{}`), domain.PolicyAllow, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B edita la regla de A (err=%v)", err)
	}
	if err := st.DeletePolicy(ctxB, polA.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("aislamiento roto: B borra la regla de A (err=%v)", err)
	}

	// A sí carga la suya (habilitada) por el motor.
	loadA, err := st.LoadPolicies(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("LoadPolicies(org-a): %v", err)
	}
	if len(loadA) != 1 || loadA[0].ID != polA.ID || loadA[0].Action != domain.PolicyBlock {
		t.Fatalf("A perdió su regla: %+v", loadA)
	}
}

func TestPolicyDecisionIsolationAndRetention(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))

	ctxA := tenantCtx("org-a")
	ctxB := tenantCtx("org-b")
	now := time.Now().UTC()

	// Dos decisiones de A: una antigua (48h) y una reciente (1h).
	old := domain.PolicyDecision{
		OrgID: "org-a", PolicyID: "11111111-1111-1111-1111-111111111111",
		PolicyName: "bloquear salesforce", Action: domain.PolicyBlock,
		RequestID: "req-old", Host: "api.salesforce.com", Method: "POST",
		OccurredAt: now.Add(-48 * time.Hour),
	}
	recent := old
	recent.RequestID = "req-recent"
	recent.OccurredAt = now.Add(-1 * time.Hour)
	for _, d := range []domain.PolicyDecision{old, recent} {
		if err := st.RecordDecision(context.Background(), d); err != nil {
			t.Fatalf("RecordDecision: %v", err)
		}
	}

	// B no ve las decisiones de A (RLS).
	rowsB, _, err := st.ListDecisionsPage(ctxB, "", nil, 100)
	if err != nil {
		t.Fatalf("B ListDecisionsPage: %v", err)
	}
	if len(rowsB) != 0 {
		t.Fatalf("aislamiento roto: B ve %d decisiones de A", len(rowsB))
	}
	// A ve las dos.
	rowsA, _, err := st.ListDecisionsPage(ctxA, "", nil, 100)
	if err != nil {
		t.Fatalf("A ListDecisionsPage: %v", err)
	}
	if len(rowsA) != 2 {
		t.Fatalf("A debería ver 2 decisiones, ve %d", len(rowsA))
	}

	// Retención: purgar lo anterior a 24h borra solo la antigua; el borrado de A no
	// afecta a B (RLS en el DELETE).
	n, err := st.PrunePolicyDecisions(ctxA, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("PrunePolicyDecisions: %v", err)
	}
	if n != 1 {
		t.Fatalf("la purga debería borrar 1 fila, borró %d", n)
	}
	rowsA, _, err = st.ListDecisionsPage(ctxA, "", nil, 100)
	if err != nil {
		t.Fatalf("A ListDecisionsPage tras purga: %v", err)
	}
	if len(rowsA) != 1 || rowsA[0].RequestID != "req-recent" {
		t.Fatalf("tras la purga debe quedar solo la reciente: %+v", rowsA)
	}
}
