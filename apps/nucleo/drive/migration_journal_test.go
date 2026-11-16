// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"errors"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestPrivateDriveRetiresOnlyFulfilledBeginIntents(t *testing.T) {
	f := newCatalogTransferFixture(t)
	key := "begin_" + digest([]byte(f.old.binding.Target.DeviceID))
	work := beginWork{Scope: readerScope, Epoch: readerEpoch, Target: f.old.binding.Target}
	if err := f.journal.Write(f.source.owner(), migrationNamespace, key, work); err != nil {
		t.Fatal(err)
	}
	if err := f.source.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	var retained beginWork
	if found, err := f.journal.Read(f.source.owner(), migrationNamespace, key, &retained); err != nil || found {
		t.Fatal("completed begin remained an unsent operation", err)
	}
	if err := f.journal.Write(f.source.owner(), migrationNamespace, key, work); err != nil {
		t.Fatal(err)
	}
	for _, version := range f.versions {
		if version.ObjectID == "drive_migration_"+digest([]byte(readerEpoch)) {
			delete(f.old.records, version.Reference)
		}
	}
	if err := f.source.Initialize(t.Context()); err == nil {
		t.Fatal("missing authenticated receipt allowed reconciliation")
	}
	if found, err := f.journal.Read(f.source.owner(), migrationNamespace, key, &retained); err != nil || !found {
		t.Fatal("failed initialization erased its intent", err)
	}
}

func TestPrivateDriveArchivesTransferMapOnlyAfterMatchingIsolatedAcknowledgment(t *testing.T) {
	f := newCatalogTransferFixture(t)
	if err := f.migrate(t); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.target.PrivateMigrationReceipt(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids, err := f.journal.RecordIDs(f.target.owner(), identityMigrationNamespace)
	if err != nil || len(ids) < 2 {
		t.Fatal("transfer map fixture missing", err)
	}
	state := storage.IdentityCutoverState{Version: 1, Scope: readerScope, Phase: "frozen", Epoch: readerEpoch, SourceRooms: slices.Clone(f.old.binding.Rooms)}
	if err = f.target.AcknowledgeIdentityCutover(t.Context(), state); !errors.Is(err, ErrSource) {
		t.Fatal("unacknowledged routing retired transfer work", err)
	}
	state.Phase, state.Receipt = "isolated", &receipt
	forged := storage.Reference{RoomID: receipt.RoomID, EventID: "$wrong-receipt"}
	state.Receipt = &forged
	if err = f.target.AcknowledgeIdentityCutover(t.Context(), state); !errors.Is(err, ErrSource) {
		t.Fatal("different backend receipt retired transfer work", err)
	}
	state.Receipt = &receipt
	if pending, err := f.target.Pending(t.Context()); err != nil || pending.Count != 1 {
		t.Fatal("unacknowledged transfer is not pending", pending, err)
	}
	var plan identityMigration
	if found, err := f.journal.Read(f.target.owner(), identityMigrationNamespace, "plan", &plan); err != nil || !found {
		t.Fatal("missing completed transfer plan", err)
	}
	plan.Complete = false
	if err = f.journal.Write(f.target.owner(), identityMigrationNamespace, "plan", plan); err != nil {
		t.Fatal(err)
	}
	if err = f.target.AcknowledgeIdentityCutover(t.Context(), state); !errors.Is(err, ErrSource) {
		t.Fatal("incomplete local transfer map was retired", err)
	}
	plan.Complete = true
	if err = f.journal.Write(f.target.owner(), identityMigrationNamespace, "plan", plan); err != nil {
		t.Fatal(err)
	}
	if err = f.target.AcknowledgeIdentityCutover(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if pending, err := f.target.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("acknowledged transfer still blocks logout", pending, err)
	}
	for _, id := range ids {
		var raw any
		if found, err := f.journal.Read(f.target.owner(), identityHistoryNamespace, id, &raw); err != nil || !found {
			t.Fatal("completed transfer audit was lost", id, err)
		}
	}
	if err = f.target.AcknowledgeIdentityCutover(t.Context(), state); err != nil {
		t.Fatal("acknowledgment retry is not idempotent", err)
	}
	f.target.Close()
	if pending, err := f.target.Pending(t.Context()); err != nil || pending.Count != 0 {
		t.Fatal("close changed durable completion", pending, err)
	}
	f.assertNoPlaintextSource(t)
}
