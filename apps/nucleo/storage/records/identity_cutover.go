// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

type IdentityCutoverConfig struct {
	Scope     storage.Scope
	Service   catalogtransfer.CutoverService
	Legacy    *HistoricalSource
	Isolated  *Repository
	CopyFile  CopyPrivateFile
	Authorize func(context.Context) error
}
type IdentityCutover struct{ controller *catalogtransfer.Cutover }

func NewIdentityCutover(cfg IdentityCutoverConfig) (*IdentityCutover, error) {
	if !cfg.Scope.Valid() || cfg.Isolated == nil || cfg.Isolated.cfg.Scope != cfg.Scope {
		return nil, ErrSource
	}
	config := catalogtransfer.CutoverConfig{
		Scope: cfg.Scope, Service: cfg.Service, Authorize: cfg.Authorize, Start: cfg.Isolated.Start,
		CheckTarget: func(ctx context.Context) error {
			return transferError(catalogtransfer.CheckDevice(ctx, cfg.Isolated.cfg.Crypto, cfg.Scope, storage.IsolatedIdentity))
		},
		OpenTarget: cfg.Isolated.Initialize, Receipt: cfg.Isolated.PrivateMigrationReceipt, Acknowledge: cfg.Isolated.AcknowledgeIdentityCutover,
	}
	if cfg.Legacy != nil {
		source := cfg.Legacy.repository
		if source == nil || source.cfg.Scope != cfg.Scope {
			return nil, ErrSource
		}
		frozen, ok := source.cfg.Crypto.(frozenRecordSource)
		if !ok {
			return nil, ErrSource
		}
		config.PrepareLegacy = func(ctx context.Context, state storage.IdentityCutoverState) error {
			if cfg.CopyFile == nil {
				return ErrSource
			}
			if err := catalogtransfer.CheckDevice(ctx, source.cfg.Crypto, cfg.Scope, storage.LegacyIdentity); err != nil {
				return transferError(err)
			}
			return cfg.Legacy.Prepare(ctx, state)
		}
		config.FrozenLegacy = frozen.FrozenStorageBindings
		config.CopyLegacy = func(ctx context.Context) error {
			return cfg.Isolated.MigratePrivateCatalog(ctx, cfg.Legacy, cfg.CopyFile)
		}
	}
	controller, err := catalogtransfer.NewCutover(config)
	if err != nil {
		return nil, transferError(err)
	}
	return &IdentityCutover{controller: controller}, nil
}

func (c *IdentityCutover) Prepare(ctx context.Context) error {
	return transferError(c.controller.Prepare(ctx))
}
