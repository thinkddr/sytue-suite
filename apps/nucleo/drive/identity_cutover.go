// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"errors"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	catalogtransfer "git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

type (
	IdentityCutoverService = catalogtransfer.CutoverService
	// IdentityCutoverConfig borrows repositories and verified SDK devices. Authorize
	// must retain the originating session witness and recheck device verification
	// and acknowledged recovery. It must never bind to a later account's session.
	// Legacy may be nil only after the durable routing already says isolated.
	IdentityCutoverConfig struct {
		Scope     storage.Scope
		Service   IdentityCutoverService
		Legacy    *Repository
		Isolated  *Repository
		CopyFile  CopyPrivateFile
		Authorize func(context.Context) error
	}
)

// IdentityCutover borrows Drive repositories; the ordering is shared with the suite.
type IdentityCutover struct{ controller *catalogtransfer.Cutover }

func NewIdentityCutover(cfg IdentityCutoverConfig) (*IdentityCutover, error) {
	if !cfg.Scope.Valid() || cfg.Scope.Product != "drive" || cfg.Isolated == nil || cfg.Isolated.cfg.Scope != cfg.Scope {
		return nil, ErrSource
	}
	if cfg.Legacy != nil && (cfg.Legacy == cfg.Isolated || cfg.Legacy.cfg.Scope != cfg.Scope) {
		return nil, ErrSource
	}
	config := catalogtransfer.CutoverConfig{
		Scope: cfg.Scope, Service: cfg.Service, Authorize: cfg.Authorize, Start: cfg.Isolated.start,
		CheckTarget: func(ctx context.Context) error { return cutoverDevice(ctx, cfg.Isolated, storage.IsolatedIdentity) },
		OpenTarget:  cfg.Isolated.Initialize, Receipt: cfg.Isolated.PrivateMigrationReceipt, Acknowledge: cfg.Isolated.AcknowledgeIdentityCutover,
	}
	if cfg.Legacy != nil {
		config.PrepareLegacy = func(ctx context.Context, state storage.IdentityCutoverState) error {
			if cfg.CopyFile == nil {
				return ErrSource
			}
			if err := cutoverDevice(ctx, cfg.Legacy, storage.LegacyIdentity); err != nil {
				return err
			}
			return cfg.Legacy.initializeHistoricalSource(ctx, state, cfg.Service, cfg.Authorize)
		}
		config.FrozenLegacy = func(ctx context.Context) (storage.FrozenBinding, error) {
			proof, ok := cfg.Legacy.cfg.Crypto.(frozenCrypto)
			if !ok {
				return storage.FrozenBinding{}, ErrSource
			}
			return proof.FrozenStorageBindings(ctx)
		}
		config.CopyLegacy = func(ctx context.Context) error {
			return cfg.Isolated.MigratePrivateCatalog(ctx, cfg.Legacy, cfg.CopyFile)
		}
	}
	controller, err := catalogtransfer.NewCutover(config)
	if errors.Is(err, catalogtransfer.ErrSource) {
		err = ErrSource
	}
	if err != nil {
		return nil, err
	}
	return &IdentityCutover{controller: controller}, nil
}

func (c *IdentityCutover) Prepare(ctx context.Context) error {
	err := c.controller.Prepare(ctx)
	if errors.Is(err, catalogtransfer.ErrSource) {
		return ErrSource
	}
	if errors.Is(err, catalogtransfer.ErrNotReady) {
		return ErrNotReady
	}
	return err
}

func cutoverDevice(ctx context.Context, repo *Repository, version storage.IdentityVersion) error {
	if repo == nil {
		return ErrSource
	}
	err := catalogtransfer.CheckDevice(ctx, repo.cfg.Crypto, repo.cfg.Scope, version)
	if errors.Is(err, catalogtransfer.ErrSource) {
		return ErrSource
	}
	return err
}
