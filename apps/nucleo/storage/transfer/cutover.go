// SPDX-License-Identifier: Apache-2.0
package transfer

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

var ErrNotReady = errors.New("transfer: identity source has not reached its frozen boundary")

type CutoverService interface {
	MatrixStorageMigration(context.Context, storage.Scope) (storage.IdentityCutoverState, error)
	BeginMatrixStorageMigration(context.Context, storage.Scope, string) (storage.IdentityCutoverState, error)
	CompleteMatrixStorageMigration(context.Context, storage.Scope, string, storage.Reference) (storage.IdentityCutoverState, error)
}

// CutoverConfig borrows verified repositories and their original work witness.
// Legacy callbacks may be nil only when routing has already become isolated.
type CutoverConfig struct {
	Scope         storage.Scope
	Service       CutoverService
	Authorize     func(context.Context) error
	Start         func(context.Context) (context.Context, func(), error)
	CheckTarget   func(context.Context) error
	OpenTarget    func(context.Context) error
	PrepareLegacy func(context.Context, storage.IdentityCutoverState) error
	FrozenLegacy  func(context.Context) (storage.FrozenBinding, error)
	CopyLegacy    func(context.Context) error
	Receipt       func(context.Context) (storage.Reference, error)
	Acknowledge   func(context.Context, storage.IdentityCutoverState) error
}
type Cutover struct {
	cfg    CutoverConfig
	mu     sync.Mutex
	copied bool
	epoch  string
}

func NewCutover(cfg CutoverConfig) (*Cutover, error) {
	if !cfg.Scope.Valid() || cfg.Service == nil || cfg.Authorize == nil || cfg.Start == nil || cfg.CheckTarget == nil || cfg.OpenTarget == nil || cfg.Receipt == nil || cfg.Acknowledge == nil {
		return nil, ErrSource
	}
	return &Cutover{cfg: cfg}, nil
}

func (c *Cutover) Prepare(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, release, err := c.cfg.Start(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := c.cfg.Authorize(ctx); err != nil {
		return err
	}
	if err := c.cfg.CheckTarget(ctx); err != nil {
		return err
	}
	state, err := c.cfg.Service.MatrixStorageMigration(ctx, c.cfg.Scope)
	if err != nil {
		return err
	}
	if state.Validate(c.cfg.Scope) != nil {
		return ErrSource
	}
	if c.copied && c.epoch != state.Epoch {
		return ErrSource
	}
	if state.Phase == "isolated" {
		return c.openIsolated(ctx, state)
	}
	return c.copyAndComplete(ctx, state)
}

func (c *Cutover) freeze(ctx context.Context, state storage.IdentityCutoverState) (storage.IdentityCutoverState, error) {
	if c.cfg.PrepareLegacy == nil || c.cfg.FrozenLegacy == nil || c.cfg.CopyLegacy == nil {
		return storage.IdentityCutoverState{}, ErrSource
	}
	if err := c.cfg.PrepareLegacy(ctx, state); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	if state.Phase == "legacy" {
		state.Epoch = uuid.NewString()
	}
	if err := c.cfg.Authorize(ctx); err != nil {
		return storage.IdentityCutoverState{}, err
	}
	epoch := state.Epoch
	state, err := c.cfg.Service.BeginMatrixStorageMigration(ctx, c.cfg.Scope, epoch)
	if err != nil {
		return storage.IdentityCutoverState{}, err
	}
	if state.Validate(c.cfg.Scope) != nil || state.Epoch != epoch {
		return storage.IdentityCutoverState{}, ErrSource
	}
	if state.Phase != "frozen" && state.Phase != "isolated" {
		return storage.IdentityCutoverState{}, ErrNotReady
	}
	return state, nil
}

func (c *Cutover) copyAndComplete(ctx context.Context, state storage.IdentityCutoverState) error {
	state, err := c.freeze(ctx, state)
	if err != nil {
		return err
	}
	if state.Phase == "isolated" {
		return c.openIsolated(ctx, state)
	}
	frozen, err := c.cfg.FrozenLegacy(ctx)
	if err != nil {
		return err
	}
	if !frozen.Valid() || frozen.Binding.Scope != c.cfg.Scope {
		return ErrSource
	}
	rooms := slices.Sorted(slices.Values(frozen.Binding.Rooms))
	if !slices.Equal(rooms, slices.Sorted(slices.Values(state.SourceRooms))) {
		return ErrSource
	}
	if c.copied && c.epoch != state.Epoch {
		return ErrSource
	}
	if !c.copied {
		if err := c.cfg.CopyLegacy(ctx); err != nil {
			return err
		}
		c.copied, c.epoch = true, state.Epoch
	}
	receipt, err := c.cfg.Receipt(ctx)
	if err != nil {
		return err
	}
	return c.complete(ctx, state, receipt)
}

func (c *Cutover) complete(ctx context.Context, state storage.IdentityCutoverState, receipt storage.Reference) error {
	if err := c.cfg.Authorize(ctx); err != nil {
		return err
	}
	complete, err := c.cfg.Service.CompleteMatrixStorageMigration(ctx, c.cfg.Scope, state.Epoch, receipt)
	if err != nil {
		return err
	}
	if complete.Validate(c.cfg.Scope) != nil || complete.Phase != "isolated" || complete.Epoch != state.Epoch || complete.Receipt == nil || *complete.Receipt != receipt {
		return ErrSource
	}
	if err := c.cfg.Authorize(ctx); err != nil {
		return err
	}
	return c.cfg.Acknowledge(ctx, complete)
}

func (c *Cutover) openIsolated(ctx context.Context, state storage.IdentityCutoverState) error {
	if err := c.cfg.OpenTarget(ctx); err != nil {
		return err
	}
	receipt, err := c.cfg.Receipt(ctx)
	if err != nil {
		return err
	}
	if state.Receipt == nil || *state.Receipt != receipt {
		return ErrSource
	}
	if err := c.cfg.Authorize(ctx); err != nil {
		return err
	}
	return c.cfg.Acknowledge(ctx, state)
}

// CheckDevice checks the exact SDK identity before any source-side effect.
func CheckDevice(ctx context.Context, port storage.RecordStore, scope storage.Scope, expected storage.IdentityVersion) error {
	if port == nil || !scope.Valid() {
		return ErrSource
	}
	identity, ok := port.(interface {
		StorageIdentityVersion() storage.IdentityVersion
	})
	if !ok || identity.StorageIdentityVersion() != expected {
		return ErrSource
	}
	binding, err := port.PrepareStorage(ctx, scope)
	if err != nil {
		return err
	}
	if !binding.Valid() || binding.Scope != scope {
		return ErrSource
	}
	return port.CheckStoragePublicationTarget(binding.Target)
}
