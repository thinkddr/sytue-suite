// SPDX-License-Identifier: Apache-2.0
// Package records owns exact application JSON on verified devices. Cryptography
// remains in the maintained adapter; domain rules are supplied by each app.
package records

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const (
	MaxEntityJSON = 32 << 20
)

var (
	ErrSource   = errors.New("records: private source is invalid or changed")
	ErrNotReady = errors.New("records: verified private import is not complete")
	ErrClosed   = errors.New("records: private repository is closed")
	ErrConflict = errors.New("records: private revisions require resolution")
	ErrNotFound = errors.New("records: private resource not found")
	ErrShared   = errors.New("records: shared data requires encrypted recipient access")
)

// Crypto is the existing maintained adapter. All secrets and cryptography stay
// there. Notes defines domain JSON and its revision relationships only.
type Crypto interface {
	storage.RecordStore
	StorageIdentityVersion() storage.IdentityVersion
	UploadStorageFileAt(context.Context, *api.Drive, *offline.Store, string, storage.PublicationTarget, storage.FileRequest, io.ReaderAt) (storage.Reference, error)
	AcknowledgeStorageFile(context.Context, *offline.Store, string, storage.Reference) error
	DownloadStorageFile(context.Context, *api.Drive, storage.Reference, string, string, io.Writer) (storage.FileRequest, error)
	StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error)
}

type Config struct {
	// Bound only to the original admitted operation; historical readers keep nil.
	BindPublicationContext offline.WorkContextBinder
	WorkLease              offline.WorkLease
	Context                context.Context
	Scope                  storage.Scope
	Crypto                 Crypto
	Gateway                *api.Drive
	Journal                *offline.Store
	Source                 Source
	Product                string
	EntityKind             string
	ReceiptKind            string
	ValidateRow            func(context.Context, string, string, []byte) error
	ValidateCatalog        func(context.Context, Catalog) error
	// BinaryKind and ValidateBinary are an optional pair. Existing JSON-only
	// consumers reject binary snapshots rather than interpreting bytes as JSON.
	BinaryKind     func(string) bool
	ValidateBinary func(context.Context, SnapshotRow) error
	// OnWrite, opcional, se llama tras cada mutación aceptada (Mutate y sus variantes),
	// fuera del cerrojo del diario y sin datos: es el aviso al widget nativo de que su app
	// acaba de escribir en el repositorio privado, que no pasa por HTTP (ADR 0078). Tiene
	// que volver enseguida. Los lectores históricos lo dejan a nil.
	OnWrite func()
}

type Repository struct {
	cfg           Config
	gate          *offline.WorkGate
	mu            sync.Mutex
	closed, ready bool
	binding       storage.Binding
	catalog       storage.AtomicCatalogSnapshot
	entries       map[storage.Reference]storage.RecordEntry
	byObject      map[string][]storage.ObjectVersion
	heads         map[string][]storage.ObjectVersion
}

type entity struct {
	Kind     string `json:"kind"`
	Entity   string `json:"entity"`
	ID       string `json:"id"`
	SHA256   string `json:"sha256"`
	Epoch    string `json:"epoch,omitempty"`
	Baseline string `json:"baseline,omitempty"`
	Seed     bool   `json:"seed,omitempty"`
	Binary   bool   `json:"binary,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type content struct {
	Kind string `json:"kind"`
	JSON []byte `json:"json"`
}

func New(cfg Config) (*Repository, error) {
	return newRepository(cfg, storage.IsolatedIdentity)
}

func newRepository(cfg Config, identity storage.IdentityVersion) (*Repository, error) {
	if cfg.Context == nil || !cfg.Scope.Valid() || cfg.Scope.Product != cfg.Product || cfg.Product == "" || cfg.EntityKind == "" || cfg.ReceiptKind == "" || cfg.EntityKind == cfg.ReceiptKind || cfg.ValidateRow == nil || cfg.ValidateCatalog == nil || cfg.Crypto == nil || cfg.Gateway == nil || cfg.Source == nil || !cfg.Journal.Encrypted() {
		return nil, ErrSource
	}
	if cfg.Crypto.StorageIdentityVersion() != identity {
		return nil, ErrSource
	}
	if (cfg.BinaryKind == nil) != (cfg.ValidateBinary == nil) {
		return nil, ErrSource
	}
	if cfg.BinaryKind != nil && (cfg.Product == "drive" || cfg.Product == "mail") {
		// These two products own separate existing import/staging lifecycles.
		return nil, ErrSource
	}
	return &Repository{cfg: cfg, gate: offline.NewWorkGateWithLease(cfg.Context, cfg.WorkLease)}, nil
}

func (r *Repository) PrivateStorageScope() storage.Scope { return r.cfg.Scope }
func (r *Repository) owner() string {
	return r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00" + r.cfg.Scope.Product
}

func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (r *Repository) object(kind, id string) string {
	return r.cfg.Product + "_" + hashBytes([]byte(kind+"\x00"+id))
}

func (r *Repository) Start(parent context.Context) (context.Context, func(), error) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return nil, nil, ErrClosed
	}
	ctx, done, err := r.gate.StartBound(parent, r.cfg.BindPublicationContext)
	if errors.Is(err, offline.ErrWorkClosed) {
		return nil, nil, ErrClosed
	}
	return ctx, done, err
}

func (r *Repository) Close() {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	if r.gate != nil {
		r.gate.Close()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, heads := range r.heads {
		for _, version := range heads {
			clear(version.Commit.Metadata)
		}
	}
	for _, version := range r.catalog.History {
		clear(version.Commit.Metadata)
	}
	for _, versions := range r.byObject {
		for _, version := range versions {
			clear(version.Commit.Metadata)
		}
	}
	r.heads = nil
	r.catalog = storage.AtomicCatalogSnapshot{}
	r.entries = nil
	r.byObject = nil
	r.ready = false
}

func (r *Repository) refresh(ctx context.Context) error {
	snapshot, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if errors.Is(err, storage.ErrCatalogSource) {
		return ErrSource
	}
	if err != nil {
		return err
	}
	heads := map[string][]storage.ObjectVersion{}
	for _, version := range snapshot.Heads {
		heads[version.ObjectID] = append(heads[version.ObjectID], version)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r.binding = snapshot.Binding
	r.heads = heads
	r.catalog = snapshot
	r.entries = snapshot.Entries
	r.byObject = make(map[string][]storage.ObjectVersion)
	for _, version := range snapshot.History {
		r.byObject[version.ObjectID] = append(r.byObject[version.ObjectID], cloneVersion(version))
	}
	return nil
}

func (r *Repository) ReadyVersion(kind, id string) (storage.ObjectVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return storage.ObjectVersion{}, ErrClosed
	}
	if !r.ready {
		return storage.ObjectVersion{}, ErrNotReady
	}
	return r.versionLocked(kind, id)
}

func (r *Repository) versionLocked(kind, id string) (storage.ObjectVersion, error) {
	versions := r.heads[r.object(kind, id)]
	if len(versions) == 0 {
		return storage.ObjectVersion{}, ErrNotFound
	}
	if len(versions) != 1 {
		return storage.ObjectVersion{}, ErrConflict
	}
	return cloneVersion(versions[0]), nil
}

func (r *Repository) description(version storage.ObjectVersion) (entity, error) {
	var value entity
	if json.Unmarshal(version.Commit.Metadata, &value) != nil || value.Kind != r.cfg.EntityKind || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: value.Entity, ID: value.ID}) || value.SHA256 == "" || len(value.SHA256) != 64 || version.ObjectID != r.object(value.Entity, value.ID) {
		return value, ErrSource
	}
	if value.Binary != r.binaryKind(value.Entity) || value.Size < 0 || value.Size > storage.MaxFileBytes || !value.Binary && value.Size != 0 {
		return value, ErrSource
	}
	return value, nil
}

func (r *Repository) readRecordJSON(ctx context.Context, version storage.ObjectVersion, pointer storage.ContentPointer) ([]byte, error) {
	record, err := r.cfg.Crypto.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
	if err != nil {
		return nil, err
	}
	defer clear(record.Payload)
	var value content
	if record.Validate(r.cfg.Scope) != nil || record.ObjectID != version.ObjectID || record.RevisionID != pointer.RevisionID || record.Role != "content" || json.Unmarshal(record.Payload, &value) != nil || value.Kind != r.cfg.EntityKind {
		return nil, ErrSource
	}
	return value.JSON, nil
}

func (r *Repository) readFileJSON(ctx context.Context, version storage.ObjectVersion, pointer storage.ContentPointer) ([]byte, error) {
	details, err := r.cfg.Crypto.StorageFileInfo(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return nil, err
	}
	if details.SizeBytes < 0 || details.SizeBytes > MaxEntityJSON {
		return nil, ErrSource
	}
	var output bytes.Buffer
	request, err := r.cfg.Crypto.DownloadStorageFile(ctx, r.cfg.Gateway, pointer.Reference, version.ObjectID, pointer.RevisionID, boundedJSONWriter{buffer: &output})
	if err != nil {
		clear(output.Bytes())
		return nil, err
	}
	if request.ObjectID != version.ObjectID || request.RevisionID != pointer.RevisionID || request.SizeBytes != details.SizeBytes || int64(output.Len()) != details.SizeBytes {
		clear(output.Bytes())
		return nil, ErrSource
	}
	return output.Bytes(), nil
}

func (r *Repository) readJSON(ctx context.Context, version storage.ObjectVersion) ([]byte, error) {
	description, err := r.description(version)
	if err != nil {
		return nil, err
	}
	if description.Binary {
		return nil, ErrSource
	}
	pointer := version.Commit.Content
	if pointer == nil {
		return nil, ErrNotFound
	}
	var data []byte
	switch pointer.Kind {
	case "record":
		data, err = r.readRecordJSON(ctx, version, *pointer)
	case "file":
		data, err = r.readFileJSON(ctx, version, *pointer)
	default:
		return nil, ErrSource
	}
	if err != nil {
		return nil, err
	}
	if len(data) > MaxEntityJSON || !json.Valid(data) || hashBytes(data) != description.SHA256 {
		clear(data)
		return nil, ErrSource
	}
	if err := r.cfg.ValidateRow(ctx, description.Entity, description.ID, data); err != nil {
		clear(data)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		clear(data)
		return nil, err
	}
	return data, nil
}

type publication struct {
	Version  int                       `json:"v"`
	Scope    storage.Scope             `json:"scope"`
	Kind     string                    `json:"kind"`
	ID       string                    `json:"id"`
	Revision string                    `json:"revision"`
	Input    string                    `json:"input"`
	State    string                    `json:"state"`
	Target   storage.PublicationTarget `json:"target"`
	Parents  []storage.Reference       `json:"parents"`
	Content  *storage.Reference        `json:"content,omitempty"`
	Index    *storage.Reference        `json:"index,omitempty"`
	Epoch    string                    `json:"epoch,omitempty"`
	Baseline string                    `json:"baseline,omitempty"`
	Seed     bool                      `json:"seed,omitempty"`
	File     bool                      `json:"file,omitempty"`
	Binary   bool                      `json:"binary,omitempty"`
	Size     int64                     `json:"size,omitempty"`
	Staged   *offline.StagedFile       `json:"staged,omitempty"`
}

func (r *Repository) publicationWork(ctx context.Context, key, kind, id, state string, raw []byte, baseline *entity, observed *mutationObservation) (publication, error) {
	if r.binaryKind(kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) || len(raw) > MaxEntityJSON || !json.Valid(raw) || (state != "live" && state != "trash") {
		return publication{}, ErrSource
	}
	var work publication
	found, err := r.cfg.Journal.Read(r.owner(), r.namespace(), key, &work)
	if err != nil {
		return work, err
	}
	if !found {
		work, err = r.newPublication(kind, id, state, raw, baseline, observed)
		if err != nil {
			return work, err
		}
		if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
			return work, err
		}
	}
	if err := work.validate(r.cfg.Scope, kind, id, state, raw, baseline); err != nil {
		return work, err
	}
	if observed != nil && (work.Target != observed.Target || !slices.Equal(work.Parents, observed.Parents)) {
		return work, ErrSource
	}
	if err := ctx.Err(); err != nil {
		return work, err
	}
	return work, r.cfg.Crypto.CheckStoragePublicationTarget(work.Target)
}

func (work publication) validate(scope storage.Scope, kind, id, state string, raw []byte, baseline *entity) error {
	if work.Content != nil && (!work.Content.Valid() || work.Content.RoomID != work.Target.RoomID) {
		return ErrSource
	}
	if work.Binary || work.Size != 0 || work.Staged != nil || work.Version != 1 || work.Scope != scope || work.Kind != kind || work.ID != id || work.Input != hashBytes(raw) || work.State != state || !storage.Identifier(work.Revision) || work.File != (len(raw) > 16<<10) {
		return ErrSource
	}
	if baseline != nil && (work.Epoch != baseline.Epoch || work.Baseline != baseline.Baseline || work.Seed != baseline.Seed) {
		return ErrSource
	}
	return nil
}

func (r *Repository) newPublication(kind, id, state string, raw []byte, baseline *entity, observed *mutationObservation) (publication, error) {
	work, err := r.newPublicationHeader(kind, id, state, baseline, observed)
	if err != nil {
		return publication{}, err
	}
	work.Input, work.File = hashBytes(raw), len(raw) > 16<<10
	return work, nil
}

// newPublicationHeader plans against the current heads and target, or, for
// retained intent, against exactly what its prepare observed. A changed target
// is a conflict: the original device's publication cannot be adopted here.
func (r *Repository) newPublicationHeader(kind, id, state string, baseline *entity, observed *mutationObservation) (publication, error) {
	r.mu.Lock()
	obj := r.object(kind, id)
	parents := slices.Clone(r.heads[obj])
	target := r.binding.Target
	if observed != nil {
		if target != observed.Target {
			r.mu.Unlock()
			return publication{}, ErrConflict
		}
		parents = parents[:0]
		for _, ref := range observed.Parents {
			index := slices.IndexFunc(r.byObject[obj], func(version storage.ObjectVersion) bool { return version.Reference == ref })
			if index < 0 {
				r.mu.Unlock()
				return publication{}, ErrSource
			}
			parents = append(parents, cloneVersion(r.byObject[obj][index]))
		}
	}
	r.mu.Unlock()
	if len(parents) > 1 {
		return publication{}, ErrConflict
	}
	refs := make([]storage.Reference, len(parents))
	for i, parent := range parents {
		refs[i] = parent.Reference
	}
	lineage := entity{}
	if baseline != nil {
		lineage = *baseline
	} else if len(parents) == 1 {
		previous, err := r.description(parents[0])
		if err != nil {
			return publication{}, err
		}
		lineage.Epoch, lineage.Baseline = previous.Epoch, previous.Baseline
	}
	return publication{Version: 1, Scope: r.cfg.Scope, Kind: kind, ID: id, Revision: uuid.NewString(), State: state, Target: target, Parents: refs, Epoch: lineage.Epoch, Baseline: lineage.Baseline, Seed: lineage.Seed}, nil
}

func (r *Repository) publishContent(ctx context.Context, key string, work *publication, raw []byte) error {
	if work.Content != nil {
		return nil
	}
	obj := r.object(work.Kind, work.ID)
	var reference storage.Reference
	var err error
	if work.File {
		metadata, _ := json.Marshal(map[string]string{"kind": r.cfg.EntityKind, "entity": work.Kind, "id": work.ID})
		reference, err = r.cfg.Crypto.UploadStorageFileAt(ctx, r.cfg.Gateway, r.cfg.Journal, key+"_file", work.Target, storage.FileRequest{ObjectID: obj, RevisionID: work.Revision, SizeBytes: int64(len(raw)), Metadata: metadata}, bytes.NewReader(raw))
	} else {
		payload, _ := json.Marshal(content{Kind: r.cfg.EntityKind, JSON: raw})
		defer clear(payload)
		reference, err = r.cfg.Crypto.PutStorageRecordAt(ctx, work.Target, work.Revision+"_content", storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: obj, RevisionID: work.Revision, Role: "content", Payload: payload})
	}
	if err != nil {
		return err
	}
	if !reference.Valid() || reference.RoomID != work.Target.RoomID {
		return ErrSource
	}
	work.Content = &reference
	return r.cfg.Journal.Write(r.owner(), r.namespace(), key, *work)
}

func (r *Repository) publicationCommit(work publication) storage.ObjectCommit {
	pointerKind := "record"
	if work.File {
		pointerKind = "file"
	}
	metadata, _ := json.Marshal(r.publicationEntity(work))
	return storage.ObjectCommit{Kind: storage.ObjectKind, State: work.State, Metadata: metadata, Parents: work.Parents, Content: &storage.ContentPointer{Kind: pointerKind, RevisionID: work.Revision, Reference: *work.Content}}
}

func (r *Repository) checkPublishedContent(ctx context.Context, version storage.ObjectVersion, raw []byte) error {
	copied, err := r.readJSON(ctx, version)
	if err != nil {
		return err
	}
	defer clear(copied)
	if !bytes.Equal(copied, raw) {
		return ErrSource
	}
	return nil
}

func (r *Repository) publishIndex(ctx context.Context, work publication, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	obj := r.object(work.Kind, work.ID)
	version, err := r.cfg.Crypto.PutStorageObjectAt(ctx, work.Target, work.Revision+"_index", obj, work.Revision+"_index", commit)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	actual, err := r.cfg.Crypto.StorageObject(ctx, version.Reference, obj)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	wanted, _ := json.Marshal(commit)
	returned, _ := json.Marshal(actual.Commit)
	if !version.Reference.Valid() || version.Reference.RoomID != work.Target.RoomID || actual.ObjectID != obj || actual.RevisionID != work.Revision+"_index" || !bytes.Equal(wanted, returned) {
		return storage.ObjectVersion{}, ErrSource
	}
	return actual, nil
}

func (r *Repository) publishJSON(ctx context.Context, key, kind, id, state string, raw []byte, baseline *entity, observed *mutationObservation) error {
	work, err := r.publicationWork(ctx, key, kind, id, state, raw, baseline, observed)
	if err != nil {
		return err
	}
	if err := r.publishContent(ctx, key, &work, raw); err != nil {
		return err
	}
	commit := r.publicationCommit(work)
	before := storage.ObjectVersion{ObjectID: r.object(kind, id), RevisionID: work.Revision + "_index", Commit: commit}
	if err := r.checkPublishedContent(ctx, before, raw); err != nil {
		return err
	}
	actual, err := r.publishIndex(ctx, work, commit)
	if err != nil {
		return err
	}
	if err := r.checkPublishedContent(ctx, actual, raw); err != nil {
		return err
	}
	work.Index = &actual.Reference
	if err := r.cfg.Journal.Write(r.owner(), r.namespace(), key, work); err != nil {
		return err
	}
	if work.File {
		if err := r.cfg.Crypto.AcknowledgeStorageFile(ctx, r.cfg.Journal, key+"_file", actual.Reference); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
			return err
		}
	}
	return nil
}

type boundedJSONWriter struct{ buffer *bytes.Buffer }

func (w boundedJSONWriter) Write(p []byte) (int, error) {
	if len(p) > MaxEntityJSON-w.buffer.Len() {
		return 0, ErrSource
	}
	return w.buffer.Write(p)
}

func (r *Repository) namespace() string { return r.cfg.Product + "-private-v1" }

func cloneVersion(version storage.ObjectVersion) storage.ObjectVersion {
	version.Commit.Metadata = slices.Clone(version.Commit.Metadata)
	version.Commit.Parents = slices.Clone(version.Commit.Parents)
	if version.Commit.Content != nil {
		pointer := *version.Commit.Content
		version.Commit.Content = &pointer
	}
	return version
}
