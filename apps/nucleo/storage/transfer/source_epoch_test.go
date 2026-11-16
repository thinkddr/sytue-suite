// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"context"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestInitialSourceEpochStableAcrossDevicesAndBoundToAllScopeFields(t *testing.T) {
	scope := storage.Scope{Tenant: "tenant", Subject: "subject", Product: "mail"}
	first, err := InitialSourceEpoch(scope)
	if err != nil || !storage.MigrationEpoch(first) {
		t.Fatal(first, err)
	}
	second, err := InitialSourceEpoch(scope)
	if err != nil || second != first {
		t.Fatal("fresh device minted a different first source epoch", second, err)
	}
	for _, field := range []string{"tenant", "subject", "product"} {
		changed := scope
		switch field {
		case "tenant":
			changed.Tenant = "other"
		case "subject":
			changed.Subject = "other"
		case "product":
			changed.Product = "calendar"
		}
		next, err := InitialSourceEpoch(changed)
		if err != nil || next == first {
			t.Fatal("source candidate crossed scope", field, next, err)
		}
	}
	if _, err := InitialSourceEpoch(storage.Scope{}); err == nil {
		t.Fatal("invalid scope gained a source candidate")
	}
}

func TestInitialSourceCandidateNeverReplacesAcceptedEpochOnFreshDevice(t *testing.T) {
	for range 2 {
		cfg := sourceBeginFixture(t)
		started := time.Now().UTC()
		accepted := "1e70b7c3-ef1b-4473-bfe8-507323fd8b70"
		cfg.Current = storage.LegacyMigrationState{Scope: cfg.Scope, Mode: "frozen", Epoch: accepted, StartedAt: &started}
		cfg.NewEpoch = func(context.Context) (string, error) {
			t.Fatal("fresh WAL discarded the accepted source epoch")
			return InitialSourceEpoch(cfg.Scope)
		}
		cfg.Begin = func(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error) {
			t.Fatal("fresh device recaptured an accepted source")
			return storage.LegacyMigrationState{}, ErrSource
		}
		state, intent, err := BeginSource(t.Context(), cfg)
		if err != nil || state.Epoch != accepted || intent.Epoch != accepted {
			t.Fatal("accepted source epoch was replaced", state, intent, err)
		}
	}
}
