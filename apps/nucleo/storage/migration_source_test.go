// SPDX-License-Identifier: Apache-2.0
package storage

import (
	"testing"
	"time"
)

func TestSameRetainedSourceRequiresTheCompleteValidatedSnapshot(t *testing.T) {
	started := time.Date(2026, 11, 2, 12, 0, 0, 123456789, time.UTC)
	original := LegacyMigrationState{Scope: Scope{Tenant: "tenant", Subject: "owner", Product: "drive"}, Mode: "frozen", Epoch: "12345678-1234-1234-1234-123456789abc", StartedAt: &started, Rows: 2, Blobs: 1}
	otherZone := started.In(time.FixedZone("fixture", 3600))
	same := original
	same.StartedAt = &otherZone
	if !original.SameRetainedSource(same) {
		t.Fatal("same retained instant was treated as another SQL source")
	}
	for _, fault := range []string{"tenant", "subject", "product", "mode", "epoch", "time", "rows", "blobs", "nil-start", "legacy", "invalid-count"} {
		t.Run(fault, func(t *testing.T) {
			bad := original
			switch fault {
			case "tenant":
				bad.Scope.Tenant = "other"
			case "subject":
				bad.Scope.Subject = "other"
			case "product":
				bad.Scope.Product = "mail"
			case "mode":
				bad.Mode = "private"
			case "epoch":
				bad.Epoch = "22345678-1234-1234-1234-123456789abc"
			case "time":
				changed := started.Add(time.Nanosecond)
				bad.StartedAt = &changed
			case "rows":
				bad.Rows++
			case "blobs":
				bad.Blobs = 0
			case "nil-start":
				bad.StartedAt = nil
			case "legacy":
				bad = LegacyMigrationState{Scope: original.Scope, Mode: "legacy"}
			case "invalid-count":
				bad.Rows = 1 << 53
			}
			if original.SameRetainedSource(bad) || bad.SameRetainedSource(original) {
				t.Fatal("changed or invalid retained SQL source compared equal", fault)
			}
		})
	}
	if (LegacyMigrationState{Scope: original.Scope, Mode: "legacy"}).SameRetainedSource(LegacyMigrationState{Scope: original.Scope, Mode: "legacy"}) {
		t.Fatal("empty legacy routing became an authenticated retained source")
	}
}
