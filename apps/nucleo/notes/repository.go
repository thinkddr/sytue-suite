// SPDX-License-Identifier: Apache-2.0
// Package notes owns Notepads domain rules on the shared private record engine.
package notes

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/records"
)

const (
	entityKind    = "org.sytue.notes.entity.v1"
	receiptKind   = "org.sytue.notes.snapshot.v1"
	MaxEntityJSON = records.MaxEntityJSON
)

var (
	ErrSource   = records.ErrSource
	ErrNotReady = records.ErrNotReady
	ErrClosed   = records.ErrClosed
	ErrConflict = records.ErrConflict
	ErrNotFound = records.ErrNotFound
	ErrShared   = records.ErrShared
)

type (
	Crypto = records.Crypto
	Config = records.Config
)

type Repository struct {
	engine *records.Repository
	cfg    Config
}

var _ api.NotesData = (*Repository)(nil)

func New(cfg Config) (*Repository, error) {
	configuration, err := recordsConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	r := &Repository{cfg: configuration}
	engine, err := records.New(configuration)
	if err != nil {
		return nil, err
	}
	r.engine = engine
	return r, nil
}

// Both identity paths use the same domain validation. Callers cannot opt out
// of private ownership, relationships or the attachment-content boundary.
func recordsConfiguration(cfg Config) (Config, error) {
	if !cfg.Scope.Valid() || cfg.Scope.Product != "notes" {
		return cfg, ErrSource
	}
	validator := &Repository{cfg: cfg}
	cfg.Product = "notes"
	cfg.EntityKind = entityKind
	cfg.ReceiptKind = receiptKind
	cfg.BinaryKind, cfg.ValidateBinary = nil, nil
	cfg.ValidateRow = func(_ context.Context, kind, id string, raw []byte) error {
		return validator.validatePersonal(kind, id, raw)
	}
	cfg.ValidateCatalog = validator.validateRelations
	return cfg, nil
}
func (r *Repository) PrivateStorageScope() storage.Scope   { return r.cfg.Scope }
func (r *Repository) Close()                               { r.engine.Close() }
func (r *Repository) Initialize(ctx context.Context) error { return r.engine.Initialize(ctx) }
func (r *Repository) ImportLegacySnapshot(ctx context.Context) error {
	return r.engine.ImportLegacySnapshot(ctx)
}

func (r *Repository) ReadRaw(ctx context.Context, kind, id string) ([]byte, error) {
	return r.engine.ReadRaw(ctx, kind, id)
}

func (r *Repository) start(ctx context.Context) (context.Context, func(), error) {
	return r.engine.Start(ctx)
}

func (r *Repository) selected(kind string) ([]storage.ObjectVersion, error) {
	return r.engine.Selected(kind)
}

func (r *Repository) readyVersion(kind, id string) (storage.ObjectVersion, error) {
	return r.engine.ReadyVersion(kind, id)
}

func (r *Repository) readJSON(ctx context.Context, v storage.ObjectVersion) ([]byte, error) {
	return r.engine.ReadJSON(ctx, v)
}

func (r *Repository) mutate(ctx context.Context, operation string, input any, prepare func(context.Context) (string, string, string, []byte, error)) ([]byte, error) {
	return r.engine.Mutate(ctx, operation, input, prepare)
}
