// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Storage delegates authenticated IO to the SAME maintained browser repository.
// It contains no key material, cipher, network address or plaintext fallback.
type Storage struct {
	port     Port
	identity Identity
	sources  *Sources
	policies *Policies
	mu       sync.RWMutex
	binding  storage.Binding
}

func OpenStorage(ctx context.Context, port Port, sources *Sources, policies ...*Policies) (*Storage, error) {
	identity, err := port.Current(ctx)
	if err != nil || identity.Validate() != nil || sources == nil {
		return nil, ErrRejected
	}
	if len(policies) > 1 {
		return nil, ErrRejected
	}
	var registry *Policies
	if len(policies) == 1 {
		registry = policies[0]
	}
	return &Storage{port: port, identity: identity, sources: sources, policies: registry}, nil
}
func (s *Storage) FileScope() storage.Scope { return s.identity.Scope }
func (s *Storage) StorageIdentityVersion() storage.IdentityVersion {
	if strings.HasPrefix(s.identity.Binding.MatrixUserID, storage.IsolatedMatrixUserPrefix(s.identity.Scope)) {
		return storage.IsolatedIdentity
	}
	return storage.LegacyIdentity
}

func (s *Storage) callOwned(ctx context.Context, method Method, input any) ([]byte, error) {
	var err error
	var raw []byte
	if input != nil {
		raw, err = json.Marshal(input)
		if err != nil {
			return nil, err
		}
		defer clear(raw)
	}
	return s.callBytesOwned(ctx, method, raw)
}

func (s *Storage) callBytesOwned(ctx context.Context, method Method, raw []byte) ([]byte, error) {
	current, err := s.port.Current(ctx)
	if err != nil {
		return nil, err
	}
	if current != s.identity {
		return nil, ErrRejected
	}
	response, err := s.port.Call(ctx, s.identity.Scope, method, raw)
	if err != nil {
		return nil, errors.Join(err, discardHostReply(ctx, s.port, s.identity.Scope, method, response))
	}
	if err = ctx.Err(); err == nil {
		current, err = s.port.Current(ctx)
		if err == nil && current != s.identity {
			err = ErrRejected
		}
	}
	if err != nil {
		return nil, errors.Join(err, discardHostReply(ctx, s.port, s.identity.Scope, method, response))
	}
	return response, nil
}

func (s *Storage) call(ctx context.Context, method Method, input, out any) error {
	response, err := s.callOwned(ctx, method, input)
	if err != nil {
		return err
	}
	defer clear(response)
	if out != nil && json.Unmarshal(response, out) != nil {
		return errors.Join(ErrRejected, discardHostReply(ctx, s.port, s.identity.Scope, method, response))
	}
	return nil
}

func (s *Storage) PrepareStorage(ctx context.Context, scope storage.Scope) (storage.Binding, error) {
	var b storage.Binding
	if scope != s.identity.Scope {
		return b, ErrRejected
	}
	err := s.call(ctx, StoragePrepare, nil, &b)
	if err == nil && (!b.Valid() || b.Scope != scope || b.Target.DeviceID != s.identity.Binding.DeviceID) {
		err = ErrRejected
	}
	if err == nil {
		s.mu.Lock()
		s.binding = b
		s.binding.Rooms = slices.Clone(b.Rooms)
		s.mu.Unlock()
	}
	return b, err
}

func (s *Storage) Prepare(ctx context.Context) error {
	_, err := s.PrepareStorage(ctx, s.identity.Scope)
	return err
}

func (s *Storage) StorageBindings(ctx context.Context) (storage.Binding, error) {
	var b storage.Binding
	err := s.call(ctx, StorageBindings, nil, &b)
	if err == nil && (!b.Valid() || b.Scope != s.identity.Scope || b.Target.DeviceID != s.identity.Binding.DeviceID) {
		err = ErrRejected
	}
	if err == nil {
		s.mu.Lock()
		s.binding = b
		s.binding.Rooms = slices.Clone(b.Rooms)
		s.mu.Unlock()
	}
	return b, err
}

func (s *Storage) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	// Pin cached authenticated routing; the host revalidates current membership on publication.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !target.Valid() || !s.binding.Valid() || !slices.Contains(s.binding.Rooms, target.RoomID) || target.DeviceID != s.identity.Binding.DeviceID {
		return ErrRejected
	}
	return nil
}

func (s *Storage) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	var ref storage.Reference
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	defer clear(record.Payload)
	if s.CheckStoragePublicationTarget(target) != nil || !storage.Identifier(transaction) || record.Validate(s.identity.Scope) != nil {
		return ref, ErrRejected
	}
	err := s.publish(ctx, target, StoragePutRecord, struct {
		Target      storage.PublicationTarget `json:"target"`
		Transaction string                    `json:"transaction"`
		Record      storage.Record            `json:"record"`
	}{target, transaction, record}, &ref)
	if err == nil && (!ref.Valid() || ref.RoomID != target.RoomID) {
		err = ErrRejected
	}
	if err != nil {
		return ref, err
	}
	// A transport may recover a retained accepted transaction after restart.
	// Its public receipt cannot acknowledge a different original Go intent.
	actual, err := s.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role)
	defer clear(actual.Payload)
	if err == nil && !record.MatchesExact(actual) {
		err = ErrRejected
	}
	return ref, err
}

type fileIdentity struct {
	Reference storage.Reference `json:"reference"`
	Object    string            `json:"object"`
	Revision  string            `json:"revision"`
}

func validFile(ref storage.Reference, object, revision string) bool {
	return ref.Valid() && storage.Identifier(object) && storage.Identifier(revision)
}

func (s *Storage) ReadStorageRecord(ctx context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	var record storage.Record
	if !ref.Valid() || !storage.Identifier(object) {
		return record, ErrRejected
	}
	err := s.call(ctx, StorageReadRecord, struct {
		Reference storage.Reference `json:"reference"`
		Object    string            `json:"object"`
		Role      string            `json:"role"`
	}{ref, object, role}, &record)
	if err == nil && (record.Validate(s.identity.Scope) != nil || record.ObjectID != object || record.Role != role) {
		clear(record.Payload)
		err = ErrRejected
	}
	return record, err
}

func (s *Storage) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	var version storage.ObjectVersion
	if s.CheckStoragePublicationTarget(target) != nil || !storage.Identifier(transaction) || !storage.Identifier(object) || !storage.Identifier(revision) || !commit.Valid() {
		return version, ErrRejected
	}
	raw, err := json.Marshal(commit)
	if err != nil {
		return version, err
	}
	defer clear(raw)
	var frozen storage.ObjectCommit
	if json.Unmarshal(raw, &frozen) != nil || !frozen.Valid() {
		return version, ErrRejected
	}
	defer clear(frozen.Metadata)
	err = s.publish(ctx, target, StoragePutObject, struct {
		Target      storage.PublicationTarget `json:"target"`
		Transaction string                    `json:"transaction"`
		Object      string                    `json:"object"`
		Revision    string                    `json:"revision"`
		Commit      storage.ObjectCommit      `json:"commit"`
	}{target, transaction, object, revision, frozen}, &version)
	if err == nil && (!version.Reference.Valid() || version.Reference.RoomID != target.RoomID || version.ObjectID != object || version.RevisionID != revision || !version.Commit.Valid()) {
		err = ErrRejected
	}
	defer clear(version.Commit.Metadata)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	actual, err := s.StorageObject(ctx, version.Reference, object)
	if err == nil && (actual.RevisionID != revision || !frozen.MatchesExact(actual.Commit)) {
		err = ErrRejected
	}
	if err != nil {
		clear(actual.Commit.Metadata)
		return storage.ObjectVersion{}, err
	}
	return actual, nil
}

func (s *Storage) StorageObject(ctx context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	var version storage.ObjectVersion
	if !ref.Valid() || !storage.Identifier(object) {
		return version, ErrRejected
	}
	err := s.call(ctx, StorageReadObject, struct {
		Reference storage.Reference `json:"reference"`
		Object    string            `json:"object"`
	}{ref, object}, &version)
	if err == nil && (version.Reference != ref || version.ObjectID != object || !storage.Identifier(version.RevisionID) || !version.Commit.Valid()) {
		err = ErrRejected
	}
	return version, err
}

func (s *Storage) StorageObjectHistory(ctx context.Context, room, cursor string, limit int) (storage.ObjectPage, error) {
	var page storage.ObjectPage
	if !(storage.Reference{RoomID: room, EventID: "$validation"}).Valid() || len(cursor) > 16384 || limit < 1 || limit > 100 {
		return page, ErrRejected
	}
	err := s.call(ctx, StorageHistory, struct {
		Room   string `json:"room"`
		Cursor string `json:"cursor"`
		Limit  int    `json:"limit"`
	}{room, cursor, limit}, &page)
	if err == nil {
		if len(page.Entries) > limit || len(page.Next) > 16384 {
			err = ErrRejected
		}
		for _, entry := range page.Entries {
			if entry.Reference.RoomID != room {
				err = ErrRejected
				break
			}
			if e := storage.ValidateCatalogEntry(entry, s.identity.Scope); e != nil {
				err = e
				break
			}
		}
	}
	return page, err
}

func (s *Storage) Info(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	var request storage.FileRequest
	if !validFile(ref, object, revision) {
		return request, ErrRejected
	}
	err := s.call(ctx, StorageFileInfo, fileIdentity{ref, object, revision}, &request)
	if err == nil && (request.Validate(s.identity.Scope) != nil || request.ObjectID != object || request.RevisionID != revision) {
		err = ErrRejected
	}
	return request, err
}

func (s *Storage) Commitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	var result struct {
		Request storage.FileRequest `json:"request"`
		SHA256  string              `json:"sha256"`
	}
	if !validFile(ref, object, revision) {
		return result.Request, "", ErrRejected
	}
	err := s.call(ctx, StorageFileCommitment, fileIdentity{ref, object, revision}, &result)
	if err == nil && (result.Request.Validate(s.identity.Scope) != nil || result.Request.ObjectID != object || result.Request.RevisionID != revision || !ValidID(result.SHA256)) {
		err = ErrRejected
	}
	return result.Request, result.SHA256, err
}

func (s *Storage) Upload(ctx context.Context, job string, target storage.PublicationTarget, request storage.FileRequest, source io.ReaderAt) (storage.Reference, error) {
	var ref storage.Reference
	if !storage.Identifier(job) || s.CheckStoragePublicationTarget(target) != nil || request.Validate(s.identity.Scope) != nil || source == nil {
		return ref, ErrRejected
	}
	id, release, err := s.sources.Add(ctx, s.port, s.identity, source, request.SizeBytes)
	if err != nil {
		return ref, err
	}
	defer release()
	err = s.publish(ctx, target, StorageFileUpload, struct {
		Job     string                    `json:"job"`
		Target  storage.PublicationTarget `json:"target"`
		Request storage.FileRequest       `json:"request"`
		Source  string                    `json:"source"`
	}{job, target, request, id}, &ref)
	if err == nil && (!ref.Valid() || ref.RoomID != target.RoomID) {
		err = ErrRejected
	}
	return ref, err
}

func (s *Storage) Acknowledge(ctx context.Context, job string, ref storage.Reference) error {
	if !storage.Identifier(job) || !ref.Valid() {
		return ErrRejected
	}
	return s.call(ctx, StorageFileAck, struct {
		Job       string            `json:"job"`
		Reference storage.Reference `json:"reference"`
	}{job, ref}, nil)
}

var (
	_ storage.RecordStore = (*Storage)(nil)
	_ storage.BoundFiles  = (*Storage)(nil)
)
