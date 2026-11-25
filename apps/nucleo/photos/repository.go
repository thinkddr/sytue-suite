// SPDX-License-Identifier: Apache-2.0
// Package photos owns the private Photos graph on the shared record engine.
package photos

import (
	"context"
	"errors"
	"io"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type (
	Crypto        = records.Crypto
	Source        = records.Source
	SnapshotState = records.SnapshotState
	SnapshotRow   = records.SnapshotRow
	SnapshotPage  = records.SnapshotPage
	Config        = records.Config
)

var (
	ErrSource   = records.ErrSource
	ErrNotReady = records.ErrNotReady
	ErrClosed   = records.ErrClosed
	ErrConflict = records.ErrConflict
	ErrNotFound = records.ErrNotFound
	ErrShared   = records.ErrShared
)

type Repository struct {
	engine                  *records.Repository
	scope                   storage.Scope
	journal                 *offline.Store
	mediaMu                 sync.RWMutex
	editsByAsset            map[string]string
	membershipCache         map[string]membershipProjection
	albumCheckpoints        map[string]albumCheckpointProjection
	albumProjectionsRetired bool
	sourcePlans             *SourcePlanStore
}

func binaryKind(kind string) bool {
	return kind == "original" || kind == "variant" || kind == albumPageKind || kind == albumIndexKind
}

func New(cfg Config) (*Repository, error) {
	r := &Repository{scope: cfg.Scope, journal: cfg.Journal}
	engine, err := records.New(recordsConfigurationFor(cfg, r))
	if err != nil {
		return nil, err
	}
	r.engine = engine
	if source, ok := cfg.Source.(*PlannedSource); ok {
		if source.scope != cfg.Scope || source.plans == nil || source.plans.cfg.Scope != cfg.Scope {
			engine.Close()
			return nil, ErrSourcePlan
		}
		r.sourcePlans = source.plans
	}
	return r, nil
}

// recordsConfiguration fixes the same domain contract for the isolated
// repository and the narrow historical source; no caller replaces validators.
func recordsConfiguration(cfg Config) records.Config {
	r := &Repository{scope: cfg.Scope}
	return recordsConfigurationFor(cfg, r)
}

func recordsConfigurationFor(cfg Config, r *Repository) records.Config {
	return records.Config{
		WorkLease: cfg.WorkLease, Context: cfg.Context, Scope: cfg.Scope, Product: "photos", EntityKind: "org.sytue.photos.entity.v1", ReceiptKind: "org.sytue.photos.snapshot.v1", Crypto: cfg.Crypto, Gateway: cfg.Gateway, Journal: cfg.Journal, Source: cfg.Source, OnWrite: cfg.OnWrite,
		ValidateRow: func(_ context.Context, kind, id string, raw []byte) error {
			if err := model.ValidateRow(cfg.Scope, model.Row{Kind: kind, ID: id, JSON: raw}); err != nil {
				return err
			}
			if kind == "album" {
				_, err := r.albumSet(raw)
				return err
			}
			if kind == "asset" {
				_, _, err := r.ingestedAsset(raw)
				return err
			}
			return nil
		},
		ValidateCatalog: r.validateCatalog, BinaryKind: binaryKind, ValidateBinary: func(_ context.Context, row SnapshotRow) error {
			if !binaryKind(row.Kind) {
				return ErrSource
			}
			return row.Validate()
		},
	}
}

func (r *Repository) PrivateStorageScope() storage.Scope { return r.scope }
func (r *Repository) Close() {
	r.retireAlbumProjections()
	r.engine.Close()
	if r.sourcePlans != nil {
		r.sourcePlans.Close()
	}
	r.mediaMu.Lock()
	r.editsByAsset = nil
	r.membershipCache = nil
	r.albumCheckpoints = nil
	r.mediaMu.Unlock()
}

func (r *Repository) Initialize(ctx context.Context) error {
	if err := r.engine.Initialize(ctx); err != nil {
		return err
	}
	return r.indexEdits(ctx)
}

func (r *Repository) ImportLegacySnapshot(ctx context.Context) error {
	if err := r.engine.ImportLegacySnapshot(ctx); err != nil {
		return err
	}
	return r.indexEdits(ctx)
}

func (r *Repository) ReadRaw(ctx context.Context, kind, id string) ([]byte, error) {
	return r.engine.ReadRaw(ctx, kind, id)
}

// The source plan uses this repository's WAL namespace. Its gate joins the same
// registered owner, while Pending counts that shared journal exactly once.
func (r *Repository) Drain(ctx context.Context) error {
	r.retireAlbumProjections()
	if r.sourcePlans != nil {
		return errors.Join(r.sourcePlans.Drain(ctx), r.engine.Drain(ctx))
	}
	return r.engine.Drain(ctx)
}

func (r *Repository) Resume() error {
	var err error
	if r.sourcePlans != nil {
		err = errors.Join(r.sourcePlans.Resume(), r.engine.Resume())
	} else {
		err = r.engine.Resume()
	}
	if err == nil {
		r.mediaMu.Lock()
		r.albumProjectionsRetired = false
		r.mediaMu.Unlock()
	}
	return err
}

func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.engine.Pending(ctx)
}

func (r *Repository) validateCatalog(ctx context.Context, catalog records.Catalog) error {
	_, err := r.readSnapshot(ctx, catalog, true)
	return err
}

func (r *Repository) snapshot(ctx context.Context, catalog records.Catalog) (*model.Snapshot, error) {
	return r.readSnapshot(ctx, catalog, false)
}

func (r *Repository) readSnapshot(ctx context.Context, catalog records.Catalog, validationOnly bool) (*model.Snapshot, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	// La fila asset decide si una ingesta en curso es visible: sin ella, la fila
	// media queda dormida y no entra en el grafo (ver ingestMediaRow).
	live := map[string]bool{}
	for _, entry := range entries {
		if entry.Kind == "asset" {
			live[entry.ID] = true
		}
	}
	rows := []model.Row{}
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
	}()
	blobs := []storage.StreamSourceRow{}
	for _, entry := range entries {
		if entry.Binary {
			binaryCatalog, ok := catalog.(records.BinaryCatalog)
			if !ok {
				return nil, ErrSource
			}
			row, err := binaryCatalog.BinaryInfo(ctx, entry)
			if err != nil {
				return nil, err
			}
			blobs = append(blobs, row)
			continue
		}
		row, media, visible, err := r.snapshotEntry(ctx, catalog, entry, live, validationOnly)
		if err != nil {
			return nil, err
		}
		if !visible {
			continue
		}
		if media.Kind != "" {
			blobs = append(blobs, media)
		}
		rows = append(rows, row)
	}
	snapshot, err := model.NewSnapshot(r.scope, rows)
	if err != nil {
		return nil, err
	}
	if err := snapshot.PersonalCompatible(); err != nil {
		return nil, ErrShared
	}
	if err := r.validatePrivateAlbums(ctx, catalog, rows, validationOnly); err != nil {
		return nil, err
	}
	linked, err := r.validateIngestedAssets(ctx, catalog, rows)
	if err != nil {
		return nil, err
	}
	for _, media := range linked {
		blobs = append(blobs, media.Descriptor())
	}
	if err := snapshot.ValidateMedia(blobs, linked...); err != nil {
		return nil, err
	}
	if err := r.rememberMemberships(entries, rows); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *Repository) snapshotEntry(ctx context.Context, catalog records.Catalog, entry records.Entry, live map[string]bool, validationOnly bool) (model.Row, storage.StreamSourceRow, bool, error) {
	row, err := r.snapshotRow(ctx, catalog, entry, validationOnly && entry.Kind != "media")
	if err != nil {
		return model.Row{}, storage.StreamSourceRow{}, false, err
	}
	if entry.Kind != "media" {
		return row, storage.StreamSourceRow{}, true, nil
	}
	visible, inject, file, err := ingestMediaRow(ctx, catalog, row.JSON, r.scope, live)
	if err != nil || !visible {
		clear(row.JSON)
		return model.Row{}, storage.StreamSourceRow{}, false, err
	}
	if validationOnly {
		projected, err := model.SourceValidationRow(r.scope, row)
		clear(row.JSON)
		if err != nil {
			return model.Row{}, storage.StreamSourceRow{}, false, err
		}
		row = projected
	}
	if inject {
		return row, file.Row, true, nil
	}
	return row, storage.StreamSourceRow{}, true, nil
}

func (r *Repository) snapshotRow(ctx context.Context, catalog records.Catalog, entry records.Entry, validationOnly bool) (model.Row, error) {
	if validationOnly && entry.Kind == "album_asset" {
		row, reused, err := r.membershipValidationRow(ctx, entry)
		if err != nil || reused {
			return row, err
		}
	}
	raw, err := catalog.ReadJSON(ctx, entry)
	if err != nil {
		return model.Row{}, err
	}
	if entry.Kind == "edit" {
		if err := r.validateEditRenditions(ctx, catalog, raw); err != nil {
			clear(raw)
			return model.Row{}, err
		}
	}
	row := model.Row{Kind: entry.Kind, ID: entry.ID, JSON: raw}
	if !validationOnly {
		return row, nil
	}
	projected, err := model.SourceValidationRow(r.scope, row)
	clear(raw)
	return projected, err
}

func (r *Repository) Snapshot(ctx context.Context) (*model.Snapshot, error) {
	work, done, err := r.engine.Start(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	catalog, err := r.engine.Catalog()
	if err != nil {
		return nil, err
	}
	return r.snapshot(work, catalog)
}

// Original preserves the original's bytes; it never exports a rendered image.
// An ingested photo keeps them as a retained file linked from its media row.
func (r *Repository) Original(ctx context.Context, asset string, out io.Writer) error {
	_, err := r.engine.ReadBinary(ctx, "original", asset, out)
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	file, err := r.linkedOriginal(ctx, asset)
	if err != nil {
		return err
	}
	return r.engine.ReadRetainedFile(ctx, file, out)
}

// ReadVariant reads a retained thumbnail/preview by its immutable media ID.
// New variants are produced by an authorized device worker, never by the server.
func (r *Repository) ReadVariant(ctx context.Context, id string, out io.Writer) error {
	_, err := r.engine.ReadBinary(ctx, "variant", id, out)
	return err
}

// AuditBinaries verifies provider availability before any source retirement decision.
func (r *Repository) AuditBinaries(ctx context.Context) (records.BinaryAudit, error) {
	return r.engine.AuditBinaries(ctx)
}
