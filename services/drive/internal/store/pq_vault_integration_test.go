// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/drive/db"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func testPQWrapper(t *testing.T, tenant, product, subject string) recoverywrap.PQVaultWrapper {
	t.Helper()
	auk, err := recoverywrap.NewUnlockKey()
	if err != nil {
		t.Fatal(err)
	}
	id, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	secret := recoverywrap.PQVaultSecret{
		V:        recoverywrap.PQVaultVersion,
		Profile:  id.Profile(),
		Identity: id.String(),
		OwnerKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	w, err := recoverywrap.WrapPQVault(auk, tenant, product, subject, secret)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestPQVaultRLSIsolationIdempotencyPaginationAndReset(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	alice := store.WithOwner(tenantCtx("acme"), "alice")

	// Sin inicializar todavía.
	empty, err := st.GetPQVault(alice, "drive")
	if err != nil {
		t.Fatal(err)
	}
	if empty.VaultID != "" || len(empty.Keys) != 0 {
		t.Fatalf("esperaba bóveda vacía, obtuve %+v", empty)
	}

	w1 := testPQWrapper(t, "acme", "drive", "alice")
	state, err := st.PutPQVaultKey(alice, "drive", w1, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if state.VaultID == "" || state.ActiveKeyID != w1.KeyID || len(state.Keys) != 1 {
		t.Fatalf("estado inicial inesperado: %+v", state)
	}

	// RLS independiente de los predicados de aplicación en las 3 tablas.
	for _, vis := range []struct {
		tenant, owner string
		want          int
	}{{"acme", "alice", 1}, {"acme", "bob", 0}, {"other", "alice", 0}} {
		if err := h.Pool.InTenantTx(tenantCtx(vis.tenant), func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `SELECT set_config('app.owner_sub',$1,true)`, vis.owner); err != nil {
				return err
			}
			for _, table := range []string{"pq_vaults", "pq_vault_keys"} {
				var n int
				if err := tx.QueryRow(t.Context(), fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&n); err != nil {
					return err
				}
				if n != vis.want {
					return fmt.Errorf("RLS en %s para %+v: got %d want %d", table, vis, n, vis.want)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Escritura idempotente y conflicto si cambia el cuerpo.
	p1 := json.RawMessage(`{"org.sytue.storage_pq":{"v":1,"n":1}}`)
	p2 := json.RawMessage(`{"org.sytue.storage_pq":{"v":1,"n":2}}`)
	rec1, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "tx-1", p1)
	if err != nil {
		t.Fatal(err)
	}
	rec1Retry, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "tx-1", p1)
	if err != nil || rec1Retry.EventID != rec1.EventID {
		t.Fatalf("reintento idempotente falló: rec=%+v err=%v", rec1Retry, err)
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "tx-1", p2); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("misma tx con cuerpo distinto: got %v want ErrConflict", err)
	}
	rec2, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "tx-2", p2)
	if err != nil {
		t.Fatal(err)
	}

	// Paginación inversa (más reciente primero).
	page1, err := st.ListPQVaultRecords(alice, "drive", state.VaultID, 0, 1)
	if err != nil || len(page1.Entries) != 1 || page1.Entries[0].EventID != rec2.EventID || page1.NextCursor == "" {
		t.Fatalf("page1 inesperada: %+v err=%v", page1, err)
	}
	got1, err := st.GetPQVaultRecord(alice, "drive", state.VaultID, rec1.EventID)
	if err != nil || got1.EventID != rec1.EventID {
		t.Fatalf("GetPQVaultRecord: got %+v err=%v", got1, err)
	}

	// Reset crea un vault_id nuevo y borra envoltorios previos.
	w2 := testPQWrapper(t, "acme", "drive", "alice")
	resetState, err := st.PutPQVaultKey(alice, "drive", w2, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if resetState.VaultID == state.VaultID || resetState.ActiveKeyID != w2.KeyID || len(resetState.Keys) != 1 {
		t.Fatalf("reset inesperado: %+v", resetState)
	}
}
