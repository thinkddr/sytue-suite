// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"encoding/json"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateMigrationReceiptRequiresCompleteIsolatedCatalog(t *testing.T) {
	f := newCatalogTransferFixture(t)
	if _, err := f.source.PrivateMigrationReceipt(t.Context()); !errors.Is(err, ErrSource) {
		t.Fatal("legacy identity yielded a cutover receipt", err)
	}
	if _, err := f.target.PrivateMigrationReceipt(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("empty destination yielded receipt", err)
	}
	if err := f.migrate(t); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.target.PrivateMigrationReceipt(t.Context())
	if err != nil || !receipt.Valid() || receipt.RoomID != f.next.binding.Target.RoomID {
		t.Fatal("isolated receipt", receipt, err)
	}
	clone, err := New(f.target.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer clone.Close()
	if _, err := clone.PrivateMigrationReceipt(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("uninitialized repository yielded receipt", err)
	}
	if err = f.target.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	again, err := f.target.PrivateMigrationReceipt(t.Context())
	if err != nil || again != receipt {
		t.Fatal("receipt changed on reopen", again, err)
	}
	record := f.next.records[receipt]
	var commit storage.ObjectCommit
	if err = json.Unmarshal(record.Payload, &commit); err != nil {
		t.Fatal(err)
	}
	if commit.Content == nil {
		t.Fatal("missing receipt content")
	}
	saved := f.next.records[commit.Content.Reference]
	altered := saved
	altered.Payload = []byte(`{"tampered":true}`)
	f.next.records[commit.Content.Reference] = altered
	if _, err = f.target.PrivateMigrationReceipt(t.Context()); err == nil {
		t.Fatal("unauthenticated receipt body accepted")
	}
	f.next.records[commit.Content.Reference] = saved
	commit.State = "deleted"
	record.Payload, err = json.Marshal(commit)
	if err != nil {
		t.Fatal(err)
	}
	f.next.records[receipt] = record
	if _, err = f.target.PrivateMigrationReceipt(t.Context()); err == nil {
		t.Fatal("removed completion yielded a receipt")
	}
	f.assertNoPlaintextSource(t)
}
