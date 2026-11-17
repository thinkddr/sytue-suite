// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type favoriteRow struct {
	ID        string     `json:"id"`
	Subject   string     `json:"subject"`
	FileID    string     `json:"file_id"`
	FolderID  string     `json:"folder_id"`
	CreatedAt time.Time  `json:"created_at"`
	DeletedAt *time.Time `json:"deleted_at"`
}

func (row favoriteRow) target() (string, string) {
	if row.FileID != "" {
		return "file", row.FileID
	}
	return "folder", row.FolderID
}

type favoriteMark struct {
	node resource
	row  favoriteRow
	raw  []byte
}

func clearMarks(marks map[string]favoriteMark) {
	for _, mark := range marks {
		clear(mark.raw)
	}
}

func (r *Repository) favoriteMarks(ctx context.Context, tree privateTree) (map[string]favoriteMark, error) {
	marks := map[string]favoriteMark{}
	for _, node := range tree.marks {
		if err := tree.unique(node); err != nil {
			clearMarks(marks)
			return nil, err
		}
		if node.index.Commit.State == "deleted" {
			continue
		}
		_, _, raw, err := r.readEntry(ctx, node.index)
		var row favoriteRow
		if err == nil && (json.Unmarshal(raw, &row) != nil || row.ID != node.item.SourceID || row.Subject != r.cfg.Scope.Subject || (row.FileID == "") == (row.FolderID == "") || row.CreatedAt.IsZero()) {
			err = ErrSource
		}
		kind, id := row.target()
		if err == nil && !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
			err = ErrSource
		}
		key := objectID(kind, id)
		if _, duplicate := marks[key]; err == nil && duplicate {
			err = ErrConflict
		}
		if err != nil {
			clear(raw)
			clearMarks(marks)
			return nil, err
		}
		marks[key] = favoriteMark{node: node, row: row, raw: raw}
	}
	return marks, nil
}

// Favorites resolves personal stars and paths inside one private projection.
// Foreign targets retained by migration are not opened through a plaintext API.
func (r *Repository) Favorites(parent context.Context) (api.Compartidos, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	tree, err := r.resourceTree(ctx)
	if err != nil {
		return nil, err
	}
	marks, err := r.favoriteMarks(ctx, tree)
	if err != nil {
		return nil, err
	}
	defer clearMarks(marks)
	type selected struct {
		node resource
		at   time.Time
	}
	nodes := []selected{}
	for _, node := range tree.nodes {
		mark, exists := marks[node.index.ObjectID]
		if !exists || mark.row.DeletedAt != nil || node.item.DeletedAt != nil || node.item.IsRoot || node.index.Commit.State != "live" {
			continue
		}
		_, visible, err := tree.parentPath(ctx, node)
		if err != nil {
			return nil, err
		}
		if visible {
			nodes = append(nodes, selected{node: node, at: mark.row.CreatedAt})
		}
	}
	slices.SortFunc(nodes, func(a, b selected) int {
		if a.node.item.SourceKind != b.node.item.SourceKind {
			if a.node.item.SourceKind == "folder" {
				return -1
			}
			return 1
		}
		if order := b.at.Compare(a.at); order != 0 {
			return order
		}
		return strings.Compare(a.node.item.SourceID, b.node.item.SourceID)
	})
	result := api.Compartidos{}
	for _, node := range nodes {
		view, err := r.queryView(ctx, tree, node.node)
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

type favoriteRequest struct {
	Kind api.DriveKind `json:"kind"`
	ID   string        `json:"id"`
	On   bool          `json:"on"`
}

type favoriteIntent struct {
	InputSHA256 string            `json:"input_sha256"`
	Base        storage.Reference `json:"base,omitzero"`
	Item        summary           `json:"item"`
	Raw         []byte            `json:"raw"`
}

// SetFavorite keeps the imported identity and unknown JSON fields. Removing a
// star is a private tombstone; retries retain their exact destination and parents.
func (r *Repository) SetFavorite(parent context.Context, kind api.DriveKind, id string, on bool) error {
	if !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: resourceKind(kind), ID: id}) || resourceKind(kind) == "" {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	request := favoriteRequest{Kind: kind, ID: id, On: on}
	return r.writeReady(ctx, func(ctx context.Context) error { return r.setFavorite(ctx, request) })
}

func (r *Repository) setFavorite(ctx context.Context, request favoriteRequest) error {
	input, _ := json.Marshal(request)
	hash := digest(input)
	key := "favorite_" + hash
	var work rowWork
	defer func() {
		if work.Favorite != nil {
			clear(work.Favorite.Raw)
		}
	}()
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, &work)
	if err != nil {
		return err
	}
	if !found {
		work, err = r.newFavoriteIntent(ctx, request, hash)
		if err != nil || work.Favorite == nil {
			return err
		}
		if err := r.saveWork(ctx, key, work); err != nil {
			return err
		}
	}
	if err := r.checkFavoriteIntent(ctx, work, request, hash); err != nil {
		return err
	}
	raw, err := r.commitEntry(ctx, key, &work, work.Favorite.Item, work.Favorite.Raw, nil)
	clear(raw)
	if err != nil {
		return err
	}
	return r.cfg.Journal.Delete(r.owner(), migrationNamespace, key)
}

func (r *Repository) newFavoriteIntent(ctx context.Context, request favoriteRequest, hash string) (rowWork, error) {
	tree, err := r.resourceTree(ctx)
	if err != nil {
		return rowWork{}, err
	}
	if request.On {
		if err := r.favoriteTarget(ctx, tree, request); err != nil {
			return rowWork{}, err
		}
	}
	marks, err := r.favoriteMarks(ctx, tree)
	if err != nil {
		return rowWork{}, err
	}
	defer clearMarks(marks)
	mark, exists := marks[objectID(resourceKind(request.Kind), request.ID)]
	if !exists && !request.On || exists && (mark.row.DeletedAt == nil) == request.On {
		return rowWork{}, nil
	}
	now := time.Now().UTC()
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(r.owner()+"\x00favorite\x00"+resourceKind(request.Kind)+"\x00"+request.ID)).String()
	raw, _ := json.Marshal(map[string]any{"id": id, "subject": r.cfg.Scope.Subject, resourceKind(request.Kind) + "_id": request.ID, "created_at": now, "updated_at": now})
	item := summary{}
	base := storage.Reference{}
	parents := []storage.Reference{}
	if exists {
		clear(raw)
		id, item, base = mark.row.ID, mark.node.item, mark.node.index.Reference
		parents = append(parents, base)
		patch := map[string]any{"updated_at": now, "deleted_at": now}
		if request.On {
			patch["created_at"], patch["deleted_at"] = now, nil
		}
		raw, err = patchRaw(mark.raw, patch)
		if err != nil {
			return rowWork{}, err
		}
	}
	next, err := summarize(storage.LegacySourceRow{Kind: "favorite", ID: id, Payload: raw}, item.Epoch)
	if err != nil {
		clear(raw)
		return rowWork{}, err
	}
	next.Seed, next.BaselineSHA256 = false, item.BaselineSHA256
	binding, err := r.cfg.Crypto.StorageBindings(ctx)
	if err != nil || !binding.Valid() || binding.Scope != r.cfg.Scope {
		clear(raw)
		if err == nil {
			err = ErrSource
		}
		return rowWork{}, err
	}
	intent := &favoriteIntent{InputSHA256: hash, Base: base, Item: next, Raw: raw}
	return rowWork{V: 1, Scope: r.cfg.Scope, Epoch: next.Epoch, SourceKind: "favorite", SourceID: id, RawSHA256: next.RawSHA256, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}, Parents: parents, ParentsSet: true, Favorite: intent}, nil
}

func (*Repository) favoriteTarget(ctx context.Context, tree privateTree, request favoriteRequest) error {
	for _, node := range tree.nodes {
		if node.item.SourceKind != resourceKind(request.Kind) || node.item.SourceID != request.ID {
			continue
		}
		if node.item.IsRoot {
			return ErrRootImmutable
		}
		if node.item.DeletedAt != nil || node.index.Commit.State != "live" {
			return ErrNotFound
		}
		_, visible, err := tree.parentPath(ctx, node)
		if err != nil {
			return err
		}
		if visible {
			return nil
		}
	}
	return ErrNotFound
}

func (work rowWork) matchesFavorite(scope storage.Scope, hash string) bool {
	intent := work.Favorite
	return intent != nil && work.Creation == nil && work.Mutation == nil && intent.InputSHA256 == hash && work.matches(scope, intent.Item) && work.SourceKind == "favorite" && !intent.Item.Seed && digest(intent.Raw) == work.RawSHA256 && work.ParentsSet
}

func (row favoriteRow) matches(request favoriteRequest, sourceID, subject string) bool {
	kind, id := row.target()
	return row.ID == sourceID && row.Subject == subject && (row.DeletedAt == nil) == request.On && !row.CreatedAt.IsZero() && kind == resourceKind(request.Kind) && id == request.ID && (row.FileID == "") != (row.FolderID == "")
}

func (r *Repository) checkFavoriteIntent(ctx context.Context, work rowWork, request favoriteRequest, hash string) error {
	if !work.matchesFavorite(r.cfg.Scope, hash) {
		return ErrSource
	}
	intent := work.Favorite
	state := "live"
	if !request.On {
		state = "trash"
	}
	if err := verifyProjection(intent.Item, state, intent.Raw); err != nil {
		return err
	}
	var row favoriteRow
	if json.Unmarshal(intent.Raw, &row) != nil || !row.matches(request, work.SourceID, r.cfg.Scope.Subject) {
		return ErrSource
	}
	if err := r.checkFavoriteBase(ctx, work, row); err != nil {
		return err
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	if request.On {
		tree, err := r.resourceTree(ctx)
		if err != nil {
			return err
		}
		return r.favoriteTarget(ctx, tree, request)
	}
	return nil
}

func (r *Repository) checkFavoriteBase(ctx context.Context, work rowWork, row favoriteRow) error {
	intent := work.Favorite
	if intent.Base == (storage.Reference{}) {
		if intent.Item.Epoch != "" || intent.Item.BaselineSHA256 != "" || row.DeletedAt != nil || len(work.Parents) != 0 {
			return ErrSource
		}
		return nil
	}
	if !intent.Base.Valid() || !slices.Equal([]storage.Reference{intent.Base}, work.Parents) {
		return ErrSource
	}
	prior, err := r.cfg.Crypto.StorageObject(ctx, intent.Base, objectID("favorite", work.SourceID))
	if err != nil {
		return err
	}
	base, _, raw, err := r.readEntry(ctx, prior)
	defer clear(raw)
	if err != nil {
		return err
	}
	var original favoriteRow
	if json.Unmarshal(raw, &original) != nil || original.ID != row.ID || original.Subject != row.Subject || original.FileID != row.FileID || original.FolderID != row.FolderID || base.Epoch != intent.Item.Epoch || base.BaselineSHA256 != intent.Item.BaselineSHA256 {
		return ErrSource
	}
	return nil
}
