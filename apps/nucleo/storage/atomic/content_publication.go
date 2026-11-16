// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Every content intent is durable before SDK preparation/publication. These
// rows are private locators/checksums, never SDK receipts or JSON authority.
type contentPublication struct {
	Version     int                       `json:"v"`
	Scope       storage.Scope             `json:"scope"`
	PlanID      string                    `json:"plan_id"`
	Fingerprint string                    `json:"fingerprint"`
	Count       uint64                    `json:"count"`
	Target      storage.PublicationTarget `json:"target"`
}
type contentPublicationMember struct {
	Version      int                       `json:"v"`
	Scope        storage.Scope             `json:"scope"`
	PlanID       string                    `json:"plan_id"`
	Ordinal      uint64                    `json:"ordinal"`
	MemberSHA256 string                    `json:"member_sha256"`
	Target       storage.PublicationTarget `json:"target"`
	Pointer      *storage.ContentPointer   `json:"pointer,omitempty"`
}

func inputPublicationKey(operation string) string {
	return "publication_" + atomicOperationKey(operation)
}

func inputContentReceiptKey(operation string, ordinal uint64) string {
	return "content_" + atomicMemberKey(operation, ordinal)
}

func expectedContentWork(data *capturedInputPlan) contentPublication {
	return contentPublication{1, data.root.Scope, data.root.ID, data.root.Fingerprint, data.root.Count, data.root.Target}
}

func (member contentPublicationMember) matches(data *capturedInputPlan, ordinal uint64, input inputMember) bool {
	if member.Version != 1 || member.Scope != data.root.Scope || member.PlanID != data.root.ID || member.Ordinal != ordinal || member.MemberSHA256 != inputMemberHash(input) || member.Target != data.root.Target {
		return false
	}
	if input.Spec.Content == nil {
		return member.Pointer == nil
	}
	return member.Pointer == nil || (member.Pointer.Valid() && member.Pointer.Kind == input.Spec.Content.Kind && member.Pointer.RevisionID == input.Spec.Content.Request.RevisionID && member.Pointer.Reference.RoomID == member.Target.RoomID)
}

func contentPublicationGuard(ctx context.Context, data *capturedInputPlan, records storage.RecordStore, files storage.BoundFiles, write bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if records == nil {
		return storage.ErrRecord
	}
	binding, err := records.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !binding.Valid() || binding.Scope != data.root.Scope {
		return storage.ErrCatalogSource
	}
	if files != nil && files.FileScope() != data.root.Scope {
		return storage.ErrCatalogSource
	}
	if write {
		if err := records.CheckStoragePublicationTarget(data.root.Target); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// PublishInputContents is the content-only stage of the atomic publisher. The
// original caller's admitted context/SDK/work lease is retained unchanged. ALL
// inputs and every member intent exist before network IO; each actual SDK
// readback precedes checkpointing its pointer. It publishes no application index,
// manifest page or root and never deletes anything. Full accepted-root recovery
// and durable finalizing cleanup belong to the enclosing batch state machine.
func PublishInputContents(ctx context.Context, plan InputPlan, records storage.RecordStore, files storage.BoundFiles) ([]VerifiedInputContent, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return nil, err
	}
	// Revalidate all private source bytes before the first SDK/network operation.
	checked, err := LoadInputs(ctx, data.journal, data.root.Scope, data.root.Operation)
	if err != nil {
		return nil, err
	}
	if checked.data.root != data.root {
		return nil, storage.ErrRecord
	}
	for ordinal, hash := range data.members {
		if checked.data.members[ordinal] != hash {
			return nil, storage.ErrRecord
		}
	}
	result := make([]VerifiedInputContent, 0, data.root.Count)
	err = data.journal.Exclusive(ctx, atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputPublicationKey(data.root.Operation), func() error {
		needsWrite, err := ensureContentWorkAndMemberIntents(ctx, plan, data)
		if err != nil {
			return err
		}
		if err := prepareStorageForContent(ctx, data, records, files, needsWrite); err != nil {
			return err
		}
		for ordinal := uint64(0); ordinal < data.root.Count; ordinal++ {
			proof, err := publishAndVerifyContentMember(ctx, plan, data, ordinal, records, files)
			if err != nil {
				return err
			}
			result = append(result, proof)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ensureContentWorkAndMemberIntents(ctx context.Context, plan InputPlan, data *capturedInputPlan) (bool, error) {
	expected := expectedContentWork(data)
	var work contentPublication
	found, err := data.journal.Read(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputPublicationKey(data.root.Operation), &work)
	if err != nil {
		return false, err
	}
	if found && work != expected {
		return false, storage.ErrRecord
	}
	if !found {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if err := data.journal.Write(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputPublicationKey(data.root.Operation), expected); err != nil {
			return false, err
		}
	}
	needsWrite := false
	for ordinal := uint64(0); ordinal < data.root.Count; ordinal++ {
		_, input, err := plan.readCapturedMember(ctx, ordinal)
		if err != nil {
			return false, err
		}
		var member contentPublicationMember
		found, err := data.journal.Read(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputContentReceiptKey(data.root.Operation, ordinal), &member)
		if err != nil {
			return false, err
		}
		if !found {
			member = contentPublicationMember{Version: 1, Scope: data.root.Scope, PlanID: data.root.ID, Ordinal: ordinal, MemberSHA256: inputMemberHash(input), Target: data.root.Target}
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := data.journal.Write(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputContentReceiptKey(data.root.Operation, ordinal), member); err != nil {
				return false, err
			}
		}
		if !member.matches(data, ordinal, input) {
			return false, storage.ErrRecord
		}
		if input.Spec.Content != nil && member.Pointer == nil {
			needsWrite = true
		}
	}
	return needsWrite, nil
}

func prepareStorageForContent(ctx context.Context, data *capturedInputPlan, records storage.RecordStore, files storage.BoundFiles, needsWrite bool) error {
	if err := contentPublicationGuard(ctx, data, records, files, false); err != nil {
		return err
	}
	// Preparation remains on the same original SDK/context. No historical
	// projection, context.Background or destination retarget is manufactured here.
	if !needsWrite {
		return nil
	}
	if err := contentPublicationGuard(ctx, data, records, files, true); err != nil {
		return err
	}
	binding, err := records.PrepareStorage(ctx, data.root.Scope)
	if err != nil {
		return err
	}
	if !binding.Valid() || binding.Scope != data.root.Scope {
		return storage.ErrCatalogSource
	}
	return nil
}

func publishAndVerifyContentMember(ctx context.Context, plan InputPlan, data *capturedInputPlan, ordinal uint64, records storage.RecordStore, files storage.BoundFiles) (VerifiedInputContent, error) {
	_, input, err := plan.readCapturedMember(ctx, ordinal)
	if err != nil {
		return VerifiedInputContent{}, err
	}
	var member contentPublicationMember
	found, err := data.journal.Read(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputContentReceiptKey(data.root.Operation, ordinal), &member)
	if err != nil {
		return VerifiedInputContent{}, err
	}
	if !found || !member.matches(data, ordinal, input) {
		return VerifiedInputContent{}, storage.ErrRecord
	}
	if input.Spec.Content != nil && member.Pointer == nil {
		if err := contentPublicationGuard(ctx, data, records, files, true); err != nil {
			return VerifiedInputContent{}, err
		}
		pointer, err := publishInputContent(ctx, plan, ordinal, input, records, files)
		if err != nil {
			return VerifiedInputContent{}, err
		}
		member.Pointer = &pointer
	}
	proof, err := VerifyInputContent(ctx, plan, ordinal, records, files, member.Pointer)
	if err != nil {
		return VerifiedInputContent{}, err
	}
	if err := contentPublicationGuard(ctx, data, records, files, false); err != nil {
		return VerifiedInputContent{}, err
	}
	if err := data.journal.Write(atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), inputContentReceiptKey(data.root.Operation, ordinal), member); err != nil {
		return VerifiedInputContent{}, err
	}
	return proof, nil
}

func publishInputContent(ctx context.Context, plan InputPlan, ordinal uint64, input inputMember, records storage.RecordStore, files storage.BoundFiles) (storage.ContentPointer, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return storage.ContentPointer{}, err
	}
	content := input.Spec.Content
	if content == nil {
		return storage.ContentPointer{}, storage.ErrRecord
	}
	job, err := plan.ContentJob(ordinal)
	if err != nil {
		return storage.ContentPointer{}, err
	}
	source, err := plan.OpenContent(ctx, ordinal)
	if err != nil {
		return storage.ContentPointer{}, err
	}
	defer func() { _ = source.Close() }()
	var ref storage.Reference
	switch content.Kind {
	case "record":
		raw, err := io.ReadAll(io.NewSectionReader(source, 0, content.Request.SizeBytes))
		if err != nil {
			return storage.ContentPointer{}, err
		}
		defer clear(raw)
		record := storage.Record{Version: 1, Scope: data.root.Scope, ObjectID: input.Spec.ObjectID, RevisionID: content.Request.RevisionID, Role: "content", Payload: raw, Previous: content.Request.Previous}
		sum := sha256.Sum256(raw)
		if record.Validate(data.root.Scope) != nil || hex.EncodeToString(sum[:]) != content.SHA256 {
			return storage.ContentPointer{}, storage.ErrRecord
		}
		ref, err = records.PutStorageRecordAt(ctx, data.root.Target, job, record)
		if err != nil {
			return storage.ContentPointer{}, err
		}
	case "file":
		if files == nil || files.FileScope() != data.root.Scope {
			return storage.ContentPointer{}, storage.ErrRecord
		}
		ref, err = files.Upload(ctx, job, data.root.Target, content.Request, source)
		if err != nil {
			return storage.ContentPointer{}, err
		}
	default:
		return storage.ContentPointer{}, storage.ErrRecord
	}
	if !ref.Valid() || ref.RoomID != data.root.Target.RoomID {
		return storage.ContentPointer{}, storage.ErrRecord
	}
	return storage.ContentPointer{Kind: content.Kind, RevisionID: content.Request.RevisionID, Reference: ref}, nil
}

// The same deterministic transaction is replayed only by its original live SDK.
// A current reader B may inspect checkpoints, never send as original device A.
func inputTransaction(id string, ordinal uint64, kind string) string {
	sum := sha256.Sum256([]byte(id + "\x00" + kind + "\x00" + strconv.FormatUint(ordinal, 10)))
	return "atomic_" + hex.EncodeToString(sum[:])
}
