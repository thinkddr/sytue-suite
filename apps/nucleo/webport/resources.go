// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Resources owns the original Go work registrations for one verified host
// capability. Its host witness is acquired for the complete work operation;
// neither an RPC caller's replacement context nor a naked nil lease is enough.
type Resources struct {
	owner    *Storage
	lease    offline.WorkLease
	life     context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	retiring bool
	work     map[offline.PrivateWork]struct{}
}

func OpenResources(ctx context.Context, owner *Storage) (*Resources, error) {
	return OpenResourcesForLifetime(ctx, ctx, owner)
}

func OpenResourcesForLifetime(ctx, lifetime context.Context, owner *Storage) (*Resources, error) {
	if owner == nil || ctx == nil || lifetime == nil {
		return nil, ErrRejected
	}
	life, cancel := context.WithCancel(lifetime)
	r := &Resources{owner: owner, life: life, cancel: cancel, work: make(map[offline.PrivateWork]struct{})}
	r.lease = offline.ReentrantWorkLease(r.hostWorkLease())
	if err := r.Revalidate(ctx); err != nil {
		cancel()
		return nil, err
	}
	return r, nil
}
func (r *Resources) Context() context.Context           { return r.life }
func (r *Resources) PrivateStorageScope() storage.Scope { return r.owner.identity.Scope }
func (r *Resources) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.life.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return offline.ErrWorkClosed
	}
	current, err := r.owner.port.Current(ctx)
	if err != nil {
		return err
	}
	if current != r.owner.identity {
		return ErrRejected
	}
	return ctx.Err()
}

func (r *Resources) Revalidate(ctx context.Context) error {
	if err := r.Check(ctx); err != nil {
		return err
	}
	if err := r.owner.call(ctx, Authorize, nil, nil); err != nil {
		return err
	}
	return r.Check(ctx)
}

// WorkLease returns one stable source per immutable owner lifetime. Nested Go
// operations borrow the original host witness through the same shared helper
// as native clients; another Resources instance cannot borrow this source.
func (r *Resources) WorkLease() offline.WorkLease { return r.lease }

func (r *Resources) hostWorkLease() offline.WorkLease {
	return func(ctx context.Context) (func(), error) {
		if err := r.Revalidate(ctx); err != nil {
			return nil, err
		}
		var value struct {
			Lease string `json:"lease"`
		}
		if err := r.owner.call(ctx, LeaseAcquire, nil, &value); err != nil {
			return nil, err
		}
		if !ValidID(value.Lease) {
			return nil, offline.ErrWorkLease
		}
		var once sync.Once
		release := func() {
			once.Do(func() {
				r.mu.Lock()
				retiring := r.retiring
				r.mu.Unlock()
				if !retiring {
					_ = r.owner.call(context.WithoutCancel(ctx), LeaseRelease, value, nil)
				}
			})
		}
		if err := r.Revalidate(ctx); err != nil {
			release()
			return nil, err
		}
		return release, nil
	}
}

func validPrivateWork(work offline.PrivateWork) bool {
	if work == nil || !reflect.TypeOf(work).Comparable() {
		return false
	}
	value := reflect.ValueOf(work)
	if value.Kind() == reflect.Pointer || value.Kind() == reflect.Chan {
		return !value.IsNil()
	}
	return true
}

func (r *Resources) Register(ctx context.Context, work offline.PrivateWork) (func(), error) {
	if !validPrivateWork(work) {
		return nil, ErrRejected
	}
	if work.PrivateStorageScope() != r.PrivateStorageScope() {
		return nil, ErrRejected
	}
	if err := r.Revalidate(ctx); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, offline.ErrWorkClosed
	}
	if _, found := r.work[work]; found {
		r.mu.Unlock()
		return nil, ErrRejected
	}
	r.work[work] = struct{}{}
	r.mu.Unlock()
	var once sync.Once
	unregister := func() { once.Do(func() { r.mu.Lock(); delete(r.work, work); r.mu.Unlock() }) }
	if err := r.Revalidate(ctx); err != nil {
		unregister()
		return nil, err
	}
	return unregister, nil
}

// CloseAndDrain ends admission before joining each actual registered worker.
// Unknown pending work stays retained in its encrypted journal; it is never
// interpreted as permission to discard it or erase that account's keys early.
func (r *Resources) CloseAndDrain(ctx context.Context) error {
	r.mu.Lock()
	r.closed = true
	r.cancel()
	work := make([]offline.PrivateWork, 0, len(r.work))
	for value := range r.work {
		work = append(work, value)
	}
	r.mu.Unlock()
	var joined error
	for _, value := range work {
		joined = errors.Join(joined, value.Drain(ctx))
	}
	return joined
}

var _ offline.WorkResourceGuard = (*Resources)(nil)

// RetirePort is called by the fixed port lifecycle hook, never a domain RPC.
// The host releases its retained witnesses only after Go drain acknowledgement.
func (r *Resources) RetirePort() { r.mu.Lock(); r.retiring = true; r.mu.Unlock() }
