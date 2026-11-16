// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageFileCompletedReceiptSurvivesCrashBeforeCatalogCommit(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	source := []byte("retained immutable source")
	request := fileRequest(int64(len(source)))
	ref, err := f.upload(t.Context(), "awaiting_catalog", request, bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	var state storageFileUpload
	found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "awaiting_catalog", &state)
	if err != nil || !found || state.Published == nil || *state.Published != ref || !state.Verified {
		t.Fatal("completed receipt was retired before the catalog committed", err)
	}
	recordsBefore, putsBefore := len(records.records), gateway.puts
	after, err := f.upload(t.Context(), "awaiting_catalog", request, bytes.NewReader(source))
	if err != nil || after != ref || len(records.records) != recordsBefore || gateway.puts != putsBefore {
		t.Fatal("a crash between upload and catalog duplicated completed data", err)
	}
}

func completedCatalog(t *testing.T, f storageFileIO, records *fileRecordFixture, request StorageFileRequest, ref EventRef, change func(*storage.Record, *storage.ObjectCommit)) EventRef {
	t.Helper()
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"integer":9007199254740993}`), Content: &storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: ref}}
	record := storage.Record{Version: 1, Scope: f.scope, ObjectID: request.ObjectID, RevisionID: "catalog", Role: "index"}
	if change != nil {
		change(&record, &commit)
	}
	var err error
	record.Payload, err = json.Marshal(commit)
	if err != nil {
		t.Fatal(err)
	}
	result, err := records.PutStorageRecord(t.Context(), "catalog", record)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStorageFileReceiptRetiresOnlyAfterExactAuthenticatedCatalogReference(t *testing.T) {
	for _, change := range []string{"scope", "object", "role", "kind", "revision", "reference", "deleted", "missing", "valid"} {
		t.Run(change, func(t *testing.T) {
			f, gateway, records, _ := fileFixture(t)
			original := []byte("immutable private original")
			request := fileRequest(int64(len(original)))
			ref, err := f.upload(t.Context(), "catalog_pending", request, bytes.NewReader(original))
			if err != nil {
				t.Fatal(err)
			}
			catalog := completedCatalog(t, f, records, request, ref, func(record *storage.Record, commit *storage.ObjectCommit) {
				switch change {
				case "scope":
					record.Scope.Subject = "other"
				case "object":
					record.ObjectID = "other"
				case "role":
					record.Role = "manifest"
				case "kind":
					commit.Content.Kind = "record"
				case "revision":
					commit.Content.RevisionID = "old"
				case "reference":
					commit.Content.Reference.EventID = "$foreign"
				case "deleted":
					commit.State = "deleted"
					commit.Content = nil
				}
			})
			if change == "missing" {
				catalog.EventID = "$missing"
			}
			err = f.acknowledge(t.Context(), "catalog_pending", catalog)
			var retained storageFileUpload
			found, readErr := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "catalog_pending", &retained)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if change != "valid" {
				if err == nil || !found || retained.Published == nil || *retained.Published != ref {
					t.Fatal("uncommitted or changed catalog retired a verified receipt", change, err)
				}
			} else {
				if err != nil || found {
					t.Fatal("exact catalog did not release completed journal", err)
				}
				if err := f.acknowledge(t.Context(), "catalog_pending", catalog); !errors.Is(err, ErrStorageUploadMissing) {
					t.Fatal("missing journal was treated as proof of application completion", err)
				}
			}
			if gateway.puts != 1 || len(gateway.data) != 1 || len(records.records) != 2 {
				t.Fatal("acknowledgement altered remote files or history")
			}
		})
	}
}

func TestStorageFileUnverifiedCopyCannotBeAcknowledgedByCatalog(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	source := []byte("immutable original retained during failure")
	request := fileRequest(int64(len(source)))
	gateway.tamper = true
	if _, err := f.upload(t.Context(), "unverified", request, bytes.NewReader(source)); err == nil {
		t.Fatal("unverified replacement succeeded")
	}
	var state storageFileUpload
	if found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "unverified", &state); err != nil || !found || state.Published == nil || state.Verified {
		t.Fatal("failed copy marked verified", err)
	}
	catalog := completedCatalog(t, f, records, request, *state.Published, nil)
	if err := f.acknowledge(t.Context(), "unverified", catalog); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("catalog hid failed replacement verification", err)
	}
	gateway.tamper = false
	if _, err := f.upload(t.Context(), "unverified", request, bytes.NewReader(source)); err != nil {
		t.Fatal(err)
	}
	if err := f.acknowledge(t.Context(), "unverified", catalog); err != nil {
		t.Fatal("verified retry did not confirm retained catalog", err)
	}
}

func TestStorageFileCatalogACKAfterDeviceClosesRetainsJournal(t *testing.T) {
	f, _, records, _ := fileFixture(t)
	original := []byte("immutable private source")
	request := fileRequest(int64(len(original)))
	ref, err := f.upload(t.Context(), "closing_device", request, bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	catalog := completedCatalog(t, f, records, request, ref, nil)
	closed := false
	f.closed = func() bool { return closed }
	f.records = &changedRecordReply{fileRecordFixture: records, change: func(*storage.Record) { closed = true }}
	if err := f.acknowledge(t.Context(), "closing_device", catalog); !errors.Is(err, ErrClosed) {
		t.Fatal("closed device acknowledged pending file", err)
	}
	var state storageFileUpload
	if found, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "closing_device", &state); err != nil || !found || state.Published == nil || *state.Published != ref {
		t.Fatal("late catalog read erased a closed device's journal", err)
	}
}
