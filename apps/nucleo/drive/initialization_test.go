// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type preparationFixture struct {
	Crypto
	binding storage.Binding
	err     error
}

func (s preparationFixture) PrepareStorage(context.Context, storage.Scope) (storage.Binding, error) {
	return s.binding, s.err
}

type unopenedSource struct {
	Source
	calls int
}

func (s *unopenedSource) PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error) {
	s.calls++
	return storage.LegacyMigrationState{}, ErrSource
}

func TestInitializeRequiresVerifiedKeysAndMatchingScopeBeforeAnySourceEffect(t *testing.T) {
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	denied := errors.New("artificial device is not verified")
	foreign := storage.Binding{Scope: storage.Scope{Tenant: "other", Subject: "alice", Product: "drive"}, Target: storage.PublicationTarget{RoomID: "!private:test", DeviceID: "DEVICE"}, Rooms: []string{"!private:test"}}
	for _, preparation := range []preparationFixture{{err: denied}, {binding: foreign}} {
		source := &unopenedSource{}
		repository, err := New(Config{Context: t.Context(), Scope: readerScope, Crypto: preparation, Source: source, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Journal: journal})
		if err != nil {
			t.Fatal(err)
		}
		err = repository.Initialize(t.Context())
		repository.Close()
		if err == nil || source.calls != 0 {
			t.Fatal("source accessed before verified scoped keys", source.calls, err)
		}
	}
}

func TestRefreshRequiresCompleteInitializationAndRejectsClosedRepository(t *testing.T) {
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	source := &unopenedSource{}
	// Neither SDK nor source has usable methods: a premature access would fail.
	repository, err := New(Config{Context: t.Context(), Scope: readerScope, Crypto: preparationFixture{}, Source: source, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Journal: journal})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Refresh(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("refresh exposed partial initialization", err)
	}
	repository.Close()
	if err := repository.Refresh(t.Context()); !errors.Is(err, ErrClosed) || source.calls != 0 {
		t.Fatal("closed refresh accessed retained originals", err, source.calls)
	}
}

type failedRefresh struct {
	Crypto
	binding storage.Binding
	err     error
	calls   int
}

func (f *failedRefresh) StorageBindings(context.Context) (storage.Binding, error) {
	return f.binding, nil
}

func (f *failedRefresh) StorageObjectHistory(context.Context, string, string, int) (storage.ObjectPage, error) {
	f.calls++
	if f.calls == 1 {
		return storage.ObjectPage{Next: "next"}, nil
	}
	return storage.ObjectPage{}, f.err
}

func TestFailedRefreshRetainsLastPrivateProjection(t *testing.T) {
	r := queryFixture(t, 1)
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	r.cfg.Journal = journal
	failure := errors.New("artificial interrupted catalog page")
	probe := &failedRefresh{Crypto: r.cfg.Crypto, err: failure, binding: storage.Binding{Scope: r.cfg.Scope, Target: storage.PublicationTarget{RoomID: "!private:test", DeviceID: "DEVICE"}, Rooms: []string{"!private:test"}}}
	r.cfg.Crypto = probe
	if err := r.Refresh(t.Context()); !errors.Is(err, failure) || probe.calls != 2 {
		t.Fatal("refresh did not report its incomplete history", probe.calls, err)
	}
	page, err := r.Search(t.Context(), "reunion")
	if err != nil || len(page.Files) != 1 || page.Files[0].ID != "file_001" {
		t.Fatal("failed refresh destroyed last authenticated projection", page, err)
	}
}
