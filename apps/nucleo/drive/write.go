// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Serialize publication with refresh across all processes of this account.
// Initialization holds the same lock, so no partial import becomes writable.
func (r *Repository) writeReady(ctx context.Context, fn func(context.Context) error) error {
	return r.cfg.Journal.Exclusive(ctx, r.owner(), migrationNamespace, "initialize", func() error {
		if err := r.requireReady(); err != nil {
			return err
		}
		if err := r.refresh(ctx); err != nil {
			return err
		}
		viewCtx, err := r.observeCurrent(ctx)
		if err != nil {
			return err
		}
		return fn(viewCtx)
	})
}

func (r *Repository) requireReady() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	if !r.ready {
		return ErrNotReady
	}
	return r.ctx.Err()
}

func (r *Repository) parents(object string) ([]storage.Reference, error) {
	versions := r.versions(object)
	if len(versions) > 16 {
		return nil, ErrConflict
	}
	refs := make([]storage.Reference, len(versions))
	for i, version := range versions {
		refs[i] = version.Reference
	}
	return refs, nil
}

func (r *Repository) putRecord(ctx context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	record.Payload = bytes.Clone(record.Payload)
	defer clear(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	if record.Validate(r.cfg.Scope) != nil {
		return storage.Reference{}, ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	reference, err := r.cfg.Crypto.PutStorageRecordAt(ctx, target, transaction, record)
	if err != nil {
		return storage.Reference{}, err
	}
	if !reference.Valid() || reference.RoomID != target.RoomID {
		return storage.Reference{}, ErrSource
	}
	copied, err := r.cfg.Crypto.ReadStorageRecord(ctx, reference, record.ObjectID, record.Role)
	if err != nil {
		return storage.Reference{}, err
	}
	if copied.Validate(r.cfg.Scope) != nil || copied.ObjectID != record.ObjectID || copied.RevisionID != record.RevisionID || copied.Role != record.Role || !bytes.Equal(copied.Payload, record.Payload) || !slices.Equal(copied.Previous, record.Previous) {
		return storage.Reference{}, ErrSource
	}
	if err := ctx.Err(); err != nil {
		return storage.Reference{}, err
	}
	return reference, nil
}

func (r *Repository) putObject(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	// Freeze the complete intent, including parent branches, before the SDK waits.
	payload, err := json.Marshal(commit)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	var original storage.ObjectCommit
	if json.Unmarshal(payload, &original) != nil || !original.Valid() {
		return storage.ObjectVersion{}, ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(target); err != nil {
		return storage.ObjectVersion{}, err
	}
	version, err := r.cfg.Crypto.PutStorageObjectAt(ctx, target, transaction, object, revision, original)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	got, err := json.Marshal(version.Commit)
	if err != nil || version.ObjectID != object || version.RevisionID != revision || !version.Reference.Valid() || version.Reference.RoomID != target.RoomID || !bytes.Equal(got, payload) {
		return storage.ObjectVersion{}, ErrSource
	}
	// A successful publication is not evidence that the complete catalog has
	// observed it. Reload the authenticated history before changing our view.
	if err := r.refresh(ctx); err != nil {
		return storage.ObjectVersion{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return storage.ObjectVersion{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return storage.ObjectVersion{}, err
	}
	accepted, found, err := storage.AcceptedAtomicObject(r.snapshot, r.cfg.Scope, object, revision, &version.Reference)
	if err != nil || !found || !original.MatchesExact(accepted.Commit) {
		return storage.ObjectVersion{}, ErrSource
	}
	return accepted, nil
}
