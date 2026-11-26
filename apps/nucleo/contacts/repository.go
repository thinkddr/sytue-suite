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
	// fotos guarda la última instantánea con la revisión del catálogo de la que salió
	// (foto.go): mientras no cambie, no se vuelve a leer el almacén.
	fotos fotos
}

func New(cfg Config) (*Repository, error) {
	cfg, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}

	repo := &Repository{scope: cfg.Scope}
	// Abrir ya lee y valida la libreta entera: lo leído es la primera instantánea
	// guardada, y la primera consulta no vuelve a leerla (foto.go).
	cfg.ValidateCatalog = func(ctx context.Context, catalog records.Catalog) error {
		owner, ok := catalog.(records.CatalogOwner)
		var core *records.Repository
		var revision uint64
		if ok {
			core, revision = owner.CatalogOwner()
		}
		read, err := readCatalogIndexes(ctx, repo.scope, catalog)
		if err != nil {
			return err
		}
		if core != nil && core == repo.core {
			repo.fotos.poner(&fotoGuardada{revision: revision, snapshot: read.snapshot, favorites: read.favorites, trash: read.trash})
		}
		return nil
	}
	core, err := records.New(cfg)
	if err != nil {
		return nil, err
	}
	repo.core = core
	return repo, nil
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

func (r *Repository) Close() {
	r.core.Close()
	r.fotos.olvidar()
}

// Snapshot es la libreta entera, validada. Es la guardada si el catálogo no ha cambiado
// desde que se leyó (foto.go); si no, se lee. No se puede modificar: quien la recibe
// la consulta o deriva otra (WithAggregates, WithIndexes).
func (r *Repository) Snapshot(parent context.Context) (*Snapshot, error) {
	foto, err := r.foto(parent)
	if err != nil {
		return nil, err
	}
	return foto.snapshot.copia(), nil
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
	read, err := readCatalogIndexes(ctx, scope, catalog)
	return read.snapshot, err
}

// lectura es una instantánea leída del catálogo con los índices de favoritos y de
// papelera con que se compuso (nil los que no hay).
type lectura struct {
	snapshot  *Snapshot
	favorites *FavoriteIndex
	trash     *TrashIndex
}

func readCatalogIndexes(ctx context.Context, scope storage.Scope, catalog records.Catalog) (lectura, error) {
	snapshot, projection, err := readCatalogProjection(ctx, scope, catalog)
	if err != nil {
		return lectura{}, err
	}
	return lectura{snapshot: snapshot, favorites: projection.favorites, trash: projection.trash}, nil
}

func readCatalogProjection(ctx context.Context, scope storage.Scope, catalog records.Catalog) (*Snapshot, catalogProjection, error) {
	entries, err := catalog.Entries()
	if err != nil {
		return nil, catalogProjection{}, err
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
			return nil, catalogProjection{}, err
		}
		buffers = append(buffers, raw)
		if err := projection.add(scope, entry, raw); err != nil {
			return nil, catalogProjection{}, err
		}
	}
	snapshot, err := NewSnapshot(scope, projection.rows)
	if err != nil {
		return nil, catalogProjection{}, err
	}
	snapshot, err = snapshot.WithAggregates(projection.changes)
	if err != nil {
		return nil, catalogProjection{}, err
	}
	snapshot, err = snapshot.WithIndexes(projection.favorites, projection.trash)
	if err != nil {
		return nil, catalogProjection{}, err
	}

	if err := ctx.Err(); err != nil {
		return nil, catalogProjection{}, err
	}
	// Completion requires renderable domain data too. No partially usable agenda
	// can open after a malformed date, projection, card or group relationship.
	if err := validateViews(snapshot); err != nil {
		return nil, catalogProjection{}, err
	}
	return snapshot, projection, nil
}

// validateViews proyecta la instantánea entera: lo proyectado se queda en ella
// (vistas.go) para las consultas que vengan.
func validateViews(snapshot *Snapshot) error {
	if _, err := snapshot.libros(); err != nil {
		return err
	}
	if _, err := snapshot.contactos(); err != nil {
		return err
	}
	if _, err := snapshot.grupos(); err != nil {
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
