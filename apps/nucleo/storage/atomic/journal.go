// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	StateStaging  = "staging"
	StateCaptured = "captured"
)

// Mutation describes an exact intended revision, including its original
// base references and payload checksum. It grants no SDK or publication access.
// The domain must validate/read back content through its original maintained
// adapter before this intent may be published.
type Mutation struct {
	ObjectID      string               `json:"object_id"`
	Commit        storage.ObjectCommit `json:"commit"`
	PayloadSHA256 string               `json:"payload_sha256,omitempty"`
}

// A root is durable BEFORE member staging starts. All staging rows share a work
// namespace, so interruption can never report an empty pending-work inventory.
// Captured means every exact member intent exists locally, not that anything was
// published. This local struct is not a verified receipt or an author capability.
type Work struct {
	Version     int                       `json:"v"`
	Scope       storage.Scope             `json:"scope"`
	Target      storage.PublicationTarget `json:"target"`
	Operation   string                    `json:"operation"`
	BatchID     string                    `json:"batch_id"`
	Fingerprint string                    `json:"fingerprint"`
	Count       uint64                    `json:"count"`
	State       string                    `json:"state"`
}

type Member struct {
	Version     int           `json:"v"`
	Scope       storage.Scope `json:"scope"`
	BatchID     string        `json:"batch_id"`
	Fingerprint string        `json:"fingerprint"`
	Ordinal     uint64        `json:"ordinal"`
	Mutation    Mutation      `json:"mutation"`
}

func atomicValidHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

func atomicOwner(scope storage.Scope) string {
	return scope.Tenant + "\x00" + scope.Subject + "\x00" + scope.Product
}

func atomicOperationKey(operation string) string {
	sum := sha256.Sum256([]byte(operation))
	return "root_" + hex.EncodeToString(sum[:])
}

func atomicMemberKey(operation string, ordinal uint64) string {
	return "member_" + atomicOperationKey(operation)[5:] + "_" + strconv.FormatUint(ordinal, 10)
}

func (m Mutation) valid() bool {
	if !storage.Identifier(m.ObjectID) || !m.Commit.Valid() {
		return false
	}
	if m.Commit.Content == nil {
		return m.PayloadSHA256 == ""
	}
	return atomicValidHash(m.PayloadSHA256)
}

func cloneMutation(value Mutation) Mutation {
	value.Commit.Metadata = slices.Clone(value.Commit.Metadata)
	value.Commit.Parents = slices.Clone(value.Commit.Parents)
	if value.Commit.Content != nil {
		pointer := *value.Commit.Content
		value.Commit.Content = &pointer
	}
	return value
}

func atomicMutationFingerprint(mutations []Mutation) (string, error) {
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	for _, mutation := range mutations {
		if !mutation.valid() {
			return "", storage.ErrRecord
		}
		if err := encoder.Encode(mutation); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (work Work) valid(scope storage.Scope, operation string) bool {
	return work.Version == 1 && work.Scope == scope && scope.Valid() && work.Target.Valid() && work.Operation == operation && storage.Identifier(operation) && storage.Identifier(work.BatchID) && atomicValidHash(work.Fingerprint) && work.Count > 0 && (work.State == StateStaging || work.State == StateCaptured)
}

func (work Work) member(ordinal uint64, mutation Mutation) Member {
	return Member{Version: 1, Scope: work.Scope, BatchID: work.BatchID, Fingerprint: work.Fingerprint, Ordinal: ordinal, Mutation: cloneMutation(mutation)}
}

func (member Member) matches(work Work, ordinal uint64, mutation Mutation) bool {
	expected := work.member(ordinal, mutation)
	return member.Version == expected.Version && member.Scope == expected.Scope && member.BatchID == expected.BatchID && member.Fingerprint == expected.Fingerprint && member.Ordinal == expected.Ordinal && member.Mutation.ObjectID == expected.Mutation.ObjectID && member.Mutation.PayloadSHA256 == expected.Mutation.PayloadSHA256 && member.Mutation.Commit.MatchesExact(expected.Mutation.Commit)
}

// PreparedRecord constructs private application data only. Publication still
// requires the original SDK's own live target/policy; another device cannot gain
// that permission by copying a work root or choosing the same batch ID.
func (member Member) PreparedRecord() (storage.Record, error) {
	if member.Version != 1 || !member.Scope.Valid() || !storage.Identifier(member.BatchID) || !atomicValidHash(member.Fingerprint) || !member.Mutation.valid() {
		return storage.Record{}, storage.ErrRecord
	}
	sum := sha256.Sum256([]byte(member.BatchID + "\x00" + strconv.FormatUint(member.Ordinal, 10)))
	record := storage.Record{Version: 1, Scope: member.Scope, ObjectID: member.Mutation.ObjectID, RevisionID: "prepared_" + hex.EncodeToString(sum[:]), Role: "index"}
	for _, parent := range member.Mutation.Commit.Parents {
		record.Previous = append(record.Previous, parent.EventID)
	}
	record.Payload, _ = json.Marshal(storage.AtomicPrepared{Kind: storage.ObjectPreparedKind, BatchID: member.BatchID, Ordinal: member.Ordinal, Commit: member.Mutation.Commit})
	if record.Validate(member.Scope) != nil {
		return storage.Record{}, storage.ErrRecord
	}
	return record, nil
}

// Capture persists prepared-index intents whose content references have
// ALREADY passed independent readback. It is not an input/payload staging plan;
// a full batch writer must durably retain replayable payloads before publishing
// any new content, then capture the resulting exact references here.
// It sends nothing, acquires no SDK publication authority, never
// retargets a previous writer, and never replaces a partially captured plan with
// changed input. Callers keep their original work lease/cancellation until done.
func Capture(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string, target storage.PublicationTarget, mutations []Mutation) (Work, error) {
	return captureBatch(ctx, journal, scope, operation, target, mutations, "")
}

// captureBatch accepts only a batch ID already retained by the full input plan.
// The ID is private application data and grants no SDK writer authority.
func captureBatch(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string, target storage.PublicationTarget, mutations []Mutation, capturedBatch string) (Work, error) {
	if err := ctx.Err(); err != nil {
		return Work{}, err
	}
	if !journal.Encrypted() || !scope.Valid() || !storage.Identifier(operation) || !target.Valid() || len(mutations) == 0 || capturedBatch != "" && !storage.Identifier(capturedBatch) {
		return Work{}, storage.ErrRecord
	}
	owned := make([]Mutation, len(mutations))
	objects := map[string]bool{}
	for i, mutation := range mutations {
		owned[i] = cloneMutation(mutation)
		if !owned[i].valid() || objects[owned[i].ObjectID] {
			return Work{}, storage.ErrRecord
		}
		objects[owned[i].ObjectID] = true
	}
	fingerprint, err := atomicMutationFingerprint(owned)
	if err != nil {
		return Work{}, err
	}
	var result Work
	err = journal.Exclusive(ctx, atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey(operation), func() error {
		work, err := loadOrCreateBatchWork(ctx, journal, scope, operation, target, fingerprint, capturedBatch, uint64(len(owned)))
		if err != nil {
			return err
		}
		for i, mutation := range owned {
			if err := captureBatchMember(ctx, journal, work, uint64(i), mutation); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		work.State = StateCaptured
		if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey(operation), work); err != nil {
			return err
		}
		result = work
		return nil
	})
	if err != nil {
		return Work{}, err
	}
	return result, nil
}

func loadOrCreateBatchWork(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string, target storage.PublicationTarget, fingerprint, capturedBatch string, count uint64) (Work, error) {
	var work Work
	found, err := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey(operation), &work)
	if err != nil {
		return Work{}, err
	}
	if !found {
		id := capturedBatch
		if id == "" {
			id = rand.Text()
		}
		work = Work{Version: 1, Scope: scope, Target: target, Operation: operation, BatchID: id, Fingerprint: fingerprint, Count: count, State: StateStaging}
	}
	if !work.valid(scope, operation) || work.Target != target || work.Fingerprint != fingerprint || work.Count != count || capturedBatch != "" && work.BatchID != capturedBatch {
		return Work{}, storage.ErrRecord
	}
	if work.State == StateStaging {
		if err := ctx.Err(); err != nil {
			return Work{}, err
		}
		if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey(operation), work); err != nil {
			return Work{}, err
		}
	}
	return work, nil
}

func captureBatchMember(ctx context.Context, journal *offline.Store, work Work, ordinal uint64, mutation Mutation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	scope := work.Scope
	operation := work.Operation
	var member Member
	found, err := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicMemberKey(operation, ordinal), &member)
	if err != nil {
		return err
	}
	if !found {
		if work.State != StateStaging {
			return storage.ErrRecord
		}
		member = work.member(ordinal, mutation)
		if _, err := member.PreparedRecord(); err != nil {
			return err
		}
		if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicMemberKey(operation, ordinal), member); err != nil {
			return err
		}
	}
	if !member.matches(work, ordinal, mutation) {
		return storage.ErrRecord
	}
	return nil
}

// Load is a local pending-work lookup, never a publication or receipt
// proof. Missing/corrupt roots are errors; staging work remains retained.
func Load(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string) (Work, error) {
	if err := ctx.Err(); err != nil {
		return Work{}, err
	}
	if !journal.Encrypted() || !scope.Valid() || !storage.Identifier(operation) {
		return Work{}, storage.ErrRecord
	}
	var work Work
	found, err := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), atomicOperationKey(operation), &work)
	if err != nil {
		return Work{}, err
	}
	if !found || !work.valid(scope, operation) {
		return Work{}, storage.ErrRecord
	}
	return work, nil
}

// ReadMember seeks one durable member directly; it never enumerates a
// bounded prefix and mistakes a larger batch for an empty or complete plan.
func ReadMember(ctx context.Context, journal *offline.Store, work Work, ordinal uint64) (Member, error) {
	current, err := Load(ctx, journal, work.Scope, work.Operation)
	if err != nil {
		return Member{}, err
	}
	if current != work || ordinal >= work.Count || work.State != StateCaptured {
		return Member{}, storage.ErrRecord
	}
	var member Member
	found, err := journal.Read(atomicOwner(work.Scope), storage.AtomicJournalNamespace(work.Scope.Product), atomicMemberKey(work.Operation, ordinal), &member)
	if err != nil {
		return Member{}, err
	}
	if !found || member.Version != 1 || member.Scope != work.Scope || member.BatchID != work.BatchID || member.Fingerprint != work.Fingerprint || member.Ordinal != ordinal || !member.Mutation.valid() {
		return Member{}, storage.ErrRecord
	}
	if _, err := member.PreparedRecord(); err != nil {
		return Member{}, fmt.Errorf("atomic member: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Member{}, err
	}
	return member, nil
}
