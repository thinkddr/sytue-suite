// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// These are encrypted replay checkpoints, not accepted SDK proofs. Missing ACK
// leaves the original record/transaction intact; a different writer cannot send it.
type batchRecordReceipt struct {
	Version      int                       `json:"v"`
	Scope        storage.Scope             `json:"scope"`
	BatchID      string                    `json:"batch_id"`
	Kind         string                    `json:"kind"`
	Ordinal      uint64                    `json:"ordinal"`
	Target       storage.PublicationTarget `json:"target"`
	RecordSHA256 string                    `json:"record_sha256"`
	Reference    *storage.Reference        `json:"reference,omitempty"`
}

func inputBatchRecordKey(operation, kind string, ordinal uint64) string {
	return inputBatchKey(operation) + "_record_" + kind + "_" + strconv.FormatUint(ordinal, 10)
}

// expected was derived from the opaque plan's complete member stream at entry.
// Recheck the immutable root in O(1) per record; ALL original members are checked
// again by the complete SDK batch verifier before a root is accepted.
func checkBatchPublication(ctx context.Context, plan InputPlan, expected batchPublication) (*capturedInputPlan, batchPublication, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return nil, batchPublication{}, err
	}
	if !atomicValidHash(expected.MemberSHA256) || !expected.matches(data, expected.MemberSHA256) {
		return nil, batchPublication{}, storage.ErrRecord
	}
	var current batchPublication
	found, err := data.journal.Read(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputBatchKey(data.root.Operation), &current)
	if err != nil {
		return nil, batchPublication{}, err
	}
	if !found || !current.Captured || !current.matches(data, expected.MemberSHA256) || current.BatchID != expected.BatchID {
		return nil, batchPublication{}, storage.ErrRecord
	}
	return data, current, ctx.Err()
}

func readExactBatchRecord(ctx context.Context, records storage.RecordStore, scope storage.Scope, reference storage.Reference, expected storage.Record) error {
	if !reference.Valid() || expected.Validate(scope) != nil {
		return storage.ErrRecord
	}
	wanted, err := storage.AtomicRecordSHA256(expected)
	if err != nil {
		return err
	}
	actual, err := records.ReadStorageRecord(ctx, reference, expected.ObjectID, expected.Role)
	defer clear(actual.Payload)
	if err != nil {
		return err
	}
	digest, err := storage.AtomicRecordSHA256(actual)
	if err != nil || actual.Validate(scope) != nil || digest != wanted {
		return storage.ErrRecord
	}
	return ctx.Err()
}

func publishBatchRecord(ctx context.Context, plan InputPlan, root batchPublication, records storage.RecordStore, kind string, ordinal uint64, record storage.Record) (storage.Reference, error) {
	if records == nil || kind != "prepared" && kind != "page" && kind != "root" || record.Validate(root.Scope) != nil {
		return storage.Reference{}, storage.ErrRecord
	}
	data, current, err := checkBatchPublication(ctx, plan, root)
	if err != nil {
		return storage.Reference{}, err
	}
	sha, err := storage.AtomicRecordSHA256(record)
	if err != nil {
		return storage.Reference{}, err
	}
	owner, namespace := atomicOwner(root.Scope), storage.AtomicJournalNamespace(root.Scope.Product)
	key := inputBatchRecordKey(root.Operation, kind, ordinal)
	receipt, err := loadOrCreateBatchRecordReceipt(data, current, root, owner, namespace, key, kind, ordinal, sha)
	if err != nil {
		return storage.Reference{}, err
	}
	if receipt.Reference != nil {
		if err := verifyExistingBatchRecordReceipt(ctx, plan, root, records, record, *receipt.Reference); err != nil {
			return storage.Reference{}, err
		}
		return *receipt.Reference, nil
	}
	// Read-only recovery above never calls Prepare or requires producer A live.
	// Publication below ALWAYS retains A's original SDK policy and exact target.
	_, current, err = checkBatchPublication(ctx, plan, root)
	if err != nil {
		return storage.Reference{}, err
	}
	if current.State != StateStaging {
		return storage.Reference{}, storage.ErrRecord
	}
	if err := records.CheckStoragePublicationTarget(root.Target); err != nil {
		return storage.Reference{}, err
	}
	ref, err := records.PutStorageRecordAt(ctx, root.Target, inputTransaction(root.PlanID, ordinal, kind), record)
	if err != nil {
		return storage.Reference{}, err
	}
	if err := verifyExistingBatchRecordReceipt(ctx, plan, root, records, record, ref); err != nil {
		return storage.Reference{}, err
	}
	receipt.Reference = &ref
	if err := data.journal.Write(owner, namespace, key, receipt); err != nil {
		return storage.Reference{}, err
	}
	return ref, nil
}

func loadOrCreateBatchRecordReceipt(data *capturedInputPlan, current, root batchPublication, owner, namespace, key, kind string, ordinal uint64, sha string) (batchRecordReceipt, error) {
	var receipt batchRecordReceipt
	found, err := data.journal.Read(owner, namespace, key, &receipt)
	if err != nil {
		return batchRecordReceipt{}, err
	}
	expected := batchRecordReceipt{Version: 1, Scope: root.Scope, BatchID: root.BatchID, Kind: kind, Ordinal: ordinal, Target: root.Target, RecordSHA256: sha}
	if !found {
		if current.State != StateStaging {
			return batchRecordReceipt{}, storage.ErrRecord
		}
		// The actual original target is immutable. This local row precedes all SDK IO.
		receipt = expected
		if err := data.journal.Write(owner, namespace, key, receipt); err != nil {
			return batchRecordReceipt{}, err
		}
	}
	originalRef := receipt.Reference
	receipt.Reference = nil
	if receipt != expected {
		return batchRecordReceipt{}, storage.ErrRecord
	}
	receipt.Reference = originalRef
	return receipt, nil
}

func verifyExistingBatchRecordReceipt(ctx context.Context, plan InputPlan, root batchPublication, records storage.RecordStore, record storage.Record, ref storage.Reference) error {
	if ref.RoomID != root.Target.RoomID {
		return storage.ErrRecord
	}
	if err := readExactBatchRecord(ctx, records, root.Scope, ref, record); err != nil {
		return err
	}
	_, _, err := checkBatchPublication(ctx, plan, root)
	return err
}
