// SPDX-License-Identifier: Apache-2.0
package photos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourcePlanReference is the complete server-retained routing value. The server
// receives no pin, media descriptor, content key or plaintext plan.
type SourcePlanReference = model.SourcePlanReference

type SourcePlanAcceptor interface {
	AcceptSourcePlan(context.Context, storage.Scope, string, string, storage.Reference) (SourcePlanReference, error)
}

type SourcePlanCrypto interface {
	Crypto
	AcknowledgeStorageFileLink(context.Context, *offline.Store, string, storage.Reference) error
}

type SourcePlanConfig struct {
	Context   context.Context
	WorkLease offline.WorkLease
	Scope     storage.Scope
	Crypto    SourcePlanCrypto
	Gateway   *api.Drive
	Journal   *offline.Store
	Originals OriginalPinAuthority
}

type SourcePlanStore struct {
	cfg   SourcePlanConfig
	gate  *offline.WorkGate
	mu    sync.Mutex
	cache *sourcePlanCache
}

func NewSourcePlanStore(cfg SourcePlanConfig) (*SourcePlanStore, error) {
	if cfg.Context == nil || !cfg.Scope.Valid() || cfg.Scope.Product != "photos" || cfg.Crypto == nil || cfg.Crypto.StorageIdentityVersion() != storage.IsolatedIdentity || cfg.Gateway == nil || !cfg.Journal.Encrypted() {
		return nil, ErrSourcePlan
	}
	return &SourcePlanStore{cfg: cfg, gate: offline.NewWorkGateWithLease(cfg.Context, cfg.WorkLease)}, nil
}

func (s *SourcePlanStore) Close()                          { s.gate.Close(); s.invalidatePlanCache() }
func (s *SourcePlanStore) Drain(ctx context.Context) error { return s.gate.Drain(ctx) }
func (s *SourcePlanStore) Resume() error                   { return s.gate.Resume() }
func (s *SourcePlanStore) owner() string {
	return s.cfg.Scope.Tenant + "\x00" + s.cfg.Scope.Subject + "\x00photos"
}

func sourcePlanObject(epoch string) string {
	return "photos_source_plan_" + planBytesHash([]byte(epoch))
}
func sourcePlanJob(epoch string) string { return "source_plan_" + planBytesHash([]byte(epoch)) }

type sourcePlanWork struct {
	Version int                       `json:"v"`
	Scope   storage.Scope             `json:"scope"`
	Epoch   string                    `json:"epoch"`
	Target  storage.PublicationTarget `json:"target"`
	File    offline.StagedFile        `json:"file"`
	Link    *storage.Reference        `json:"link,omitempty"`
	Format  string                    `json:"format,omitempty"`
	Job     string                    `json:"job,omitempty"`
}

func (w sourcePlanWork) request() storage.FileRequest {
	kind := w.Format
	if kind == "" {
		kind = sourcePlanKind
	}
	return storage.FileRequest{ObjectID: sourcePlanObject(w.Epoch), RevisionID: w.Epoch, SizeBytes: w.File.SizeBytes, Metadata: sourcePlanMetadata(kind)}
}

func (w sourcePlanWork) journalKey() string {
	if w.Job != "" {
		return w.Job
	}
	return sourcePlanJob(w.Epoch)
}

// Publish retains and reads back the plan through the maintained SDK BEFORE
// invoking the server CAS. A failed CAS or changed accepted reference preserves
// the same encrypted spool and SDK upload WAL for an exact retry.
func (s *SourcePlanStore) Publish(parent context.Context, plan SourcePlan, acceptor SourcePlanAcceptor) (SourcePlanReference, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return SourcePlanReference{}, err
	}
	defer done()
	if plan.Scope != s.cfg.Scope || acceptor == nil {
		return SourcePlanReference{}, ErrSourcePlan
	}
	if err := plan.Validate(ctx, s.cfg.Originals); err != nil {
		return SourcePlanReference{}, err
	}
	binding, err := s.cfg.Crypto.PrepareStorage(ctx, s.cfg.Scope)
	if err != nil {
		return SourcePlanReference{}, err
	}
	if !binding.Valid() || binding.Scope != s.cfg.Scope {
		return SourcePlanReference{}, ErrSourcePlan
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return SourcePlanReference{}, err
	}
	defer clear(raw)
	if len(raw) > maxSourcePlanBytes {
		return SourcePlanReference{}, ErrSourcePlan
	}
	var retained SourcePlanReference
	err = s.cfg.Journal.Exclusive(ctx, s.owner(), "photos-private-v1", sourcePlanJob(plan.Epoch), func() error {
		var err error
		retained, err = s.publish(ctx, plan, binding.Target, raw, acceptor)
		return err
	})
	return retained, err
}

func (s *SourcePlanStore) publish(ctx context.Context, plan SourcePlan, target storage.PublicationTarget, raw []byte, acceptor SourcePlanAcceptor) (SourcePlanReference, error) {
	work, err := s.planWork(ctx, plan, target, raw)
	if err != nil {
		return SourcePlanReference{}, err
	}
	ref, err := s.publishPlanWork(ctx, &work)
	if err != nil {
		return SourcePlanReference{}, err
	}
	verified, err := s.read(ctx, ref)
	if err != nil {
		return SourcePlanReference{}, err
	}
	readback, err := json.Marshal(verified)
	if err != nil {
		return SourcePlanReference{}, err
	}
	defer clear(readback)
	if !bytes.Equal(raw, readback) {
		return SourcePlanReference{}, ErrSourcePlan
	}
	accepted, err := acceptor.AcceptSourcePlan(ctx, s.cfg.Scope, plan.Epoch, plan.CAS, ref.Link)
	if err != nil {
		return SourcePlanReference{}, err
	}
	if accepted != ref {
		return SourcePlanReference{}, ErrSourcePlan
	}
	// Only the exact durable control-plane acceptance can retire local work.
	if err := s.cfg.Crypto.AcknowledgeStorageFileLink(ctx, s.cfg.Journal, sourcePlanJob(plan.Epoch)+"_file", ref.Link); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return SourcePlanReference{}, err
	}
	if err := s.cfg.Journal.DeleteStagedFile(s.owner(), "photos-private-stage-v1", work.File); err != nil {
		return SourcePlanReference{}, err
	}
	if err := s.cfg.Journal.Delete(s.owner(), "photos-private-v1", sourcePlanJob(plan.Epoch)); err != nil {
		return SourcePlanReference{}, err
	}
	return ref, nil
}

func (s *SourcePlanStore) planWork(ctx context.Context, plan SourcePlan, target storage.PublicationTarget, raw []byte) (sourcePlanWork, error) {
	var work sourcePlanWork
	found, err := s.cfg.Journal.Read(s.owner(), "photos-private-v1", sourcePlanJob(plan.Epoch), &work)
	if err != nil {
		return work, err
	}
	if found {
		if work.Version != 1 || work.Scope != s.cfg.Scope || work.Epoch != plan.Epoch || !work.Target.Valid() || work.File.SizeBytes != int64(len(raw)) || work.File.SHA256 != planBytesHash(raw) {
			return work, ErrSourcePlan
		}
		if err := s.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
			return work, err
		}
		return work, nil
	}
	file, err := s.cfg.Journal.StageFile(ctx, s.owner(), "photos-private-stage-v1", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return work, err
	}
	work = sourcePlanWork{Version: 1, Scope: s.cfg.Scope, Epoch: plan.Epoch, Target: target, File: file}
	if err := s.cfg.Journal.Write(s.owner(), "photos-private-v1", sourcePlanJob(plan.Epoch), work); err != nil {
		return work, err
	}
	return work, nil
}

func (s *SourcePlanStore) publishPlanWork(ctx context.Context, work *sourcePlanWork) (SourcePlanReference, error) {
	if work.Link != nil {
		return SourcePlanReference{Epoch: work.Epoch, Link: *work.Link}, nil
	}
	reader, err := s.cfg.Journal.OpenStagedFile(ctx, s.owner(), "photos-private-stage-v1", work.File)
	if err != nil {
		return SourcePlanReference{}, err
	}
	defer reader.Close()
	request := work.request()
	root, err := s.cfg.Crypto.UploadStorageFileAt(ctx, s.cfg.Gateway, s.cfg.Journal, sourcePlanJob(work.Epoch)+"_file", work.Target, request, reader)
	if err != nil {
		return SourcePlanReference{}, err
	}
	payload, err := json.Marshal(storage.FileLink{Kind: storage.FileLinkKind, Content: storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: root}})
	if err != nil {
		return SourcePlanReference{}, err
	}
	link, err := s.cfg.Crypto.PutStorageRecordAt(ctx, work.Target, sourcePlanJob(work.Epoch)+"_link", storage.Record{Version: 1, Scope: s.cfg.Scope, ObjectID: request.ObjectID, RevisionID: request.RevisionID, Role: "content", Payload: payload})
	if err != nil {
		return SourcePlanReference{}, err
	}
	if !link.Valid() || link.RoomID != work.Target.RoomID {
		return SourcePlanReference{}, ErrSourcePlan
	}
	work.Link = &link
	if err := s.cfg.Journal.Write(s.owner(), "photos-private-v1", work.journalKey(), work); err != nil {
		return SourcePlanReference{}, err
	}
	return SourcePlanReference{Epoch: work.Epoch, Link: link}, nil
}

func (s *SourcePlanStore) Read(parent context.Context, ref SourcePlanReference) (SourcePlan, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return SourcePlan{}, err
	}
	defer done()
	return s.read(ctx, ref)
}

// ReadMetadata authenticates the complete retained Photos plan independently
// of current Drive access. It proves preserved metadata, not provider availability
// or migration completion. Before Begin, ValidateOrigins remains mandatory.
func (s *SourcePlanStore) ReadMetadata(parent context.Context, ref SourcePlanReference) (SourcePlan, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		s.invalidatePlanCache()
		return SourcePlan{}, err
	}
	defer done()
	return s.readMetadata(ctx, ref, true)
}

// AuditMetadata checks the actual complete sidecar bytes even when a cached
// authenticated commitment is available. Source retirement must include this
// availability check; metadata readiness alone is not such a proof.
func (s *SourcePlanStore) AuditMetadata(parent context.Context, ref SourcePlanReference) (SourcePlan, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		s.invalidatePlanCache()
		return SourcePlan{}, err
	}
	defer done()
	return s.readMetadata(ctx, ref, false)
}

func (s *SourcePlanStore) ValidateOrigins(parent context.Context, plan SourcePlan) error {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	if plan.Scope != s.cfg.Scope {
		return ErrSourcePlan
	}
	return plan.Validate(ctx, s.cfg.Originals)
}

type planSink struct{ bytes.Buffer }

func (s *planSink) Write(raw []byte) (int, error) {
	if len(raw) > maxSourcePlanBytes-s.Len() {
		return 0, ErrSourcePlan
	}
	return s.Buffer.Write(raw)
}

func (s *SourcePlanStore) read(ctx context.Context, ref SourcePlanReference) (SourcePlan, error) {
	plan, err := s.readMetadata(ctx, ref, true)
	if err != nil {
		return SourcePlan{}, err
	}
	if err := plan.Validate(ctx, s.cfg.Originals); err != nil {
		s.invalidatePlanCache()
		return SourcePlan{}, err
	}
	return plan, nil
}

func (s *SourcePlanStore) readMetadata(ctx context.Context, ref SourcePlanReference, allowCache bool) (_ SourcePlan, resultErr error) {
	defer func() {
		if resultErr != nil {
			s.invalidatePlanCache()
		}
	}()
	pointer, request, err := s.planFile(ctx, ref)
	if err != nil {
		return SourcePlan{}, err
	}
	if !bytes.Equal(request.Metadata, sourcePlanMetadata(sourcePlanKind)) {
		return SourcePlan{}, ErrSourcePlan
	}
	if allowCache {
		if cached, found, err := s.readPlanCache(ctx, ref, pointer, request); err != nil {
			return SourcePlan{}, err
		} else if found {
			return cached, nil
		}
	}
	var sink planSink
	defer func() { clear(sink.Bytes()) }()
	copied, err := s.cfg.Crypto.DownloadStorageFile(ctx, s.cfg.Gateway, pointer.Reference, request.ObjectID, request.RevisionID, &sink)
	if err != nil {
		return SourcePlan{}, err
	}
	if int64(sink.Len()) != request.SizeBytes || !samePlanRequest(copied, request) {
		return SourcePlan{}, ErrSourcePlan
	}
	var plan SourcePlan
	if json.Unmarshal(sink.Bytes(), &plan) != nil || plan.Scope != s.cfg.Scope || plan.Epoch != ref.Epoch {
		return SourcePlan{}, ErrSourcePlan
	}
	if err := plan.validateStructure(ctx); err != nil {
		return SourcePlan{}, err
	}
	s.rememberPlan(ref, pointer, request, planBytesHash(sink.Bytes()), plan)
	return plan, nil
}

func (s *SourcePlanStore) planFile(ctx context.Context, ref SourcePlanReference) (storage.ContentPointer, storage.FileRequest, error) {
	if !ref.Valid() {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrSourcePlan
	}
	record, err := s.cfg.Crypto.ReadStorageRecord(ctx, ref.Link, sourcePlanObject(ref.Epoch), "content")
	if err != nil {
		return storage.ContentPointer{}, storage.FileRequest{}, err
	}
	defer clear(record.Payload)
	link, err := storage.DecodeFileLink(record, s.cfg.Scope)
	if err != nil || record.ObjectID != sourcePlanObject(ref.Epoch) || record.RevisionID != ref.Epoch {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrSourcePlan
	}
	request, err := s.cfg.Crypto.StorageFileInfo(ctx, link.Content.Reference, record.ObjectID, record.RevisionID)
	if err != nil {
		return storage.ContentPointer{}, storage.FileRequest{}, err
	}
	if !validPlanRequest(s.cfg.Scope, record, request) {
		return storage.ContentPointer{}, storage.FileRequest{}, ErrSourcePlan
	}
	return link.Content, request, nil
}

type (
	planCommitments interface {
		StorageFileCommitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error)
	}
	sourcePlanCache struct {
		ref     SourcePlanReference
		pointer storage.ContentPointer
		request storage.FileRequest
		sha     string
		plan    SourcePlan
	}
)

func cloneSourcePlan(plan SourcePlan) SourcePlan {
	plan.Backend = slices.Clone(plan.Backend)
	plan.Originals = slices.Clone(plan.Originals)
	for i := range plan.Originals {
		plan.Originals[i].Pin = bytes.Clone(plan.Originals[i].Pin)
	}
	return plan
}

func (s *SourcePlanStore) invalidatePlanCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache != nil {
		for _, original := range s.cache.plan.Originals {
			clear(original.Pin)
		}
		clear(s.cache.request.Metadata)
		s.cache = nil
	}
}

func (s *SourcePlanStore) rememberPlan(ref SourcePlanReference, pointer storage.ContentPointer, request storage.FileRequest, sha string, plan SourcePlan) {
	request.Metadata = bytes.Clone(request.Metadata)
	request.Previous = slices.Clone(request.Previous)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache != nil {
		for _, original := range s.cache.plan.Originals {
			clear(original.Pin)
		}
		clear(s.cache.request.Metadata)
	}
	s.cache = &sourcePlanCache{ref: ref, pointer: pointer, request: request, sha: sha, plan: cloneSourcePlan(plan)}
}

func samePlanRequest(a, b storage.FileRequest) bool {
	return a.ObjectID == b.ObjectID && a.RevisionID == b.RevisionID && a.SizeBytes == b.SizeBytes && bytes.Equal(a.Metadata, b.Metadata) && slices.Equal(a.Previous, b.Previous)
}

func (s *SourcePlanStore) readPlanCache(ctx context.Context, ref SourcePlanReference, pointer storage.ContentPointer, request storage.FileRequest) (SourcePlan, bool, error) {
	proof, ok := s.cfg.Crypto.(planCommitments)
	if !ok {
		return SourcePlan{}, false, nil
	}
	s.mu.Lock()
	if s.cache == nil {
		s.mu.Unlock()
		return SourcePlan{}, false, nil
	}
	cache := *s.cache
	cache.plan = cloneSourcePlan(cache.plan)
	cache.request.Metadata = bytes.Clone(cache.request.Metadata)
	cache.request.Previous = slices.Clone(cache.request.Previous)
	s.mu.Unlock()
	defer clear(cache.request.Metadata)
	if cache.ref != ref {
		s.invalidatePlanCache()
		return SourcePlan{}, false, nil
	}
	if cache.pointer != pointer || cache.plan.Scope != s.cfg.Scope || !samePlanRequest(cache.request, request) {
		return SourcePlan{}, false, ErrSourcePlan
	}
	actual, sha, err := proof.StorageFileCommitment(ctx, pointer.Reference, request.ObjectID, request.RevisionID)
	if err != nil {
		return SourcePlan{}, false, err
	}
	if actual.Validate(s.cfg.Scope) != nil || !samePlanRequest(actual, request) || sha != cache.sha {
		return SourcePlan{}, false, ErrSourcePlan
	}
	if err := ctx.Err(); err != nil {
		return SourcePlan{}, false, err
	}
	return cache.plan, true, nil
}

func validPlanRequest(scope storage.Scope, record storage.Record, request storage.FileRequest) bool {
	if request.Validate(scope) != nil || request.ObjectID != record.ObjectID || request.RevisionID != record.RevisionID {
		return false
	}
	return bytes.Equal(request.Metadata, sourcePlanMetadata(sourcePlanKind)) && request.SizeBytes <= maxSourcePlanBytes || bytes.Equal(request.Metadata, sourcePlanMetadata(sourceArchiveKind)) && request.SizeBytes <= storage.MaxFileBytes
}

func sourcePlanMetadata(kind string) []byte { return []byte(`{"kind":"` + kind + `"}`) }

func (s *SourcePlanStore) archiveFormat(parent context.Context, ref SourcePlanReference) (bool, error) {
	ctx, done, err := s.gate.Start(parent)
	if err != nil {
		return false, err
	}
	defer done()
	_, request, err := s.planFile(ctx, ref)
	if err != nil {
		return false, err
	}
	return bytes.Equal(request.Metadata, sourcePlanMetadata(sourceArchiveKind)), nil
}

var _ io.Writer = (*planSink)(nil)
