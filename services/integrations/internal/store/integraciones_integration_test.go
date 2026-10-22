// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration
// +build integration

package store_test

import (
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	integrationsdb "git.sytue.eu/sytue/one/services/integrations/db"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Apagar en una organización no apaga en otra, y volver a encender borra la fila.
func TestIntegracionApagadaPorOrganizacion(t *testing.T) {
	h := pgtest.New(t, integrationsdb.Migrations, integrationsdb.MigrationsDir)
	st := store.New(h.Pool, newCipher(t))
	ctxA, ctxB := tenantCtx("org-a"), tenantCtx("org-b")

	if err := st.SetIntegrationEnabled(ctxA, "dropbox", false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetIntegrationEnabled(ctxA, "dropbox", false); err != nil {
		t.Fatalf("apagar dos veces: %v", err)
	}
	if off, err := st.DisabledIntegrations(ctxA); err != nil || !off["dropbox"] {
		t.Fatalf("A: %v, %v", off, err)
	}
	if off, err := st.DisabledIntegrations(ctxB); err != nil || len(off) != 0 {
		t.Fatalf("B ve lo de A: %v, %v", off, err)
	}
	if err := st.SetIntegrationEnabled(ctxA, "dropbox", true); err != nil {
		t.Fatal(err)
	}
	if off, _ := st.DisabledIntegrations(ctxA); off["dropbox"] {
		t.Fatal("sigue apagada tras encenderla")
	}
}
