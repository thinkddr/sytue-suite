// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RefreshProjection prepares a small domain index against the fully validated
// candidate. Its returned function publishes only in-memory state, performs no
// IO and must not call back into the repository. It runs with the catalog swap.
type RefreshProjection func(context.Context, Catalog) (func(), error)

// RefreshStateSource supplies read-only snapshot metadata when ordinary State
// also resumes a retained source journal. Refresh never acknowledges that work.
type RefreshStateSource interface {
	RefreshState(context.Context, storage.Scope) (SnapshotState, error)
}

func (r *Repository) RefreshPrivateData(ctx context.Context) error {
	return r.RefreshWithProjection(ctx, nil)
}

// RefreshWithProjection explicitly refreshes an already open private account.
// Mutations share the journal exclusion. Readers keep the preceding coherent
// catalog until every receipt, graph and commitment has been checked; failures
// neither change readiness nor reconcile/delete any pending journal operation.
func (r *Repository) RefreshWithProjection(parent context.Context, prepare RefreshProjection) error {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		candidate, publish, err := r.refreshedProjection(ctx, prepare)
		if err != nil {
			return err
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed {
			return ErrClosed
		}
		if !r.ready {
			return ErrNotReady
		}
		if !r.binding.Same(candidate.binding) {
			return ErrSource
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		loaded := candidate.loadedNow()
		r.noteHeadsLocked(candidate.heads, loaded)
		r.loaded = loaded
		r.binding, r.catalog, r.entries, r.byObject, r.heads = candidate.binding, candidate.catalog, candidate.entries, candidate.byObject, candidate.heads
		if publish != nil {
			publish()
		}
		return nil
	})
}

func (r *Repository) refreshedProjection(ctx context.Context, prepare RefreshProjection) (*Repository, func(), error) {
	if err := r.requireReady(); err != nil {
		return nil, nil, err
	}
	if _, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope); err != nil {
		return nil, nil, err
	}
	stateFn := r.cfg.Source.State
	if source, ok := r.cfg.Source.(RefreshStateSource); ok {
		stateFn = source.RefreshState
	}
	state, err := stateFn(ctx, r.cfg.Scope)
	if err != nil {
		return nil, nil, err
	}
	if err := r.validateState(state); err != nil {
		return nil, nil, err
	}
	if state.Mode == "legacy" {
		return nil, nil, ErrNotReady
	}
	// Lo ya comprobado vale igual para el candidato: va por referencia (verified_cache.go).
	candidate := &Repository{cfg: r.cfg, gate: r.gate, verified: r.verified}
	// openCompleted validates into a detached projection, without WAL retirement.
	if err := candidate.openCompleted(ctx, state); err != nil {
		return nil, nil, err
	}
	var publish func()
	if prepare != nil {
		publish, err = prepare(ctx, catalogView{r: candidate, requireReady: true})
		if err != nil {
			return nil, nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(candidate.binding.Target); err != nil {
		return nil, nil, err
	}
	return candidate, publish, nil
}
