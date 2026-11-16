// SPDX-License-Identifier: Apache-2.0
package calendar

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar/model"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

type Config = records.Config

type Repository struct {
	core  *records.Repository
	scope storage.Scope
}

var (
	_ api.CalendarData    = (*Repository)(nil)
	_ offline.PrivateWork = (*Repository)(nil)
)

func New(cfg Config) (*Repository, error) {
	cfg, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	core, err := records.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Repository{core: core, scope: cfg.Scope}, nil
}

func recordsConfiguration(cfg Config) (Config, error) {
	scope := cfg.Scope
	if !scope.Valid() || scope.Product != "calendar" {
		return cfg, model.ErrSnapshot
	}
	cfg.Product = "calendar"
	cfg.EntityKind = "org.sytue.calendar.entity.v1"
	cfg.ReceiptKind = "org.sytue.calendar.snapshot.v1"
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		return validateEntity(scope, kind, id, raw)
	}
	cfg.ValidateCatalog = func(ctx context.Context, catalog records.Catalog) error {
		_, err := readCatalog(ctx, scope, catalog)
		return err
	}
	return cfg, nil
}

func (r *Repository) PrivateStorageScope() storage.Scope   { return r.scope }
func (r *Repository) Initialize(ctx context.Context) error { return r.core.Initialize(ctx) }
func (r *Repository) ImportLegacySnapshot(ctx context.Context) error {
	return r.core.ImportLegacySnapshot(ctx)
}
func (r *Repository) Close()                          { r.core.Close() }
func (r *Repository) Drain(ctx context.Context) error { return r.core.Drain(ctx) }
func (r *Repository) Resume() error                   { return r.core.Resume() }
func (r *Repository) Pending(ctx context.Context) (offline.PendingWork, error) {
	return r.core.Pending(ctx)
}

func validateEntity(scope storage.Scope, kind, id string, raw []byte) error {
	if model.SourceKind(kind) {
		return model.ValidateRow(scope, model.Row{Kind: kind, ID: id, JSON: raw})
	}
	var a model.Aggregate
	if json.Unmarshal(raw, &a) != nil || a.Kind != kind || a.ID != id {
		return model.ErrSnapshot
	}
	return a.Validate(scope)
}

func readCatalog(ctx context.Context, scope storage.Scope, catalog records.Catalog) (*model.Snapshot, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	rows := []model.Row{}
	changes := []model.Aggregate{}
	defer func() {
		for _, row := range rows {
			clear(row.JSON)
		}
		for _, a := range changes {
			for _, row := range a.Rows {
				clear(row.JSON)
			}
		}
	}()
	for _, entry := range entries {
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		if err := validateEntity(scope, entry.Kind, entry.ID, raw); err != nil {
			clear(raw)
			return nil, err
		}
		if model.SourceKind(entry.Kind) {
			rows = append(rows, model.Row{Kind: entry.Kind, ID: entry.ID, JSON: raw})
			continue
		}
		var a model.Aggregate
		err = json.Unmarshal(raw, &a)
		clear(raw)
		if err != nil {
			return nil, model.ErrSnapshot
		}
		state := "live"
		for _, row := range a.Rows {
			if row.ID == a.ID && (row.Kind == "calendar" || row.Kind == "object") && rowDeleted(row) {
				state = "trash"
			}
		}
		if entry.State != state {
			return nil, model.ErrSnapshot
		}
		changes = append(changes, a)
	}
	baseline, err := model.NewSnapshot(scope, rows)
	if err != nil {
		return nil, err
	}
	result, err := baseline.WithAggregates(changes)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) Snapshot(parent context.Context) (*model.Snapshot, error) {
	ctx, done, err := r.core.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	catalog, err := r.core.Catalog()
	if err != nil {
		return nil, err
	}
	return readCatalog(ctx, r.scope, catalog)
}
