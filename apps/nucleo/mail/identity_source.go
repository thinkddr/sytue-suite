// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

// HistoricalSource is an explicit, narrow migration capability. It exposes no
// mailbox data or CRUD facade, and accepts only the server's retained SQL epoch.
// Preparing a new SQL epoch belongs to the authenticated source enrollment.
type HistoricalSource struct {
	repository *Repository
	source     Source
}

type frozenMailSource interface {
	FrozenStorageBindings(context.Context) (storage.FrozenBinding, error)
}

type HistoricalSourceEnrollment interface {
	Source
	BeginPrivateStorageMigration(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error)
}

// OpenHistoricalSource explicitly captures the authenticated retained source.
// An existing private catalog with an unaccepted SQL epoch is blocked before
// Begin. Even an authenticated empty catalog still imports and verifies a real
// source receipt; SQL counts never stand in for completed E2E history.
func OpenHistoricalSource(parent context.Context, cfg Config, source HistoricalSourceEnrollment) (*HistoricalSource, error) {
	if !validHistoricalSourceConfig(cfg) || source == nil {
		return nil, ErrRecord
	}
	maximum, err := limits.MessageBytes(cfg.MaxMessageBytes)
	if err != nil {
		return nil, ErrRecord
	}
	cfg.MaxMessageBytes = maximum
	gate := offline.NewWorkGateWithLease(cfg.Context, cfg.WorkLease) //nolint:contextcheck // Admission inherits both the repository lifetime and parent via Start.
	defer gate.Close()                                               //nolint:contextcheck // Close cancels this temporary gate and waits for its retained permission.
	ctx, done, err := gate.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	state, err := captureHistoricalSource(ctx, cfg, source)
	if err != nil {
		return nil, err
	}
	cfg.MigrationState = state
	return NewHistoricalSource(cfg, source) //nolint:contextcheck // The returned repository retains cfg.Context, not this temporary admission context.
}

func validHistoricalSourceConfig(cfg Config) bool {
	if cfg.Context == nil || cfg.WorkLease == nil || cfg.Crypto == nil || !cfg.Scope.Valid() || cfg.Scope.Product != "mail" || cfg.Crypto.StorageIdentityVersion() != storage.LegacyIdentity || cfg.Journal == nil || !cfg.Journal.Encrypted() {
		return false
	}
	_, ok := cfg.Crypto.(frozenMailSource)
	return ok
}

func captureHistoricalSource(ctx context.Context, cfg Config, source HistoricalSourceEnrollment) (storage.LegacyMigrationState, error) {
	current, err := source.PrivateStorageCapabilities(ctx, cfg.Scope)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	if err := historicalCapabilities(cfg, current); err != nil {
		return storage.LegacyMigrationState{}, err
	}
	binding, err := cfg.Crypto.PrepareStorage(ctx, cfg.Scope)
	if err != nil {
		return storage.LegacyMigrationState{}, err
	}
	if !binding.Valid() || binding.Scope != cfg.Scope {
		return storage.LegacyMigrationState{}, ErrRecord
	}
	if current.Mode == "legacy" {
		snapshot, err := storage.LoadAtomicCatalog(ctx, cfg.Crypto, cfg.Scope)
		if err != nil {
			return storage.LegacyMigrationState{}, err
		}
		if len(snapshot.Entries) != 0 {
			return storage.LegacyMigrationState{}, ErrRecord
		}
	}
	state, _, err := transfer.BeginSource(ctx, transfer.SourceBeginConfig{
		Scope: cfg.Scope, Owner: cfg.Scope.Tenant + "\x00" + cfg.Scope.Subject + "\x00mail", Namespace: importNamespace, IntentKey: "snapshot", Journal: cfg.Journal, Current: current.LegacyMigrationState, Target: binding.Target,
		NewEpoch: func(ctx context.Context) (string, error) {
			if retained, ok := source.(transfer.SourceEpochProvider); ok {
				return retained.SnapshotEpoch(ctx, cfg.Scope)
			}
			return uuid.NewString(), nil
		}, CheckTarget: cfg.Crypto.CheckStoragePublicationTarget,
		Begin: func(ctx context.Context, scope storage.Scope, epoch string) (storage.LegacyMigrationState, error) {
			accepted, err := source.BeginPrivateStorageMigration(ctx, scope, epoch)
			if err != nil {
				return accepted, err
			}
			fresh, err := source.PrivateStorageCapabilities(ctx, scope)
			if err != nil {
				return accepted, err
			}
			if err := historicalCapabilities(cfg, fresh); err != nil {
				return accepted, err
			}
			if !sameSource(accepted, fresh.LegacyMigrationState) {
				return accepted, ErrRecord
			}
			return accepted, nil
		},
	})
	return state, err
}

func historicalCapabilities(cfg Config, current storage.AppMigrationState) error {
	maximum, err := limits.MessageBytes(current.MaxBodyBytes)
	if err != nil || current.Validate(cfg.Scope) != nil || current.Shared || !current.PrivateIngressReady || maximum != cfg.MaxMessageBytes {
		return ErrNotReady
	}
	return nil
}

func NewHistoricalSource(cfg Config, source Source) (*HistoricalSource, error) {
	if cfg.WorkLease == nil || source == nil || cfg.Crypto == nil {
		return nil, ErrRecord
	}
	if _, ok := cfg.Crypto.(frozenMailSource); !ok {
		return nil, ErrRecord
	}
	r, err := newRepository(cfg, storage.LegacyIdentity)
	if err != nil {
		return nil, err
	}
	return &HistoricalSource{repository: r, source: source}, nil
}

// Prepare may import the explicitly retained SQL snapshot only before the
// Matrix identity freeze. From freezing onward it checks existing E2E history.
func (s *HistoricalSource) Prepare(ctx context.Context, state storage.IdentityCutoverState) error {
	r := s.repository
	if state.Validate(r.cfg.Scope) != nil || state.Phase == "isolated" {
		return ErrRecord
	}
	err := r.run(ctx, false, func(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) error {
		if err := r.checkSourceState(ctx, s.source); err != nil {
			return err
		}
		receipt, err := r.completedReceipt(ctx, snapshot)
		if err != nil {
			return err
		}
		if state.Phase == "legacy" {
			if err := r.materializeSettings(ctx, snapshot, receipt); err != nil {
				return err
			}
		}
		return s.retireSourceBegin()
	})
	if err == nil || !errors.Is(err, ErrNotReady) || state.Phase != "legacy" {
		return err
	}
	if err := r.ImportRetainedSource(ctx, s.source); err != nil {
		return err
	}
	return r.run(ctx, false, func(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) error {
		if _, err := r.completedReceipt(ctx, snapshot); err != nil {
			return err
		}
		return s.retireSourceBegin()
	})
}

func (s *HistoricalSource) retireSourceBegin() error { return s.repository.retireSourceBegin() }

// retireSourceBegin borra la intención de la época una vez importada. Una
// intención de otra época o de otro destino no se borra: falla cerrado.
func (r *Repository) retireSourceBegin() error {
	owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
	var work transfer.SourceBeginIntent
	found, err := r.cfg.Journal.Read(owner, importNamespace, "snapshot", &work)
	if err != nil || !found {
		return err
	}
	if work.Version != 1 || work.Scope != r.cfg.Scope || work.Epoch != r.cfg.MigrationState.Epoch {
		return ErrRecord
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	return r.cfg.Journal.Delete(owner, importNamespace, "snapshot")
}

func (s *HistoricalSource) PrivateStorageScope() storage.Scope {
	return s.repository.PrivateStorageScope()
}

func (s *HistoricalSource) RetainedState() storage.LegacyMigrationState {
	state := s.repository.cfg.MigrationState
	if state.StartedAt != nil {
		started := *state.StartedAt
		state.StartedAt = &started
	}
	return state
}
func (s *HistoricalSource) Close()                          { s.repository.Close() }
func (s *HistoricalSource) Drain(ctx context.Context) error { return s.repository.Drain(ctx) }
func (s *HistoricalSource) Resume() error                   { return s.repository.Resume() }
func (s *HistoricalSource) Pending(ctx context.Context) (offline.PendingWork, error) {
	return s.repository.Pending(ctx)
}
