// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type restoreRequest struct {
	File    string `json:"file"`
	Version int    `json:"version"`
}

// RestoreVersion creates a new content version, retaining the current version
// and every ancestor. All content references remain private and immutable.
func (r *Repository) RestoreVersion(parent context.Context, id string, n int) error {
	if n < 1 || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: "file", ID: id}) {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	request := restoreRequest{File: id, Version: n}
	return r.writeReady(ctx, func(ctx context.Context) error { return r.restoreVersion(ctx, request) })
}

func (r *Repository) restoreVersion(ctx context.Context, request restoreRequest) error {
	encoded, _ := json.Marshal(request)
	input := digest(encoded)
	key := "restore_version_" + input
	var work rowWork
	defer func() {
		if work.Creation != nil {
			clear(work.Creation.Raw)
		}
	}()
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return err
	}
	if !found {
		work, err = r.newRestoreVersionIntent(ctx, request, input)
		if err != nil {
			return err
		}
		if err := r.saveWork(ctx, key, work); err != nil {
			return err
		}
	}
	if err := r.checkRestoreVersionIntent(ctx, work, request, input); err != nil {
		return err
	}
	raw, err := r.commitCreation(ctx, key, &work)
	clear(raw)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.cfg.Journal.Delete(r.owner(), migrationNamespace, key)
}

func (r *Repository) newRestoreVersionIntent(ctx context.Context, request restoreRequest, input string) (rowWork, error) {
	versions, err := r.fileVersions(ctx, request.File)
	if err != nil {
		return rowWork{}, err
	}
	var selected *privateVersion
	for i := range versions {
		if versions[i].view.Version == request.Version && !versions[i].view.IsCurrent {
			selected = &versions[i]
			break
		}
	}
	if selected == nil {
		return rowWork{}, ErrNotFound
	}
	base, err := r.version(ctx, "file", request.File)
	if err != nil {
		return rowWork{}, err
	}
	prior, _, original, err := r.readEntry(ctx, base)
	defer clear(original)
	if err != nil {
		return rowWork{}, err
	}
	if prior.Version < 1 || prior.Version >= 0x7fffffff {
		return rowWork{}, ErrSource
	}
	pointer, sum, err := r.contentReference(ctx, selected.observation, selected.item, selected.binary)
	if err != nil {
		return rowWork{}, err
	}
	raw, err := patchRaw(original, map[string]any{"size_bytes": selected.item.SizeBytes, "content_type": selected.item.ContentType, "checksum": sum, "version": prior.Version + 1, "updated_at": time.Now().UTC(), "author_sub": r.cfg.Scope.Subject})
	if err != nil {
		return rowWork{}, err
	}
	item, err := summarize(storage.LegacySourceRow{Kind: "file", ID: request.File, Payload: raw}, prior.Epoch)
	if err != nil {
		clear(raw)
		return rowWork{}, err
	}
	item.Seed, item.BaselineSHA256 = false, prior.BaselineSHA256
	binding, err := r.cfg.Crypto.StorageBindings(ctx)
	if err != nil {
		clear(raw)
		return rowWork{}, err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		clear(raw)
		return rowWork{}, ErrSource
	}
	intent := &creationIntent{InputSHA256: input, Item: item, Raw: raw, Binary: &pointer, FromObject: base.ObjectID, From: base.Reference, Restored: &contentSource{ObjectID: selected.index.ObjectID, Reference: selected.index.Reference}}
	return rowWork{V: 1, Scope: r.cfg.Scope, Epoch: item.Epoch, SourceKind: "file", SourceID: request.File, RawSHA256: item.RawSHA256, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}, ParentsSet: true, Parents: []storage.Reference{base.Reference}, Creation: intent}, nil
}

func (work rowWork) matchesRestore(scope storage.Scope, request restoreRequest, input string) bool {
	intent := work.Creation
	if intent == nil || intent.Restored == nil || intent.Binary == nil {
		return false
	}
	return work.matches(scope, intent.Item) && work.SourceKind == "file" && work.SourceID == request.File && work.Mutation == nil && intent.Staged == nil && intent.InputSHA256 == input && digest(intent.Raw) == work.RawSHA256 && work.validFileRevisionBase() && intent.From.Valid() && validCopyOrigin(intent.Restored.Reference, intent.Restored.ObjectID)
}

func (r *Repository) checkRestoreVersionIntent(ctx context.Context, work rowWork, request restoreRequest, input string) error {
	if !work.matchesRestore(r.cfg.Scope, request, input) {
		return ErrSource
	}
	intent := work.Creation
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	if err := r.liveFolder(ctx, intent.Item.ParentID, ""); err != nil {
		return err
	}
	if err := r.checkFileRevisionBase(ctx, work); err != nil {
		return err
	}
	index, err := r.cfg.Crypto.StorageObject(ctx, intent.Restored.Reference, intent.Restored.ObjectID)
	if err != nil {
		return err
	}
	source, data, raw, err := r.readEntry(ctx, index)
	clear(raw)
	if err != nil {
		return err
	}
	if data.Binary == nil || source.Version != request.Version || !source.restorableFile(request.File) {
		return ErrSource
	}
	expected, sum, err := r.contentReference(ctx, data.observation, source, *data.Binary)
	if err != nil {
		return err
	}
	if expected != *intent.Binary || sum != intent.Item.Checksum || source.SizeBytes != intent.Item.SizeBytes || source.ContentType != intent.Item.ContentType {
		return ErrSource
	}
	return verifyProjection(intent.Item, "live", intent.Raw)
}

func (item summary) restorableFile(id string) bool {
	return item.SourceKind == "file" && item.SourceID == id || item.SourceKind == "version" && item.ParentID == id
}
