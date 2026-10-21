// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// El alias de sistema se crea hacia el buzón de destino y repetirlo no cambia nada;
// no le quita la dirección a otra cuenta, no se crea si la política no lo deja y sin
// destino no hay alias.
func TestAsegurarAliasDeSistema(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ops := seedAccount(t, tenantCtx("corporate"), st, "operaciones@sytue.eu")
	seedAccount(t, tenantCtx("acme"), st, "ana@sytue.eu")
	si := func(store.ResolvedAddress) bool { return true }

	for range 2 {
		if err := st.AsegurarAlias(t.Context(), "calendario@sytue.eu", "operaciones@sytue.eu", si); err != nil {
			t.Fatalf("AsegurarAlias: %v", err)
		}
	}
	if r, err := st.LookupAddress(t.Context(), "calendario@sytue.eu"); err != nil || r.AccountID != ops.ID {
		t.Fatalf("calendario@ entrega a %v (%v), quiero el buzón de operaciones", r.AccountID, err)
	}
	if err := st.AsegurarAlias(t.Context(), "ana@sytue.eu", "operaciones@sytue.eu", si); !errors.Is(err, domain.ErrAliasExists) {
		t.Fatalf("sobre la dirección de otra cuenta = %v, quiero ErrAliasExists", err)
	}
	no := func(store.ResolvedAddress) bool { return false }
	if err := st.AsegurarAlias(t.Context(), "otro@sytue.eu", "operaciones@sytue.eu", no); err == nil {
		t.Fatal("sin permiso de la política se creó igual")
	}
	if err := st.AsegurarAlias(t.Context(), "otro@sytue.eu", "nadie@sytue.eu", si); !errors.Is(err, domain.ErrNoSuchUser) {
		t.Fatalf("sin destino = %v, quiero ErrNoSuchUser", err)
	}
}
