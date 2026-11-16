// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// These encrypted rows are durable recovery data, never an SDK proof, an ACK,
// or permission to delete. StagedFile descriptors identify the plan's own
// encrypted copies; the borrowed Open callbacks are not recorded or retired.
type finalizingCapture struct {
	Version       int              `json:"v"`
	Batch         batchPublication `json:"batch"`
	MembersSHA256 string           `json:"members_sha256"`
}
type finalizingCaptureMember struct {
	Version      int               `json:"v"`
	BatchID      string            `json:"batch_id"`
	Ordinal      uint64            `json:"ordinal"`
	Input        inputMember       `json:"input"`
	Prepared     Member            `json:"prepared"`
	Reference    storage.Reference `json:"reference"`
	RecordSHA256 string            `json:"record_sha256"`
}

func finalizingCaptureKey(operation string) string {
	return "finalizing_" + atomicOperationKey(operation)
}

func finalizingCaptureMemberKey(operation string, ordinal uint64) string {
	return finalizingCaptureKey(operation) + "_member_" + strconv.FormatUint(ordinal, 10)
}

// CaptureFinalizationReceipt records a complete accepted batch in encrypted
// local history. It re-reads the SDK and all inputs before and after capture.
// This non-destructive stage never ACKs an upload, removes staged/original data,
// or authorizes a later cleanup from JSON. The caller supplies a live session
// lease and retains the same SDK/context; no publication target is re-created.
// A failed or cancelled call can still have persisted the final marker; retry
// re-reads and verifies it and never assumes that an error means absence.
//
//nolint:gocognit,gocyclo // one sequential capture/readback/verify of the final marker; splitting it would scatter the retry-safe ordering of security-critical steps
func CaptureFinalizationReceipt(ctx context.Context, plan InputPlan, records storage.RecordStore, files storage.BoundFiles, acquire offline.WorkLease) error {
	if acquire == nil || records == nil {
		return storage.ErrRecord
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	release, err := acquire(ctx)
	if err != nil {
		return err
	}
	if release == nil {
		return offline.ErrWorkLease
	}
	defer release()
	data, err := plan.current(ctx)
	if err != nil {
		return err
	}
	owner := atomicOwner(data.root.Scope)
	active := storage.AtomicJournalNamespace(data.root.Scope.Product)
	history := storage.AtomicHistoryNamespace(data.root.Scope.Product)
	if history == "" || history == active {
		return storage.ErrRecord
	}
	return data.journal.Exclusive(ctx, owner, active, inputBatchKey(data.root.Operation)+"_publish", func() error {
		if err := checkFinalizingInputs(ctx, plan); err != nil {
			return err
		}
		var root batchPublication
		found, err := data.journal.Read(owner, active, inputBatchKey(data.root.Operation), &root)
		if err != nil {
			return err
		}
		if !found || root.State != "accepted" || root.Root == nil {
			return storage.ErrRecord
		}
		if _, _, err := checkBatchPublication(ctx, plan, root); err != nil {
			return err
		}
		binding, err := records.StorageBindings(ctx)
		if err != nil {
			return err
		}
		if !binding.Valid() || binding.Scope != root.Scope {
			return storage.ErrCatalogSource
		}
		binding.Rooms = slices.Clone(binding.Rooms)
		snapshot, err := storage.LoadAtomicCatalog(ctx, records, root.Scope)
		if err != nil {
			return err
		}
		if !binding.Same(snapshot.Binding) {
			return storage.ErrCatalogSource
		}
		if _, err := verifyInputBatch(ctx, plan, root, records, files, snapshot, *root.Root); err != nil {
			return err
		}
		work, err := Load(ctx, data.journal, root.Scope, inputIndexOperation(root.PlanID))
		if err != nil {
			return err
		}
		if work.BatchID != root.BatchID || work.Count != root.Count || work.Target != root.Target || work.State != StateCaptured {
			return storage.ErrRecord
		}
		refs, err := collectPreparedBatchEntries(snapshot, root)
		if err != nil {
			return err
		}
		var prior finalizingCapture
		completed, err := data.journal.Read(owner, history, finalizingCaptureKey(root.Operation), &prior)
		if err != nil {
			return err
		}
		membersSHA256, err := captureFinalizingMembers(ctx, plan, data, owner, history, root, work, refs, completed)
		if err != nil {
			return err
		}
		if err := verifyFinalizingReceiptReadback(ctx, plan, root, records, files, binding); err != nil {
			return err
		}
		capture := finalizingCapture{Version: 1, Batch: root, MembersSHA256: membersSHA256}
		return writeFinalizingExact(ctx, data.journal, owner, history, finalizingCaptureKey(root.Operation), capture, !completed)
	})
}

func collectPreparedBatchEntries(snapshot storage.AtomicCatalogSnapshot, root batchPublication) (map[uint64]storage.RecordEntry, error) {
	refs := make(map[uint64]storage.RecordEntry, root.Count)
	for _, entry := range snapshot.Entries {
		value, _, err := storage.DecodeAtomicPrepared(entry, root.Scope)
		if err != nil || value.BatchID != root.BatchID {
			continue
		}
		if value.Ordinal >= root.Count {
			return nil, storage.ErrRecord
		}
		if _, duplicate := refs[value.Ordinal]; duplicate {
			return nil, storage.ErrRecord
		}
		refs[value.Ordinal] = entry
	}
	if uint64(len(refs)) != root.Count {
		return nil, storage.ErrIncompleteCatalog
	}
	return refs, nil
}

func captureFinalizingMembers(ctx context.Context, plan InputPlan, data *capturedInputPlan, owner, history string, root batchPublication, work Work, refs map[uint64]storage.RecordEntry, completed bool) (string, error) {
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for ordinal := uint64(0); ordinal < root.Count; ordinal++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		_, input, err := plan.readCapturedMember(ctx, ordinal)
		if err != nil {
			return "", err
		}
		prepared, err := ReadMember(ctx, data.journal, work, ordinal)
		if err != nil {
			return "", err
		}
		expected, err := prepared.PreparedRecord()
		if err != nil {
			return "", err
		}
		actual := refs[ordinal]
		if !expected.MatchesExact(actual.Record) {
			return "", storage.ErrRecord
		}
		digest, err := storage.AtomicRecordSHA256(actual.Record)
		if err != nil {
			return "", err
		}
		member := finalizingCaptureMember{Version: 1, BatchID: root.BatchID, Ordinal: ordinal, Input: input, Prepared: prepared, Reference: actual.Reference, RecordSHA256: digest}
		if err := writeFinalizingExact(ctx, data.journal, owner, history, finalizingCaptureMemberKey(root.Operation, ordinal), member, !completed); err != nil {
			return "", err
		}
		if err := encoder.Encode(member); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func verifyFinalizingReceiptReadback(ctx context.Context, plan InputPlan, root batchPublication, records storage.RecordStore, files storage.BoundFiles, binding storage.Binding) error {
	// The final marker is written only after fresh readback of the same root,
	// all original inputs and the current SDK binding; partial history is not
	// a completion receipt and may be retried only with the same exact rows.
	fresh, err := storage.LoadAtomicCatalog(ctx, records, root.Scope)
	if err != nil {
		return err
	}
	if !binding.Same(fresh.Binding) {
		return storage.ErrCatalogSource
	}
	if _, err := verifyInputBatch(ctx, plan, root, records, files, fresh, *root.Root); err != nil {
		return err
	}
	if err := checkFinalizingInputs(ctx, plan); err != nil {
		return err
	}
	return batchBindingGuard(ctx, records, binding)
}

func checkFinalizingInputs(ctx context.Context, plan InputPlan) error {
	data, err := plan.current(ctx)
	if err != nil {
		return err
	}
	checked, err := LoadInputs(ctx, data.journal, data.root.Scope, data.root.Operation)
	if err != nil {
		return err
	}
	if checked.data.root != data.root || len(checked.data.members) != len(data.members) {
		return storage.ErrRecord
	}
	for ordinal, hash := range data.members {
		if checked.data.members[ordinal] != hash {
			return storage.ErrRecord
		}
	}
	return ctx.Err()
}

func writeFinalizingExact(ctx context.Context, journal *offline.Store, owner, namespace, key string, value any, allowCreate bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	defer clear(raw)
	var existing json.RawMessage
	found, err := journal.Read(owner, namespace, key, &existing)
	defer clear(existing)
	if err != nil {
		return err
	}
	if found {
		if !bytes.Equal(existing, raw) {
			return storage.ErrRecord
		}
		return ctx.Err()
	}
	if !allowCreate {
		return storage.ErrRecord
	}
	if err := journal.Write(owner, namespace, key, json.RawMessage(raw)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var saved json.RawMessage
	found, err = journal.Read(owner, namespace, key, &saved)
	defer clear(saved)
	if err != nil {
		return err
	}
	if !found || !bytes.Equal(saved, raw) {
		return storage.ErrRecord
	}
	return ctx.Err()
}
