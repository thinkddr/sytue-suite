// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	historicalInitializationKey struct{}
	historicalInitialization    struct {
		repository *Repository
		state      storage.IdentityCutoverState
		service    IdentityCutoverService
		authorize  storage.PublicationPolicy
	}
)

type historicalPublicationBinder interface {
	RequireAuthorizedStoragePublication(context.Context, storage.Scope, storage.PublicationTarget, storage.PublicationPolicy) (context.Context, error)
}

// Only the original identity cutover creates this capability. Historical reads
// can resume after freezing; creating an epoch or publishing a retained source
// requires the original legacy phase, original SQL epoch and live SDK1 policy.
func (r *Repository) initializeHistoricalSource(ctx context.Context, state storage.IdentityCutoverState, service IdentityCutoverService, authorize storage.PublicationPolicy) error {
	if state.Validate(r.cfg.Scope) != nil || state.Phase == "isolated" || service == nil || authorize == nil {
		return ErrSource
	}
	if err := cutoverDevice(ctx, r, storage.LegacyIdentity); err != nil {
		return err
	}
	state.SourceRooms = slices.Clone(state.SourceRooms)
	slices.Sort(state.SourceRooms)
	operation := &historicalInitialization{repository: r, state: state, service: service, authorize: authorize}
	return r.Initialize(context.WithValue(ctx, historicalInitializationKey{}, operation))
}

func (r *Repository) historicalInitialization(ctx context.Context) (*historicalInitialization, error) {
	operation, ok := ctx.Value(historicalInitializationKey{}).(*historicalInitialization)
	if !ok || operation == nil || operation.repository != r || operation.service == nil || operation.authorize == nil || operation.state.Validate(r.cfg.Scope) != nil {
		return nil, ErrSource
	}
	return operation, nil
}

func (o *historicalInitialization) authorizeCurrent(ctx context.Context) error {
	if o.state.Phase != "legacy" {
		return ErrNotReady
	}
	if err := o.authorize(ctx); err != nil {
		return err
	}
	current, err := o.service.MatrixStorageMigration(ctx, o.repository.cfg.Scope)
	if err != nil {
		return err
	}
	if current.Validate(o.repository.cfg.Scope) != nil || current.Phase != "legacy" {
		return ErrNotReady
	}
	rooms := slices.Clone(current.SourceRooms)
	slices.Sort(rooms)
	if !slices.Equal(rooms, o.state.SourceRooms) {
		return ErrSource
	}
	return o.authorize(ctx)
}

func (o *historicalInitialization) publicationContext(ctx context.Context, binding storage.Binding, source storage.LegacyMigrationState) (context.Context, error) {
	r := o.repository
	if !binding.Valid() || binding.Scope != r.cfg.Scope || source.Validate(r.cfg.Scope) != nil || source.Mode != "frozen" {
		return nil, ErrSource
	}
	sourceStarted := *source.StartedAt
	source.StartedAt = &sourceStarted
	binder, ok := r.cfg.Crypto.(historicalPublicationBinder)
	if !ok {
		return nil, ErrSource
	}
	policy := func(current context.Context) error {
		if err := o.authorizeCurrent(current); err != nil {
			return err
		}
		if err := r.cfg.Crypto.CheckStoragePublicationTarget(binding.Target); err != nil {
			return err
		}
		latest, err := r.cfg.Source.PrivateStorageState(current, r.cfg.Scope)
		if err != nil {
			return err
		}
		if !source.SameRetainedSource(latest) {
			return ErrSource
		}
		return current.Err()
	}
	if err := policy(ctx); err != nil {
		return nil, err
	}
	bound, err := binder.RequireAuthorizedStoragePublication(ctx, r.cfg.Scope, binding.Target, policy)
	if err != nil {
		return nil, err
	}
	if bound == nil {
		return nil, ErrSource
	}
	if projector, ok := r.cfg.Crypto.(storage.HistoricalPublicationProjector); ok {
		bound, err = projector.HistoricalStoragePublicationContext(bound)
		if err != nil {
			return nil, err
		}
		if bound == nil {
			return nil, ErrSource
		}
	}
	if err := policy(bound); err != nil {
		return nil, err
	}
	return bound, nil
}
