// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type creationIntent struct {
	InputSHA256 string              `json:"input_sha256"`
	Item        summary             `json:"item"`
	Raw         []byte              `json:"raw"`
	Binary      *bodyPointer        `json:"binary,omitempty"`
	FromObject  string              `json:"from_object,omitempty"`
	From        storage.Reference   `json:"from,omitzero"`
	Staged      *offline.StagedFile `json:"staged,omitempty"`
	Restored    *contentSource      `json:"restored,omitempty"`
	Expected    *uploadExpectation  `json:"expected,omitempty"`
}

type contentSource struct {
	ObjectID  string            `json:"object_id"`
	Reference storage.Reference `json:"reference"`
}

// CreateFolder keeps its generated identity and timestamps through ambiguous
// responses. A repeated successful user action still reports a name conflict.
func (r *Repository) CreateFolder(parent context.Context, id, name string) (api.Folder, error) {
	if !storage.ValidDriveName(name) {
		return api.Folder{}, ErrInvalidName
	}
	if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: "folder", ID: id}) {
		return api.Folder{}, ErrSource
	}
	name = strings.TrimSpace(name)
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Folder{}, err
	}
	defer done()
	var result api.Folder
	err = r.writeReady(ctx, func(ctx context.Context) error {
		var err error
		result, err = r.createFolder(ctx, id, name)
		return err
	})
	return result, err
}

func (r *Repository) createFolder(ctx context.Context, parent, name string) (api.Folder, error) {
	input, _ := json.Marshal(struct{ Operation, Parent, Name string }{"create_folder", parent, name})
	inputHash := digest(input)
	key := "create_folder_" + inputHash
	var work rowWork
	defer func() {
		if work.Creation != nil {
			clear(work.Creation.Raw)
		}
	}()
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return api.Folder{}, err
	}
	if !found {
		work, err = r.newFolderIntent(ctx, parent, name, inputHash)
		if err != nil {
			return api.Folder{}, err
		}
		if err := r.saveWork(ctx, key, work); err != nil {
			return api.Folder{}, err
		}
	}
	if err := r.checkFolderIntent(ctx, work, parent, name, inputHash); err != nil {
		return api.Folder{}, err
	}
	intent := work.Creation
	data := entry{Kind: entryKind, SourceKind: "folder", SourceID: work.SourceID, Raw: bytes.Clone(intent.Raw)}
	defer clear(data.Raw)
	if err := r.publishEntry(ctx, key, &work, intent.Item, data); err != nil {
		return api.Folder{}, err
	}
	stored, err := r.cfg.Crypto.StorageObject(ctx, *work.Catalog, objectID("folder", work.SourceID))
	if err != nil {
		return api.Folder{}, err
	}
	fresh, release, err := r.temporaryObservation(ctx)
	if err != nil {
		return api.Folder{}, err
	}
	defer release()
	_, _, raw, err := r.readEntry(fresh, stored)
	defer clear(raw)
	if err != nil {
		return api.Folder{}, err
	}
	if !bytes.Equal(raw, intent.Raw) {
		return api.Folder{}, ErrSource
	}
	var folder api.Folder
	if json.Unmarshal(raw, &folder) != nil || folder.ID != work.SourceID {
		return api.Folder{}, ErrSource
	}
	if err := r.cfg.Journal.Delete(r.owner(), migrationNamespace, key); err != nil {
		return api.Folder{}, err
	}
	return folder, nil
}

func (r *Repository) newFolderIntent(ctx context.Context, parent, name, inputHash string) (rowWork, error) {
	if err := r.liveFolder(ctx, parent, ""); err != nil {
		return rowWork{}, err
	}
	id := uuid.NewString()
	if err := r.uniqueSibling(ctx, summary{SourceKind: "folder", SourceID: id}, parent, name); err != nil {
		return rowWork{}, err
	}
	now := time.Now().UTC()
	raw, err := json.Marshal(map[string]any{"id": id, "parent_id": parent, "name": name, "is_root": false, "is_group": false, "owner_sub": r.cfg.Scope.Subject, "created_at": now, "updated_at": now})
	if err != nil {
		return rowWork{}, err
	}
	item, err := summarize(storage.LegacySourceRow{Kind: "folder", ID: id, Payload: raw}, "")
	if err != nil {
		clear(raw)
		return rowWork{}, err
	}
	binding, err := r.cfg.Crypto.StorageBindings(ctx)
	if err != nil {
		clear(raw)
		return rowWork{}, err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		clear(raw)
		return rowWork{}, ErrSource
	}
	intent := &creationIntent{InputSHA256: inputHash, Item: item, Raw: raw}
	return rowWork{V: 1, Scope: r.cfg.Scope, SourceKind: "folder", SourceID: id, RawSHA256: item.RawSHA256, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}, ParentsSet: true, Creation: intent}, nil
}

func (r *Repository) checkFolderIntent(ctx context.Context, work rowWork, parent, name, inputHash string) error {
	intent := work.Creation
	if intent == nil || !intent.matchesFolder(parent, name, inputHash) || !work.matches(r.cfg.Scope, intent.Item) {
		return ErrSource
	}
	if work.Mutation != nil || !work.ParentsSet || len(work.Parents) != 0 || len(work.Bodies) != 0 || work.Epoch != "" {
		return ErrSource
	}
	if intent.Binary != nil || intent.Staged != nil || intent.Restored != nil || intent.FromObject != "" || intent.From != (storage.Reference{}) {
		return ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	if err := r.liveFolder(ctx, parent, work.SourceID); err != nil {
		return err
	}
	for _, version := range r.versions(objectID("folder", work.SourceID)) {
		if version.RevisionID != work.Revision+"_index" {
			return ErrConflict
		}
	}
	return verifyProjection(intent.Item, "live", intent.Raw)
}

func (intent creationIntent) matchesFolder(parent, name, inputHash string) bool {
	item := intent.Item
	return intent.InputSHA256 == inputHash && item.SourceKind == "folder" && item.ParentID == parent && item.Name == name && !item.IsRoot && item.DeletedAt == nil && item.Epoch == "" && !item.Seed && item.BaselineSHA256 == "" && digest(intent.Raw) == item.RawSHA256 && sourceRowValid(storage.LegacySourceRow{Kind: "folder", ID: item.SourceID, Payload: intent.Raw}, "")
}
