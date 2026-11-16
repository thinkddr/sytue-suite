// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// RetainedSources owns auxiliary migration readers, never their borrowed SDK.
// Unknown pending work preserves every registration and device. Closing a reader
// after a valid count retains its encrypted journal, including nonzero work.
type RetainedSources struct {
	resources      WorkResourceGuard
	invalid        error
	sources        []PrivateWork
	unregister     []func()
	mu             sync.Mutex
	closed, paused bool
	retained       bool
	owner          PrivateWork
	inventory      func(context.Context) (PendingWork, error)
}

func NewRetainedSources(resources WorkResourceGuard, invalid error, sources ...PrivateWork) (*RetainedSources, error) {
	return newRetainedSources(resources, nil, nil, invalid, sources...)
}

// NewComposedSources belongs to one already-registered owner. Children share
// its scope and unique journal inventory; they are never registered or counted
// again as independent broker jobs. Every child count is still authenticated.
func NewComposedSources(resources WorkResourceGuard, owner PrivateWork, inventory func(context.Context) (PendingWork, error), invalid error, sources ...PrivateWork) (*RetainedSources, error) {
	if invalid == nil {
		invalid = ErrWorkLease
	}
	if nilWorkReference(owner) || inventory == nil {
		return nil, invalid
	}
	return newRetainedSources(resources, owner, inventory, invalid, sources...)
}

func newRetainedSources(resources WorkResourceGuard, owner PrivateWork, inventory func(context.Context) (PendingWork, error), invalid error, sources ...PrivateWork) (*RetainedSources, error) {
	if invalid == nil {
		invalid = ErrWorkLease
	}
	if nilWorkReference(resources) || !resources.PrivateStorageScope().Valid() {
		return nil, invalid
	}
	if owner != nil && (!reflect.TypeOf(owner).Comparable() || owner.PrivateStorageScope() != resources.PrivateStorageScope()) {
		return nil, invalid
	}
	result := &RetainedSources{resources: resources, owner: owner, inventory: inventory, invalid: invalid}
	for _, source := range sources {
		if err := result.appendSource(source); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *RetainedSources) appendSource(source PrivateWork) error {
	if nilWorkReference(source) || !reflect.TypeOf(source).Comparable() || source.PrivateStorageScope() != s.resources.PrivateStorageScope() || source == s.owner {
		return s.invalid
	}
	for _, existing := range s.sources {
		if existing == source {
			return nil
		}
	}
	s.sources = append(s.sources, source)
	s.unregister = append(s.unregister, nil)
	return nil
}

// Add retains membership even if broker registration fails. Callers perform no
// child IO until it succeeds; retry keeps every prior registration and journal.
func (s *RetainedSources) Add(ctx context.Context, source PrivateWork) error {
	s.mu.Lock()
	if s.closed || s.retained {
		s.mu.Unlock()
		return ErrWorkClosed
	}
	if s.paused {
		s.mu.Unlock()
		return ErrWorkPaused
	}
	err := s.appendSource(source)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.Register(ctx)
}

// Register is idempotent and retains partial successful registration for retry.
// It runs before the cutover imports or freezes any source.
func (s *RetainedSources) Register(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.retained {
		return ErrWorkClosed
	}
	if s.paused {
		return ErrWorkPaused
	}
	if err := s.resources.Revalidate(ctx); err != nil {
		return err
	}
	if s.owner != nil {
		return s.resources.Revalidate(ctx)
	}
	for i, source := range s.sources {
		if s.unregister[i] != nil {
			continue
		}
		unregister, err := s.resources.Register(ctx, source)
		if err != nil {
			return err
		}
		s.unregister[i] = unregister
	}
	return s.resources.Revalidate(ctx)
}

// Retire cancels and joins all readers before closing any of them. A failed or
// foreign pending count cannot become permission to release a device or WAL.
func (s *RetainedSources) Retire(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if !s.retained {
		if err := s.drainLocked(ctx); err != nil {
			return err
		}
	}
	if _, err := s.pendingLocked(ctx); err != nil {
		return err
	}
	s.closeLocked(true)
	return nil
}

// CloseRetaining joins and erases live readers without discarding their sealed
// journal or releasing broker registrations. A later destructive Retire still
// requires an authenticated pending inventory, even after these readers close.
func (s *RetainedSources) CloseRetaining(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.retained {
		return nil
	}
	if err := s.drainLocked(ctx); err != nil {
		return err
	}
	s.closeLocked(false)
	return nil
}

func (s *RetainedSources) closeLocked(unregister bool) {
	for i, source := range s.sources {
		source.Close()
		if unregister && s.unregister[i] != nil {
			s.unregister[i]()
			s.unregister[i] = nil
		}
	}
	if unregister {
		s.closed = true
	} else {
		s.retained = true
	}
}

// Drain pauses admission and joins every child before a reversible shutdown.
func (s *RetainedSources) Drain(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drainLocked(ctx)
}

func (s *RetainedSources) drainLocked(ctx context.Context) error {
	s.paused = true
	for _, source := range s.sources {
		if err := source.Drain(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *RetainedSources) Resume(ctx context.Context) error {
	if err := s.resources.Check(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.retained {
		return ErrWorkClosed
	}
	for _, source := range s.sources {
		if err := source.Resume(); err != nil {
			return errors.Join(err, s.drainLocked(ctx))
		}
	}
	s.paused = false
	return nil
}

func (s *RetainedSources) Pending(ctx context.Context) (PendingWork, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingLocked(ctx)
}

func (s *RetainedSources) pendingLocked(ctx context.Context) (PendingWork, error) {
	out := PendingWork{Scope: s.resources.PrivateStorageScope()}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	largest := 0
	for _, source := range s.sources {
		pending, err := source.Pending(ctx)
		if err != nil {
			return out, err
		}
		if pending.Scope != out.Scope || pending.Count < 0 || pending.Count > math.MaxInt-out.Count {
			return out, s.invalid
		}
		out.Count += pending.Count
		if pending.Count > largest {
			largest = pending.Count
		}
	}
	if s.inventory != nil {
		actual, err := s.inventory(ctx)
		if err != nil {
			return out, err
		}
		if actual.Scope != out.Scope || actual.Count < largest {
			return out, s.invalid
		}
		out = actual
	}
	return out, nil
}

// WorkResourceGuard supplies the same authenticated registration used by every
// platform. It contains no SDK keys, tokens, UI or transport implementation.
type WorkResourceGuard interface {
	PrivateStorageScope() storage.Scope
	Check(context.Context) error
	Revalidate(context.Context) error
	Register(context.Context, PrivateWork) (func(), error)
}

func nilWorkReference(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// MemberCount reports retained children without exposing or changing ownership.
func (s *RetainedSources) MemberCount() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.sources) }
