// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"fmt"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

type IdentityCutoverConfig struct {
	Scope           storage.Scope
	Service         transfer.CutoverService
	Legacy          *HistoricalSource
	CopyFile        CopyPrivateFile
	Authorize       func(context.Context) error
	AuthorizeLegacy storage.PublicationPolicy
}

// IdentityCutover supplies only Mail's domain ports to the suite controller.
// The controller owns freeze, copied-receipt confirmation and final routing ACK.
func (r *Repository) IdentityCutover(cfg IdentityCutoverConfig) (*transfer.Cutover, error) {
	if r == nil || cfg.Scope != r.cfg.Scope {
		return nil, ErrRecord
	}
	configuration := transfer.CutoverConfig{
		Scope: cfg.Scope, Service: cfg.Service, Authorize: cfg.Authorize, Start: r.start,
		CheckTarget: func(ctx context.Context) error {
			return transfer.CheckDevice(ctx, r.cfg.Crypto, cfg.Scope, storage.IsolatedIdentity)
		},
		OpenTarget: r.Initialize, Receipt: r.PrivateMigrationReceipt, Acknowledge: r.AcknowledgeIdentityCutover,
	}
	if cfg.Legacy != nil {
		previous := cfg.Legacy.repository
		if previous == nil || previous.cfg.Scope != cfg.Scope || cfg.CopyFile == nil || cfg.AuthorizeLegacy == nil {
			return nil, ErrRecord
		}
		frozen, ok := previous.cfg.Crypto.(frozenMailSource)
		if !ok {
			return nil, ErrRecord
		}
		configuration.PrepareLegacy = func(ctx context.Context, state storage.IdentityCutoverState) error {
			if err := transfer.CheckDevice(ctx, previous.cfg.Crypto, cfg.Scope, storage.LegacyIdentity); err != nil {
				return err
			}
			if err := cfg.Legacy.PrepareAuthorized(ctx, state, cfg.AuthorizeLegacy); err != nil {
				return fmt.Errorf("prepare retained Mail source: %w", err)
			}
			return nil
		}
		configuration.FrozenLegacy = func(ctx context.Context) (storage.FrozenBinding, error) {
			binding, err := frozen.FrozenStorageBindings(ctx)
			if err != nil {
				return binding, fmt.Errorf("verify frozen Mail source: %w", err)
			}
			return binding, nil
		}
		configuration.CopyLegacy = func(ctx context.Context) error {
			if err := r.MigratePrivateCatalog(ctx, cfg.Legacy, cfg.CopyFile); err != nil {
				return fmt.Errorf("transfer retained Mail catalog: %w", err)
			}
			return nil
		}
	}
	return transfer.NewCutover(configuration)
}
