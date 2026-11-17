// SPDX-License-Identifier: Apache-2.0
package contacts

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/contacts/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

// Config uses the suite's shared records engine and maintained crypto adapter.
// Product identifiers and validators are fixed by New, never caller overrides.
type Config = records.Config

type Repository struct {
	core  *records.Repository
	scope storage.Scope
}

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

// recordsConfiguration is shared by private data and historical source
// validation; product kinds and graph checks remain owned by Contacts.
func recordsConfiguration(cfg Config) (Config, error) {
	scope := cfg.Scope
	if !scope.Valid() || scope.Product != "contacts" {
		return cfg, ErrSnapshot
	}
	cfg.Product = "contacts"
	cfg.EntityKind = "org.sytue.contacts.entity.v1"
	cfg.ReceiptKind = "org.sytue.contacts.snapshot.v1"
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
func (r *Repository) Close() { r.core.Close() }

func (r *Repository) Snapshot(parent context.Context) (*Snapshot, error) {
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

func validateEntity(scope storage.Scope, kind, id string, raw []byte) error {
	if kind == model.FavoriteIndexKind {
		var state FavoriteIndex
		if json.Unmarshal(raw, &state) != nil || state.ID != id {
			return ErrSnapshot
		}
		return state.Validate(scope)
	}
	if kind == model.TrashIndexKind {
		var state TrashIndex
		if json.Unmarshal(raw, &state) != nil || state.ID != id {
			return ErrSnapshot
		}
		return state.Validate(scope)
	}
	if model.SourceKind(kind) {
		return ValidateRow(scope, Row{Kind: kind, ID: id, JSON: raw})
	}
	var aggregate Aggregate
	if json.Unmarshal(raw, &aggregate) != nil || aggregate.Kind != kind || aggregate.ID != id {
		return ErrSnapshot
	}
	return aggregate.Validate(scope)
}

func readCatalog(ctx context.Context, scope storage.Scope, catalog records.Catalog) (*Snapshot, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, err
	}
	projection := catalogProjection{rows: make([]Row, 0, len(entries))}
	buffers := [][]byte{}
	defer func() {
		for _, raw := range buffers {
			clear(raw)
		}
	}()
	for _, entry := range entries {
		raw, err := catalog.ReadJSON(ctx, entry)
		if err != nil {
			return nil, err
		}
		buffers = append(buffers, raw)
		if err := projection.add(scope, entry, raw); err != nil {
			return nil, err
		}
	}
	snapshot, err := NewSnapshot(scope, projection.rows)
	if err != nil {
		return nil, err
	}
	snapshot, err = snapshot.WithAggregates(projection.changes)
	if err != nil {
		return nil, err
	}
	snapshot, err = snapshot.WithIndexes(projection.favorites, projection.trash)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Completion requires renderable domain data too. No partially usable agenda
	// can open after a malformed date, projection, card or group relationship.
	if err := validateViews(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func validateViews(snapshot *Snapshot) error {
	if _, err := snapshot.Books(); err != nil {
		return err
	}
	if _, err := snapshot.Contacts(); err != nil {
		return err
	}
	if _, err := snapshot.Groups(); err != nil {
		return err
	}
	return nil
}

type catalogProjection struct {
	rows      []Row
	changes   []Aggregate
	favorites *FavoriteIndex
	trash     *TrashIndex
}

func (p *catalogProjection) add(scope storage.Scope, entry records.Entry, raw []byte) error {
	if err := validateEntity(scope, entry.Kind, entry.ID, raw); err != nil {
		return err
	}
	if model.SourceKind(entry.Kind) {
		p.rows = append(p.rows, Row{Kind: entry.Kind, ID: entry.ID, JSON: raw})
		return nil
	}
	switch entry.Kind {
	case model.FavoriteIndexKind:
		if p.favorites != nil {
			return ErrSnapshot
		}
		p.favorites = &FavoriteIndex{}
		if json.Unmarshal(raw, p.favorites) != nil {
			return ErrSnapshot
		}
	case model.TrashIndexKind:
		if p.trash != nil {
			return ErrSnapshot
		}
		p.trash = &TrashIndex{}
		if json.Unmarshal(raw, p.trash) != nil {
			return ErrSnapshot
		}
	default:
		var change Aggregate
		if json.Unmarshal(raw, &change) != nil {
			return ErrSnapshot
		}
		p.changes = append(p.changes, change)
	}
	return nil
}
