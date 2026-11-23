// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//go:build integration

package store_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
	state, err := st.PutPQVaultKey(alice, "drive", w1, true, nil)
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
	// El recibo de un ACK del correo (ADR 0087): registro de la bóveda actual.
	if key, err := st.PQVaultReceipt(alice, "drive", state.VaultID, rec1.EventID); err != nil || key != state.ActiveKeyID {
		t.Fatalf("PQVaultReceipt: %q %v", key, err)
	}
	if _, err := st.PQVaultReceipt(alice, "drive", state.VaultID, "$pqe_NADA"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("PQVaultReceipt sin registro: %v", err)
	}
	if _, err := st.PQVaultReceipt(alice, "drive", "!pqv_otra", rec1.EventID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("PQVaultReceipt en otra bóveda: %v", err)
	}
	// Por su transaction_id (los secretos de entrada del correo, ADR 0087).
	found, err := st.FindPQVaultRecord(alice, "drive", state.VaultID, "tx-1")
	if err != nil || found.EventID != rec1.EventID {
		t.Fatalf("FindPQVaultRecord: got %+v err=%v", found, err)
	}
	if _, err := st.FindPQVaultRecord(alice, "drive", state.VaultID, "tx-nada"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindPQVaultRecord sin registro: %v", err)
	}
	if _, err := st.FindPQVaultRecord(alice, "drive", "!pqv_otra", "tx-1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("FindPQVaultRecord en otra bóveda: %v", err)
	}

	// Reset crea un vault_id nuevo y borra envoltorios previos.
	w2 := testPQWrapper(t, "acme", "drive", "alice")
	// Con lo que se vio antes de publicar (versión vieja) no se vacía nada.
	if _, err := st.PutPQVaultKey(alice, "drive", w2, true, &store.PQVaultExpect{VaultID: state.VaultID, Version: state.Version}); !errors.Is(err, domain.ErrPQVaultChanged) {
		t.Fatalf("vaciar con una versión vieja: %v", err)
	}
	if _, err := st.GetPQVaultRecord(alice, "drive", state.VaultID, rec1.EventID); err != nil {
		t.Fatal("un vaciado rechazado borró registros", err)
	}
	seen, err := st.GetPQVault(alice, "drive")
	if err != nil || seen.Version != state.Version+2 {
		t.Fatalf("cada registro nuevo sube la versión (y el reintento no): %+v %v", seen, err)
	}
	resetState, err := st.PutPQVaultKey(alice, "drive", w2, true, &store.PQVaultExpect{VaultID: seen.VaultID, Version: seen.Version})
	if err != nil {
		t.Fatal(err)
	}
	if resetState.Version != seen.Version+1 {
		t.Fatalf("el vaciado no subió la versión: %+v", resetState)
	}
	if resetState.VaultID == state.VaultID || resetState.ActiveKeyID != w2.KeyID || len(resetState.Keys) != 1 {
		t.Fatalf("reset inesperado: %+v", resetState)
	}
	// …y vacía de verdad: no queda ningún registro de la bóveda anterior, y un
	// transaction_id ya usado vuelve a servir con otro cuerpo en la nueva.
	if _, err := st.GetPQVaultRecord(alice, "drive", state.VaultID, rec1.EventID); err == nil {
		t.Fatal("el reinicio dejó registros de la bóveda anterior")
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", resetState.VaultID, "tx-1", p2); err != nil {
		t.Fatalf("tx-1 tras el reinicio: %v", err)
	}
}

// La carrera de la ADR 0086 («pasa Matrix a ella», punto a): un cliente nativo
// ve una copia a medias sin recibo y decide vaciarla; entre medias, la web
// termina su importación y publica el recibo. El vaciado con lo que vio se
// rechaza y el recibo sigue ahí.
func TestPQVaultResetRefusesWhenAReceiptLandsAfterTheRead(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	alice := store.WithOwner(tenantCtx("acme"), "alice")
	state, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "copia-1", json.RawMessage(`{"org.sytue.storage_pq":{"n":1}}`)); err != nil {
		t.Fatal(err)
	}
	visto, err := st.GetPQVault(alice, "drive")
	if err != nil {
		t.Fatal(err)
	}
	recibo, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "recibo-web", json.RawMessage(`{"org.sytue.storage_pq":{"n":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	expect := &store.PQVaultExpect{VaultID: visto.VaultID, Version: visto.Version}
	if _, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, expect); !errors.Is(err, domain.ErrPQVaultChanged) {
		t.Fatalf("el vaciado con lo visto antes del recibo no se rechazó: %v", err)
	}
	if _, err := st.GetPQVaultRecord(alice, "drive", state.VaultID, recibo.EventID); err != nil {
		t.Fatal("el vaciado rechazado borró el recibo", err)
	}
	// Una bóveda que no es la vista tampoco se vacía (ni una que no existía).
	if _, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, &store.PQVaultExpect{}); !errors.Is(err, domain.ErrPQVaultChanged) {
		t.Fatalf("vaciar «la bóveda que no existía»: %v", err)
	}
}

// Los nativos ya instalados (v3.22 y anteriores) vacían sin expectativa: se les
// vacía como antes, sin comparar, salvo con la marca de copia activa de otro.
func TestPQVaultUncheckedResetFromOlderClients(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	alice := store.WithOwner(tenantCtx("acme"), "alice")
	state, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "r-1", json.RawMessage(`{"org.sytue.storage_pq":{"n":1}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ClaimPQVaultCopy(alice, "drive", store.PQVaultCopyClaim{ID: strings.Repeat("a", 26), Holder: "native-linux", Kind: "matrix-copy", Epoch: "e", TTL: 5 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	viejo := &store.PQVaultExpect{Unchecked: true}
	if _, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, viejo); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("un cliente anterior vació durante la copia de otro: %v", err)
	}
	if _, err := st.ReleasePQVaultCopy(alice, "drive", strings.Repeat("a", 26)); err != nil {
		t.Fatal(err)
	}
	vaciada, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, viejo)
	if err != nil || vaciada.VaultID == state.VaultID || vaciada.Version != state.Version+2 {
		t.Fatalf("el vaciado de un cliente anterior: %+v %v", vaciada, err)
	}
	if page, err := st.ListPQVaultRecords(alice, "drive", vaciada.VaultID, 0, 10); err != nil || len(page.Entries) != 0 {
		t.Fatalf("no vació: %+v %v", page, err)
	}
}

// La marca de copia en curso: la tiene quien la pone; mientras vale, nadie más
// publica ni vacía; quien copia escribe y vacía con ella; caduca, se renueva y se
// retira; y quien escribe con una marca que ya no está se para.
func TestPQVaultCopyMarker(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	alice := store.WithOwner(tenantCtx("acme"), "alice")
	claim := func(id string) store.PQVaultCopyClaim {
		return store.PQVaultCopyClaim{ID: id, Holder: "native-linux", Kind: "matrix-copy", Epoch: "epoca", TTL: 5 * time.Minute}
	}
	a, b := strings.Repeat("a", 26), strings.Repeat("b", 26)
	if _, err := st.ClaimPQVaultCopy(alice, "drive", claim(a)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("una marca sin bóveda: %v", err)
	}
	state, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	puesta, err := st.ClaimPQVaultCopy(alice, "drive", claim(a))
	if err != nil || puesta.Copy == nil || puesta.Copy.ID != a || !puesta.Copy.Active || puesta.Copy.Holder != "native-linux" || puesta.Copy.Epoch != "epoca" {
		t.Fatalf("marca: %+v %v", puesta.Copy, err)
	}
	if _, err := st.ClaimPQVaultCopy(alice, "drive", claim(b)); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("otra marca encima de una activa: %v", err)
	}
	conA, conB := store.WithPQVaultCopy(alice, a), store.WithPQVaultCopy(alice, b)
	payload := func(n int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"org.sytue.storage_pq":{"n":%d}}`, n))
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", state.VaultID, "web-1", payload(1)); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("la web publicó durante la copia: %v", err)
	}
	if _, err := st.AppendPQVaultRecord(conB, "drive", state.VaultID, "otro-1", payload(1)); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("publicó con una marca ajena: %v", err)
	}
	if _, err := st.AppendPQVaultRecord(conA, "drive", state.VaultID, "copia-1", payload(1)); err != nil {
		t.Fatalf("quien copia no pudo publicar: %v", err)
	}
	visto, err := st.GetPQVault(alice, "drive")
	if err != nil {
		t.Fatal(err)
	}
	expect := &store.PQVaultExpect{VaultID: visto.VaultID, Version: visto.Version}
	if _, err := st.PutPQVaultKey(alice, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, expect); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("otro vació durante la copia: %v", err)
	}
	vaciada, err := st.PutPQVaultKey(conA, "drive", testPQWrapper(t, "acme", "drive", "alice"), true, expect)
	if err != nil || vaciada.Copy == nil || vaciada.Copy.ID != a || vaciada.VaultID == state.VaultID {
		t.Fatalf("quien copia vacía y conserva su marca: %+v %v", vaciada, err)
	}
	renovada, err := st.ClaimPQVaultCopy(alice, "drive", claim(a))
	if err != nil || !renovada.Copy.StartedAt.Equal(puesta.Copy.StartedAt) || !renovada.Copy.ExpiresAt.After(puesta.Copy.ExpiresAt) {
		t.Fatalf("renovar no conserva el inicio o no alarga: %+v %v", renovada.Copy, err)
	}
	if _, err := st.ReleasePQVaultCopy(alice, "drive", b); !errors.Is(err, domain.ErrPQVaultChanged) {
		t.Fatalf("retirar una marca que no está: %v", err)
	}
	if out, err := st.ReleasePQVaultCopy(alice, "drive", a); err != nil || out.Copy != nil {
		t.Fatalf("retirar: %+v %v", out, err)
	}
	if out, err := st.ReleasePQVaultCopy(alice, "drive", a); err != nil || out.Copy != nil {
		t.Fatalf("retirar dos veces: %+v %v", out, err)
	}
	// Cancelada, quien copiaba se para; los demás vuelven a publicar.
	if _, err := st.AppendPQVaultRecord(conA, "drive", vaciada.VaultID, "copia-2", payload(2)); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("siguió copiando con la marca cancelada: %v", err)
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", vaciada.VaultID, "web-1", payload(1)); err != nil {
		t.Fatal(err)
	}
	// Caducada: no impide nada, pero sigue a la vista y se puede tomar otra.
	if _, err := st.ClaimPQVaultCopy(alice, "drive", claim(a)); err != nil {
		t.Fatal(err)
	}
	if err := h.Pool.InTenantTx(tenantCtx("acme"), func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SELECT set_config('app.owner_sub','alice',true)`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE pq_vaults SET copy_expires_at=now()-interval '1 second'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	caducada, err := st.GetPQVault(alice, "drive")
	if err != nil || caducada.Copy == nil || caducada.Copy.Active {
		t.Fatalf("la marca caducada: %+v %v", caducada.Copy, err)
	}
	if _, err := st.AppendPQVaultRecord(alice, "drive", vaciada.VaultID, "web-2", payload(2)); err != nil {
		t.Fatalf("una marca caducada sigue bloqueando: %v", err)
	}
	if _, err := st.ClaimPQVaultCopy(alice, "drive", claim(b)); err != nil {
		t.Fatalf("no se pudo tomar una marca caducada: %v", err)
	}
	if _, err := st.AppendPQVaultRecord(conA, "drive", vaciada.VaultID, "copia-3", payload(3)); !errors.Is(err, domain.ErrPQVaultCopyInProgress) {
		t.Fatalf("la copia de la marca tomada por otro siguió: %v", err)
	}
	// Bob no ve ni toca la de Alice.
	bob := store.WithOwner(tenantCtx("acme"), "bob")
	if out, err := st.GetPQVault(bob, "drive"); err != nil || out.Copy != nil || out.VaultID != "" {
		t.Fatalf("RLS de la marca: %+v %v", out, err)
	}
}
