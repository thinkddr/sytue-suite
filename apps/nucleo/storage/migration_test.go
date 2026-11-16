// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package storage

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestLegacyMigrationFramesKeepExactBytesAndUTF8Order(t *testing.T) {
	scope := Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	epoch := "12345678-1234-1234-1234-123456789abc"
	now := time.Now()
	state := LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &now, Rows: 2, Blobs: 1}
	if err := state.Validate(scope); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"integer":900719925474099512345}`)
	page := LegacySourcePage{Scope: scope, Epoch: epoch, Rows: []LegacySourceRow{{Kind: "provenance", ID: "\ue000", Payload: raw}, {Kind: "provenance", ID: "𐀀", Payload: raw}}}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var recovered LegacySourcePage
	if err := json.Unmarshal(encoded, &recovered); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Validate(scope, epoch); err != nil || !bytes.Equal(recovered.Rows[0].Payload, raw) {
		t.Fatal("domain source changed", err)
	}
	recovered.Rows[0], recovered.Rows[1] = recovered.Rows[1], recovered.Rows[0]
	if err := recovered.Validate(scope, epoch); err == nil {
		t.Fatal("out-of-order frame accepted")
	}
	for _, bad := range []string{"", "00000000-0000-0000-0000-000000000000", "12345678-1234-1234-1234-123456789ABC", epoch + "x"} {
		if MigrationEpoch(bad) {
			t.Fatal("invalid epoch", bad)
		}
	}
	for _, change := range []func(*LegacyMigrationState){func(s *LegacyMigrationState) { s.Scope.Subject = "bob" }, func(s *LegacyMigrationState) { s.Mode = "building" }, func(s *LegacyMigrationState) { s.Rows = 1 << 53 }, func(s *LegacyMigrationState) { s.Blobs = 3 }, func(s *LegacyMigrationState) { s.StartedAt = nil }, func(s *LegacyMigrationState) { s.Mode = "legacy" }} {
		invalid := state
		change(&invalid)
		if err := invalid.Validate(scope); err == nil {
			t.Fatal("invalid source state accepted", invalid)
		}
	}
}
