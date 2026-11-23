// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package drive owns private Drive application data on authorized devices.
// Source APIs are used only during migration. No data operation falls back to
// legacy plaintext storage when keys, history, or an encrypted copy are missing.
package drive

import (
	"context"
	"errors"
	"io"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var (
	ErrSource        = errors.New("drive: invalid or changed private migration source")
	ErrClosed        = errors.New("drive: private repository closed")
	ErrConflict      = errors.New("drive: multiple private revisions require resolution")
	ErrNotReady      = errors.New("drive: private migration is not complete")
	ErrNotFound      = errors.New("drive: private resource not found")
	ErrInvalidName   = errors.New("drive: invalid private file or folder name")
	ErrRootImmutable = errors.New("drive: private root is immutable")
	ErrCycle         = errors.New("drive: private folder move would create a cycle")
)

type Source interface {
	PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error)
	BeginPrivateStorageMigration(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error)
	PrivateStorageSourcePage(context.Context, storage.Scope, string, storage.LegacySourceCursor) (storage.LegacySourcePage, error)
	OpenPrivateStorageSource(context.Context, storage.Scope, string, string, string, int64) (io.ReadCloser, error)
}

// Crypto is the existing maintained SDK adapter, bound to one private scope.
// Domain code never receives recovery secrets or implements encryption itself.
type Crypto interface {
	storage.RecordStore
	UploadStorageFileAt(context.Context, *api.Drive, *offline.Store, string, storage.PublicationTarget, storage.FileRequest, io.ReaderAt) (storage.Reference, error)
	AcknowledgeStorageFile(context.Context, *offline.Store, string, storage.Reference) error
	DownloadStorageFile(context.Context, *api.Drive, storage.Reference, string, string, io.Writer) (storage.FileRequest, error)
	StorageFileInfo(context.Context, storage.Reference, string, string) (storage.FileRequest, error)
	StorageFileCommitment(context.Context, storage.Reference, string, string) (storage.FileRequest, string, error)
}

type Config struct {
	BindPublicationContext offline.WorkContextBinder
	WorkLease              offline.WorkLease
	Context                context.Context
	Scope                  storage.Scope
	Crypto                 Crypto
	Gateway                *api.Drive
	Source                 Source
	Journal                *offline.Store
	// DirectImport es solo para la bóveda directa (ADR 0086): una identidad
	// aislada que no es de Matrix y que, como la web, importa ella misma la
	// fuente del servidor (congelarla, copiar cada fila y dejar el recibo). Sin
	// él, una identidad aislada nunca importa: en Matrix lo hace el corte de
	// identidad. Ver initializeDirect.
	DirectImport bool
}

// The existing native API facade dispatches personal content to this repository.
var _ api.DriveData = (*Repository)(nil)

type Repository struct {
	cfg         Config
	ctx         context.Context
	cancel      context.CancelFunc
	gate        *offline.WorkGate
	mu          sync.Mutex
	closed      bool
	binding     storage.Binding
	entries     map[storage.Reference]storage.RecordEntry
	snapshot    storage.AtomicCatalogSnapshot
	observation *observation
	heads       map[string][]storage.ObjectVersion
	ready       bool
}

func New(cfg Config) (*Repository, error) {
	if cfg.Context == nil || !cfg.Scope.Valid() || cfg.Scope.Product != "drive" || cfg.Crypto == nil || cfg.Gateway == nil || cfg.Source == nil || !cfg.Journal.Encrypted() {
		return nil, ErrSource
	}
	ctx, cancel := context.WithCancel(cfg.Context)
	return &Repository{cfg: cfg, ctx: ctx, cancel: cancel, gate: offline.NewWorkGateWithLease(ctx, cfg.WorkLease)}, nil
}

func (r *Repository) start(parent context.Context) (context.Context, func(), error) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return nil, nil, ErrClosed
	}
	ctx, done, err := r.gate.StartBound(parent, r.cfg.BindPublicationContext)
	if errors.Is(err, offline.ErrWorkClosed) {
		return nil, nil, ErrClosed
	}
	if err != nil {
		return nil, nil, err
	}
	admission := &admittedObservations{repo: r, views: map[*observation]int{}}
	view, err := admission.holdCurrent()
	if err != nil {
		done()
		return nil, nil, err
	}
	ctx = context.WithValue(ctx, observationKey{r}, observedContext{admission: admission, view: view})
	return ctx, func() { admission.close(); done() }, nil
}

// Close cancels and drains this repository before discarding private projections.
// The caller owns the shared SDK device and the retained migration originals.
func (r *Repository) Close() {
	r.mu.Lock()
	r.closed = true
	r.cancel()
	r.mu.Unlock()
	r.gate.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observation != nil {
		r.observation.retired = true
		if r.observation.refs == 0 {
			clearObservation(r.observation)
		}
	} else {
		for _, versions := range r.heads {
			for _, version := range versions {
				clear(version.Commit.Metadata)
			}
		}
	}
	r.entries = nil
	r.snapshot = storage.AtomicCatalogSnapshot{}
	r.observation = nil
	r.heads = nil
	r.ready = false
}

// PrivateStorageScope returns the immutable owner of this repository.
func (r *Repository) PrivateStorageScope() storage.Scope { return r.cfg.Scope }

func (r *Repository) owner() string {
	return r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00" + r.cfg.Scope.Product
}

// Refresh replaces the projection only after authenticated remote history has
// loaded completely. It uses the same lease as writes and never reopens the
// legacy source. A failed refresh retains the last private projection.
func (r *Repository) Refresh(parent context.Context) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.writeReady(ctx, func(ctx context.Context) error { return ctx.Err() })
}
