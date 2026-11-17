// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const uploadSourceNamespace = "drive-private-upload-source-v1"

type uploadRequest struct {
	Folder      string             `json:"folder"`
	Name        string             `json:"name"`
	ContentType string             `json:"content_type"`
	SizeBytes   int64              `json:"size_bytes"`
	Expected    *uploadExpectation `json:"expected,omitempty"`
}

type uploadExpectation struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Checksum string `json:"checksum,omitempty"`
}

func (expected uploadExpectation) valid() bool {
	return storage.SourceCursorValid(storage.LegacySourceCursor{Kind: "file", ID: expected.ID}) && expected.Version > 0 && expected.Version < 0x7fffffff && (expected.Checksum == "" || validHash(expected.Checksum))
}

func sameUploadExpectation(a, b *uploadExpectation) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.valid() && b.valid() && *a == *b
}

func (request uploadRequest) identity() string {
	raw, _ := json.Marshal(request)
	return digest(raw)
}

// Upload accepts even non-seekable OS input. The existing encrypted store keeps
// a bounded-memory immutable source until the private cloud commit is verified.
// It never acknowledges a cache-only upload or removes the user's original.
func (r *Repository) Upload(parent context.Context, folder, name, contentType string, input io.Reader, size int64) (api.File, error) {
	return r.upload(parent, folder, name, contentType, input, size, nil)
}

// ReplaceFile binds the existing upload/WAL implementation to the original
// version observed by an editor. Unacknowledged attempts retain these exact
// expectations and parents when another device publishes in the meantime.
func (r *Repository) ReplaceFile(parent context.Context, original api.File, contentType string, input io.Reader, size int64) (api.File, error) {
	expected := uploadExpectation{ID: original.ID, Version: original.Version, Checksum: strings.ToLower(original.Checksum)}
	if !expected.valid() || original.DeletedAt != nil {
		return api.File{}, ErrSource
	}
	return r.upload(parent, original.FolderID, original.Name, contentType, input, size, &expected)
}

var _ api.DriveFileReplacer = (*Repository)(nil)

func (r *Repository) upload(parent context.Context, folder, name, contentType string, input io.Reader, size int64, expected *uploadExpectation) (api.File, error) {
	if !storage.ValidDriveName(name) {
		return api.File{}, ErrInvalidName
	}
	if input == nil || size < 0 || size > storage.MaxFileBytes || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: "folder", ID: folder}) {
		return api.File{}, ErrSource
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if len(contentType) > 1024 || strings.ContainsAny(contentType, "\r\n\x00") {
		return api.File{}, ErrSource
	}
	request := uploadRequest{Folder: folder, Name: strings.TrimSpace(name), ContentType: contentType, SizeBytes: size, Expected: expected}
	ctx, done, err := r.start(parent)
	if err != nil {
		return api.File{}, err
	}
	defer done()
	var result api.File
	err = r.writeReady(ctx, func(ctx context.Context) error {
		if err := r.liveFolder(ctx, request.Folder, ""); err != nil {
			return err
		}
		staged, err := r.cfg.Journal.StageFile(ctx, r.owner(), uploadSourceNamespace, input, size)
		if err != nil {
			return err
		}
		// A retry captures its input independently, then compares commitments. It
		// cannot cause retained SDK keys to encrypt different source bytes.
		retained := false
		defer func() {
			if !retained {
				_ = r.cfg.Journal.DeleteStagedFile(r.owner(), uploadSourceNamespace, staged)
			}
		}()
		var work rowWork
		key, err := r.uploadIntent(ctx, request, staged, &work)
		// An atomic journal write can report an ambiguous filesystem error after
		// replacement. Keep the captured source whenever it may be referenced.
		if work.Creation != nil {
			defer clear(work.Creation.Raw)
			if work.Creation.Staged != nil {
				retained = work.Creation.Staged.ID == staged.ID
			}
		}
		if err != nil {
			return err
		}
		result, err = r.finishUpload(ctx, key, &work, request)
		return err
	})
	return result, err
}

func (r *Repository) uploadIntent(ctx context.Context, request uploadRequest, staged offline.StagedFile, work *rowWork) (string, error) {
	input := request.identity()
	key := "upload_" + input
	found, err := r.cfg.Journal.Read(r.owner(), migrationNamespace, key, work)
	if err != nil {
		return "", err
	}
	if !found {
		*work, err = r.newUploadIntent(ctx, request, input, staged)
		if err != nil {
			return "", err
		}
		if err := r.saveWork(ctx, key, *work); err != nil {
			return "", err
		}
	}
	intent := work.Creation
	if intent == nil || intent.Staged == nil || intent.InputSHA256 != input || intent.Staged.SHA256 != staged.SHA256 || intent.Staged.SizeBytes != staged.SizeBytes {
		return "", ErrSource
	}
	return key, nil
}

// ResumeUploads needs only the retained encrypted input, not an open OS picker
// or the original path. The caller owns scheduling and cancellation with its
// verified device lifecycle. One invalid decoded intent does not block unrelated jobs;
// partial successes are returned alongside the accumulated errors.
func (r *Repository) ResumeUploads(parent context.Context) ([]api.File, error) {
	ctx, done, err := r.start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	completed := []api.File{}
	err = r.writeReady(ctx, func(ctx context.Context) error {
		rows, err := r.cfg.Journal.List(r.owner(), migrationNamespace)
		if err != nil {
			return err
		}
		defer func() {
			for _, row := range rows {
				clear(row.Value)
			}
		}()
		var failures []error
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(failures, err)...)
			}
			if !strings.HasPrefix(row.ID, "upload_") {
				clear(row.Value)
				continue
			}
			file, err := r.resumeUpload(ctx, row)
			clear(row.Value)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			completed = append(completed, file)
		}
		return errors.Join(failures...)
	})
	return completed, err
}

func (r *Repository) resumeUpload(ctx context.Context, row offline.Record) (api.File, error) {
	var work rowWork
	defer func() {
		if work.Creation != nil {
			clear(work.Creation.Raw)
		}
	}()
	if json.Unmarshal(row.Value, &work) != nil || work.Creation == nil || work.Creation.Staged == nil {
		return api.File{}, ErrSource
	}
	item := work.Creation.Item
	request := uploadRequest{Folder: item.ParentID, Name: item.Name, ContentType: item.ContentType, SizeBytes: item.SizeBytes, Expected: work.Creation.Expected}
	input := request.identity()
	if row.ID != "upload_"+input || work.Creation.InputSHA256 != input {
		return api.File{}, ErrSource
	}
	return r.finishUpload(ctx, row.ID, &work, request)
}

func (r *Repository) newUploadIntent(ctx context.Context, request uploadRequest, input string, staged offline.StagedFile) (rowWork, error) {
	item, raw, base, err := r.uploadData(ctx, request, staged.SHA256)
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
	intent := &creationIntent{InputSHA256: input, Item: item, Raw: raw, Staged: &staged}
	if request.Expected != nil {
		expected := *request.Expected
		intent.Expected = &expected
	}
	work := rowWork{V: 1, Scope: r.cfg.Scope, Epoch: item.Epoch, SourceKind: "file", SourceID: item.SourceID, RawSHA256: item.RawSHA256, Revision: uuid.NewString(), Target: binding.Target, Bodies: map[string]*fileWork{}, ParentsSet: true, Creation: intent}
	if base != nil {
		intent.FromObject, intent.From = base.ObjectID, base.Reference
		work.Parents = []storage.Reference{base.Reference}
	}
	return work, nil
}

func (r *Repository) uploadDestination(ctx context.Context, request uploadRequest) (string, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := ""
	byObject := map[string][]storage.ObjectVersion{}
	for _, version := range view.snapshot.Heads {
		byObject[version.ObjectID] = append(byObject[version.ObjectID], version)
	}
	for _, heads := range byObject {
		for _, index := range heads {
			item, err := decodeSummary(index)
			if err != nil || item.SourceKind != "file" || item.ParentID != request.Folder || item.Name != request.Name || index.Commit.State != "live" {
				continue
			}
			if len(heads) != 1 || id != "" {
				return "", ErrConflict
			}
			id = item.SourceID
		}
	}
	return id, nil
}

func (r *Repository) uploadData(ctx context.Context, request uploadRequest, sum string) (summary, []byte, *storage.ObjectVersion, error) {
	id, err := r.uploadDestination(ctx, request)
	if err != nil {
		return summary{}, nil, nil, err
	}
	if request.Expected != nil && (!request.Expected.valid() || id != request.Expected.ID) {
		return summary{}, nil, nil, ErrConflict
	}
	now := time.Now().UTC()
	if id == "" {
		id = uuid.NewString()
		raw, err := json.Marshal(map[string]any{"id": id, "folder_id": request.Folder, "name": request.Name, "size_bytes": request.SizeBytes, "content_type": request.ContentType, "checksum": sum, "version": 1, "owner_sub": r.cfg.Scope.Subject, "created_at": now, "updated_at": now})
		if err != nil {
			return summary{}, nil, nil, err
		}
		item, err := summarize(storage.LegacySourceRow{Kind: "file", ID: id, Payload: raw}, "")
		return item, raw, nil, err
	}
	base, err := r.version(ctx, "file", id)
	if err != nil {
		return summary{}, nil, nil, err
	}
	prior, _, original, err := r.readEntry(ctx, base)
	defer clear(original)
	if err != nil {
		return summary{}, nil, nil, err
	}
	if prior.Version < 1 || prior.Version >= 0x7fffffff {
		return summary{}, nil, nil, ErrSource
	}
	if request.Expected != nil && (prior.Version != request.Expected.Version || prior.Checksum != request.Expected.Checksum) {
		return summary{}, nil, nil, ErrConflict
	}
	raw, err := patchRaw(original, map[string]any{"size_bytes": request.SizeBytes, "content_type": request.ContentType, "checksum": sum, "version": prior.Version + 1, "updated_at": now, "author_sub": r.cfg.Scope.Subject})
	if err != nil {
		return summary{}, nil, nil, err
	}
	item, err := summarize(storage.LegacySourceRow{Kind: "file", ID: id, Payload: raw}, prior.Epoch)
	item.Seed, item.BaselineSHA256 = false, prior.BaselineSHA256
	return item, raw, &base, err
}

func (work rowWork) matchesUpload(scope storage.Scope, request uploadRequest) bool {
	intent := work.Creation
	if intent == nil || intent.Staged == nil || !intent.matchesUploadInput(request) {
		return false
	}
	item := intent.Item
	return work.matches(scope, item) && work.SourceKind == "file" && work.Mutation == nil && intent.Restored == nil && work.validFileRevisionBase() && digest(intent.Raw) == work.RawSHA256 && item.ParentID == request.Folder && item.Name == request.Name && item.ContentType == request.ContentType && item.SizeBytes == request.SizeBytes && item.Checksum == intent.Staged.SHA256 && item.SizeBytes == intent.Staged.SizeBytes && validHash(item.Checksum)
}

func (intent *creationIntent) matchesUploadInput(request uploadRequest) bool {
	if intent == nil || !sameUploadExpectation(intent.Expected, request.Expected) || intent.InputSHA256 != request.identity() {
		return false
	}
	return intent.Expected == nil || intent.Expected.ID == intent.Item.SourceID && intent.Expected.Version == intent.Item.Version-1
}

func (work rowWork) validFileRevisionBase() bool {
	intent := work.Creation
	item := intent.Item
	if !work.ParentsSet || item.Seed || !item.validLineage() {
		return false
	}
	if intent.From == (storage.Reference{}) {
		return len(work.Parents) == 0 && intent.FromObject == "" && work.Epoch == "" && item.Version == 1
	}
	return intent.From.Valid() && intent.FromObject == objectID("file", work.SourceID) && slices.Equal(work.Parents, []storage.Reference{intent.From}) && item.Version > 1
}

func (r *Repository) checkFileRevisionBase(ctx context.Context, work rowWork) error {
	intent := work.Creation
	if intent.From == (storage.Reference{}) {
		return nil
	}
	prior, err := r.cfg.Crypto.StorageObject(ctx, intent.From, intent.FromObject)
	if err != nil {
		return err
	}
	base, _, raw, err := r.readEntry(ctx, prior)
	clear(raw)
	if err != nil {
		return err
	}
	item := intent.Item
	if intent.Expected != nil && (!intent.Expected.valid() || intent.Expected.ID != base.SourceID || intent.Expected.Version != base.Version || intent.Expected.Checksum != base.Checksum) {
		return ErrSource
	}
	if base.Version < 1 || base.Version >= 0x7fffffff || item.Version != base.Version+1 || base.ParentID != item.ParentID || base.Name != item.Name || base.Epoch != item.Epoch || base.BaselineSHA256 != item.BaselineSHA256 || base.DeletedAt != nil {
		return ErrSource
	}
	return nil
}

func (r *Repository) finishUpload(ctx context.Context, key string, work *rowWork, request uploadRequest) (api.File, error) {
	if !work.matchesUpload(r.cfg.Scope, request) {
		return api.File{}, ErrSource
	}
	intent := work.Creation
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return api.File{}, err
	}
	if err := verifyProjection(intent.Item, "live", intent.Raw); err != nil {
		return api.File{}, err
	}
	if err := r.liveFolder(ctx, request.Folder, ""); err != nil {
		return api.File{}, err
	}
	if err := r.checkFileRevisionBase(ctx, *work); err != nil {
		return api.File{}, err
	}
	if intent.Binary == nil {
		if err := r.uploadCapturedBody(ctx, key, work); err != nil {
			return api.File{}, err
		}
	}
	if _, err := intent.Binary.source(intent.Item, "binary"); err != nil || intent.Binary.Origin != (bodyOrigin{}) {
		return api.File{}, ErrSource
	}
	if _, _, err := r.contentReference(ctx, nil, intent.Item, *intent.Binary); err != nil {
		return api.File{}, err
	}
	raw, err := r.commitCreation(ctx, key, work)
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
	if err := r.cfg.Journal.DeleteStagedFile(r.owner(), uploadSourceNamespace, *intent.Staged); err != nil {
		return api.File{}, err
	}
	if err := r.cfg.Journal.Delete(r.owner(), migrationNamespace, key); err != nil {
		return api.File{}, err
	}
	return file, nil
}

func (r *Repository) uploadCapturedBody(ctx context.Context, key string, work *rowWork) error {
	intent := work.Creation
	source, err := r.cfg.Journal.OpenStagedFile(ctx, r.owner(), uploadSourceNamespace, *intent.Staged)
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	pointer, err := r.ensureBody(ctx, key, work, intent.Item, "binary", source, intent.Staged.SizeBytes, func() (string, error) { return intent.Staged.SHA256, nil })
	if err != nil {
		return err
	}
	intent.Binary = &pointer
	return r.saveWork(ctx, key, *work)
}
