// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// Los dominios salen de las direcciones de TODAS las organizaciones, principales y
// alias, una vez cada uno y en minúsculas.
func TestDominiosDeTodasLasOrganizaciones(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	acme, otra := tenantCtx("acme"), tenantCtx("otra")
	ada := seedAccount(t, acme, st, "ada@Acme.eu")
	seedAccount(t, acme, st, "bob@acme.eu")
	seedAccount(t, otra, st, "eva@otra.es")
	if _, err := st.CreateAlias(acme, ada.ID, "ventas@tienda.acme.eu"); err != nil {
		t.Fatalf("CreateAlias: %v", err)
	}
	got, err := st.Dominios(acme)
	if err != nil {
		t.Fatalf("Dominios: %v", err)
	}
	if want := []string{"acme.eu", "otra.es", "tienda.acme.eu"}; !slices.Equal(got, want) {
		t.Fatalf("Dominios = %v, quiero %v", got, want)
	}
}
