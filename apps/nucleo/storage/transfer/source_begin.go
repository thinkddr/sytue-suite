// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"context"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourceBeginIntent preserves the existing snapshot WAL wire contract. A retry
// must use this epoch and original publication device, including after lost ACK.
type SourceBeginIntent struct {
	Version int                       `json:"v"`
	Scope   storage.Scope             `json:"scope"`
	Epoch   string                    `json:"epoch"`
	Target  storage.PublicationTarget `json:"target"`
}

type SourceBeginConfig struct {
	Scope                       storage.Scope
	Owner, Namespace, IntentKey string
	Journal                     *offline.Store
	Current                     storage.LegacyMigrationState
	Target                      storage.PublicationTarget
	NewEpoch                    func(context.Context) (string, error)
	Begin                       func(context.Context, storage.Scope, string) (storage.LegacyMigrationState, error)
	CheckTarget                 func(storage.PublicationTarget) error
}

// BeginSource retains an epoch before source-side effects. The domain must
// first authenticate its complete catalog and reject shared/unreconciled data;
// this helper never infers an empty baseline from SQL counts. Its own lock is
// distinct from the caller's catalog lock and its replaceable WAL record.
func BeginSource(ctx context.Context, cfg SourceBeginConfig) (storage.LegacyMigrationState, SourceBeginIntent, error) {
	state := cfg.Current
	var work SourceBeginIntent
	if !cfg.valid() {
		return state, work, ErrSource
	}
	err := cfg.Journal.Exclusive(ctx, cfg.Owner, cfg.Namespace, cfg.IntentKey+"-begin-admission", func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := cfg.CheckTarget(cfg.Target); err != nil {
			return err
		}
		var err error
		work, err = cfg.retainedIntent(ctx)
		if err != nil {
			return err
		}
		if work.Version != 1 || work.Scope != cfg.Scope || !storage.MigrationEpoch(work.Epoch) || !work.Target.Valid() {
			return ErrSource
		}
		if err := cfg.CheckTarget(work.Target); err != nil {
			return err
		}
		if state.Mode == "legacy" {
			state, err = cfg.Begin(ctx, cfg.Scope, work.Epoch)
			if err != nil {
				return err
			}
		}
		if state.Validate(cfg.Scope) != nil || state.Mode == "legacy" || state.Epoch != work.Epoch {
			return ErrSource
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return cfg.CheckTarget(work.Target)
	})
	return state, work, err
}

func (cfg SourceBeginConfig) retainedIntent(ctx context.Context) (SourceBeginIntent, error) {
	var work SourceBeginIntent
	found, err := cfg.Journal.Read(cfg.Owner, cfg.Namespace, cfg.IntentKey, &work)
	if err != nil || found {
		return work, err
	}
	epoch := cfg.Current.Epoch
	if cfg.Current.Mode == "legacy" {
		if cfg.NewEpoch == nil {
			return work, ErrSource
		}
		epoch, err = cfg.NewEpoch(ctx)
		if err != nil {
			return work, err
		}
	}
	if !storage.MigrationEpoch(epoch) {
		return work, ErrSource
	}
	work = SourceBeginIntent{Version: 1, Scope: cfg.Scope, Epoch: epoch, Target: cfg.Target}
	if err := cfg.Journal.Write(cfg.Owner, cfg.Namespace, cfg.IntentKey, work); err != nil {
		return work, err
	}
	return work, nil
}

func (cfg SourceBeginConfig) valid() bool {
	return cfg.Scope.Valid() && cfg.Owner == cfg.Scope.Tenant+"\x00"+cfg.Scope.Subject+"\x00"+cfg.Scope.Product && cfg.Namespace != "" && len(cfg.Namespace) <= 128 && !strings.ContainsRune(cfg.Namespace, 0) && cfg.IntentKey != "" && len(cfg.IntentKey) <= 128 && !strings.ContainsRune(cfg.IntentKey, 0) && cfg.Journal != nil && cfg.Journal.Encrypted() && cfg.Current.Validate(cfg.Scope) == nil && cfg.Target.Valid() && cfg.Begin != nil && cfg.CheckTarget != nil
}
