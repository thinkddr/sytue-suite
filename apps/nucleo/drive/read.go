// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func (r *Repository) version(ctx context.Context, kind, id string) (storage.ObjectVersion, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return storage.ObjectVersion{}, ErrClosed
	}
	if !r.ready {
		return storage.ObjectVersion{}, ErrNotReady
	}
	var versions []storage.ObjectVersion
	object := objectID(kind, id)
	for _, version := range view.snapshot.Heads {
		if version.ObjectID == object {
			versions = append(versions, version)
		}
	}
	if len(versions) == 0 {
		return storage.ObjectVersion{}, ErrNotFound
	}
	if len(versions) != 1 {
		return storage.ObjectVersion{}, ErrConflict
	}
	if versions[0].Commit.State == "deleted" {
		return storage.ObjectVersion{}, ErrNotFound
	}
	return versions[0], nil
}

// ReadRaw returns the complete original JSON, retaining domain integer bytes.
func (r *Repository) ReadRaw(parent context.Context, kind, id string) ([]byte, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	return r.readRaw(ctx, kind, id)
}

func (r *Repository) readRaw(ctx context.Context, kind, id string) ([]byte, error) {
	version, err := r.version(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	_, _, raw, err := r.readEntry(ctx, version)
	return raw, err
}

func (r *Repository) Root(parent context.Context) (api.Folder, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Folder{}, err
	}
	defer done()
	view, err := r.currentObservation(ctx)
	if err != nil {
		return api.Folder{}, err
	}
	r.mu.Lock()
	if !r.ready {
		r.mu.Unlock()
		return api.Folder{}, ErrNotReady
	}
	versions := []storage.ObjectVersion{}
	for _, head := range view.snapshot.Heads {
		item, err := decodeSummary(head)
		if err == nil && item.SourceKind == "folder" && item.IsRoot && head.Commit.State == "live" {
			versions = append(versions, head)
		}
	}
	r.mu.Unlock()
	if len(versions) == 0 {
		return api.Folder{}, ErrNotFound
	}
	if len(versions) != 1 {
		return api.Folder{}, ErrConflict
	}
	_, _, raw, err := r.readEntry(ctx, versions[0])
	defer clear(raw)
	if err != nil {
		return api.Folder{}, err
	}
	var folder api.Folder
	if json.Unmarshal(raw, &folder) != nil || !folder.IsRoot {
		return api.Folder{}, ErrSource
	}
	return folder, nil
}

func (r *Repository) Folder(parent context.Context, id string) (api.Folder, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Folder{}, err
	}
	defer done()
	raw, err := r.readRaw(ctx, "folder", id)
	defer clear(raw)
	if err != nil {
		return api.Folder{}, err
	}
	var folder api.Folder
	if json.Unmarshal(raw, &folder) != nil || folder.ID != id {
		return api.Folder{}, ErrSource
	}
	if err := r.liveFolder(ctx, id, ""); err != nil {
		return api.Folder{}, err
	}
	return folder, nil
}

func (r *Repository) File(parent context.Context, id string) (api.File, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.File{}, err
	}
	defer done()
	raw, err := r.readRaw(ctx, "file", id)
	defer clear(raw)
	if err != nil {
		return api.File{}, err
	}
	file, err := fileView(raw, id)
	if err != nil {
		return api.File{}, err
	}
	if file.DeletedAt != nil {
		return api.File{}, ErrNotFound
	}
	if err := r.liveFolder(ctx, file.FolderID, ""); err != nil {
		return api.File{}, err
	}
	return file, nil
}

func (r *Repository) Children(parent context.Context, id, cursor string) (api.Children, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.Children{}, err
	}
	defer done()
	version, err := r.version(ctx, "folder", id)
	if err != nil {
		return api.Children{}, err
	}
	_, _, raw, err := r.readEntry(ctx, version)
	if err != nil {
		return api.Children{}, err
	}
	var folder api.Folder
	err = json.Unmarshal(raw, &folder)
	clear(raw)
	if err != nil || folder.ID != id {
		return api.Children{}, ErrSource
	}
	if folder.DeletedAt != nil {
		return api.Children{}, ErrNotFound
	}
	if err := r.liveFolder(ctx, id, ""); err != nil {
		return api.Children{}, err
	}
	after, err := r.childrenCursor(ctx, id, cursor)
	if err != nil {
		return api.Children{}, err
	}
	candidates, err := r.childVersions(ctx, id)
	if err != nil {
		return api.Children{}, err
	}
	return r.readChildren(ctx, id, after, candidates)
}

func (r *Repository) childrenCursor(ctx context.Context, id, cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > 4096 {
		return "", ErrSource
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil {
		return "", ErrSource
	}
	var pin struct{ Folder, After string }
	if json.Unmarshal(raw, &pin) != nil || pin.Folder != id {
		return "", ErrSource
	}
	version, err := r.version(ctx, "file", pin.After)
	if err != nil {
		return "", err
	}
	last, err := decodeSummary(version)
	if err != nil || last.ParentID != id {
		return "", ErrSource
	}
	return childKey(last), nil
}

func childKey(item summary) string { return strings.ToLower(item.Name) + "\x00" + item.SourceID }

func (r *Repository) childVersions(ctx context.Context, id string) ([]storage.ObjectVersion, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	candidates := []storage.ObjectVersion{}
	byObject := map[string][]storage.ObjectVersion{}
	for _, version := range view.snapshot.Heads {
		byObject[version.ObjectID] = append(byObject[version.ObjectID], version)
	}
	for _, heads := range byObject {
		for _, head := range heads {
			item, err := decodeSummary(head)
			if err != nil || item.SourceKind != "folder" && item.SourceKind != "file" || item.ParentID != id || head.Commit.State != "live" {
				continue
			}
			if len(heads) != 1 {
				return nil, ErrConflict
			}
			candidates = append(candidates, head)
		}
	}
	slices.SortFunc(candidates, func(a, b storage.ObjectVersion) int {
		x, _ := decodeSummary(a)
		y, _ := decodeSummary(b)
		if x.SourceKind != y.SourceKind {
			if x.SourceKind == "folder" {
				return -1
			}
			return 1
		}
		return strings.Compare(childKey(x), childKey(y))
	})
	return candidates, nil
}

func (r *Repository) readChildren(ctx context.Context, id, after string, candidates []storage.ObjectVersion) (api.Children, error) {
	output := api.Children{Folders: []api.Folder{}, Files: []api.File{}}
	for _, version := range candidates {
		item, err := decodeSummary(version)
		if err != nil {
			return api.Children{}, err
		}
		if item.SourceKind == "file" {
			if childKey(item) <= after {
				continue
			}
			if len(output.Files) == 500 {
				last := output.Files[len(output.Files)-1]
				encoded, _ := json.Marshal(struct{ Folder, After string }{Folder: id, After: last.ID})
				output.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
				break
			}
		}
		if err := r.appendChild(ctx, &output, version, item.SourceKind); err != nil {
			return api.Children{}, err
		}
	}
	return output, nil
}

func (r *Repository) appendChild(ctx context.Context, output *api.Children, version storage.ObjectVersion, kind string) error {
	item, _, raw, err := r.readEntry(ctx, version)
	if err != nil {
		return err
	}
	defer clear(raw)
	if kind == "folder" {
		var child api.Folder
		if json.Unmarshal(raw, &child) != nil {
			return ErrSource
		}
		output.Folders = append(output.Folders, child)
	} else {
		file, err := fileView(raw, item.SourceID)
		if err != nil {
			return err
		}
		output.Files = append(output.Files, file)
	}
	return nil
}

func (r *Repository) Download(parent context.Context, id string) (io.ReadCloser, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	version, err := r.version(ctx, "file", id)
	if err != nil {
		done()
		return nil, err
	}
	item, data, raw, err := r.readEntry(ctx, version)
	clear(raw)
	if err != nil {
		done()
		return nil, err
	}
	if version.Commit.State != "live" || data.Binary == nil {
		done()
		return nil, ErrNotFound
	}
	if err := r.requireLive(ctx, item); err != nil {
		done()
		return nil, err
	}
	return r.streamBody(ctx, done, data.observation, item, *data.Binary), nil
}

func (r *Repository) streamBody(ctx context.Context, done func(), view *observation, item summary, pointer bodyPointer) io.ReadCloser {
	return r.stream(ctx, done, func(ctx context.Context, out io.Writer) error {
		return r.readBody(ctx, view, pointer, item, "binary", out)
	})
}

func (*Repository) stream(ctx context.Context, done func(), write func(context.Context, io.Writer) error) io.ReadCloser {
	return storage.PipeRead(ctx, done, write)
}
