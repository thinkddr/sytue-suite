// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

// ADR 0082: la clave se guarda envuelta ANTES de publicarla. Si la custodia
// falla, la cuenta sigue sin identidad (nada que nadie pueda volver a abrir), y el
// reintento custodia y publica la MISMA clave, que abre la cuenta.
func TestBootstrapRecoveryWrappedEscrowsBeforePublishing(t *testing.T) {
	h := &resetHomeserver{accountData: map[string]json.RawMessage{}, deviceKeys: map[string]json.RawMessage{}, bodies: map[string]json.RawMessage{}}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	c := newResetClient(t, server, "")
	ctx := t.Context()

	if err := c.BootstrapRecoveryWrapped(ctx, dummyUIA, nil); !errors.Is(err, ErrNoRecoveryEscrow) {
		t.Fatalf("sin custodia: %v", err)
	}
	refused := errors.New("custodia caída")
	var firstKey, firstID string
	if err := c.BootstrapRecoveryWrapped(ctx, dummyUIA, func(_ context.Context, keyID, key string) error {
		firstID, firstKey = keyID, key
		return refused
	}); !errors.Is(err, refused) {
		t.Fatalf("custodia fallida: %v", err)
	}
	h.mu.Lock()
	published := h.uiaDone || len(h.crossSigning) != 0
	h.mu.Unlock()
	if published || firstKey == "" || firstID == "" {
		t.Fatalf("se publicó una clave sin custodia (publicada=%v)", published)
	}
	var stored, storedID string
	if err := c.BootstrapRecoveryWrapped(ctx, dummyUIA, func(_ context.Context, keyID, key string) error {
		stored, storedID = key, keyID
		return nil
	}); err != nil {
		t.Fatalf("reintento: %v", err)
	}
	if stored != firstKey || storedID != firstID {
		t.Fatal("el reintento custodió otra clave")
	}
	if err := c.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if id, err := c.DefaultRecoveryKeyID(ctx); err != nil || id != storedID {
		t.Fatalf("DefaultRecoveryKeyID = %q, %v; quiero %q", id, err, storedID)
	}
	if err := c.Recover(ctx, stored); err != nil {
		t.Fatalf("la clave custodiada no abre la cuenta: %v", err)
	}
	if c.AccountUserID() != "@sytue_person:matrix.test" {
		t.Fatalf("AccountUserID = %q", c.AccountUserID())
	}
}

// Restablecer con custodia: si la custodia falla, la identidad vieja sigue en pie
// y su clave sigue abriendo; si funciona, la clave nueva no se devuelve y abre.
func TestResetRecoveryWrappedEscrowsBeforePublishing(t *testing.T) {
	h := &resetHomeserver{accountData: map[string]json.RawMessage{}, deviceKeys: map[string]json.RawMessage{}, bodies: map[string]json.RawMessage{}}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	c := newResetClient(t, server, "")
	ctx := t.Context()
	old, err := c.BootstrapRecovery(ctx, dummyUIA)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	oldMaster := string(h.crossSigning["master_key"])
	h.mu.Unlock()

	if err := c.ResetRecoveryWrapped(ctx, dummyUIA, func(context.Context, string, string) error { return errors.New("caída") }); err == nil {
		t.Fatal("restablecer sin custodia funcionó")
	}
	h.mu.Lock()
	unchanged := string(h.crossSigning["master_key"]) == oldMaster && len(h.deleted) == 0
	h.mu.Unlock()
	if !unchanged {
		t.Fatal("una custodia fallida tocó la identidad vieja")
	}
	if err := c.Recover(ctx, old); err != nil {
		t.Fatalf("la clave vieja ya no abre: %v", err)
	}
	var fresh string
	if err := c.ResetRecoveryWrapped(ctx, dummyUIA, func(_ context.Context, _, key string) error { fresh = key; return nil }); err != nil {
		t.Fatalf("restablecer: %v", err)
	}
	if fresh == "" || fresh == old {
		t.Fatal("no se custodió una clave nueva")
	}
	if err := c.Recover(ctx, fresh); err != nil {
		t.Fatalf("la clave nueva no abre: %v", err)
	}
}

func TestLocalRecoveryKeyOnlyForStorageAccounts(t *testing.T) {
	h := &resetHomeserver{accountData: map[string]json.RawMessage{}, deviceKeys: map[string]json.RawMessage{}, bodies: map[string]json.RawMessage{}}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	if _, _, err := newResetClient(t, server, "").LocalRecoveryKey(t.Context()); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("Rooms: %v", err)
	}
	other := &resetHomeserver{accountData: map[string]json.RawMessage{}, deviceKeys: map[string]json.RawMessage{}, bodies: map[string]json.RawMessage{}}
	storageServer := httptest.NewTLSServer(other)
	defer storageServer.Close()
	c := newResetClient(t, storageServer, "drive")
	var stored, storedID string
	if err := c.BootstrapRecoveryWrapped(t.Context(), dummyUIA, func(_ context.Context, keyID, key string) error {
		stored, storedID = key, keyID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	keyID, key, err := c.LocalRecoveryKey(t.Context())
	if err != nil || key != stored || keyID != storedID {
		t.Fatalf("LocalRecoveryKey = %q, %v", keyID, err)
	}
}
