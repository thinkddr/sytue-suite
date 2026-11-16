// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PrivateRepository is the application boundary after complete source import.
// Initialize must authenticate the entire receipt before exposing any content.
type PrivateRepository interface {
	offline.PrivateWork
	Initialize(context.Context) error
}

// ResourceGuard is implemented by Resources for every maintained SDK client.
type ResourceGuard interface {
	Context() context.Context
	PrivateStorageScope() storage.Scope
	Check(context.Context) error
	Revalidate(context.Context) error
	WorkLease() offline.WorkLease
	Register(context.Context, offline.PrivateWork) (func(), error)
}

// RepositoryConfig keeps domain migration in its owner while sharing lifecycle.
// Create receives the repository lifetime, not a short-lived request context.
// Prepare may migrate a retained source; it runs only with both devices checked.
// A failed Prepare retains the same repository and borrowed clients for retry.
type RepositoryConfig[R PrivateRepository] struct {
	Create  func(context.Context) (R, error)
	Prepare func(context.Context, R) error
}

// RepositoryCycle serializes preparation and guards callbacks from an old native
// presenter. It owns the repository, never its Matrix devices or recovery keys.
// Open/Run/Drain/Close run off the UI thread. Dispatch is the only UI handoff.
type RepositoryCycle[R PrivateRepository] struct {
	resources  ResourceGuard
	cfg        RepositoryConfig[R]
	gate       *offline.WorkGate
	openMu     sync.Mutex
	retireMu   sync.Mutex
	mu         sync.Mutex
	repo       R
	hasRepo    bool
	ready      bool
	paused     bool
	closed     bool
	unregister func()
}

func NewRepositoryCycle[R PrivateRepository](resources ResourceGuard, cfg RepositoryConfig[R]) (*RepositoryCycle[R], error) {
	if nilReference(resources) || cfg.Create == nil || !resources.PrivateStorageScope().Valid() || resources.WorkLease() == nil {
		return nil, ErrPrivateResources
	}
	ctx := resources.Context()
	if err := resources.Check(ctx); err != nil {
		return nil, err
	}
	return &RepositoryCycle[R]{resources: resources, cfg: cfg, gate: offline.NewWorkGateWithLease(ctx, resources.WorkLease())}, nil
}

// Open verifies resources, constructs once, prepares and initializes. Failed
// initialization cannot mount an empty private account or enable stale callbacks.
func (c *RepositoryCycle[R]) Open(parent context.Context) error {
	ctx, done, err := c.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	c.openMu.Lock()
	defer c.openMu.Unlock()
	c.mu.Lock()
	c.ready = false
	c.mu.Unlock()
	if err := c.resources.Revalidate(ctx); err != nil {
		return err
	}
	repo, err := c.repository(ctx)
	if err != nil {
		return err
	}
	if c.cfg.Prepare != nil {
		if err := c.cfg.Prepare(ctx, repo); err != nil {
			return err
		}
	}
	if err := c.resources.Revalidate(ctx); err != nil {
		return err
	}
	if err := repo.Initialize(ctx); err != nil {
		return err
	}
	if err := c.resources.Revalidate(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.paused {
		return offline.ErrWorkPaused
	}
	c.ready = true
	return nil
}

func (c *RepositoryCycle[R]) repository(ctx context.Context) (R, error) {
	c.mu.Lock()
	repo, exists := c.repo, c.hasRepo
	c.mu.Unlock()
	if !exists {
		var err error
		repo, err = c.cfg.Create(c.resources.Context()) //nolint:contextcheck // The repository retains presenter lifetime; its registration/preparation use this cancellable operation context.
		if err != nil {
			return repo, err
		}
		if nilReference(repo) || repo.PrivateStorageScope() != c.resources.PrivateStorageScope() {
			return repo, ErrPrivateResources
		}
		c.mu.Lock()
		c.repo, c.hasRepo = repo, true
		c.mu.Unlock()
	}
	c.mu.Lock()
	registered := c.unregister != nil
	c.mu.Unlock()
	if !registered {
		unregister, err := c.resources.Register(ctx, repo)
		if err != nil {
			return repo, err
		}
		c.mu.Lock()
		c.unregister = unregister
		c.mu.Unlock()
	}
	return repo, nil
}

// Run admits work only for a completely initialized, still-current presenter.
// The repository supplies its domain serialization and durable operation journal.
func (c *RepositoryCycle[R]) Run(parent context.Context, work func(context.Context, R) error) error {
	if work == nil {
		return ErrPrivateResources
	}
	ctx, done, err := c.gate.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	repo, err := c.current(ctx)
	if err != nil {
		return err
	}
	return work(ctx, repo)
}

func (c *RepositoryCycle[R]) current(ctx context.Context) (R, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.resources.Check(ctx); err != nil {
		return c.repo, err
	}
	if c.closed || c.paused || !c.ready || !c.hasRepo {
		return c.repo, offline.ErrWorkPaused
	}
	return c.repo, nil
}

// Dispatch retains this exact repository in a UI callback. A later account,
// failed refresh or pending shutdown suppresses it instead of acquiring a new
// account's presenter. Dispatch and the host's view detach run on the UI thread.
func (c *RepositoryCycle[R]) Dispatch(dispatch func(func()), apply func(R)) {
	if dispatch == nil || apply == nil {
		return
	}
	dispatch(func() {
		repo, err := c.current(c.resources.Context())
		if err == nil {
			apply(repo)
		}
	})
}

// Drain pauses admission, cancels and waits for operations, then asks the
// repository for retained durable work. An error or an unexpected scope NEVER
// becomes a zero count or permission to erase a journal or revoke a device.
func (c *RepositoryCycle[R]) Drain(ctx context.Context) (offline.PendingWork, error) {
	c.retireMu.Lock()
	defer c.retireMu.Unlock()
	return c.drain(ctx)
}

func (c *RepositoryCycle[R]) drain(ctx context.Context) (offline.PendingWork, error) {
	pending := offline.PendingWork{Scope: c.resources.PrivateStorageScope()}
	c.mu.Lock()
	c.paused = true
	c.mu.Unlock()
	if err := c.gate.Drain(ctx); err != nil {
		return pending, err
	}
	c.mu.Lock()
	repo, exists := c.repo, c.hasRepo
	c.mu.Unlock()
	if !exists {
		return pending, nil
	}
	if err := repo.Drain(ctx); err != nil {
		return pending, err
	}
	result, err := repo.Pending(ctx)
	if err != nil {
		return pending, err
	}
	if result.Scope != pending.Scope || result.Count < 0 {
		return pending, ErrPrivateResources
	}
	return result, nil
}

// Resume follows an aborted shutdown of the same identity. It never revives a
// cancelled presenter or treats retained work as permission to erase credentials.
func (c *RepositoryCycle[R]) Resume(ctx context.Context) error {
	c.retireMu.Lock()
	defer c.retireMu.Unlock()
	if err := c.resources.Check(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return offline.ErrWorkClosed
	}
	repo, exists := c.repo, c.hasRepo
	c.mu.Unlock()
	if err := c.gate.Resume(); err != nil {
		return err
	}
	if exists {
		if err := repo.Resume(); err != nil {
			_ = c.gate.Drain(ctx)
			return err
		}
	}
	c.mu.Lock()
	c.paused = false
	c.mu.Unlock()
	return nil
}

// Close requires a clean, drained repository. Retained work and failures leave
// the repository and both borrowed clients intact for recovery or explicit host
// policy. This function never logs out, erases credentials or discards work.
func (c *RepositoryCycle[R]) Close(ctx context.Context) error {
	c.retireMu.Lock()
	defer c.retireMu.Unlock()
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil
	}
	pending, err := c.drain(ctx)
	if err != nil {
		return err
	}
	if pending.Count != 0 {
		return ErrPrivatePending
	}
	c.closeRepository() //nolint:contextcheck // Drain joined all operations before closing the retained repository lifetime.
	return nil
}

// RetirePreservingWork closes a drained view/repository while retaining its WAL
// and native credentials. It is suitable for account switches or app shutdown,
// and grants no permission to logout, discard work or erase any device key.
func (c *RepositoryCycle[R]) RetirePreservingWork(ctx context.Context) (offline.PendingWork, error) {
	c.retireMu.Lock()
	defer c.retireMu.Unlock()
	pending, err := c.drain(ctx)
	if err != nil {
		return pending, err
	}
	c.closeRepository() //nolint:contextcheck // Drain joined all operations before closing the retained repository lifetime.
	return pending, nil
}

func (c *RepositoryCycle[R]) closeRepository() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed, c.ready = true, false
	repo, exists, unregister := c.repo, c.hasRepo, c.unregister
	c.unregister = nil
	c.mu.Unlock()
	c.gate.Close()
	if exists {
		repo.Close()
	}
	if unregister != nil {
		unregister()
	}
}
