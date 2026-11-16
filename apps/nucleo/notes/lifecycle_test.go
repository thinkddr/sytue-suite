// SPDX-License-Identifier: Apache-2.0
package notes

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

func TestCompletedPrivateImportReconcilesOriginalJournalAfterDeviceHandoff(t *testing.T) {
	first, port, source := notesFixture(t)
	port.loseIndex = true
	if err := first.ImportLegacySnapshot(t.Context()); err == nil {
		t.Fatal("lost ACK did not interrupt")
	}
	partial, err := first.Pending(t.Context())
	if err != nil || partial.Count == 0 || partial.Scope != testScope {
		t.Fatal("interrupted import was not pending", partial, err)
	}
	first.Close()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := first.cfg
	cfg.Journal = journal
	port.binding.Target.DeviceID = "NEXT_DEVICE"
	next, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	opened := source.opens
	writes := port.writes
	old, err := New(first.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if err := old.Initialize(t.Context()); err != nil {
		t.Fatal("original journal did not reconcile accepted work", err)
	}
	pending, err := old.Pending(t.Context())
	if err != nil || pending.Count != 0 {
		t.Fatal("completed original journal remained pending", pending, err)
	}
	if source.opens != opened || port.writes != writes {
		t.Fatal("journal reconciliation reimported plaintext or republished")
	}
}

func TestPrivateRecordsDrainPreservesPendingAndRequiresExplicitResume(t *testing.T) {
	r, port, _ := notesFixture(t)
	port.loseIndex = true
	if err := r.ImportLegacySnapshot(t.Context()); err == nil {
		t.Fatal("lost ACK did not interrupt")
	}
	before, err := r.Pending(t.Context())
	if err != nil || before.Count == 0 {
		t.Fatal(err)
	}
	if err := r.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := r.Initialize(t.Context()); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("paused repository admitted IO", err)
	}
	after, err := r.Pending(t.Context())
	if err != nil || after != before {
		t.Fatal("drain discarded pending work", after, before, err)
	}
	if err := r.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := r.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal("resume did not retain publication", err)
	}
	after, err = r.Pending(t.Context())
	if err != nil || after.Count != 0 {
		t.Fatal("accepted import did not retire local receipts", after, err)
	}
}

func TestCompletedImportDoesNotDiscardMismatchedOriginalJournal(t *testing.T) {
	first, port, _ := notesFixture(t)
	port.loseIndex = true
	if err := first.ImportLegacySnapshot(t.Context()); err == nil {
		t.Fatal("lost ACK did not interrupt")
	}
	first.Close()
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := first.cfg
	cfg.Journal = journal
	next, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err := next.ImportLegacySnapshot(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows, err := first.cfg.Journal.List(first.engine.PrivateStorageScope().Tenant+"\x00alice\x00notes", "notes-private-v1")
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, row := range rows {
		if !strings.HasPrefix(row.ID, "import_") {
			continue
		}
		var value map[string]json.RawMessage
		if err := json.Unmarshal(row.Value, &value); err != nil {
			t.Fatal(err)
		}
		value["input"] = json.RawMessage(`"changed-unconfirmed-body"`)
		if err := first.cfg.Journal.Write("acme\x00alice\x00notes", "notes-private-v1", row.ID, value); err != nil {
			t.Fatal(err)
		}
		changed = true
		break
	}
	if !changed {
		t.Fatal("fixture did not retain publication")
	}
	original, err := New(first.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	if err := original.Initialize(t.Context()); !errors.Is(err, ErrSource) {
		t.Fatal("mismatched journal accepted", err)
	}
	pending, err := original.Pending(t.Context())
	if err != nil || pending.Count == 0 {
		t.Fatal("mismatched journal discarded", pending, err)
	}
	if _, err := original.ListNotebooks(t.Context()); !errors.Is(err, ErrNotReady) {
		t.Fatal("failed reconciliation exposed ready view", err)
	}
}
