// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type verifiedInputContent struct {
	plan    InputPlan
	ordinal uint64
	reader  storage.AtomicCatalogReader
	files   storage.FileCommitmentReader
	binding storage.Binding
	pointer *storage.ContentPointer
}

// VerifiedInputContent is exact original-SDK readback for one captured input.
// A pointer, group ID, caller SHA or JSON cannot construct it. It authorizes no
// publication or cleanup; it only supplies independently verified index data.
type VerifiedInputContent struct{ data *verifiedInputContent }

func VerifyInputContent(ctx context.Context, plan InputPlan, ordinal uint64, reader storage.AtomicCatalogReader, files storage.FileCommitmentReader, pointer *storage.ContentPointer) (VerifiedInputContent, error) {
	data, member, err := plan.readCapturedMember(ctx, ordinal)
	if err != nil {
		return VerifiedInputContent{}, err
	}
	if reader == nil {
		return VerifiedInputContent{}, storage.ErrRecord
	}
	binding, err := reader.StorageBindings(ctx)
	if err != nil {
		return VerifiedInputContent{}, err
	}
	if !binding.Valid() || binding.Scope != data.root.Scope {
		return VerifiedInputContent{}, storage.ErrCatalogSource
	}
	binding.Rooms = slices.Clone(binding.Rooms)
	var retained *storage.ContentPointer
	if pointer != nil {
		copied := *pointer
		retained = &copied
	}
	proof := VerifiedInputContent{&verifiedInputContent{plan, ordinal, reader, files, binding, retained}}
	if _, err := proof.read(ctx, member); err != nil {
		return VerifiedInputContent{}, err
	}
	return proof, nil
}

func (data *verifiedInputContent) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if data == nil || data.reader == nil || !data.binding.Valid() {
		return storage.ErrRecord
	}
	current, err := data.reader.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !data.binding.Same(current) {
		return storage.ErrCatalogSource
	}
	return ctx.Err()
}

func (proof VerifiedInputContent) read(ctx context.Context, member inputMember) (Mutation, error) {
	data := proof.data
	if err := data.check(ctx); err != nil {
		return Mutation{}, err
	}
	spec := member.Spec
	mutation := Mutation{ObjectID: spec.ObjectID, Commit: storage.ObjectCommit{Kind: storage.ObjectKind, State: spec.State, Metadata: slices.Clone(spec.Metadata), Parents: slices.Clone(spec.Parents)}}
	if spec.Content == nil {
		if data.pointer != nil {
			return Mutation{}, storage.ErrRecord
		}
	} else {
		copied, err := verifyInputContentPointer(ctx, data, spec)
		if err != nil {
			return Mutation{}, err
		}
		mutation.Commit.Content = copied
		mutation.PayloadSHA256 = spec.Content.SHA256
	}
	if !mutation.valid() {
		return Mutation{}, storage.ErrRecord
	}
	if err := data.check(ctx); err != nil {
		return Mutation{}, err
	}
	if _, _, err := data.plan.readCapturedMember(ctx, data.ordinal); err != nil {
		return Mutation{}, err
	}
	return mutation, nil
}

//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func verifyInputContentPointer(ctx context.Context, data *verifiedInputContent, spec inputSpec) (*storage.ContentPointer, error) {
	scope := data.binding.Scope
	pointer := data.pointer
	content := spec.Content
	if pointer == nil || !pointer.Valid() || pointer.Kind != content.Kind || pointer.RevisionID != content.Request.RevisionID || pointer.Reference.RoomID != data.plan.data.root.Target.RoomID || !slices.Contains(data.binding.Rooms, pointer.Reference.RoomID) {
		return nil, storage.ErrRecord
	}
	switch content.Kind {
	case "record":
		record, err := data.reader.ReadStorageRecord(ctx, pointer.Reference, spec.ObjectID, "content")
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(record.Payload)
		if record.Validate(scope) != nil || record.ObjectID != spec.ObjectID || record.Role != "content" || record.RevisionID != content.Request.RevisionID || !slices.Equal(record.Previous, content.Request.Previous) || int64(len(record.Payload)) != content.Request.SizeBytes || hex.EncodeToString(sum[:]) != content.SHA256 {
			return nil, storage.ErrRecord
		}
	case "file":
		if data.files == nil || data.files.FileScope() != scope {
			return nil, storage.ErrRecord
		}
		request, sha, err := data.files.Commitment(ctx, pointer.Reference, spec.ObjectID, content.Request.RevisionID)
		if err != nil {
			return nil, err
		}
		if request.Validate(scope) != nil || !request.MatchesExact(content.Request) || sha != content.SHA256 {
			return nil, storage.ErrRecord
		}
	default:
		return nil, storage.ErrRecord
	}
	copied := *pointer
	return &copied, nil
}

// Mutation revalidates the same source and exact bytes at use. It never turns a
// stale/retired/JSON-created proof into a capability or a fabricated ObjectVersion.
func (proof VerifiedInputContent) Mutation(ctx context.Context) (Mutation, error) {
	if proof.data == nil {
		return Mutation{}, storage.ErrRecord
	}
	_, member, err := proof.data.plan.readCapturedMember(ctx, proof.data.ordinal)
	if err != nil {
		return Mutation{}, err
	}
	return proof.read(ctx, member)
}

// CaptureVerified freezes ALL independently read-back content pointers in their
// original input order. It still sends nothing and uses the original target.
// SDK publication authority and durable finalizing receipts remain separate.
func CaptureVerified(ctx context.Context, plan InputPlan, proofs []VerifiedInputContent) (Work, error) {
	return captureVerifiedBatch(ctx, plan, proofs, "")
}

func captureVerifiedBatch(ctx context.Context, plan InputPlan, proofs []VerifiedInputContent, capturedBatch string) (Work, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return Work{}, err
	}
	if uint64(len(proofs)) != data.root.Count {
		return Work{}, storage.ErrRecord
	}
	mutations := make([]Mutation, len(proofs))
	var binding storage.Binding
	for i, proof := range proofs {
		if proof.data == nil || proof.data.plan.data == nil || proof.data.plan.data.root != data.root || proof.data.ordinal != uint64(i) {
			return Work{}, storage.ErrRecord
		}
		if i == 0 {
			binding = proof.data.binding
		} else if !binding.Same(proof.data.binding) {
			return Work{}, storage.ErrCatalogSource
		}
		mutation, err := proof.Mutation(ctx)
		if err != nil {
			return Work{}, err
		}
		mutations[i] = mutation
	}
	if _, err := plan.current(ctx); err != nil {
		return Work{}, err
	}
	return captureBatch(ctx, data.journal, data.root.Scope, inputIndexOperation(data.root.ID), data.root.Target, mutations, capturedBatch)
}

func inputIndexOperation(id string) string {
	sum := sha256.Sum256([]byte(id))
	return "input_index_" + hex.EncodeToString(sum[:])
}
