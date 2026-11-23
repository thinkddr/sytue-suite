// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

func TestReciboDeBoveda(t *testing.T) {
	key := strings.Repeat("ab", 32)
	vault := "!pqv_" + strings.Repeat("A", 26)
	event := "$pqe_" + strings.Repeat("B", 26)
	st := &fakePQVaultStore{
		state:   store.PQVaultState{VaultID: vault, ActiveKeyID: key},
		records: []store.PQVaultRecord{{EventID: event, VaultID: vault}},
	}
	svc := &Service{pqVault: st}
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	ctx := store.WithOwner(context.Background(), "alice")
	target := storage.PQVaultTarget(vault, key)
	receipt := storage.Reference{RoomID: vault, EventID: event}
	if err := svc.VerifyPQVaultReceipt(ctx, scope, target, receipt); err != nil {
		t.Fatalf("recibo bueno: %v", err)
	}
	otroDispositivo := target
	otroDispositivo.DeviceID = "pq-" + strings.Repeat("cd", 12)
	otraBoveda := storage.PQVaultTarget("!pqv_"+strings.Repeat("C", 26), key)
	casos := map[string]struct {
		ctx     context.Context
		target  storage.PublicationTarget
		receipt storage.Reference
		want    error
	}{
		"registro que no está":    {ctx, target, storage.Reference{RoomID: vault, EventID: "$pqe_" + strings.Repeat("Z", 26)}, domain.ErrNotFound},
		"otro dispositivo":        {ctx, otroDispositivo, receipt, domain.ErrNotFound},
		"bóveda que ya no es":     {ctx, otraBoveda, storage.Reference{RoomID: otraBoveda.RoomID, EventID: event}, domain.ErrNotFound},
		"recibo de otra sala":     {ctx, target, storage.Reference{RoomID: "!otra:matrix", EventID: event}, domain.ErrInvalidName},
		"evento de Matrix":        {ctx, target, storage.Reference{RoomID: vault, EventID: "$abc:matrix"}, domain.ErrInvalidName},
		"propietario que no casa": {store.WithOwner(context.Background(), "bob"), target, receipt, domain.ErrPermission},
	}
	for nombre, c := range casos {
		if err := svc.VerifyPQVaultReceipt(c.ctx, scope, c.target, c.receipt); !errors.Is(err, c.want) {
			t.Errorf("%s: %v; quiero %v", nombre, err, c.want)
		}
	}
}
