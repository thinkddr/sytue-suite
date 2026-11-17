// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type metadataRequest struct {
	Operation string        `json:"operation"`
	Kind      api.DriveKind `json:"kind"`
	ID        string        `json:"id"`
	Value     string        `json:"value,omitempty"`
}

type metadataIntent struct {
	InputSHA256 string            `json:"input_sha256"`
	Base        storage.Reference `json:"base"`
	Item        summary           `json:"item"`
	Raw         []byte            `json:"raw"`
	Binary      *bodyPointer      `json:"binary,omitempty"`
}

// These operations confirm only after their encrypted catalog commit is read
// back. They never acknowledge an offline-only edit or upload the media again.
func (r *Repository) Rename(ctx context.Context, kind api.DriveKind, id, name string) error {
	if !storage.ValidDriveName(name) {
		return ErrInvalidName
	}
	return r.mutateMetadata(ctx, metadataRequest{Operation: "rename", Kind: kind, ID: id, Value: strings.TrimSpace(name)})
}

func (r *Repository) Move(ctx context.Context, kind api.DriveKind, id, parent string) error {
	return r.mutateMetadata(ctx, metadataRequest{Operation: "move", Kind: kind, ID: id, Value: parent})
}

func (r *Repository) Delete(ctx context.Context, kind api.DriveKind, id string) error {
	return r.mutateMetadata(ctx, metadataRequest{Operation: "trash", Kind: kind, ID: id})
}

func (r *Repository) Restore(ctx context.Context, kind api.DriveKind, id string) error {
	return r.mutateMetadata(ctx, metadataRequest{Operation: "restore", Kind: kind, ID: id})
}

func resourceKind(kind api.DriveKind) string {
	switch kind {
	case api.DriveFile:
		return "file"
	case api.DriveFolder:
		return "folder"
	default:
		return ""
	}
}

func (r *Repository) mutateMetadata(parent context.Context, request metadataRequest) error {
	kind := resourceKind(request.Kind)
	if kind == "" || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: request.ID}) {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	// Keep the existing cross-process repository lock: refreshing history must
	// not replace a projection immediately after another local write commits.
	return r.writeReady(ctx, func(ctx context.Context) error {
		return r.applyMetadata(ctx, kind, request)
	})
}

func (r *Repository) applyMetadata(ctx context.Context, kind string, request metadataRequest) error {
	input, err := json.Marshal(request)
	if err != nil {
		return err
	}
	inputHash := digest(input)
	key := "mutation_" + inputHash
	var work rowWork
	defer func() {
		if work.Mutation != nil {
			clear(work.Mutation.Raw)
		}
	}()
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return err
	}
	if !found {
		work, err = r.newMetadataIntent(ctx, kind, request, inputHash)
		if err != nil {
			return err
		}
		if work.Mutation == nil {
			return nil
		} // The requested state already exists.
		if err := r.saveWork(ctx, key, work); err != nil {
			return err
		}
	}
	if err := r.checkMetadataIntent(ctx, work, request, inputHash); err != nil {
		return err
	}
	data, err := r.metadataEntry(ctx, key, &work)
	if err != nil {
		return err
	}
	defer clear(data.Raw)
	if err := r.publishEntry(ctx, key, &work, work.Mutation.Item, data); err != nil {
		return err
	}
	stored, err := r.cfg.Crypto.StorageObject(ctx, *work.Catalog, objectID(kind, request.ID))
	if err != nil {
		return err
	}
	fresh, release, err := r.temporaryObservation(ctx)
	if err != nil {
		return err
	}
	defer release()
	item, entry, raw, err := r.readEntry(fresh, stored)
	defer clear(raw)
	if err != nil {
		return err
	}
	if item.RawSHA256 != work.RawSHA256 || !bytes.Equal(raw, work.Mutation.Raw) || !sameBody(entry.Binary, work.Mutation.Binary) {
		return ErrSource
	}
	return r.cfg.Journal.Delete(r.owner(), migrationNamespace, key)
}

func sameBody(a, b *bodyPointer) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (r *Repository) newMetadataIntent(ctx context.Context, kind string, request metadataRequest, inputHash string) (rowWork, error) {
	version, err := r.version(ctx, kind, request.ID)
	if err != nil {
		return rowWork{}, err
	}
	item, data, raw, err := r.readEntry(ctx, version)
	defer clear(raw)
	if err != nil {
		return rowWork{}, err
	}
	if item.IsRoot {
		return rowWork{}, ErrRootImmutable
	}
	updated, err := r.changedMetadata(ctx, item, raw, request)
	if err != nil {
		return rowWork{}, err
	}
	if updated == nil {
		return rowWork{}, nil
	}
	next, err := summarize(storage.LegacySourceRow{Kind: kind, ID: request.ID, Payload: updated}, item.Epoch)
	if err != nil {
		clear(updated)
		return rowWork{}, err
	}
	next.Seed, next.BaselineSHA256 = false, item.BaselineSHA256
	binding, err := r.cfg.Crypto.StorageBindings(ctx)
	if err != nil {
		clear(updated)
		return rowWork{}, err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		clear(updated)
		return rowWork{}, ErrSource
	}
	intent := &metadataIntent{InputSHA256: inputHash, Base: version.Reference, Item: next, Raw: updated, Binary: data.Binary}
	return rowWork{V: 1, Scope: r.cfg.Scope, Epoch: item.Epoch, SourceKind: kind, SourceID: request.ID, RawSHA256: next.RawSHA256, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}, Parents: []storage.Reference{version.Reference}, ParentsSet: true, Mutation: intent}, nil
}

func (r *Repository) checkMetadataIntent(ctx context.Context, work rowWork, request metadataRequest, inputHash string) error {
	kind, id := resourceKind(request.Kind), request.ID
	intent := work.Mutation
	if intent == nil || work.Creation != nil || !intent.matches(kind, id, inputHash) || !work.matches(r.cfg.Scope, intent.Item) {
		return ErrSource
	}
	if !work.ParentsSet || !slices.Equal(work.Parents, []storage.Reference{intent.Base}) {
		return ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	prior, err := r.cfg.Crypto.StorageObject(ctx, intent.Base, objectID(kind, id))
	if err != nil {
		return err
	}
	base, original, raw, err := r.readEntry(ctx, prior)
	defer clear(raw)
	if err != nil {
		return err
	}
	if base.IsRoot || base.Epoch != intent.Item.Epoch || base.BaselineSHA256 != intent.Item.BaselineSHA256 {
		return ErrSource
	}
	state := "live"
	if intent.Item.DeletedAt != nil {
		state = "trash"
	}
	if err := verifyProjection(intent.Item, state, intent.Raw); err != nil {
		return err
	}
	if !sameBody(original.Binary, intent.Binary) {
		return ErrSource
	}
	if request.Operation == "label" {
		return checkLabelTransition(raw, intent.Raw, request.Value)
	}
	return nil
}

func (intent metadataIntent) matches(kind, id, inputHash string) bool {
	return intent.InputSHA256 == inputHash && intent.Item.SourceKind == kind && intent.Item.SourceID == id && !intent.Item.Seed && intent.Base.Valid() && sourceRowValid(storage.LegacySourceRow{Kind: kind, ID: id, Payload: intent.Raw}, intent.Item.Epoch) && digest(intent.Raw) == intent.Item.RawSHA256
}

func (r *Repository) metadataEntry(ctx context.Context, key string, work *rowWork) (entry, error) {
	intent := work.Mutation
	data := entry{Kind: entryKind, SourceKind: intent.Item.SourceKind, SourceID: intent.Item.SourceID, Binary: intent.Binary}
	if len(intent.Raw) <= inlineSourceBytes {
		data.Raw = bytes.Clone(intent.Raw)
		return data, nil
	}
	pointer, err := r.ensureBody(ctx, key, work, intent.Item, "metadata", bytes.NewReader(intent.Raw), int64(len(intent.Raw)), func() (string, error) { return digest(intent.Raw), nil })
	if err != nil {
		return entry{}, err
	}
	data.Metadata = &pointer
	return data, nil
}

func (r *Repository) changedMetadata(ctx context.Context, item summary, raw []byte, request metadataRequest) ([]byte, error) {
	if request.Operation == "label" {
		return r.changedLabel(ctx, item, raw, request.Value)
	}
	patch, changed, err := r.metadataPatch(ctx, item, request)
	if err != nil || !changed {
		return nil, err
	}
	patch["updated_at"] = time.Now().UTC()
	return patchRaw(raw, patch)
}

func patchRaw(raw []byte, patch map[string]any) ([]byte, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, ErrSource
	}
	for key, value := range patch {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		fields[key] = encoded
	}
	return json.Marshal(fields)
}

func (r *Repository) metadataPatch(ctx context.Context, item summary, request metadataRequest) (map[string]any, bool, error) {
	patch := map[string]any{}
	switch request.Operation {
	case "rename":
		if err := r.requireLive(ctx, item); err != nil {
			return nil, false, err
		}
		if item.Name == request.Value {
			return nil, false, nil
		}
		if err := r.uniqueSibling(ctx, item, item.ParentID, request.Value); err != nil {
			return nil, false, err
		}
		patch["name"] = request.Value
	case "move":
		if err := r.requireLive(ctx, item); err != nil {
			return nil, false, err
		}
		forbidden := ""
		if item.SourceKind == "folder" {
			forbidden = item.SourceID
		}
		if err := r.liveFolder(ctx, request.Value, forbidden); err != nil {
			return nil, false, err
		}
		if item.ParentID == request.Value {
			return nil, false, nil
		}
		if err := r.uniqueSibling(ctx, item, request.Value, item.Name); err != nil {
			return nil, false, err
		}
		field := "folder_id"
		if item.SourceKind == "folder" {
			field = "parent_id"
		}
		patch[field] = request.Value
	case "trash":
		if item.DeletedAt != nil {
			return nil, false, nil
		}
		patch["deleted_at"] = time.Now().UTC()
	case "restore":
		if item.DeletedAt == nil {
			return nil, false, nil
		}
		if err := r.liveFolder(ctx, item.ParentID, ""); err != nil {
			return nil, false, err
		}
		if err := r.uniqueSibling(ctx, item, item.ParentID, item.Name); err != nil {
			return nil, false, err
		}
		patch["deleted_at"] = nil
	default:
		return nil, false, ErrSource
	}
	return patch, true, nil
}

func (r *Repository) uniqueSibling(ctx context.Context, item summary, parent, name string) error {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, head := range view.snapshot.Heads {
		other, err := decodeSummary(head)
		if err != nil || other.SourceKind != item.SourceKind || other.SourceID == item.SourceID || other.ParentID != parent || head.Commit.State != "live" {
			continue
		}
		if other.Name == name {
			return ErrConflict
		}
	}
	return nil
}

func (r *Repository) requireLive(ctx context.Context, item summary) error {
	if item.DeletedAt != nil {
		return ErrNotFound
	}
	if item.SourceKind == "folder" {
		return r.liveFolder(ctx, item.SourceID, "")
	}
	return r.liveFolder(ctx, item.ParentID, "")
}

// A folder cutover hides its descendants without a partially applied cascade.
// Previously trashed children retain their own markers when a parent is restored.
func (r *Repository) liveFolder(ctx context.Context, id, forbidden string) error {
	seen := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if id == "" {
			return ErrSource
		}
		if id == forbidden || seen[id] {
			return ErrCycle
		}
		seen[id] = true
		version, err := r.version(ctx, "folder", id)
		if err != nil {
			return err
		}
		item, err := decodeSummary(version)
		if err != nil {
			return err
		}
		if version.Commit.State != "live" || item.DeletedAt != nil {
			return ErrNotFound
		}
		if item.IsRoot {
			if item.ParentID != "" {
				return ErrSource
			}
			return nil
		}
		id = item.ParentID
	}
}
