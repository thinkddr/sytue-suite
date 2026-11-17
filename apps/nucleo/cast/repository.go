// SPDX-License-Identifier: Apache-2.0
// Package cast owns device-only video catalogs and exact encrypted media.
package cast

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/cast/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type (
	Crypto = records.Crypto
	Source = records.Source
	Config = records.Config
)

var (
	ErrSource   = records.ErrSource
	ErrShared   = records.ErrShared
	ErrNotReady = records.ErrNotReady
	ErrNotFound = records.ErrNotFound
)

type Repository struct {
	scope  storage.Scope
	engine *records.Repository
}

func New(cfg Config) (*Repository, error) {
	r := &Repository{scope: cfg.Scope}
	engine, err := records.New(r.recordsConfiguration(cfg))
	if err != nil {
		return nil, err
	}
	r.engine = engine
	return r, nil
}

func (r *Repository) recordsConfiguration(cfg Config) Config {
	cfg.Product = "cast"
	cfg.EntityKind = "org.sytue.cast.entity.v1"
	cfg.ReceiptKind = "org.sytue.cast.snapshot.v1"
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		return model.ValidateRow(r.scope, model.Row{Kind: kind, ID: id, JSON: raw})
	}
	cfg.ValidateCatalog = r.validateCatalog
	cfg.BinaryKind = func(kind string) bool { return kind == model.BinaryKind }
	cfg.ValidateBinary = func(_ context.Context, row records.SnapshotRow) error {
		if row.Kind != model.BinaryKind {
			return ErrSource
		}
		return row.Validate()
	}
	return cfg
}

func (r *Repository) PrivateStorageScope() storage.Scope   { return r.scope }
func (r *Repository) Close()                               { r.engine.Close() }
func (r *Repository) Initialize(ctx context.Context) error { return r.engine.Initialize(ctx) }
func (r *Repository) ImportLegacySnapshot(ctx context.Context) error {
	return r.engine.ImportLegacySnapshot(ctx)
}
func (r *Repository) Drain(ctx context.Context) error { return r.engine.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.engine.Resume() }
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.engine.Pending(ctx)
}

func (r *Repository) ReadRaw(ctx context.Context, kind, id string) ([]byte, error) {
	return r.engine.ReadRaw(ctx, kind, id)
}

func (r *Repository) ReadObject(ctx context.Context, id string, out io.Writer) error {
	_, err := r.engine.ReadBinary(ctx, model.BinaryKind, id, out)
	return err
}

func (r *Repository) validateCatalog(ctx context.Context, catalog records.Catalog) error {
	entries, err := catalog.Entries()
	if err != nil {
		return err
	}
	rows := []model.Row{}
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
	}()
	blobs := []storage.StreamSourceRow{}
	binaryEntries := map[string]records.Entry{}
	for _, entry := range entries {
		if entry.Binary {
			binaryCatalog, ok := catalog.(records.BinaryCatalog)
			if !ok {
				return ErrSource
			}
			descriptor, err := binaryCatalog.BinaryInfo(ctx, entry)
			if err != nil {
				return err
			}
			blobs = append(blobs, descriptor)
			binaryEntries[entry.ID] = entry
			continue
		}
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return err
		}
		rows = append(rows, model.Row{Kind: entry.Kind, ID: entry.ID, JSON: raw})
	}
	snapshot, err := model.NewSnapshot(r.scope, rows)
	if err != nil {
		return err
	}
	if err := snapshot.PersonalCompatible(); err != nil {
		if errors.Is(err, model.ErrShared) {
			return ErrShared
		}
		return err
	}
	if err := snapshot.ValidateMedia(blobs); err != nil {
		return err
	}
	return validatePlaylists(ctx, catalog, snapshot, binaryEntries)
}

type playlistWriter struct{ buffer *bytes.Buffer }

func (w playlistWriter) Write(p []byte) (int, error) {
	if len(p) > model.MaxPlaylistBytes-w.buffer.Len() {
		return 0, model.ErrSnapshot
	}
	return w.buffer.Write(p)
}

func validatePlaylists(ctx context.Context, catalog records.Catalog, snapshot *model.Snapshot, entries map[string]records.Entry) error {
	playlists := map[string][]byte{}
	defer func() {
		for _, raw := range playlists {
			clear(raw)
		}
	}()
	for _, media := range snapshot.Media() {
		if path.Ext(media.Key) != ".m3u8" {
			continue
		}
		if media.Size > model.MaxPlaylistBytes {
			return model.ErrSnapshot
		}
		reader, ok := catalog.(records.BinaryContentCatalog)
		if !ok {
			return ErrSource
		}
		var output bytes.Buffer
		descriptor, err := reader.ReadBinary(ctx, entries[media.ID], playlistWriter{buffer: &output})
		if err != nil {
			clear(output.Bytes())
			return err
		}
		if descriptor != media.Descriptor() {
			clear(output.Bytes())
			return ErrSource
		}
		playlists[media.ID] = output.Bytes()
	}
	return snapshot.ValidatePlaylists(playlists)
}

func (r *Repository) Video(ctx context.Context, id string) (model.VideoRow, error) {
	raw, err := r.engine.ReadRaw(ctx, "video", id)
	defer clear(raw)
	if err != nil {
		return model.VideoRow{}, err
	}
	var video model.VideoRow
	if json.Unmarshal(raw, &video) != nil || video.ID != id || video.Owner != r.scope.Subject || video.Visibility != model.PrivateVisibility {
		return model.VideoRow{}, ErrSource
	}
	return video, nil
}

func (r *Repository) Videos(ctx context.Context, trash bool) ([]model.VideoRow, error) {
	versions, err := r.engine.Selected("video")
	if err != nil {
		return nil, err
	}
	result := []model.VideoRow{}
	for _, version := range versions {
		raw, err := r.engine.ReadJSON(ctx, version)
		if err != nil {
			return nil, err
		}
		var video model.VideoRow
		err = json.Unmarshal(raw, &video)
		clear(raw)
		if err != nil {
			return nil, ErrSource
		}
		if (video.DeletedAt != nil) == trash {
			result = append(result, video)
		}
	}
	slices.SortFunc(result, func(a, b model.VideoRow) int {
		if order := b.CreatedAt.Compare(a.CreatedAt); order != 0 {
			return order
		}
		return strings.Compare(b.ID, a.ID)
	})
	return result, nil
}

// Rename changes private metadata only; publication audiences are never
// rewritten by an edit, and no server transcoding work is created.
func (r *Repository) Rename(ctx context.Context, id, title string) error {
	title, err := model.Titulo(title)
	if err != nil {
		return err
	}
	result, err := r.engine.Mutate(ctx, "rename", []string{id, title}, func(ctx context.Context) (string, string, string, []byte, error) {
		raw, err := r.engine.ReadRaw(ctx, "video", id)
		defer clear(raw)
		if err != nil {
			return "", "", "", nil, err
		}
		var fields map[string]json.RawMessage
		var video model.VideoRow
		if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(raw, &video) != nil || video.ID != id || video.Owner != r.scope.Subject || video.Visibility != model.PrivateVisibility {
			return "", "", "", nil, ErrSource
		}
		fields["title"], _ = json.Marshal(title)
		fields["updated_at"], _ = json.Marshal(time.Now().UTC())
		encoded, err := json.Marshal(fields)
		state := "live"
		if video.DeletedAt != nil {
			state = "trash"
		}
		return "video", id, state, encoded, err
	})
	clear(result)
	return err
}

// AuditBinaries verifies every retained object through the shared full-byte audit.
func (r *Repository) AuditBinaries(ctx context.Context) (records.BinaryAudit, error) {
	return r.engine.AuditBinaries(ctx)
}
