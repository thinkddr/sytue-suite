// SPDX-License-Identifier: Apache-2.0
package storagetest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func snapshotFixture() MailSnapshot {
	started := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	value := MailSnapshot{State: storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: storage.Scope{Tenant: "snapshot", Subject: "owner", Product: "mail"}, Mode: "frozen", Epoch: "6c778bf7-5dcc-4c5d-925b-545c5b814d1f", StartedAt: &started, Rows: 2, Blobs: 1}}}
	add := func(kind, id string, body []byte) {
		sum := sha256.Sum256(body)
		value.Rows = append(value.Rows, MailSnapshotRow{Descriptor: storage.StreamSourceRow{Kind: kind, ID: id, Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}, Body: body})
	}
	add("mime_message", "message", []byte("Subject: Artificial\r\n\r\nOriginal \x00 MIME"))
	add("mailbox", "inbox", []byte(`{"id":"inbox","role":"inbox"}`))
	add("message", "message", []byte(`{"id":"message","mailbox_id":"inbox","future":900719925474099512345}`))
	return value
}

func TestMailSnapshotKeepsCompleteOriginalsAndStableBoundedPages(t *testing.T) {
	original := snapshotFixture()
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	value, err := DecodeMailSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	after := storage.LegacySourceCursor{}
	count := 0
	for {
		page, err := value.Page(after, 1)
		if err != nil || page.Validate(value.State.Scope, value.State.Epoch, after) != nil {
			t.Fatal("export page violated its source commitments", page, err)
		}
		for _, item := range page.Rows {
			row, ok := value.Body(item.Kind, item.ID)
			if !ok || row.Descriptor != item {
				t.Fatal("page lost the original", item)
			}
			count++
		}
		if page.Next == nil {
			break
		}
		after = *page.Next
	}
	if count != len(original.Rows) {
		t.Fatal("bounded pages omitted retained originals", count)
	}
	for _, limit := range []int{0, storage.MaxLegacySourceRows + 1} {
		if _, err := value.Page(storage.LegacySourceCursor{}, limit); err == nil {
			t.Fatal("invalid page bound was accepted")
		}
	}
	if _, err := value.Page(storage.LegacySourceCursor{ID: "missing-kind"}, 1); err == nil {
		t.Fatal("malformed cursor was interpreted as a new inventory")
	}
}

func TestMailSnapshotRejectsChangedBytesDuplicateRowsAndInventedCounts(t *testing.T) {
	for _, fault := range []string{"bytes", "duplicate", "rows", "blobs", "scope", "epoch"} {
		t.Run(fault, func(t *testing.T) {
			value := snapshotFixture()
			switch fault {
			case "bytes":
				value.Rows[0].Body[0] ^= 1
			case "duplicate":
				value.Rows = append(value.Rows, value.Rows[0])
				value.State.Blobs++
			case "rows":
				value.State.Rows++
			case "blobs":
				value.State.Blobs++
			case "scope":
				value.State.Scope.Product = "calendar"
			case "epoch":
				value.State.Epoch = ""
			}
			if value.Validate() == nil {
				t.Fatal("invalid snapshot became a complete source", fault)
			}
		})
	}
}
