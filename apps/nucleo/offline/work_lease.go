// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"sync"
)

type (
	workLeaseHolderKey struct{}
	workLeaseHolder    struct {
		life   context.Context
		mu     sync.Mutex
		tokens map[*workLeaseIdentity]*workLeaseToken
	}
)

type (
	workLeaseIdentity struct{ acquire WorkLease }
	workLeaseToken    struct {
		life      context.Context
		ready     chan struct{}
		mu        sync.Mutex
		release   func()
		err       error
		retired   bool
		borrowers int
		idle      chan struct{}
	}
)

func withWorkLeaseHolder(ctx context.Context) context.Context {
	if _, ok := ctx.Value(workLeaseHolderKey{}).(*workLeaseHolder); ok {
		return ctx
	}
	return context.WithValue(ctx, workLeaseHolderKey{}, &workLeaseHolder{life: ctx, tokens: make(map[*workLeaseIdentity]*workLeaseToken)})
}

// ReentrantWorkLease must wrap a source ONCE per immutable owner/session. The
// closure's private identity, not a function code pointer, identifies that exact
// source. Inside WorkGate operations only it can borrow its already held lease.
// A different wrapper acquires its own source; outside a gate acquisition remains
// ordinary. Retired operation tokens never fall back to a fresh acquisition.
func ReentrantWorkLease(acquire WorkLease) WorkLease {
	identity := &workLeaseIdentity{acquire: acquire}
	return func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if identity.acquire == nil {
			return nil, ErrWorkLease
		}
		holder, _ := ctx.Value(workLeaseHolderKey{}).(*workLeaseHolder)
		if holder == nil {
			return acquireWorkLease(ctx, identity.acquire)
		}
		if err := holder.life.Err(); err != nil {
			return nil, err
		}
		holder.mu.Lock()
		token, exists := holder.tokens[identity]
		if !exists {
			idle := make(chan struct{})
			close(idle)
			token = &workLeaseToken{life: ctx, ready: make(chan struct{}), idle: idle}
			holder.tokens[identity] = token
		}
		holder.mu.Unlock()
		if exists {
			select {
			case <-token.ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-holder.life.Done():
				return nil, holder.life.Err()
			}
			return token.borrow(ctx, holder)
		}
		release, err := acquireWorkLease(ctx, identity.acquire)
		if err == nil {
			if failure := holder.life.Err(); failure != nil {
				release()
				release = nil
				err = failure
			}
		}
		token.mu.Lock()
		token.release, token.err = release, err
		token.mu.Unlock()
		close(token.ready)
		if err != nil {
			return nil, err
		}
		var once sync.Once
		return func() { once.Do(token.retire) }, nil
	}
}

func acquireWorkLease(ctx context.Context, acquire WorkLease) (func(), error) {
	release, err := acquire(ctx)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return nil, ErrWorkLease
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (t *workLeaseToken) borrow(ctx context.Context, holder *workLeaseHolder) (func(), error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return nil, t.err
	}
	if t.retired || t.release == nil {
		return nil, ErrWorkLease
	}
	for _, life := range []context.Context{ctx, holder.life, t.life} {
		if err := life.Err(); err != nil {
			return nil, err
		}
	}
	if t.borrowers == 0 {
		t.idle = make(chan struct{})
	}
	t.borrowers++
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.borrowers--
			if t.borrowers == 0 {
				close(t.idle)
			}
		})
	}, nil
}

func (t *workLeaseToken) retire() {
	t.mu.Lock()
	t.retired = true
	idle := t.idle
	release := t.release
	t.mu.Unlock()
	<-idle
	release()
}
