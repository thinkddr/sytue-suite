// SPDX-License-Identifier: Apache-2.0
package atomic

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ContentInput is private input, not a published pointer. Open must reopen the
// exact original source after interruption; missing input never permits publish.
// JSON records and binary files both reuse the original encrypted staged files.
type ContentInput struct {
	Kind    string
	Request storage.FileRequest
	SHA256  string
	Open    func(context.Context) (io.ReadCloser, error)
}
type Input struct {
	ObjectID string
	State    string
	Metadata []byte
	Parents  []storage.Reference
	Content  *ContentInput
}
type contentSpec struct {
	Kind    string              `json:"kind"`
	Request storage.FileRequest `json:"request"`
	SHA256  string              `json:"sha256"`
}
type inputSpec struct {
	ObjectID string              `json:"object_id"`
	State    string              `json:"state"`
	Metadata []byte              `json:"metadata"`
	Parents  []storage.Reference `json:"parents,omitempty"`
	Content  *contentSpec        `json:"content,omitempty"`
}
type inputRoot struct {
	Version     int                       `json:"v"`
	Scope       storage.Scope             `json:"scope"`
	Target      storage.PublicationTarget `json:"target"`
	Operation   string                    `json:"operation"`
	ID          string                    `json:"id"`
	Fingerprint string                    `json:"fingerprint"`
	Count       uint64                    `json:"count"`
	State       string                    `json:"state"`
}
type inputMember struct {
	Version     int                 `json:"v"`
	Scope       storage.Scope       `json:"scope"`
	PlanID      string              `json:"plan_id"`
	Fingerprint string              `json:"fingerprint"`
	Ordinal     uint64              `json:"ordinal"`
	Spec        inputSpec           `json:"spec"`
	Source      *offline.StagedFile `json:"source,omitempty"`
	Captured    bool                `json:"captured"`
}
type capturedInputPlan struct {
	journal *offline.Store
	root    inputRoot
	members map[uint64]string
}

// InputPlan is an opaque handle to ALL captured replayable inputs. JSON cannot
// construct it. It grants neither SDK read authority nor permission to publish.
// A new deliberate mutation/revert requires a NEW explicit operation ID; retry
// of the same operation must retain the original target and exact fingerprint.
type InputPlan struct{ data *capturedInputPlan }

func inputRootKey(operation string) string { return "input_" + atomicOperationKey(operation) }
func inputMemberKey(operation string, ordinal uint64) string {
	return "input_" + atomicMemberKey(operation, ordinal)
}

func cloneFileInput(r storage.FileRequest) storage.FileRequest {
	r.Metadata = slices.Clone(r.Metadata)
	r.Previous = slices.Clone(r.Previous)
	return r
}

func (input Input) spec() inputSpec {
	value := inputSpec{ObjectID: input.ObjectID, State: input.State, Metadata: slices.Clone(input.Metadata), Parents: slices.Clone(input.Parents)}
	if input.Content != nil {
		value.Content = &contentSpec{input.Content.Kind, cloneFileInput(input.Content.Request), input.Content.SHA256}
	}
	return value
}

func (spec inputSpec) valid(scope storage.Scope) bool {
	// Reuse the common metadata/parent validator without inventing a content ref.
	fields := storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: spec.Metadata, Parents: spec.Parents}
	if !storage.Identifier(spec.ObjectID) || !fields.Valid() {
		return false
	}
	if spec.State == "deleted" {
		return spec.Content == nil
	}
	if (spec.State != "live" && spec.State != "trash") || spec.Content == nil {
		return false
	}
	c := spec.Content
	return (c.Kind == "record" || c.Kind == "file") && c.Request.ObjectID == spec.ObjectID && c.Request.Validate(scope) == nil && atomicValidHash(c.SHA256) && (c.Kind != "record" || c.Request.SizeBytes <= storage.MaxRecordBytes)
}

func inputSpecsEqual(a, b inputSpec) bool {
	first, err := json.Marshal(a)
	if err != nil {
		return false
	}
	second, err := json.Marshal(b)
	return err == nil && bytes.Equal(first, second)
}

func (root inputRoot) valid(scope storage.Scope, operation string) bool {
	return root.Version == 1 && scope.Valid() && root.Scope == scope && root.Operation == operation && storage.Identifier(operation) && storage.Identifier(root.ID) && root.Target.Valid() && atomicValidHash(root.Fingerprint) && root.Count > 0 && (root.State == StateStaging || root.State == StateCaptured)
}

func (member inputMember) valid(root inputRoot, ordinal uint64) bool {
	if member.Version != 1 || member.Scope != root.Scope || member.PlanID != root.ID || member.Fingerprint != root.Fingerprint || member.Ordinal != ordinal || ordinal >= root.Count || !member.Spec.valid(root.Scope) {
		return false
	}
	if member.Spec.Content == nil {
		return member.Source == nil
	}
	return member.Source != nil && storage.Identifier(member.Source.ID) && member.Source.SizeBytes == member.Spec.Content.Request.SizeBytes && member.Source.SHA256 == member.Spec.Content.SHA256
}

func readInputRoot(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string) (inputRoot, error) {
	if err := ctx.Err(); err != nil {
		return inputRoot{}, err
	}
	if !journal.Encrypted() || !scope.Valid() || !storage.Identifier(operation) {
		return inputRoot{}, storage.ErrRecord
	}
	var root inputRoot
	found, err := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputRootKey(operation), &root)
	if err != nil {
		return inputRoot{}, err
	}
	if !found || !root.valid(scope, operation) {
		return inputRoot{}, storage.ErrRecord
	}
	return root, nil
}

func readInputMember(ctx context.Context, journal *offline.Store, root inputRoot, ordinal uint64) (inputMember, error) {
	if err := ctx.Err(); err != nil {
		return inputMember{}, err
	}
	var member inputMember
	found, err := journal.Read(atomicOwner(root.Scope), storage.AtomicJournalNamespace(root.Scope.Product), inputMemberKey(root.Operation, ordinal), &member)
	if err != nil {
		return inputMember{}, err
	}
	if !found || !member.valid(root, ordinal) {
		return inputMember{}, storage.ErrRecord
	}
	return member, nil
}

// CaptureInputs persists root/count/stream fingerprint BEFORE opening any source.
// Each exact member, including its reserved staging ID, is durable before source
// IO. No network/publication port is accepted. Nothing can publish until every
// original source and every member intent has been captured and revalidated.
func CaptureInputs(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string, target storage.PublicationTarget, inputs []Input) (InputPlan, error) {
	if err := ctx.Err(); err != nil {
		return InputPlan{}, err
	}
	if !journal.Encrypted() || !scope.Valid() || !storage.Identifier(operation) || !target.Valid() || len(inputs) == 0 {
		return InputPlan{}, storage.ErrRecord
	}
	specs, opens, fingerprint, err := prepareInputSpecs(scope, inputs)
	if err != nil {
		return InputPlan{}, err
	}
	var result InputPlan
	memberHashes := map[uint64]string{}
	err = journal.Exclusive(ctx, atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputRootKey(operation), func() error {
		root, err := loadOrCreateInputRoot(ctx, journal, scope, operation, target, fingerprint, uint64(len(specs)))
		if err != nil {
			return err
		}
		for i, spec := range specs {
			ordinal := uint64(i)
			hash, err := captureInputMember(ctx, journal, root, ordinal, spec, opens[i])
			if err != nil {
				return err
			}
			memberHashes[ordinal] = hash
		}
		root.State = StateCaptured
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputRootKey(operation), root); err != nil {
			return err
		}
		result = InputPlan{&capturedInputPlan{journal: journal, root: root, members: memberHashes}}
		return nil
	})
	if err != nil {
		return InputPlan{}, err
	}
	return result, nil
}

func prepareInputSpecs(scope storage.Scope, inputs []Input) ([]inputSpec, []func(context.Context) (io.ReadCloser, error), string, error) {
	specs := make([]inputSpec, len(inputs))
	opens := make([]func(context.Context) (io.ReadCloser, error), len(inputs))
	objects := map[string]bool{}
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	for i, input := range inputs {
		specs[i] = input.spec()
		if input.Content != nil {
			opens[i] = input.Content.Open
		}
		if !specs[i].valid(scope) || objects[specs[i].ObjectID] {
			return nil, nil, "", storage.ErrRecord
		}
		objects[specs[i].ObjectID] = true
		if err := encoder.Encode(specs[i]); err != nil {
			return nil, nil, "", err
		}
	}
	return specs, opens, hex.EncodeToString(digest.Sum(nil)), nil
}

func loadOrCreateInputRoot(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string, target storage.PublicationTarget, fingerprint string, count uint64) (inputRoot, error) {
	var root inputRoot
	found, err := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputRootKey(operation), &root)
	if err != nil {
		return inputRoot{}, err
	}
	if !found {
		root = inputRoot{1, scope, target, operation, rand.Text(), fingerprint, count, StateStaging}
	}
	if !root.valid(scope, operation) || root.Target != target || root.Fingerprint != fingerprint || root.Count != count {
		return inputRoot{}, storage.ErrRecord
	}
	if root.State == StateStaging {
		if err := ctx.Err(); err != nil {
			return inputRoot{}, err
		}
		if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputRootKey(operation), root); err != nil {
			return inputRoot{}, err
		}
	}
	return root, nil
}

func captureInputMember(ctx context.Context, journal *offline.Store, root inputRoot, ordinal uint64, spec inputSpec, open func(context.Context) (io.ReadCloser, error)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	scope := root.Scope
	operation := root.Operation
	var member inputMember
	found, err := journal.Read(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputMemberKey(operation, ordinal), &member)
	if err != nil {
		return "", err
	}
	if !found {
		if root.State != StateStaging {
			return "", storage.ErrRecord
		}
		member = inputMember{Version: 1, Scope: scope, PlanID: root.ID, Fingerprint: root.Fingerprint, Ordinal: ordinal, Spec: spec}
		if spec.Content != nil {
			member.Source = &offline.StagedFile{ID: rand.Text(), SizeBytes: spec.Content.Request.SizeBytes, SHA256: spec.Content.SHA256}
		}
		if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputMemberKey(operation, ordinal), member); err != nil {
			return "", err
		}
	}
	if !member.valid(root, ordinal) || !inputSpecsEqual(member.Spec, spec) {
		return "", storage.ErrRecord
	}
	if err := stageInputMemberSource(ctx, journal, root, member, open); err != nil {
		return "", err
	}
	if err := validateInputRecord(ctx, journal, scope, member); err != nil {
		return "", err
	}
	member.Captured = true
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := journal.Write(atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), inputMemberKey(operation, ordinal), member); err != nil {
		return "", err
	}
	return inputMemberHash(member), nil
}

func stageInputMemberSource(ctx context.Context, journal *offline.Store, root inputRoot, member inputMember, open func(context.Context) (io.ReadCloser, error)) error {
	if member.Source == nil {
		return nil
	}
	scope := root.Scope
	_, err := journal.StageFileReserved(ctx, atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), *member.Source, nil)
	if err == nil {
		return nil
	}
	if member.Captured || root.State != StateStaging || !errors.Is(err, offline.ErrStagedFile) || open == nil {
		return err
	}
	source, err := open(ctx)
	if err != nil {
		return err
	}
	if source == nil {
		return storage.ErrRecord
	}
	_, captureErr := journal.StageFileReserved(ctx, atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), *member.Source, source)
	closeErr := source.Close()
	if captureErr != nil {
		return captureErr
	}
	return closeErr
}

// LoadInputs reconstructs only a fully captured local plan, never an accepted
// SDK proof. It revalidates the entire member stream and all original staged
// bytes. Missing/corrupt rows fail; no captured source is recreated or retargeted.
func LoadInputs(ctx context.Context, journal *offline.Store, scope storage.Scope, operation string) (InputPlan, error) {
	root, err := readInputRoot(ctx, journal, scope, operation)
	if err != nil {
		return InputPlan{}, err
	}
	if root.State != StateCaptured {
		return InputPlan{}, storage.ErrRecord
	}
	digest := sha256.New()
	encoder := json.NewEncoder(digest)
	objects := map[string]bool{}
	memberHashes := map[uint64]string{}
	for ordinal := uint64(0); ordinal < root.Count; ordinal++ {
		member, err := readInputMember(ctx, journal, root, ordinal)
		if err != nil {
			return InputPlan{}, err
		}
		memberHashes[ordinal] = inputMemberHash(member)
		if !member.Captured || objects[member.Spec.ObjectID] {
			return InputPlan{}, storage.ErrRecord
		}
		objects[member.Spec.ObjectID] = true
		if err := encoder.Encode(member.Spec); err != nil {
			return InputPlan{}, err
		}
		if err := validateInputRecord(ctx, journal, scope, member); err != nil {
			return InputPlan{}, err
		}
		if member.Source != nil {
			if _, err := journal.StageFileReserved(ctx, atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), *member.Source, nil); err != nil {
				return InputPlan{}, err
			}
		}
	}
	if hex.EncodeToString(digest.Sum(nil)) != root.Fingerprint {
		return InputPlan{}, storage.ErrRecord
	}
	current, err := readInputRoot(ctx, journal, scope, operation)
	if err != nil {
		return InputPlan{}, err
	}
	if current != root {
		return InputPlan{}, storage.ErrRecord
	}
	return InputPlan{&capturedInputPlan{journal: journal, root: root, members: memberHashes}}, nil
}

func (plan InputPlan) current(ctx context.Context) (*capturedInputPlan, error) {
	data := plan.data
	if data == nil {
		return nil, storage.ErrRecord
	}
	current, err := readInputRoot(ctx, data.journal, data.root.Scope, data.root.Operation)
	if err != nil {
		return nil, err
	}
	if current != data.root || current.State != StateCaptured {
		return nil, storage.ErrRecord
	}
	return data, nil
}

func (plan InputPlan) Count() uint64 {
	if plan.data == nil {
		return 0
	}
	return plan.data.root.Count
}

func (plan InputPlan) Target() (storage.PublicationTarget, error) {
	if plan.data == nil {
		return storage.PublicationTarget{}, storage.ErrRecord
	}
	return plan.data.root.Target, nil
}

func (plan InputPlan) ContentJob(ordinal uint64) (string, error) {
	if plan.data == nil || ordinal >= plan.Count() {
		return "", storage.ErrRecord
	}
	sum := sha256.Sum256([]byte(plan.data.root.ID + "\x00" + strconv.FormatUint(ordinal, 10)))
	return "atomic_content_" + hex.EncodeToString(sum[:]), nil
}

// OpenContent returns the existing bounded, authenticated random-read stream.
// It rechecks the plan and the exact reserved descriptor; callers own Close.
func (plan InputPlan) OpenContent(ctx context.Context, ordinal uint64) (*offline.StagedReader, error) {
	data, member, err := plan.readCapturedMember(ctx, ordinal)
	if err != nil {
		return nil, err
	}
	if member.Source == nil {
		return nil, storage.ErrRecord
	}
	if _, err := data.journal.StageFileReserved(ctx, atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), *member.Source, nil); err != nil {
		return nil, err
	}
	return data.journal.OpenStagedFile(ctx, atomicOwner(data.root.Scope), storage.AtomicJournalNamespace(data.root.Scope.Product), *member.Source)
}

func validateInputRecord(ctx context.Context, journal *offline.Store, scope storage.Scope, member inputMember) error {
	if member.Spec.Content == nil || member.Spec.Content.Kind != "record" {
		return nil
	}
	if member.Source == nil {
		return storage.ErrRecord
	}
	reader, err := journal.OpenStagedFile(ctx, atomicOwner(scope), storage.AtomicJournalNamespace(scope.Product), *member.Source)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(io.NewSectionReader(reader, 0, member.Source.SizeBytes))
	if err != nil {
		return err
	}
	defer clear(raw)
	request := member.Spec.Content.Request
	record := storage.Record{Version: 1, Scope: scope, ObjectID: member.Spec.ObjectID, RevisionID: request.RevisionID, Role: "content", Payload: raw, Previous: request.Previous}
	if err := record.Validate(scope); err != nil {
		return err
	}
	return ctx.Err()
}

func inputMemberHash(member inputMember) string {
	raw, err := json.Marshal(member)
	if err != nil {
		return ""
	}
	defer clear(raw)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (plan InputPlan) readCapturedMember(ctx context.Context, ordinal uint64) (*capturedInputPlan, inputMember, error) {
	data, err := plan.current(ctx)
	if err != nil {
		return nil, inputMember{}, err
	}
	member, err := readInputMember(ctx, data.journal, data.root, ordinal)
	if err != nil {
		return nil, inputMember{}, err
	}
	if !member.Captured || inputMemberHash(member) != data.members[ordinal] {
		return nil, inputMember{}, storage.ErrRecord
	}
	return data, member, nil
}
