// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pqvault

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestVaultIsACompleteCatalogStore(t *testing.T) {
	_, cfg := newFake(t, testScope)
	vault, err := Open(t.Context(), withKey(cfg, unlockKey(1)))
	if err != nil {
		t.Fatal(err)
	}
	defer vault.Close()
	ctx, target := t.Context(), vault.PublicationTarget()
	if _, err := vault.PrepareStorage(ctx, storage.Scope{Tenant: "acme", Subject: "otra", Product: "contacts"}); !errors.Is(err, ErrIdentity) {
		t.Fatal("la bóveda de una persona no se prepara para otra", err)
	}
	if _, err := vault.PrepareStorage(ctx, testScope); err != nil {
		t.Fatal(err)
	}
	publish := func(revision, txn string, parents ...storage.Reference) storage.ObjectVersion {
		t.Helper()
		content, err := vault.PutStorageRecordAt(ctx, target, txn+"_c", record(testScope, "contact_1", revision, []byte(`{"n":"`+revision+`"}`)))
		if err != nil {
			t.Fatal(err)
		}
		commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: content}, Parents: parents}
		version, err := vault.PutStorageObjectAt(ctx, target, txn, "contact_1", revision, commit)
		if err != nil {
			t.Fatal(err)
		}
		return version
	}
	first := publish("rev_1", "txn_1")
	second := publish("rev_2", "txn_2", first.Reference)
	catalog, err := storage.LoadCatalog(ctx, vault, testScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Heads) != 1 || catalog.Heads[0].Reference != second.Reference || len(catalog.Entries) != 2 {
		t.Fatalf("el catálogo no deja como cabeza la última revisión: %+v", catalog.Heads)
	}
	if got, err := vault.StorageObject(ctx, first.Reference, "contact_1"); err != nil || got.RevisionID != "rev_1" {
		t.Fatal("una revisión anterior sigue legible", err)
	}

	content, err := vault.PutStorageRecordAt(ctx, target, "txn_3_c", record(testScope, "contact_1", "rev_3", []byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	for name, commit := range map[string]storage.ObjectCommit{
		"otra revisión":     {Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: "rev_9", Reference: content}},
		"fichero":           {Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "file", RevisionID: "rev_3", Reference: content}},
		"padre de otro":     {Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: "rev_3", Reference: content}, Parents: []storage.Reference{content}},
		"misma revisión 2x": {Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: "rev_3", Reference: content}, Parents: []storage.Reference{second.Reference}},
	} {
		revision := "rev_3"
		if name == "misma revisión 2x" {
			revision = "rev_2"
		}
		if _, err := vault.PutStorageObjectAt(ctx, target, "txn_bad", "contact_1", revision, commit); err == nil {
			t.Errorf("%s: se publicó una revisión que no cuadra", name)
		}
	}
	other := storage.PublicationTarget{RoomID: "!otra", DeviceID: target.DeviceID}
	if _, err := vault.PutStorageRecordAt(ctx, other, "txn_x", record(testScope, "contact_1", "rev_4", []byte(`{}`))); !errors.Is(err, ErrIdentity) {
		t.Fatal("una intención de otra bóveda no se redirige a esta", err)
	}
	if _, err := vault.StorageObjectHistory(ctx, "!otra", "", 10); !errors.Is(err, ErrIdentity) {
		t.Fatal("el historial de otra sala no es de esta bóveda", err)
	}
	vault.Close()
	if _, err := vault.StorageBindings(ctx); !errors.Is(err, ErrClosed) {
		t.Fatal("cerrada, no enruta nada", err)
	}
}
