// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PublishedInputBatch contains independent maintained-SDK readback, not a local
// ACK, caller-supplied ObjectVersion, or permission to retire pending work.
// Finalizing/cleanup must durably retain their own complete receipt first.
type PublishedInputBatch struct {
	Reference storage.Reference
	Proof     storage.VerifiedAtomicBatch
}

const inputBatchPageMembers = 128

func batchRecord(scope storage.Scope, batch, revision, role string, value any) (storage.Record, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return storage.Record{}, err
	}
	record := storage.Record{Version: 1, Scope: scope, ObjectID: storage.AtomicBatchObjectID(batch), RevisionID: revision, Role: role, Payload: payload}
	if record.Validate(scope) != nil {
		clear(payload)
		return storage.Record{}, storage.ErrRecord
	}
	return record, nil
}

func batchBindingGuard(ctx context.Context, records storage.RecordStore, original storage.Binding) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := records.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !original.Same(current) {
		return storage.ErrCatalogSource
	}
	return ctx.Err()
}

// verifyInputBatch binds the complete accepted SDK projection back to ALL exact
// original inputs. A valid unrelated root, changed parent or content checksum
// cannot acknowledge this operation. Historical reads need no writer A grant.
func verifyInputBatch(ctx context.Context, plan InputPlan, root batchPublication, records storage.RecordStore, files storage.BoundFiles, snapshot storage.AtomicCatalogSnapshot, reference storage.Reference) (PublishedInputBatch, error) {
	entry, ok := snapshot.Entries[reference]
	if !ok {
		return PublishedInputBatch{}, storage.ErrIncompleteCatalog
	}
	commit, err := storage.DecodeAtomicBatch(entry, root.Scope)
	if err != nil || commit.BatchID != root.BatchID || commit.Count != root.Count || reference.RoomID != root.Target.RoomID || entry.Record.RevisionID != inputTransaction(root.PlanID, 0, "root") {
		return PublishedInputBatch{}, storage.ErrRecord
	}
	proofs := make([]VerifiedInputContent, root.Count)
	prepared := make(map[uint64]storage.Record, root.Count)
	for _, entry := range snapshot.Entries {
		value, _, err := storage.DecodeAtomicPrepared(entry, root.Scope)
		if err != nil || value.BatchID != root.BatchID {
			continue
		}
		if value.Ordinal >= root.Count {
			return PublishedInputBatch{}, storage.ErrRecord
		}
		if _, duplicate := prepared[value.Ordinal]; duplicate {
			return PublishedInputBatch{}, storage.ErrRecord
		}
		proof, err := VerifyInputContent(ctx, plan, value.Ordinal, records, files, value.Commit.Content)
		if err != nil {
			return PublishedInputBatch{}, err
		}
		proofs[value.Ordinal] = proof
		prepared[value.Ordinal] = entry.Record
	}
	if uint64(len(prepared)) != root.Count {
		return PublishedInputBatch{}, storage.ErrIncompleteCatalog
	}
	work, err := captureVerifiedBatch(ctx, plan, proofs, root.BatchID)
	if err != nil {
		return PublishedInputBatch{}, err
	}
	for ordinal := uint64(0); ordinal < root.Count; ordinal++ {
		member, err := ReadMember(ctx, plan.data.journal, work, ordinal)
		if err != nil {
			return PublishedInputBatch{}, err
		}
		expected, err := member.PreparedRecord()
		if err != nil {
			return PublishedInputBatch{}, err
		}
		actual := prepared[ordinal]
		if !expected.MatchesExact(actual) {
			return PublishedInputBatch{}, storage.ErrRecord
		}
	}
	proof, err := storage.VerifyAtomicBatch(ctx, records, root.Scope, reference)
	if err != nil {
		return PublishedInputBatch{}, err
	}
	return PublishedInputBatch{Reference: reference, Proof: proof}, nil
}

func checkpointAcceptedBatch(ctx context.Context, plan InputPlan, root batchPublication, reference storage.Reference) error {
	data, current, err := checkBatchPublication(ctx, plan, root)
	if err != nil {
		return err
	}
	if !reference.Valid() || reference.RoomID != root.Target.RoomID || current.Root != nil && *current.Root != reference {
		return storage.ErrRecord
	}
	current.Root = &reference
	current.State = "accepted"
	if err := ctx.Err(); err != nil {
		return err
	}
	return data.journal.Write(atomicOwner(root.Scope), storage.AtomicJournalNamespace(root.Scope.Product), inputBatchKey(root.Operation), current)
}

// PublishInputBatch publishes prepared members and bounded hash-linked pages,
// then ONE visibility root. The original immutable target/transactions and
// admitted context are retained. On every retry, including a new current B,
// accepted-root lookup/readback happens BEFORE any writer check or Prepare.
// This stage never deletes input, file work, receipts or any pending journal.
//
//nolint:gocognit,gocyclo // ordered publication protocol (accepted-root readback BEFORE writer check/Prepare); kept in one function so that order stays auditable
func PublishInputBatch(ctx context.Context, plan InputPlan, records storage.RecordStore, files storage.BoundFiles) (PublishedInputBatch, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return PublishedInputBatch{}, err
	}
	if records == nil {
		return PublishedInputBatch{}, storage.ErrRecord
	}
	binding, err := records.StorageBindings(ctx)
	if err != nil {
		return PublishedInputBatch{}, err
	}
	if !binding.Valid() || binding.Scope != data.root.Scope || !slices.Contains(binding.Rooms, data.root.Target.RoomID) {
		return PublishedInputBatch{}, storage.ErrCatalogSource
	}
	binding.Rooms = slices.Clone(binding.Rooms)
	var result PublishedInputBatch
	err = data.journal.Exclusive(ctx, atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputBatchKey(data.root.Operation)+"_publish", func() error {
		root, err := captureBatchPublication(ctx, plan)
		if err != nil {
			return fmt.Errorf("atomic staging: %w", err)
		}
		snapshot, err := storage.LoadAtomicCatalog(ctx, records, root.Scope)
		if err != nil {
			return fmt.Errorf("atomic initial catalog: %w", err)
		}
		if err := batchBindingGuard(ctx, records, binding); err != nil {
			return err
		}
		if reference, found := snapshot.Batches[root.BatchID]; found {
			result, err = verifyInputBatch(ctx, plan, root, records, files, snapshot, reference)
			if err != nil {
				return fmt.Errorf("atomic accepted inputs: %w", err)
			}
			if err := batchBindingGuard(ctx, records, binding); err != nil {
				return err
			}
			return checkpointAcceptedBatch(ctx, plan, root, reference)
		}
		if root.State != StateStaging || root.Root != nil {
			return storage.ErrIncompleteCatalog
		}
		proofs, err := PublishInputContents(ctx, plan, records, files)
		if err != nil {
			return fmt.Errorf("atomic content: %w", err)
		}
		work, err := captureVerifiedBatch(ctx, plan, proofs, root.BatchID)
		if err != nil {
			return fmt.Errorf("atomic input readback: %w", err)
		}
		if err := batchBindingGuard(ctx, records, binding); err != nil {
			return err
		}
		if err := records.CheckStoragePublicationTarget(root.Target); err != nil {
			return err
		}
		preparedBinding, err := records.PrepareStorage(ctx, root.Scope)
		if err != nil {
			return err
		}
		if !binding.Same(preparedBinding) {
			return storage.ErrCatalogSource
		}
		if err := batchBindingGuard(ctx, records, binding); err != nil {
			return err
		}
		previous, err := publishBatchPages(ctx, plan, data, root, work, records, binding)
		if err != nil {
			return err
		}
		// Re-read ALL original content after the last page and immediately before
		// visibility. Prepared/page ACKs alone never attest current content or access.
		freshWork, err := captureVerifiedBatch(ctx, plan, proofs, root.BatchID)
		if err != nil {
			return fmt.Errorf("atomic input readback: %w", err)
		}
		if freshWork != work {
			return storage.ErrRecord
		}
		commit := storage.AtomicBatchCommit{Kind: storage.ObjectBatchKind, BatchID: root.BatchID, Count: root.Count, Last: *previous}
		record, err := batchRecord(root.Scope, root.BatchID, inputTransaction(root.PlanID, 0, "root"), "index", commit)
		if err != nil {
			return fmt.Errorf("atomic root record: %w", err)
		}
		if err := batchBindingGuard(ctx, records, binding); err != nil {
			return err
		}
		reference, err := publishBatchRecord(ctx, plan, root, records, "root", 0, record)
		if err != nil {
			return fmt.Errorf("atomic root: %w", err)
		}
		snapshot, err = storage.LoadAtomicCatalog(ctx, records, root.Scope)
		if err != nil {
			return fmt.Errorf("atomic catalog: %w", err)
		}
		result, err = verifyInputBatch(ctx, plan, root, records, files, snapshot, reference)
		if err != nil {
			return fmt.Errorf("atomic accepted inputs: %w", err)
		}
		if err := batchBindingGuard(ctx, records, binding); err != nil {
			return err
		}
		return checkpointAcceptedBatch(ctx, plan, root, reference)
	})
	if err != nil {
		return PublishedInputBatch{}, err
	}
	return result, nil
}

func publishBatchPages(ctx context.Context, plan InputPlan, data *capturedInputPlan, root batchPublication, work Work, records storage.RecordStore, binding storage.Binding) (*storage.AtomicPagePointer, error) {
	var previous *storage.AtomicPagePointer
	for start := uint64(0); start < root.Count; {
		var err error
		start, previous, err = publishBatchPage(ctx, plan, data, root, work, records, binding, start, previous)
		if err != nil {
			return nil, err
		}
	}
	if previous == nil {
		return nil, storage.ErrRecord
	}
	return previous, nil
}

func publishBatchPage(ctx context.Context, plan InputPlan, data *capturedInputPlan, root batchPublication, work Work, records storage.RecordStore, binding storage.Binding, start uint64, previous *storage.AtomicPagePointer) (uint64, *storage.AtomicPagePointer, error) {
	end := start
	page := storage.AtomicBatchPage{Kind: storage.ObjectBatchPageKind, BatchID: root.BatchID, Start: start, Previous: previous}
	for end < root.Count && len(page.Members) < inputBatchPageMembers {
		ordinal := end
		if err := batchBindingGuard(ctx, records, binding); err != nil {
			return 0, nil, err
		}
		member, err := ReadMember(ctx, data.journal, work, ordinal)
		if err != nil {
			return 0, nil, err
		}
		record, err := member.PreparedRecord()
		if err != nil {
			return 0, nil, err
		}
		reference, err := publishBatchRecord(ctx, plan, root, records, "prepared", ordinal, record)
		if err != nil {
			return 0, nil, fmt.Errorf("atomic prepared: %w", err)
		}
		sha, err := storage.AtomicRecordSHA256(record)
		if err != nil {
			return 0, nil, err
		}
		candidate := page
		candidate.Members = append(slices.Clone(page.Members), storage.AtomicMember{Reference: reference, ObjectID: record.ObjectID, RevisionID: record.RevisionID, SHA256: sha})
		// The actual Record/PDU byte limit determines page boundaries, including
		// long valid IDs. A member-count limit alone cannot guarantee it fits.
		if _, err := batchRecord(root.Scope, root.BatchID, inputTransaction(root.PlanID, start, "page"), "manifest", candidate); err != nil {
			if len(page.Members) == 0 {
				return 0, nil, err
			}
			break
		}
		page = candidate
		end++
	}
	record, err := batchRecord(root.Scope, root.BatchID, inputTransaction(root.PlanID, start, "page"), "manifest", page)
	if err != nil {
		return 0, nil, fmt.Errorf("atomic page record: %w", err)
	}
	reference, err := publishBatchRecord(ctx, plan, root, records, "page", start, record)
	if err != nil {
		return 0, nil, fmt.Errorf("atomic page: %w", err)
	}
	sha, err := storage.AtomicRecordSHA256(record)
	if err != nil {
		return 0, nil, err
	}
	return end, &storage.AtomicPagePointer{Reference: reference, RevisionID: record.RevisionID, SHA256: sha}, nil
}
