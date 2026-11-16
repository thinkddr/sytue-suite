// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type sourcePublicationBinder interface {
	RequireAuthorizedStoragePublication(context.Context, storage.Scope, storage.PublicationTarget, storage.PublicationPolicy) (context.Context, error)
}

// PrepareAuthorized confines historical writes to their original source epoch,
// target and authority. Once Matrix leaves legacy, Prepare is read-only and
// cannot mint a writer capability or retry an unaccepted source import.
func (s *HistoricalSource) PrepareAuthorized(ctx context.Context, state storage.IdentityCutoverState, authorize storage.PublicationPolicy) error {
	if s == nil || s.repository == nil || authorize == nil {
		return ErrRecord
	}
	r := s.repository
	if state.Validate(r.cfg.Scope) != nil || state.Phase == "isolated" {
		return ErrRecord
	}
	if state.Phase != "legacy" {
		return s.Prepare(ctx, state)
	}
	binder, ok := r.cfg.Crypto.(sourcePublicationBinder)
	if !ok {
		return ErrRecord
	}
	binding, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
	if err != nil {
		return err
	}
	if !binding.Valid() || binding.Scope != r.cfg.Scope {
		return ErrRecord
	}
	policy := s.buildPublicationPolicy(r, binding.Target, authorize)
	if err := policy(ctx); err != nil {
		return err
	}
	bound, err := binder.RequireAuthorizedStoragePublication(ctx, r.cfg.Scope, binding.Target, policy)
	if err != nil {
		return err
	}
	if projector, ok := r.cfg.Crypto.(storage.HistoricalPublicationProjector); ok {
		bound, err = projector.HistoricalStoragePublicationContext(bound)
		if err != nil {
			return err
		}
		if bound == nil {
			return ErrRecord
		}
	}
	return s.Prepare(bound, state)
}

func (s *HistoricalSource) buildPublicationPolicy(r *Repository, target storage.PublicationTarget, authorize storage.PublicationPolicy) storage.PublicationPolicy {
	expected := r.cfg.MigrationState
	if expected.StartedAt != nil {
		started := *expected.StartedAt
		expected.StartedAt = &started
	}
	return func(current context.Context) error {
		if err := authorize(current); err != nil {
			return err
		}
		if err := r.cfg.Crypto.CheckStoragePublicationTarget(target); err != nil {
			return err
		}
		caps, err := s.source.PrivateStorageCapabilities(current, r.cfg.Scope)
		if err != nil {
			return err
		}
		if historicalCapabilities(r.cfg, caps) != nil || !sameSource(expected, caps.LegacyMigrationState) {
			return ErrRecord
		}
		return current.Err()
	}
}
