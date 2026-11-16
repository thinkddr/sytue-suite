// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) CopyFile(parent context.Context, id string) (api.File, error) {
	if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: "file", ID: id}) {
		return api.File{}, ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.File{}, err
	}
	defer done()
	var result api.File
	err = r.writeReady(ctx, func(ctx context.Context) error {
		var err error
		result, err = r.copyFile(ctx, id)
		return err
	})
	return result, err
}

func (r *Repository) copyFile(ctx context.Context, id string) (api.File, error) {
	input := digest([]byte("copy_file\x00" + id))
	key := "copy_file_" + input
	var work rowWork
	defer func() {
		if work.Creation != nil {
			clear(work.Creation.Raw)
		}
	}()
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return api.File{}, err
	}
	if !found {
		work, err = r.newCopyIntent(ctx, id, input)
		if err != nil {
			return api.File{}, err
		}
		if err := r.saveWork(ctx, key, work); err != nil {
			return api.File{}, err
		}
	}
	if err := r.checkCopyIntent(ctx, work, id, input); err != nil {
		return api.File{}, err
	}
	raw, err := r.commitCreation(ctx, key, &work)
	defer clear(raw)
	if err != nil {
		return api.File{}, err
	}
	file, err := fileView(raw, work.SourceID)
	if err != nil {
		return api.File{}, err
	}
	if err := ctx.Err(); err != nil {
		return api.File{}, err
	}
	if err := r.cfg.Journal.Delete(r.owner(), migrationNamespace, key); err != nil {
		return api.File{}, err
	}
	return file, nil
}

func (r *Repository) newCopyIntent(ctx context.Context, id, input string) (rowWork, error) {
	version, err := r.version(ctx, "file", id)
	if err != nil {
		return rowWork{}, err
	}
	item, data, raw, err := r.readEntry(ctx, version)
	defer clear(raw)
	if err != nil {
		return rowWork{}, err
	}
	if err := r.requireLive(ctx, item); err != nil {
		return rowWork{}, err
	}
	if data.Binary == nil {
		return rowWork{}, ErrSource
	}
	pointer, sum, err := r.contentReference(ctx, data.observation, item, *data.Binary)
	if err != nil {
		return rowWork{}, err
	}
	created := uuid.NewString()
	name, err := r.copyName(ctx, item, created)
	if err != nil {
		return rowWork{}, err
	}
	now := time.Now().UTC()
	updated, err := patchRaw(raw, map[string]any{"id": created, "name": name, "version": 1, "checksum": sum, "owner_sub": r.cfg.Scope.Subject, "created_at": now, "updated_at": now, "deleted_at": nil})
	if err != nil {
		return rowWork{}, err
	}
	next, err := summarize(storage.LegacySourceRow{Kind: "file", ID: created, Payload: updated}, "")
	if err != nil {
		clear(updated)
		return rowWork{}, err
	}
	binding, err := r.cfg.Crypto.StorageBindings(ctx)
	if err != nil {
		clear(updated)
		return rowWork{}, err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		clear(updated)
		return rowWork{}, ErrSource
	}
	intent := &creationIntent{InputSHA256: input, Item: next, Raw: updated, Binary: &pointer, FromObject: version.ObjectID, From: version.Reference}
	return rowWork{V: 1, Scope: r.cfg.Scope, SourceKind: "file", SourceID: created, RawSHA256: next.RawSHA256, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}, ParentsSet: true, Creation: intent}, nil
}

func (r *Repository) copyName(ctx context.Context, item summary, id string) (string, error) {
	for n := 1; n <= 1000; n++ {
		candidate := "Copia de " + item.Name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d)", candidate, n)
		}
		if !storage.ValidDriveName(candidate) {
			return "", ErrInvalidName
		}
		err := r.uniqueSibling(ctx, summary{SourceKind: "file", SourceID: id}, item.ParentID, candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, ErrConflict) {
			return "", err
		}
	}
	return "", ErrConflict
}

func (r *Repository) checkCopyIntent(ctx context.Context, work rowWork, id, input string) error {
	intent := work.Creation
	if !work.matchesCopy(r.cfg.Scope, id, input) {
		return ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	if err := r.liveFolder(ctx, intent.Item.ParentID, ""); err != nil {
		return err
	}
	original, err := r.cfg.Crypto.StorageObject(ctx, intent.From, intent.FromObject)
	if err != nil {
		return err
	}
	item, data, raw, err := r.readEntry(ctx, original)
	clear(raw)
	if err != nil {
		return err
	}
	if data.Binary == nil {
		return ErrSource
	}
	expected, sum, err := r.contentReference(ctx, data.observation, item, *data.Binary)
	if err != nil {
		return err
	}
	if expected != *intent.Binary || sum != intent.Item.Checksum || item.SizeBytes != intent.Item.SizeBytes {
		return ErrSource
	}
	return verifyProjection(intent.Item, "live", intent.Raw)
}

func (work rowWork) matchesCopy(scope storage.Scope, id, input string) bool {
	intent := work.Creation
	if intent == nil {
		return false
	}
	return intent.InputSHA256 == input && work.matches(scope, intent.Item) && work.SourceKind == "file" && intent.Binary != nil && intent.Staged == nil && intent.Restored == nil && work.Mutation == nil && work.ParentsSet && len(work.Parents) == 0 && work.Epoch == "" && !intent.Item.Seed && intent.Item.BaselineSHA256 == "" && intent.FromObject == objectID("file", id) && validCopyOrigin(intent.From, intent.FromObject) && digest(intent.Raw) == work.RawSHA256
}

func (r *Repository) commitCreation(ctx context.Context, key string, work *rowWork) ([]byte, error) {
	intent := work.Creation
	return r.commitEntry(ctx, key, work, intent.Item, intent.Raw, intent.Binary)
}

// The same publication/readback path serves domain writers and keeps large
// original JSON on the existing encrypted file transport.
func (r *Repository) commitEntry(ctx context.Context, key string, work *rowWork, item summary, original []byte, binary *bodyPointer) ([]byte, error) {
	data := entry{Kind: entryKind, SourceKind: work.SourceKind, SourceID: work.SourceID, Binary: binary}
	if len(original) <= inlineSourceBytes {
		data.Raw = bytes.Clone(original)
	} else {
		pointer, err := r.ensureBody(ctx, key, work, item, "metadata", bytes.NewReader(original), int64(len(original)), func() (string, error) { return digest(original), nil })
		if err != nil {
			return nil, err
		}
		data.Metadata = &pointer
	}
	defer clear(data.Raw)
	if err := r.publishEntry(ctx, key, work, item, data); err != nil {
		return nil, err
	}
	stored, err := r.cfg.Crypto.StorageObject(ctx, *work.Catalog, objectID(work.SourceKind, work.SourceID))
	if err != nil {
		return nil, err
	}
	fresh, release, err := r.temporaryObservation(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	_, copied, raw, err := r.readEntry(fresh, stored)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(raw, original) || !sameBody(copied.Binary, binary) {
		clear(raw)
		return nil, ErrSource
	}
	return raw, nil
}
