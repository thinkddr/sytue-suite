// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func sourceBeginFixture(t *testing.T) SourceBeginConfig {
	t.Helper()
	scope := storage.Scope{Tenant: "tenant", Subject: "subject", Product: "mail"}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	target := storage.PublicationTarget{RoomID: "!mail:fixture", DeviceID: "ORIGINAL"}
	return SourceBeginConfig{Scope: scope, Owner: "tenant\x00subject\x00mail", Namespace: "source-test", IntentKey: "snapshot", Journal: journal, Current: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}, Target: target, NewEpoch: func(context.Context) (string, error) { return "d4af959e-a5e6-48e0-a090-0800c6c4d913", nil }, CheckTarget: func(actual storage.PublicationTarget) error {
		if actual != target {
			return ErrSource
		}
		return nil
	}, Begin: func(_ context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
		started := time.Now().UTC()
		return storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &started}, nil
	}}
}

func TestSourceBeginRetainsOriginalWireIntentBeforeEffectsAndAfterLostACK(t *testing.T) {
	cfg := sourceBeginFixture(t)
	begin := cfg.Begin
	ackLost := errors.New("source begin ACK lost")
	var accepted storage.LegacyMigrationState
	calls := 0
	cfg.Begin = func(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
		calls++
		var retained SourceBeginIntent
		found, err := cfg.Journal.Read(cfg.Owner, cfg.Namespace, cfg.IntentKey, &retained)
		if err != nil || !found || retained.Epoch != epoch || retained.Target != cfg.Target {
			t.Fatal("source effect preceded its retained original intent", found, retained, err)
		}
		accepted, err = begin(ctx, scope, epoch)
		if err != nil {
			t.Fatal(err)
		}
		return storage.LegacyMigrationState{}, ackLost
	}
	_, first, err := BeginSource(t.Context(), cfg)
	if !errors.Is(err, ackLost) || calls != 1 {
		t.Fatal(first, err, calls)
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	const expected = `{"v":1,"scope":{"tenant":"tenant","subject":"subject","product":"mail"},"epoch":"d4af959e-a5e6-48e0-a090-0800c6c4d913","target":{"room_id":"!mail:fixture","device_id":"ORIGINAL"}}`
	if string(raw) != expected {
		t.Fatal("snapshot WAL wire changed", string(raw))
	}
	cfg.Current = accepted
	cfg.NewEpoch = func(context.Context) (string, error) { t.Fatal("retry minted another epoch"); return "", ErrSource }
	cfg.Begin = func(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error) {
		t.Fatal("accepted begin was repeated")
		return storage.LegacyMigrationState{}, ErrSource
	}
	state, retried, err := BeginSource(t.Context(), cfg)
	if err != nil || retried != first || state.Epoch != first.Epoch {
		t.Fatal("retry changed its retained source epoch", state, retried, err)
	}
}

func TestSourceBeginRejectsChangedEpochAndOriginalDeviceWithoutReplacingWAL(t *testing.T) {
	for _, fault := range []string{"source-epoch", "target", "bad-response"} {
		t.Run(fault, func(t *testing.T) {
			cfg := sourceBeginFixture(t)
			if fault == "bad-response" {
				cfg.Begin = func(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error) {
					return cfg.Current, nil
				}
				if _, _, err := BeginSource(t.Context(), cfg); !errors.Is(err, ErrSource) {
					t.Fatal("legacy response accepted", err)
				}
				return
			}
			state, original, err := BeginSource(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Current = state
			if fault == "source-epoch" {
				cfg.Current.Epoch = "3cfdd001-b1b3-4dfb-aa04-31dfb93a00ba"
			} else {
				cfg.Target.DeviceID = "REPLACEMENT"
			}
			if _, _, err := BeginSource(t.Context(), cfg); !errors.Is(err, ErrSource) {
				t.Fatal("changed source/device accepted", err)
			}
			var retained SourceBeginIntent
			found, err := cfg.Journal.Read(cfg.Owner, cfg.Namespace, cfg.IntentKey, &retained)
			if err != nil || !found || retained != original {
				t.Fatal("rejected retry replaced original intent", retained, err)
			}
		})
	}
}
