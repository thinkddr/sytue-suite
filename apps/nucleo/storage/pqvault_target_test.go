// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package storage

import (
	"strings"
	"testing"
)

func TestPQVaultTarget(t *testing.T) {
	key := strings.Repeat("ab", 32)
	got := PQVaultTarget("!pqv_X", key)
	// La web hace `pq-${keyId.slice(0, 24)}` (packages/offline/src/vault/pq-vault.ts).
	if got.RoomID != "!pqv_X" || got.DeviceID != "pq-"+key[:24] || !got.Valid() {
		t.Fatalf("destino = %+v", got)
	}
	if PQVaultTarget("!pqv_X", "corto").Valid() {
		t.Fatal("un identificador corto da un destino válido")
	}
}
