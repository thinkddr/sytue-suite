// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/recoverywrap"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type fakePQVaultStore struct {
	state   store.PQVaultState
	records []store.PQVaultRecord
}

func (f *fakePQVaultStore) GetPQVault(context.Context, string) (store.PQVaultState, error) {
	return f.state, nil
}

func (f *fakePQVaultStore) PutPQVaultKey(_ context.Context, _ string, w recoverywrap.PQVaultWrapper, _, _ bool) (store.PQVaultState, error) {
	f.state = store.PQVaultState{
		VaultID:     "!pqv_" + strings.Repeat("A", 26),
		ActiveKeyID: w.KeyID,
		Keys:        []recoverywrap.PQVaultWrapper{w},
	}
	return f.state, nil
}

func (f *fakePQVaultStore) AppendPQVaultRecord(_ context.Context, _, vaultID, _ string, payload json.RawMessage) (store.PQVaultRecord, error) {
	rec := store.PQVaultRecord{
		EventID: "$pqe_" + strings.Repeat("B", 26),
		VaultID: vaultID,
		Seq:     int64(len(f.records) + 1),
		Payload: payload,
	}
	f.records = append(f.records, rec)
	return rec, nil
}

func (f *fakePQVaultStore) GetPQVaultRecord(_ context.Context, _, _, eventID string) (store.PQVaultRecord, error) {
	for _, r := range f.records {
		if r.EventID == eventID {
			return r, nil
		}
	}
	return store.PQVaultRecord{}, domain.ErrNotFound
}

func (f *fakePQVaultStore) ListPQVaultRecords(_ context.Context, _, _ string, _ int64, _ int) (store.PQVaultRecordPage, error) {
	return store.PQVaultRecordPage{Entries: f.records}, nil
}

func TestPQVaultServiceValidation(t *testing.T) {
	st := &fakePQVaultStore{}
	svc := &Service{pqVault: st}
	ctx := store.WithOwner(context.Background(), "alice")

	// Producto inválido.
	if _, err := svc.GetPQVault(ctx, "desconocido"); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("producto inválido: got %v want ErrInvalidName", err)
	}
	// Envoltorio válido.
	auk, err := recoverywrap.NewUnlockKey()
	if err != nil {
		t.Fatal(err)
	}
	id, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	w, err := recoverywrap.WrapPQVault(auk, "acme", "drive", "alice", recoverywrap.PQVaultSecret{
		V:        recoverywrap.PQVaultVersion,
		Profile:  id.Profile(),
		Identity: id.String(),
		OwnerKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := svc.PutPQVaultKey(ctx, "drive", w, true, false)
	if err != nil || state.ActiveKeyID != w.KeyID {
		t.Fatalf("PutPQVaultKey: state=%+v err=%v", state, err)
	}

	// Rechaza registros en claro sin `org.sytue.storage_pq`.
	if _, err := svc.AppendPQVaultRecord(ctx, "drive", state.VaultID, "tx-1", json.RawMessage(`{"plain":"no"}`)); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("registro en claro: got %v want ErrInvalidName", err)
	}
	// Acepta registro cifrado PQ.
	rec, err := svc.AppendPQVaultRecord(ctx, "drive", state.VaultID, "tx-1", json.RawMessage(`{"org.sytue.storage_pq":{"v":1}}`))
	if err != nil || rec.EventID == "" {
		t.Fatalf("AppendPQVaultRecord: rec=%+v err=%v", rec, err)
	}
}
