// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type historicalIdentityProbe struct {
	Crypto
	version      storage.IdentityVersion
	binding      storage.Binding
	preparations int
	writeChecks  int
}

func (p *historicalIdentityProbe) StorageIdentityVersion() storage.IdentityVersion { return p.version }

func (p *historicalIdentityProbe) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	p.preparations++
	if scope != p.binding.Scope {
		return storage.Binding{}, ErrSource
	}
	return p.binding, nil
}

func (p *historicalIdentityProbe) StorageBindings(context.Context) (storage.Binding, error) {
	return p.binding, nil
}

func (p *historicalIdentityProbe) StorageObjectHistory(context.Context, string, string, int) (storage.ObjectPage, error) {
	return storage.ObjectPage{}, nil
}

func (p *historicalIdentityProbe) CheckStoragePublicationTarget(storage.PublicationTarget) error {
	p.writeChecks++
	return ErrSource
}

func (p *historicalIdentityProbe) FrozenStorageBindings(context.Context) (storage.FrozenBinding, error) {
	return storage.FrozenBinding{}, ErrSource
}

func TestHistoricalSourceCannotBecomeAnOrdinaryRepositoryOrInferEmptyFrozenContent(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "contacts"}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	crypto := &historicalIdentityProbe{version: storage.LegacyIdentity, binding: storage.Binding{Scope: scope, Target: storage.PublicationTarget{RoomID: "!legacy:test", DeviceID: "LEGACY"}, Rooms: []string{"!legacy:test"}}}
	source := &epochSource{state: SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}}
	leases := 0
	cfg := Config{
		Context: t.Context(), Scope: scope, Product: scope.Product, EntityKind: "contact.entity", ReceiptKind: "contact.receipt", Crypto: crypto, Gateway: api.NewDrive("https://gateway.test", nil, nil), Journal: journal, Source: source,
		WorkLease: func(ctx context.Context) (func(), error) { leases++; return func() {}, ctx.Err() }, ValidateRow: func(context.Context, string, string, []byte) error { return nil }, ValidateCatalog: func(context.Context, Catalog) error { return nil },
	}
	if _, err := New(cfg); !errors.Is(err, ErrSource) {
		t.Fatal("legacy SDK reached ordinary app facade", err)
	}
	withoutLease := cfg
	withoutLease.WorkLease = nil
	if _, err := NewHistoricalSource(withoutLease); !errors.Is(err, ErrSource) {
		t.Fatal("historical source lacked original work witness", err)
	}
	wrongAuthority := cfg
	wrongAuthority.BindPublicationContext = func(ctx context.Context) (context.Context, error) {
		t.Fatal("isolated binder ran for legacy source")
		return ctx, nil
	}
	if _, err := NewHistoricalSource(wrongAuthority); !errors.Is(err, ErrSource) {
		t.Fatal("historical source adopted isolated publication binder", err)
	}
	historical, err := NewHistoricalSource(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer historical.Close()
	state := storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "frozen", Epoch: uuid.NewString(), SourceRooms: []string{"!legacy:test"}}
	if err := historical.Prepare(t.Context(), state); !errors.Is(err, ErrNotReady) {
		t.Fatal("missing receipt became an empty migration", err)
	}
	if source.begins != 0 || source.captures != 0 || leases != 1 || crypto.preparations != 1 || crypto.writeChecks != 0 {
		t.Fatal("frozen source initiated an import or bypassed witness", source.begins, source.captures, leases, crypto.preparations)
	}
	crypto.version = storage.IsolatedIdentity
	if _, err := NewHistoricalSource(cfg); !errors.Is(err, ErrSource) {
		t.Fatal("historical capability accepted isolated SDK", err)
	}
}
