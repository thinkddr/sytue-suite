//go:build !js

// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestAtomicBatchJournalRetainsAllOriginalIntentsBeforeAnyPublication(t *testing.T) {
	plan, _, _, _ := inputReadbackFixture(t)
	root, err := captureBatchPublication(t.Context(), plan)
	if err != nil || root.State != StateStaging || !root.Captured || root.Root != nil || root.Target != plan.data.root.Target {
		t.Fatal("publication staging root missing before any SDK call", err)
	}
	for ordinal := uint64(0); ordinal < root.Count; ordinal++ {
		var actual batchInputIntent
		found, err := plan.data.journal.Read(atomicOwner(root.Scope), storage.AtomicJournalNamespace(root.Scope.Product), inputBatchMemberKey(root.Operation, ordinal), &actual)
		if err != nil || !found || actual != root.intent(ordinal, plan.data.members[ordinal]) {
			t.Fatal("all original input hashes were not retained", ordinal, err)
		}
	}
	again, err := captureBatchPublication(t.Context(), plan)
	if err != nil || again.BatchID != root.BatchID || again.MemberSHA256 != root.MemberSHA256 || again.Target != root.Target {
		t.Fatal("retry changed original batch or target", err)
	}
}

func TestAtomicBatchJournalCapturedMissingIntentNeverRepairsOrErasesRoot(t *testing.T) {
	plan, _, _, _ := inputReadbackFixture(t)
	root, err := captureBatchPublication(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	owner, namespace := atomicOwner(root.Scope), storage.AtomicJournalNamespace(root.Scope.Product)
	if err := plan.data.journal.Delete(owner, namespace, inputBatchMemberKey(root.Operation, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := captureBatchPublication(t.Context(), plan); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("captured missing intent repaired", err)
	}
	var retained batchPublication
	found, err := plan.data.journal.Read(owner, namespace, inputBatchKey(root.Operation), &retained)
	if err != nil || !found || retained.BatchID != root.BatchID || !retained.Captured {
		t.Fatal("incomplete batch lost its retained pending root", err)
	}
}

func TestAtomicBatchJournalInterruptedIntentCaptureRetainsBatchID(t *testing.T) {
	plan, _, _, _ := inputReadbackFixture(t)
	root, err := captureBatchPublication(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	owner, namespace := atomicOwner(root.Scope), storage.AtomicJournalNamespace(root.Scope.Product)
	root.Captured = false
	if err := plan.data.journal.Write(owner, namespace, inputBatchKey(root.Operation), root); err != nil {
		t.Fatal(err)
	}
	if err := plan.data.journal.Delete(owner, namespace, inputBatchMemberKey(root.Operation, 1)); err != nil {
		t.Fatal(err)
	}
	again, err := captureBatchPublication(t.Context(), plan)
	if err != nil || !again.Captured || again.BatchID != root.BatchID || again.Target != root.Target {
		t.Fatal("interrupted capture retargeted or lost original batch", err)
	}
}

func TestAtomicBatchJournalUsesExistingVerifiedCaptureAndRejectsForeignBatch(t *testing.T) {
	plan, reader, files, pointers := inputReadbackFixture(t)
	proofs := make([]VerifiedInputContent, len(pointers))
	for i, pointer := range pointers {
		var err error
		proofs[i], err = VerifyInputContent(t.Context(), plan, uint64(i), reader, files, pointer)
		if err != nil {
			t.Fatal(err)
		}
	}
	work, err := CaptureVerified(t.Context(), plan, proofs)
	if err != nil {
		t.Fatal(err)
	}
	root, err := captureBatchPublication(t.Context(), plan)
	if err != nil || root.BatchID != work.BatchID {
		t.Fatal("existing independently verified capture got a replacement batch", err)
	}
	if _, err := captureVerifiedBatch(t.Context(), plan, proofs, "foreign_batch"); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("same index operation retargeted to a new batch", err)
	}
	same, err := captureVerifiedBatch(t.Context(), plan, proofs, root.BatchID)
	if err != nil || same != work {
		t.Fatal("original captured batch could not resume", err)
	}
}
