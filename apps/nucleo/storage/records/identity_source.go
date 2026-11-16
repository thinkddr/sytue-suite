// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// HistoricalSource exposes no CRUD, catalog view or UI facade. The explicit
// migration may finish its retained SQL snapshot before identity freeze. Once
// freezing has started, Prepare only opens the existing authenticated receipt.
// Both identities remain under the caller's original device/session work lease.
type HistoricalSource struct{ repository *Repository }

type frozenRecordSource interface {
	storage.RecordStore
	FrozenStorageBindings(context.Context) (storage.FrozenBinding, error)
}

func NewHistoricalSource(cfg Config) (*HistoricalSource, error) {
	if cfg.WorkLease == nil || cfg.Crypto == nil || cfg.BindPublicationContext != nil {
		return nil, ErrSource
	}
	if _, ok := cfg.Crypto.(frozenRecordSource); !ok {
		return nil, ErrSource
	}
	repository, err := newRepository(cfg, storage.LegacyIdentity)
	if err != nil {
		return nil, err
	}
	return &HistoricalSource{repository: repository}, nil
}

// Prepare requires the fresh public routing read by the shared cutover. It may
// never reinterpret a frozen source without a receipt as an empty SQL import.
func (s *HistoricalSource) Prepare(ctx context.Context, state storage.IdentityCutoverState) error {
	r := s.repository
	if state.Validate(r.cfg.Scope) != nil || state.Phase == "isolated" {
		return ErrSource
	}
	err := r.Initialize(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotReady) || state.Phase != "legacy" {
		return err
	}
	if err := s.checkUncapturedSource(ctx); err != nil {
		return err
	}
	return r.ImportLegacySnapshot(ctx)
}

func (s *HistoricalSource) PrivateStorageScope() storage.Scope {
	return s.repository.PrivateStorageScope()
}
func (s *HistoricalSource) Close()                          { s.repository.Close() }
func (s *HistoricalSource) Drain(ctx context.Context) error { return s.repository.Drain(ctx) }
func (s *HistoricalSource) Resume() error                   { return s.repository.Resume() }
func (s *HistoricalSource) Pending(ctx context.Context) (offline.PendingWork, error) {
	return s.repository.Pending(ctx)
}

// A backend still saying legacy cannot overwrite an already-private catalog by
// minting a new SQL baseline. Reconciliation requires its own accepted epoch.
func (s *HistoricalSource) checkUncapturedSource(ctx context.Context) error {
	r := s.repository
	state, err := r.cfg.Source.State(ctx, r.cfg.Scope)
	if err != nil {
		return err
	}
	if err := r.validateState(state); err != nil {
		return err
	}
	if state.Mode != "legacy" {
		return nil
	}
	snapshot, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if errors.Is(err, storage.ErrCatalogSource) {
		return ErrSource
	}
	if err != nil {
		return err
	}
	if len(snapshot.Entries) != 0 {
		return ErrSource
	}
	return nil
}
