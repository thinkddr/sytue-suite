// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageFileLostRootACKKeepsOriginalPublicationAfterRoomRotation(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	original := []byte("private immutable source")
	request := fileRequest(int64(len(original)))
	records.loseRoot = true
	if _, err := f.upload(t.Context(), "publication", request, bytes.NewReader(original)); err == nil {
		t.Fatal("missing root ACK accepted")
	}
	records.target.RoomID = "!rotated:matrix.test"
	ref, err := f.upload(t.Context(), "publication", request, bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	if ref.RoomID != "!private:matrix.test" || len(records.records) != 1 || gateway.puts != 1 {
		t.Fatal("room rotation duplicated a publication", ref.RoomID, len(records.records), gateway.puts)
	}
}

func TestStorageFilePendingTargetChangesFailBeforeProviderEffects(t *testing.T) {
	for _, change := range []string{"device", "removed-room"} {
		t.Run(change, func(t *testing.T) {
			f, gateway, records, _ := fileFixture(t)
			original := []byte("private immutable source")
			request := fileRequest(int64(len(original)))
			gateway.loseNext = true
			if _, err := f.upload(t.Context(), "publication", request, bytes.NewReader(original)); err == nil {
				t.Fatal("missing blob ACK accepted")
			}
			if change == "device" {
				records.target.DeviceID = "ANOTHER"
			} else {
				records.target.RoomID, records.allowed = "!rotated:matrix.test", []string{"!rotated:matrix.test"}
			}
			if _, err := f.upload(t.Context(), "publication", request, bytes.NewReader(original)); !errors.Is(err, ErrIdentity) {
				t.Fatal("changed target accepted", err)
			}
			if len(records.records) != 0 || gateway.puts != 1 {
				t.Fatal("target rejection happened after a provider effect", len(records.records), gateway.puts)
			}
		})
	}
}

func TestStorageFileLegacyJournalIsPreservedWithoutGuessingItsTarget(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	original := []byte("retained private original")
	request := fileRequest(int64(len(original)))
	gateway.loseNext = true
	if _, err := f.upload(t.Context(), "legacy", request, bytes.NewReader(original)); err == nil {
		t.Fatal("missing blob ACK accepted")
	}
	var state storageFileUpload
	if found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "legacy", &state); !found || err != nil {
		t.Fatal("missing interrupted journal", err)
	}
	state.Version, state.Target = 1, storage.PublicationTarget{}
	if err := f.journal.Write(f.journalOwner(), storage.PendingUploadsNamespace, "legacy", state); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(state)
	if _, err := f.upload(t.Context(), "legacy", request, bytes.NewReader(original)); !errors.Is(err, ErrStoragePublicationMissing) {
		t.Fatal("unbound journal was guessed", err)
	}
	var retained storageFileUpload
	if found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "legacy", &retained); !found || err != nil {
		t.Fatal("legacy journal discarded", err)
	}
	after, _ := json.Marshal(retained)
	if !bytes.Equal(before, after) || gateway.puts != 1 || len(records.records) != 0 || string(original) != "retained private original" {
		t.Fatal("legacy retry changed retained state or original")
	}
}

func TestStoragePublicationTargetGuardRunsBeforeKeysOrNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := newClient(storageConfig(t, server, "drive"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	client.session.DeviceID = "DEVICE"
	target, err := client.StoragePublicationTarget()
	if err != nil || target.RoomID != "!drive:matrix.test" || target.DeviceID != "DEVICE" {
		t.Fatal("wrong target snapshot", target, err)
	}
	next := client.cfg.Bootstrap
	next.StorageRoomID, next.StorageRoomIDs = "!rotated:matrix.test", []string{"!drive:matrix.test", "!rotated:matrix.test"}
	if err := client.UpdateBindings(next); err != nil {
		t.Fatal(err)
	}
	if err := client.CheckStoragePublicationTarget(target); err != nil {
		t.Fatal("retained old room rejected", err)
	}
	for _, changed := range []storage.PublicationTarget{{RoomID: target.RoomID, DeviceID: "ANOTHER"}, {RoomID: "!foreign:matrix.test", DeviceID: target.DeviceID}, {RoomID: target.RoomID, DeviceID: ""}} {
		_, err := client.PutStorageRecordAt(context.Background(), changed, "txn", storage.Record{})
		if !errors.Is(err, ErrIdentity) || calls.Load() != 0 {
			t.Fatal("changed endpoint reached keys or network", err, calls.Load())
		}
		_, err = client.PutStorageObjectAt(context.Background(), changed, "txn", "object", "revision", storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{}`)})
		if !errors.Is(err, ErrIdentity) || calls.Load() != 0 {
			t.Fatal("changed catalog endpoint reached keys or network", err, calls.Load())
		}
	}
}
