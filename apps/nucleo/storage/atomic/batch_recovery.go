// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// CheckFinalizationReceipt revalidates an encrypted retained receipt against a
// fresh SDK catalog without loading the original InputPlan or staged bytes. A
// nil error is only a readback observation: it grants no ACK, deletion, writer
// authority or future availability guarantee. The original plan-owned sources
// and all journals are untouched. A live caller-owned session lease is required.
//
//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func CheckFinalizationReceipt(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string, records storage.RecordStore, files storage.BoundFiles, acquire offline.WorkLease) error {
	if !journal.Encrypted() || !scope.Valid() || !storage.Identifier(operation) || records == nil || acquire == nil {
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
	owner := atomicOwner(scope)
	active := storage.AtomicJournalNamespace(scope.Product)
	history := storage.AtomicHistoryNamespace(scope.Product)
	if history == "" || history == active {
		return storage.ErrRecord
	}
	return journal.Exclusive(ctx, owner, active, inputBatchKey(operation)+"_publish", func() error {
		capture, members, err := readFinalizingHistory(ctx, journal, scope, operation)
		if err != nil {
			return err
		}
		binding, err := records.StorageBindings(ctx)
		if err != nil {
			return err
		}
		if !binding.Valid() || binding.Scope != scope || !slices.Contains(binding.Rooms, capture.Batch.Target.RoomID) {
			return storage.ErrCatalogSource
		}
		binding.Rooms = slices.Clone(binding.Rooms)
		ref, rootSHA, err := verifyFinalizationCatalogSnapshot(ctx, scope, records, files, binding, capture, members)
		if err != nil {
			return err
		}
		// Rebuild the complete SDK proof after content reads, including the root,
		// every page and member. A cached or serialized proof never suffices.
		if _, err := storage.VerifyAtomicBatch(ctx, records, scope, ref); err != nil {
			return err
		}
		if err := verifyFinalizationCatalogUnchanged(ctx, scope, records, binding, ref, capture.Batch.BatchID, rootSHA, members); err != nil {
			return err
		}
		again, _, err := readFinalizingHistory(ctx, journal, scope, operation)
		if err != nil {
			return err
		}
		if err := sameFinalizingCapture(capture, again); err != nil {
			return err
		}
		return batchBindingGuard(ctx, records, binding)
	})
}

func verifyFinalizationCatalogSnapshot(ctx context.Context, scope storage.Scope, records storage.RecordStore, files storage.BoundFiles, binding storage.Binding, capture finalizingCapture, members []finalizingCaptureMember) (storage.Reference, string, error) {
	snapshot, err := storage.LoadAtomicCatalog(ctx, records, scope)
	if err != nil {
		return storage.Reference{}, "", err
	}
	if !binding.Same(snapshot.Binding) {
		return storage.Reference{}, "", storage.ErrCatalogSource
	}
	ref := *capture.Batch.Root
	entry, found := snapshot.Entries[ref]
	if !found {
		return storage.Reference{}, "", storage.ErrIncompleteCatalog
	}
	root, err := storage.DecodeAtomicBatch(entry, scope)
	if err != nil || root.BatchID != capture.Batch.BatchID || root.Count != capture.Batch.Count || snapshot.Batches[root.BatchID] != ref || entry.Record.RevisionID != inputTransaction(capture.Batch.PlanID, 0, "root") {
		return storage.Reference{}, "", storage.ErrRecord
	}
	rootSHA, err := storage.AtomicRecordSHA256(entry.Record)
	if err != nil {
		return storage.Reference{}, "", err
	}
	proof, err := storage.VerifyAtomicBatch(ctx, records, scope, ref)
	if err != nil {
		return storage.Reference{}, "", err
	}
	for _, member := range members {
		if err := verifyRecoveredMember(ctx, scope, records, files, proof, snapshot, member); err != nil {
			return storage.Reference{}, "", err
		}
	}
	if err := batchBindingGuard(ctx, records, binding); err != nil {
		return storage.Reference{}, "", err
	}
	return ref, rootSHA, nil
}

func verifyFinalizationCatalogUnchanged(ctx context.Context, scope storage.Scope, records storage.RecordStore, binding storage.Binding, ref storage.Reference, batchID, rootSHA string, members []finalizingCaptureMember) error {
	lastCatalog, err := storage.LoadAtomicCatalog(ctx, records, scope)
	if err != nil {
		return err
	}
	if !binding.Same(lastCatalog.Binding) {
		return storage.ErrCatalogSource
	}
	lastRoot, found := lastCatalog.Entries[ref]
	if !found {
		return storage.ErrIncompleteCatalog
	}
	lastRootSHA, err := storage.AtomicRecordSHA256(lastRoot.Record)
	if err != nil || lastRootSHA != rootSHA || lastCatalog.Batches[batchID] != ref {
		return storage.ErrRecord
	}
	for _, member := range members {
		last, found := lastCatalog.Entries[member.Reference]
		if !found {
			return storage.ErrIncompleteCatalog
		}
		digest, err := storage.AtomicRecordSHA256(last.Record)
		if err != nil || digest != member.RecordSHA256 {
			return storage.ErrRecord
		}
	}
	return batchBindingGuard(ctx, records, binding)
}

func sameFinalizingCapture(a, b finalizingCapture) error {
	firstRaw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	defer clear(firstRaw)
	lastRaw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	defer clear(lastRaw)
	if !bytes.Equal(firstRaw, lastRaw) {
		return storage.ErrRecord
	}
	return nil
}

func readFinalizingHistory(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string) (finalizingCapture, []finalizingCaptureMember, error) {
	var capture finalizingCapture
	owner, history := atomicOwner(scope), storage.AtomicHistoryNamespace(scope.Product)
	found, err := journal.Read(owner, history, finalizingCaptureKey(operation), &capture)
	if err != nil {
		return finalizingCapture{}, nil, err
	}
	if !validFinalizingCaptureHeader(found, capture, scope, operation) {
		return finalizingCapture{}, nil, storage.ErrRecord
	}
	root := capture.Batch
	input := inputRoot{Version: 1, Scope: scope, Target: root.Target, Operation: operation, ID: root.PlanID, Fingerprint: root.InputFingerprint, Count: root.Count, State: StateCaptured}
	if !input.valid(scope, operation) {
		return finalizingCapture{}, nil, storage.ErrRecord
	}
	all, original, staged, mutations := sha256.New(), sha256.New(), sha256.New(), sha256.New()
	allEncoder, inputEncoder, stagedEncoder, mutationEncoder := json.NewEncoder(all), json.NewEncoder(original), json.NewEncoder(staged), json.NewEncoder(mutations)
	members := make([]finalizingCaptureMember, 0, root.Count)
	objects := map[string]bool{}
	refs := map[storage.Reference]bool{}
	var mutationFingerprint string
	for ordinal := uint64(0); ordinal < root.Count; ordinal++ {
		if err := ctx.Err(); err != nil {
			return finalizingCapture{}, nil, err
		}
		var row finalizingCaptureMember
		found, err := journal.Read(owner, history, finalizingCaptureMemberKey(operation, ordinal), &row)
		if err != nil {
			return finalizingCapture{}, nil, err
		}
		if err := validateFinalizingHistoryMember(found, row, root, input, ordinal, refs, objects, &mutationFingerprint); err != nil {
			return finalizingCapture{}, nil, err
		}
		if err := encodeFinalizingHistoryMember(row, ordinal, allEncoder, inputEncoder, stagedEncoder, mutationEncoder); err != nil {
			return finalizingCapture{}, nil, err
		}
		members = append(members, row)
	}
	if hex.EncodeToString(all.Sum(nil)) != capture.MembersSHA256 || hex.EncodeToString(original.Sum(nil)) != root.InputFingerprint || hex.EncodeToString(staged.Sum(nil)) != root.MemberSHA256 || hex.EncodeToString(mutations.Sum(nil)) != mutationFingerprint {
		return finalizingCapture{}, nil, storage.ErrRecord
	}
	return capture, members, ctx.Err()
}

func validFinalizingCaptureHeader(found bool, capture finalizingCapture, scope storage.Scope, operation string) bool {
	root := capture.Batch
	return found && capture.Version == 1 && root.Version == 1 && root.Scope == scope && root.Operation == operation &&
		storage.Identifier(root.PlanID) && storage.Identifier(root.BatchID) && root.Target.Valid() &&
		root.State == "accepted" && root.Captured && root.Count != 0 &&
		root.Root != nil && root.Root.Valid() && root.Root.RoomID == root.Target.RoomID &&
		atomicValidHash(root.InputFingerprint) && atomicValidHash(root.MemberSHA256) && atomicValidHash(capture.MembersSHA256)
}

func validateFinalizingHistoryMember(found bool, row finalizingCaptureMember, root batchPublication, input inputRoot, ordinal uint64, refs map[storage.Reference]bool, objects map[string]bool, mutationFingerprint *string) error {
	if !validFinalizingMemberIdentity(found, row, root, input, ordinal, refs, objects) {
		return storage.ErrRecord
	}
	refs[row.Reference], objects[row.Input.Spec.ObjectID] = true, true
	p := row.Prepared
	if p.Version != 1 || p.Scope != root.Scope || p.BatchID != root.BatchID || p.Ordinal != ordinal || !atomicValidHash(p.Fingerprint) || !p.Mutation.valid() {
		return storage.ErrRecord
	}
	if ordinal == 0 {
		*mutationFingerprint = p.Fingerprint
	} else if p.Fingerprint != *mutationFingerprint {
		return storage.ErrRecord
	}
	if !recoveredMutationMatchesInput(row.Input.Spec, p.Mutation, root.Target) {
		return storage.ErrRecord
	}
	return nil
}

func validFinalizingMemberIdentity(found bool, row finalizingCaptureMember, root batchPublication, input inputRoot, ordinal uint64, refs map[storage.Reference]bool, objects map[string]bool) bool {
	return found && row.Version == 1 && row.BatchID == root.BatchID && row.Ordinal == ordinal &&
		row.Input.Captured && row.Input.valid(input, ordinal) && row.Reference.Valid() &&
		row.Reference.RoomID == root.Target.RoomID && atomicValidHash(row.RecordSHA256) &&
		!refs[row.Reference] && !objects[row.Input.Spec.ObjectID]
}

func encodeFinalizingHistoryMember(row finalizingCaptureMember, ordinal uint64, allEncoder, inputEncoder, stagedEncoder, mutationEncoder *json.Encoder) error {
	if err := allEncoder.Encode(row); err != nil {
		return err
	}
	if err := inputEncoder.Encode(row.Input.Spec); err != nil {
		return err
	}
	if err := stagedEncoder.Encode(struct {
		Ordinal uint64 `json:"ordinal"`
		SHA256  string `json:"sha256"`
	}{ordinal, inputMemberHash(row.Input)}); err != nil {
		return err
	}
	return mutationEncoder.Encode(row.Prepared.Mutation)
}

func recoveredMutationMatchesInput(input inputSpec, mutation Mutation, target storage.PublicationTarget) bool {
	commit := mutation.Commit
	if mutation.ObjectID != input.ObjectID || commit.State != input.State || !bytes.Equal(commit.Metadata, input.Metadata) || !slices.Equal(commit.Parents, input.Parents) {
		return false
	}
	if input.Content == nil {
		return commit.Content == nil && mutation.PayloadSHA256 == ""
	}
	return commit.Content != nil && commit.Content.Valid() && commit.Content.Kind == input.Content.Kind && commit.Content.RevisionID == input.Content.Request.RevisionID && commit.Content.Reference.RoomID == target.RoomID && mutation.PayloadSHA256 == input.Content.SHA256
}

func verifyRecoveredMember(ctx context.Context, scope storage.Scope, records storage.RecordStore, files storage.BoundFiles, proof storage.VerifiedAtomicBatch, snapshot storage.AtomicCatalogSnapshot, member finalizingCaptureMember) error {
	entry, found := snapshot.Entries[member.Reference]
	if !found {
		return storage.ErrIncompleteCatalog
	}
	prepared, _, err := storage.DecodeAtomicPrepared(entry, scope)
	if err != nil || prepared.BatchID != member.BatchID || prepared.Ordinal != member.Ordinal {
		return storage.ErrRecord
	}
	expected, err := member.Prepared.PreparedRecord()
	if err != nil {
		return err
	}
	digest, err := storage.AtomicRecordSHA256(entry.Record)
	if err != nil || digest != member.RecordSHA256 || !expected.MatchesExact(entry.Record) {
		return storage.ErrRecord
	}
	content, pointer := member.Input.Spec.Content, member.Prepared.Mutation.Commit.Content
	if content == nil {
		return ctx.Err()
	}
	if pointer == nil {
		return storage.ErrRecord
	}
	if err := verifyRecoveredMemberContent(ctx, scope, records, files, proof, member, content, pointer); err != nil {
		return err
	}
	return ctx.Err()
}

func verifyRecoveredMemberContent(ctx context.Context, scope storage.Scope, records storage.RecordStore, files storage.BoundFiles, proof storage.VerifiedAtomicBatch, member finalizingCaptureMember, content *contentSpec, pointer *storage.ContentPointer) error {
	switch content.Kind {
	case "record":
		record, err := records.ReadStorageRecord(ctx, pointer.Reference, member.Input.Spec.ObjectID, "content")
		if err != nil {
			return err
		}
		defer clear(record.Payload)
		sum := sha256.Sum256(record.Payload)
		if record.Validate(scope) != nil || record.ObjectID != content.Request.ObjectID || record.Role != "content" || record.RevisionID != content.Request.RevisionID || !slices.Equal(record.Previous, content.Request.Previous) || int64(len(record.Payload)) != content.Request.SizeBytes || hex.EncodeToString(sum[:]) != content.SHA256 {
			return storage.ErrRecord
		}
	case "file":
		if files == nil || files.FileScope() != scope {
			return storage.ErrCatalogSource
		}
		completion, err := proof.FileCompletion(ctx, files, member.Reference)
		if err != nil {
			return err
		}
		got, err := completion.ReadPointer(ctx, scope, content.Request, pointer.Reference, content.SHA256)
		if err != nil {
			return err
		}
		if got != *pointer {
			return storage.ErrRecord
		}
	default:
		return storage.ErrRecord
	}
	return nil
}
