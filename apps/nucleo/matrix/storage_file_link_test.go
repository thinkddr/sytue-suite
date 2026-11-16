// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func sidecarLink(t *testing.T, f storageFileIO, records *fileRecordFixture, request StorageFileRequest, root EventRef, change func(*storage.Record, *storage.FileLink)) EventRef {
	t.Helper()
	link := storage.FileLink{Kind: storage.FileLinkKind, Content: storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: root}}
	record := storage.Record{Version: 1, Scope: f.scope, ObjectID: request.ObjectID, RevisionID: request.RevisionID, Role: "content"}
	if change != nil {
		change(&record, &link)
	}
	record.Payload, _ = json.Marshal(link)
	ref, err := records.PutStorageRecord(t.Context(), "sidecar-link", record)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestStorageFileSidecarLinkACKKeepsWALUnlessExactScopeRevisionRootAndTarget(t *testing.T) {
	for _, change := range []string{"valid", "scope", "object", "role", "record revision", "file revision", "root", "missing", "room", "device", "unverified"} {
		t.Run(change, func(t *testing.T) {
			f, gateway, records, _ := fileFixture(t)
			body := []byte("private sidecar plan sentinel")
			request := fileRequest(int64(len(body)))
			root, err := f.upload(t.Context(), "sidecar_pending", request, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			ref := sidecarLink(t, f, records, request, root, func(record *storage.Record, link *storage.FileLink) {
				switch change {
				case "scope":
					record.Scope.Subject = "other"
				case "object":
					record.ObjectID = "other"
				case "role":
					record.Role = "index"
				case "record revision":
					record.RevisionID = "other"
				case "file revision":
					link.Content.RevisionID = "other"
				case "root":
					link.Content.Reference.EventID = "$other"
				}
			})
			switch change {
			case "missing":
				ref.EventID = "$missing"
			case "room":
				ref.RoomID = "!rotated:matrix.test"
			case "device":
				records.target.DeviceID = "OTHER"
			case "unverified":
				var state storageFileUpload
				if _, err := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "sidecar_pending", &state); err != nil {
					t.Fatal(err)
				}
				state.Verified = false
				if err := f.journal.Write(f.journalOwner(), storage.PendingUploadsNamespace, "sidecar_pending", state); err != nil {
					t.Fatal(err)
				}
			}
			err = f.acknowledgeLink(t.Context(), "sidecar_pending", ref)
			var remaining storageFileUpload
			found, readErr := f.journal.Read(f.journalOwner(), storage.PendingUploadsNamespace, "sidecar_pending", &remaining)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if change == "valid" {
				if err != nil || found {
					t.Fatal("valid retained link did not acknowledge WAL", err)
				}
			} else if err == nil || !found || remaining.Published == nil || *remaining.Published != root {
				t.Fatal("changed link/target discarded pending work", err)
			}
			if len(gateway.data) != 1 || len(records.records) != 2 {
				t.Fatal("sidecar ACK deleted source/provider/history")
			}
		})
	}
}

func TestStorageFileSidecarRetryDoesNotRequireOrCreateAnIndex(t *testing.T) {
	f, gateway, records, _ := fileFixture(t)
	body := []byte("source plan survives rejected CAS")
	request := fileRequest(int64(len(body)))
	root, err := f.upload(t.Context(), "sidecar_retry", request, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	ref := sidecarLink(t, f, records, request, root, nil)
	// A failed application CAS does not call ACK. Retry reuses the exact root.
	second, err := f.upload(t.Context(), "sidecar_retry", request, bytes.NewReader(body))
	if err != nil || second != root || gateway.puts != 1 {
		t.Fatal("CAS retry duplicated plan bytes", err)
	}
	if err := f.acknowledge(t.Context(), "sidecar_retry", ref); err == nil {
		t.Fatal("content sidecar was mistaken for an application index")
	}
	if err := f.acknowledgeLink(t.Context(), "sidecar_retry", ref); err != nil {
		t.Fatal(err)
	}
	if err := f.acknowledgeLink(t.Context(), "sidecar_retry", ref); !errors.Is(err, ErrStorageUploadMissing) {
		t.Fatal("absent WAL treated as proof", err)
	}
	for _, record := range records.records {
		if record.Role == "index" {
			t.Fatal("sidecar contaminated application catalog")
		}
	}
}
