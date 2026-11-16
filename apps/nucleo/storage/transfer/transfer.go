// SPDX-License-Identifier: Apache-2.0
// Package transfer preserves authenticated catalog history across a frozen
// identity boundary. Cryptography and file transport remain in the SDK.
package transfer

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
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrSource = errors.New("transfer: authenticated source or retained plan changed")

type (
	Resolve func(storage.Reference, string) (storage.Reference, error)
	Lookup  func(storage.Reference) (storage.ObjectVersion, bool)
	// Class puts prerequisites before dependent domain records. Receipts are always
	// withheld until every non-receipt revision has passed independent readback.
	Class struct{ Order uint8 }
	Rules interface {
		SourceComplete(context.Context) error
		TargetComplete(context.Context) error
		Classify(context.Context, storage.ObjectVersion, Lookup) (Class, error)
		Rewrite(context.Context, storage.ObjectVersion, storage.Record, Resolve) ([]byte, error)
		CheckPending(context.Context, storage.FrozenBinding, storage.Binding) error
	}
	// FileDocumentRules is optional. A positive bound selects structured file
	// content whose domain references must change across identity boundaries.
	// Ordinary files keep the byte-exact CopyFile path.
	FileDocumentRules interface {
		FileDocumentLimit(context.Context, storage.ObjectVersion) (int64, error)
		RewriteFileDocument(context.Context, storage.ObjectVersion, []byte, Resolve) ([]byte, error)
	}
)

type Config struct {
	Scope                                      storage.Scope
	Owner                                      string
	Namespace                                  string
	ReceiptKind                                string
	ReceiptType                                string
	Journal                                    *offline.Store
	Source, Target                             storage.RecordStore
	FrozenBindings                             func(context.Context) (storage.FrozenBinding, error)
	Rules                                      Rules
	PutObject                                  func(context.Context, storage.PublicationTarget, string, string, string, storage.ObjectCommit) (storage.ObjectVersion, error)
	PutRecord                                  func(context.Context, storage.PublicationTarget, string, storage.Record) (storage.Reference, error)
	CopyFile                                   func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error)
	SourceFileCommitment, TargetFileCommitment func(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error)
	VerifyFile                                 func(context.Context, storage.Reference, string, string) error
	AcknowledgeFile                            func(context.Context, string, storage.Reference) error
	PrepareTarget                              func(context.Context) error
	MarkReady                                  func(context.Context) error
	ReadFileDocument                           func(context.Context, storage.ObjectVersion, int64) ([]byte, error)
	PutFileDocument                            func(context.Context, storage.PublicationTarget, string, storage.FileRequest, io.ReaderAt) (storage.Reference, error)
}
type Plan struct {
	Version     int                   `json:"v"`
	Scope       storage.Scope         `json:"scope"`
	Source      storage.FrozenBinding `json:"source"`
	Destination storage.Binding       `json:"destination"`
	Inventory   string                `json:"inventory"`
	Count       int                   `json:"count"`
	Salt        string                `json:"salt"`
	Complete    bool                  `json:"complete"`
}

type Mapping struct {
	Version  int                `json:"v"`
	Plan     string             `json:"plan"`
	Source   storage.Reference  `json:"source"`
	Object   string             `json:"object"`
	Revision string             `json:"revision"`
	Role     string             `json:"role"`
	Input    string             `json:"input"`
	Result   *storage.Reference `json:"result,omitempty"`
}

type engine struct {
	cfg       Config
	state     Plan
	versions  map[storage.Reference]storage.ObjectVersion
	receipts  map[storage.Reference]bool
	documents map[storage.Reference]int64
}

func (c Config) validScope() bool {
	return c.Scope.Valid() && c.Owner == c.Scope.Tenant+"\x00"+c.Scope.Subject+"\x00"+c.Scope.Product && c.Namespace != "" && len(c.Namespace) <= 128 && !strings.ContainsRune(c.Namespace, 0) && c.ReceiptKind != "" && len(c.ReceiptKind) <= 128 && len(c.ReceiptType) <= 128 && c.Journal.Encrypted()
}

func (c Config) validPorts() bool {
	return c.Source != nil && c.Target != nil && c.FrozenBindings != nil && c.Rules != nil && c.PutObject != nil && c.PutRecord != nil && c.MarkReady != nil && c.PrepareTarget != nil
}

func (c Config) validFiles() bool {
	return c.CopyFile != nil && c.SourceFileCommitment != nil && c.TargetFileCommitment != nil && c.VerifyFile != nil && c.AcknowledgeFile != nil && (c.ReadFileDocument == nil) == (c.PutFileDocument == nil)
}

// Run must be called while both repository work leases and the destination's
// initialization journal lock are held. It never changes the source catalog.
func Run(ctx context.Context, cfg Config, frozen storage.FrozenBinding, destination storage.Binding) error {
	if !cfg.validScope() || !cfg.validPorts() || !cfg.validFiles() {
		return ErrSource
	}
	if !frozen.Valid() || !destination.Valid() || frozen.Binding.Scope != cfg.Scope || destination.Scope != cfg.Scope {
		return ErrSource
	}
	for _, room := range destination.Rooms {
		if slices.Contains(frozen.Binding.Rooms, room) {
			return ErrSource
		}
	}
	t := engine{cfg: cfg, receipts: map[storage.Reference]bool{}, documents: map[storage.Reference]int64{}}
	return t.run(ctx, frozen, destination)
}

func (t *engine) load(ctx context.Context, port storage.RecordStore) (storage.Binding, map[storage.Reference]storage.RecordEntry, error) {
	snapshot, err := storage.LoadCatalog(ctx, port, t.cfg.Scope)
	if errors.Is(err, storage.ErrCatalogSource) {
		err = ErrSource
	}
	return snapshot.Binding, snapshot.Entries, err
}

func (t *engine) readContent(ctx context.Context, version storage.ObjectVersion) (storage.Record, error) {
	pointer := version.Commit.Content
	if pointer == nil || pointer.Kind != "record" {
		return storage.Record{}, ErrSource
	}
	record, err := t.cfg.Source.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
	if err != nil {
		return storage.Record{}, err
	}
	if record.Validate(t.cfg.Scope) != nil || record.ObjectID != version.ObjectID || record.RevisionID != pointer.RevisionID || record.Role != "content" {
		clear(record.Payload)
		return storage.Record{}, ErrSource
	}
	return record, nil
}

func (t *engine) run(ctx context.Context, frozen storage.FrozenBinding, destination storage.Binding) error {
	history, inventory, err := t.captureSource(ctx, frozen)
	if err != nil {
		return err
	}
	ordered, err := t.preflight(ctx, history)
	if err != nil {
		return err
	}
	if err := t.begin(frozen, destination, inventory, len(history)); err != nil {
		return err
	}
	if err := t.cfg.PrepareTarget(ctx); err != nil {
		return err
	}
	if err := t.copyHistory(ctx, ordered, inventory, frozen, destination); err != nil {
		return err
	}
	if err := t.verify(ctx, history, inventory); err != nil {
		return err
	}
	if err := t.cfg.Rules.TargetComplete(ctx); err != nil {
		return err
	}
	if err := t.acknowledgeFiles(ctx, history); err != nil {
		return err
	}
	final, err := t.cfg.FrozenBindings(ctx)
	if err != nil {
		return err
	}
	if !t.state.Source.Same(final) {
		return ErrSource
	}
	if err := t.cfg.Rules.CheckPending(ctx, final, destination); err != nil {
		return err
	}
	t.state.Complete = true
	if err := t.cfg.Journal.Write(t.cfg.Owner, t.cfg.Namespace, "plan", t.state); err != nil {
		return err
	}
	return t.cfg.MarkReady(ctx)
}

func (t *engine) captureSource(ctx context.Context, frozen storage.FrozenBinding) ([]storage.ObjectVersion, string, error) {
	binding, entries, err := t.load(ctx, t.cfg.Source)
	if err != nil {
		return nil, "", err
	}
	if !sameFrozenRooms(frozen.Binding, binding) {
		return nil, "", ErrSource
	}
	history, inventory, err := history(t.cfg.Scope, entries)
	if err != nil {
		return nil, "", err
	}
	t.versions = make(map[storage.Reference]storage.ObjectVersion, len(history))
	for _, version := range history {
		t.versions[version.Reference] = version
	}
	if err := t.cfg.Rules.SourceComplete(ctx); err != nil {
		return nil, "", err
	}
	return history, inventory, nil
}

func (t *engine) preflight(ctx context.Context, history []storage.ObjectVersion) ([]storage.ObjectVersion, error) {
	type classified struct {
		version storage.ObjectVersion
		class   Class
		receipt bool
	}
	entries := make([]classified, 0, len(history))
	lookup := func(ref storage.Reference) (storage.ObjectVersion, bool) { v, ok := t.versions[ref]; return v, ok }
	for _, version := range history {
		class, err := t.cfg.Rules.Classify(ctx, version, lookup)
		if err != nil {
			return nil, err
		}
		if err := t.classifyFileDocument(ctx, version); err != nil {
			return nil, err
		}
		var header struct {
			Kind string `json:"kind"`
			Type string `json:"type"`
		}
		if json.Unmarshal(version.Commit.Metadata, &header) != nil {
			return nil, ErrSource
		}
		receipt := header.Kind == t.cfg.ReceiptKind && (t.cfg.ReceiptType == "" || header.Type == t.cfg.ReceiptType)
		entries = append(entries, classified{version, class, receipt})
		t.receipts[version.Reference] = receipt
	}
	slices.SortStableFunc(entries, func(a, b classified) int {
		if a.receipt != b.receipt {
			if a.receipt {
				return 1
			}
			return -1
		}
		return int(a.class.Order) - int(b.class.Order)
	})
	ordered := make([]storage.ObjectVersion, 0, len(entries))
	for _, entry := range entries {
		ordered = append(ordered, entry.version)
	}
	return ordered, nil
}

func (t *engine) copyHistory(ctx context.Context, ordered []storage.ObjectVersion, inventory string, frozen storage.FrozenBinding, destination storage.Binding) error {
	data, receipts := []storage.ObjectVersion{}, []storage.ObjectVersion{}
	for _, version := range ordered {
		if t.receipts[version.Reference] {
			receipts = append(receipts, version)
		} else {
			data = append(data, version)
		}
	}
	if err := t.cfg.Rules.CheckPending(ctx, frozen, destination); err != nil {
		return err
	}
	for _, version := range data {
		if err := t.copyVersion(ctx, version); err != nil {
			return err
		}
	}
	if err := t.verify(ctx, data, inventory); err != nil {
		return err
	}
	fresh, err := t.cfg.FrozenBindings(ctx)
	if err != nil {
		return err
	}
	if !t.state.Source.Same(fresh) {
		return ErrSource
	}
	if err := t.cfg.Rules.CheckPending(ctx, fresh, destination); err != nil {
		return err
	}
	for _, version := range receipts {
		if err := t.copyVersion(ctx, version); err != nil {
			return err
		}
	}
	return nil
}
func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}

// Transaction preserves the existing per-plan idempotency keys across upgrades.
func Transaction(plan Plan, ref storage.Reference, role string) string {
	return "transfer_" + digest([]byte(plan.Salt+"\x00"+ref.RoomID+"\x00"+ref.EventID+"\x00"+role))
}

func sameFrozenRooms(a, b storage.Binding) bool {
	a.Target.DeviceID = b.Target.DeviceID
	return a.Same(b)
}

func history(scope storage.Scope, entries map[storage.Reference]storage.RecordEntry) ([]storage.ObjectVersion, string, error) {
	refs := make([]storage.Reference, 0, len(entries))
	for ref := range entries {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b storage.Reference) int {
		if a.RoomID < b.RoomID {
			return -1
		}
		if a.RoomID > b.RoomID {
			return 1
		}
		if a.EventID < b.EventID {
			return -1
		}
		if a.EventID > b.EventID {
			return 1
		}
		return 0
	})
	ordered := make([]storage.RecordEntry, 0, len(refs))
	for _, ref := range refs {
		ordered = append(ordered, entries[ref])
	}
	history, err := storage.CatalogHistory(scope, ordered)
	if err != nil {
		return nil, "", err
	}
	encoded, err := json.Marshal(ordered)
	if err != nil {
		return nil, "", err
	}
	defer clear(encoded)
	return history, digest(encoded), nil
}

func (t *engine) begin(source storage.FrozenBinding, destination storage.Binding, inventory string, count int) error {
	found, err := t.cfg.Journal.Read(t.cfg.Owner, t.cfg.Namespace, "plan", &t.state)
	if err != nil {
		return err
	}
	if !found {
		t.state = Plan{Version: 1, Scope: t.cfg.Scope, Source: source, Destination: destination, Inventory: inventory, Count: count, Salt: rand.Text()}
	}
	s := t.state
	if s.Version != 1 || s.Scope != t.cfg.Scope || !s.Source.Same(source) || !s.Destination.Same(destination) || s.Inventory != inventory || s.Count != count || count == 0 || !storage.Identifier(s.Salt) {
		return ErrSource
	}
	if err := t.cfg.Target.CheckStoragePublicationTarget(s.Destination.Target); err != nil {
		return err
	}
	return t.cfg.Journal.Write(t.cfg.Owner, t.cfg.Namespace, "plan", s)
}

func (t *engine) transaction(ref storage.Reference, role string) string {
	return Transaction(t.state, ref, role)
}

func (t *engine) mapping(ref storage.Reference, object, revision, role, input string) (Mapping, error) {
	expected := Mapping{Version: 1, Plan: t.state.Salt, Source: ref, Object: object, Revision: revision, Role: role, Input: input}
	var work Mapping
	found, err := t.cfg.Journal.Read(t.cfg.Owner, t.cfg.Namespace, t.transaction(ref, role), &work)
	if err != nil {
		return work, err
	}
	if !found {
		work = expected
	}
	result := work.Result
	work.Result = nil
	actual, _ := json.Marshal(work)
	wanted, _ := json.Marshal(expected)
	if !bytes.Equal(actual, wanted) || result != nil && (!result.Valid() || result.RoomID != t.state.Destination.Target.RoomID) {
		return work, ErrSource
	}
	work.Result = result
	if err := t.cfg.Journal.Write(t.cfg.Owner, t.cfg.Namespace, t.transaction(ref, role), work); err != nil {
		return work, err
	}
	return work, nil
}

func (t *engine) saveMapping(work Mapping, ref storage.Reference) error {
	if !ref.Valid() || ref.RoomID != t.state.Destination.Target.RoomID || work.Result != nil && *work.Result != ref {
		return ErrSource
	}
	work.Result = &ref
	return t.cfg.Journal.Write(t.cfg.Owner, t.cfg.Namespace, t.transaction(work.Source, work.Role), work)
}

func (t *engine) indexMapping(version storage.ObjectVersion) (storage.Reference, error) {
	input, err := json.Marshal(version)
	if err != nil {
		return storage.Reference{}, err
	}
	work, err := t.mapping(version.Reference, version.ObjectID, version.RevisionID, "index", digest(input))
	if err != nil {
		return storage.Reference{}, err
	}
	if work.Result == nil {
		return storage.Reference{}, ErrSource
	}
	return *work.Result, nil
}

func (t *engine) dependency(ref storage.Reference, object string) (storage.Reference, error) {
	version, found := t.versions[ref]
	if !found || version.ObjectID != object {
		return storage.Reference{}, ErrSource
	}
	return t.indexMapping(version)
}

func (t *engine) copyVersion(ctx context.Context, version storage.ObjectVersion) error {
	commit := version.Commit
	commit.Parents = slices.Clone(commit.Parents)
	for i, parent := range commit.Parents {
		ref, err := t.dependency(parent, version.ObjectID)
		if err != nil {
			return err
		}
		commit.Parents[i] = ref
	}
	if commit.Content != nil {
		pointer := *commit.Content
		var ref storage.Reference
		var err error
		if pointer.Kind == "file" {
			ref, err = t.copyBody(ctx, version)
		} else {
			ref, err = t.copyRecord(ctx, version)
		}
		if err != nil {
			return err
		}
		pointer.Reference = ref
		commit.Content = &pointer
	}
	input, err := json.Marshal(version)
	if err != nil {
		return err
	}
	work, err := t.mapping(version.Reference, version.ObjectID, version.RevisionID, "index", digest(input))
	if err != nil {
		return err
	}
	copied, err := t.cfg.PutObject(ctx, t.state.Destination.Target, t.transaction(version.Reference, "index"), version.ObjectID, version.RevisionID, commit)
	if err != nil {
		return err
	}
	return t.saveMapping(work, copied.Reference)
}

func (t *engine) copyBody(ctx context.Context, version storage.ObjectVersion) (storage.Reference, error) {
	if t.documents[version.Reference] > 0 {
		return t.copyFileDocument(ctx, version)
	}
	pointer := version.Commit.Content
	request, sum, err := t.cfg.SourceFileCommitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return storage.Reference{}, err
	}
	if request.Validate(t.state.Scope) != nil || len(request.Previous) != 0 || !validHash(sum) {
		return storage.Reference{}, ErrSource
	}
	input, err := json.Marshal(struct {
		Request storage.FileRequest
		SHA256  string
	}{request, sum})
	if err != nil {
		return storage.Reference{}, err
	}
	work, err := t.mapping(pointer.Reference, version.ObjectID, pointer.RevisionID, "file", digest(input))
	if err != nil {
		return storage.Reference{}, err
	}
	var ref storage.Reference
	if work.Result == nil {
		ref, err = t.cfg.CopyFile(ctx, t.state.Destination.Target, t.transaction(pointer.Reference, "file"), pointer.Reference, version.ObjectID, pointer.RevisionID)
		if err != nil {
			return storage.Reference{}, err
		}
	} else {
		ref = *work.Result
	}
	actual, actualSum, err := t.cfg.TargetFileCommitment(ctx, ref, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return storage.Reference{}, err
	}
	expectedBytes, _ := json.Marshal(request)
	actualBytes, _ := json.Marshal(actual)
	if sum != actualSum || !bytes.Equal(expectedBytes, actualBytes) {
		return storage.Reference{}, ErrSource
	}
	if err := t.saveMapping(work, ref); err != nil {
		return storage.Reference{}, err
	}
	return ref, nil
}

func (t *engine) copyRecord(ctx context.Context, version storage.ObjectVersion) (storage.Reference, error) {
	pointer := version.Commit.Content
	record, input, err := t.rewrittenRecord(ctx, version)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(record.Payload)
	work, err := t.mapping(pointer.Reference, version.ObjectID, pointer.RevisionID, "content", input)
	if err != nil {
		return storage.Reference{}, err
	}
	ref, err := t.cfg.PutRecord(ctx, t.state.Destination.Target, t.transaction(pointer.Reference, "content"), record)
	if err != nil {
		return storage.Reference{}, err
	}
	if err := t.saveMapping(work, ref); err != nil {
		return storage.Reference{}, err
	}
	return ref, nil
}

func (t *engine) verify(ctx context.Context, history []storage.ObjectVersion, inventory string) error {
	if err := t.verifySource(ctx, inventory); err != nil {
		return err
	}
	destination, copied, err := t.load(ctx, t.cfg.Target)
	if err != nil {
		return err
	}
	if !t.state.Destination.Same(destination) || len(copied) != len(history) {
		return ErrSource
	}
	verifiedFiles := map[storage.Reference]bool{}
	for _, version := range history {
		pointer, err := t.verifyIndex(ctx, version, copied)
		if err != nil {
			return err
		}
		if pointer.Kind != "file" || verifiedFiles[pointer.Reference] {
			continue
		}
		verifiedFiles[pointer.Reference] = true
		if err := t.cfg.VerifyFile(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID); err != nil {
			return err
		}
	}
	return nil
}

// Recompute the expected links from authenticated source history and the pinned
// per-record mappings. Existence of the destination event alone is insufficient.

func (t *engine) expectedCommit(ctx context.Context, version storage.ObjectVersion) (storage.ObjectCommit, error) {
	commit := version.Commit
	commit.Parents = slices.Clone(commit.Parents)
	for i, parent := range commit.Parents {
		ref, err := t.dependency(parent, version.ObjectID)
		if err != nil {
			return storage.ObjectCommit{}, err
		}
		commit.Parents[i] = ref
	}
	if commit.Content == nil {
		return commit, nil
	}
	pointer := *commit.Content
	var ref storage.Reference
	var err error
	if pointer.Kind == "file" {
		ref, err = t.expectedFileReference(ctx, version)
	} else {
		ref, err = t.expectedRecordReference(ctx, version)
	}
	if err != nil {
		return storage.ObjectCommit{}, err
	}
	pointer.Reference = ref
	commit.Content = &pointer
	return commit, nil
}

// A frozen remote catalog is not evidence that this device has no unpublished
// work. Retain it and refuse cutover until the original device has settled it.

func (t *engine) acknowledgeFiles(ctx context.Context, history []storage.ObjectVersion) error {
	seen := map[storage.Reference]bool{}
	for _, version := range history {
		pointer := version.Commit.Content
		if pointer == nil || pointer.Kind != "file" || seen[pointer.Reference] {
			continue
		}
		seen[pointer.Reference] = true
		result, err := t.indexMapping(version)
		if err != nil {
			return err
		}
		job := t.transaction(pointer.Reference, "file")
		err = t.cfg.AcknowledgeFile(ctx, job, result)
		if err != nil && !errors.Is(err, storage.ErrUploadMissing) {
			return err
		}
	}
	return nil
}

func (t *engine) verifySource(ctx context.Context, inventory string) error {
	complete, err := t.cfg.FrozenBindings(ctx)
	if err != nil {
		return err
	}
	if !t.state.Source.Same(complete) {
		return ErrSource
	}
	binding, entries, err := t.load(ctx, t.cfg.Source)
	if err != nil {
		return err
	}
	_, current, err := history(t.state.Scope, entries)
	if err != nil {
		return err
	}
	if current != inventory || !sameFrozenRooms(t.state.Source.Binding, binding) {
		return ErrSource
	}
	return nil
}

func (t *engine) verifyIndex(ctx context.Context, version storage.ObjectVersion, copied map[storage.Reference]storage.RecordEntry) (storage.ContentPointer, error) {
	ref, err := t.indexMapping(version)
	if err != nil {
		return storage.ContentPointer{}, err
	}
	entry, exists := copied[ref]
	if !exists {
		return storage.ContentPointer{}, ErrSource
	}
	expected, err := t.expectedCommit(ctx, version)
	if err != nil {
		return storage.ContentPointer{}, err
	}
	actual, err := storage.DecodeObject(entry, t.state.Scope)
	if err != nil || actual.ObjectID != version.ObjectID || actual.RevisionID != version.RevisionID {
		return storage.ContentPointer{}, ErrSource
	}
	expectedJSON, _ := json.Marshal(expected)
	actualJSON, _ := json.Marshal(actual.Commit)
	if !bytes.Equal(expectedJSON, actualJSON) {
		return storage.ContentPointer{}, ErrSource
	}
	if actual.Commit.Content == nil || actual.Commit.Content.Kind != "file" {
		return storage.ContentPointer{}, nil
	}
	return *actual.Commit.Content, nil
}

func (t *engine) rewrittenRecord(ctx context.Context, version storage.ObjectVersion) (storage.Record, string, error) {
	original, err := t.readContent(ctx, version)
	if err != nil {
		return storage.Record{}, "", err
	}
	fail := func(err error) (storage.Record, string, error) {
		clear(original.Payload)
		return storage.Record{}, "", err
	}
	if len(original.Previous) != 0 {
		return fail(ErrSource)
	}
	input, err := json.Marshal(original)
	if err != nil {
		return fail(err)
	}
	sum := digest(input)
	clear(input)
	payload, err := t.cfg.Rules.Rewrite(ctx, version, original, t.dependency)
	if err != nil {
		return fail(err)
	}
	rewritten := bytes.Clone(payload)
	clear(original.Payload)
	clear(payload)
	original.Payload = rewritten
	if original.Validate(t.cfg.Scope) != nil {
		return fail(ErrSource)
	}
	return original, sum, nil
}

func (t *engine) expectedFileReference(ctx context.Context, version storage.ObjectVersion) (storage.Reference, error) {
	if t.documents[version.Reference] > 0 {
		return t.expectedFileDocumentReference(ctx, version)
	}
	pointer := version.Commit.Content
	request, sum, err := t.cfg.SourceFileCommitment(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return storage.Reference{}, err
	}
	input, err := json.Marshal(struct {
		Request storage.FileRequest
		SHA256  string
	}{request, sum})
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(input)
	work, err := t.mapping(pointer.Reference, version.ObjectID, pointer.RevisionID, "file", digest(input))
	if err != nil {
		return storage.Reference{}, err
	}
	if work.Result == nil {
		return storage.Reference{}, ErrSource
	}
	return *work.Result, nil
}

func (t *engine) expectedRecordReference(ctx context.Context, version storage.ObjectVersion) (storage.Reference, error) {
	original, input, err := t.rewrittenRecord(ctx, version)
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(original.Payload)
	pointer := version.Commit.Content
	work, err := t.mapping(pointer.Reference, version.ObjectID, pointer.RevisionID, "content", input)
	if err != nil {
		return storage.Reference{}, err
	}
	if work.Result == nil {
		return storage.Reference{}, ErrSource
	}
	copied, err := t.cfg.Target.ReadStorageRecord(ctx, *work.Result, version.ObjectID, "content")
	if err != nil {
		return storage.Reference{}, err
	}
	defer clear(copied.Payload)
	if copied.Validate(t.state.Scope) != nil || copied.ObjectID != original.ObjectID || copied.RevisionID != original.RevisionID || copied.Role != original.Role || !slices.Equal(copied.Previous, original.Previous) || !bytes.Equal(copied.Payload, original.Payload) {
		return storage.Reference{}, ErrSource
	}
	return *work.Result, nil
}

// MatchesAcknowledgment ties journal retirement to the exact retained public
// cutover and authenticated destination receipt. It grants no publication.
func (plan Plan) MatchesAcknowledgment(scope storage.Scope, state storage.IdentityCutoverState, receipt storage.Reference) bool {
	return plan.Complete && plan.Version == 1 && plan.Scope == scope && plan.Source.Valid() && plan.Destination.Valid() && plan.Source.Binding.Scope == scope && plan.Destination.Scope == scope && plan.Count > 0 && validHash(plan.Inventory) && storage.Identifier(plan.Salt) && slices.Equal(slices.Sorted(slices.Values(plan.Source.Binding.Rooms)), slices.Sorted(slices.Values(state.SourceRooms))) && plan.Destination.Target.RoomID == receipt.RoomID
}
