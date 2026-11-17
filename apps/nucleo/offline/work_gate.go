// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"errors"
	"sync"
)

var (
	ErrWorkPaused = errors.New("offline: private work is paused for session shutdown")
	ErrWorkClosed = errors.New("offline: private work is closed")
	ErrWorkLease  = errors.New("offline: private work has no valid session lease")
)

// WorkGate tracks device work without holding a session or domain mutex during
// network IO. Drain pauses admission, cancels callers and waits for their cleanup.
// It never removes a journal, a credential or an accepted publication.
type WorkGate struct {
	mu             sync.Mutex
	life           context.Context
	cancel         context.CancelFunc
	next           uint64
	active         map[uint64]context.CancelFunc
	idle           chan struct{}
	acquire        WorkLease
	paused, closed bool
}

func NewWorkGate(parent context.Context) *WorkGate {
	life, cancel := context.WithCancel(parent)
	idle := make(chan struct{})
	close(idle)
	return &WorkGate{life: life, cancel: cancel, active: make(map[uint64]context.CancelFunc), idle: idle}
}

// WorkLease binds a job to its persistent device-session witness. Implementations
// acquire a shared scope lock, recheck the witness and return its release.
type WorkLease func(context.Context) (func(), error)

func NewWorkGateWithLease(parent context.Context, acquire WorkLease) *WorkGate {
	gate := NewWorkGate(parent)
	gate.acquire = acquire
	return gate
}

func (g *WorkGate) Start(parent context.Context) (context.Context, func(), error) {
	ctx, cancel, done, err := g.start(parent)
	if err != nil {
		return nil, nil, err
	}
	ctx = withWorkLeaseHolder(ctx)
	if g.acquire == nil {
		return ctx, done, nil
	}
	release, err := g.acquire(ctx)
	if err != nil {
		done()
		return nil, nil, err
	}
	if release == nil {
		done()
		return nil, nil, ErrWorkLease
	}
	if err := ctx.Err(); err != nil {
		release()
		done()
		return nil, nil, err
	}
	var once sync.Once
	return ctx, func() { once.Do(func() { cancel(); release(); done() }) }, nil
}

// WorkContextBinder attaches operation-local adapters to the actual admitted
// caller after acquiring its lease. It grants no authentication or publication
// authority itself. Implementations must derive their result from that caller.
type WorkContextBinder func(context.Context) (context.Context, error)

type (
	boundWorkContextKey struct{}
	boundWorkWitness    struct{ gate *WorkGate }
)

// StartBound extends Start without replacing its caller, lease or cancellation.
// A discarded caller, nil result, failure or panic releases the actual lease.
func (g *WorkGate) StartBound(parent context.Context, bind WorkContextBinder) (context.Context, func(), error) {
	ctx, done, err := g.Start(parent)
	if err != nil || bind == nil {
		return ctx, done, err
	}
	accepted := false
	defer func() {
		if !accepted {
			done()
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	witness := &boundWorkWitness{gate: g}
	ctx = context.WithValue(ctx, boundWorkContextKey{}, witness)
	bound, err := bind(ctx)
	if err != nil {
		return nil, nil, err
	}
	if bound == nil || bound.Value(boundWorkContextKey{}) != witness {
		return nil, nil, ErrWorkLease
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if err := bound.Err(); err != nil {
		return nil, nil, err
	}
	accepted = true
	return bound, done, nil
}

func (g *WorkGate) start(parent context.Context) (context.Context, context.CancelFunc, func(), error) {
	if err := parent.Err(); err != nil {
		return nil, nil, nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil, nil, nil, ErrWorkClosed
	}
	if g.paused {
		return nil, nil, nil, ErrWorkPaused
	}
	if err := g.life.Err(); err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(g.life, cancel)
	if len(g.active) == 0 {
		g.idle = make(chan struct{})
	}
	g.next++
	id := g.next
	g.active[id] = cancel
	var once sync.Once
	done := func() {
		once.Do(func() {
			stop()
			cancel()
			g.mu.Lock()
			defer g.mu.Unlock()
			delete(g.active, id)
			if len(g.active) == 0 {
				close(g.idle)
			}
		})
	}
	return ctx, cancel, done, nil
}

func (g *WorkGate) Drain(ctx context.Context) error {
	g.mu.Lock()
	g.paused = true
	idle := g.idle
	cancellations := make([]context.CancelFunc, 0, len(g.active))
	for _, cancel := range g.active {
		cancellations = append(cancellations, cancel)
	}
	g.mu.Unlock()
	for _, cancel := range cancellations {
		cancel()
	}
	select {
	case <-idle:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Resume only follows an aborted shutdown. The owner must still hold the same
// persistent session witness; a closed or still-draining gate cannot be reopened.
func (g *WorkGate) Resume() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return ErrWorkClosed
	}
	if err := g.life.Err(); err != nil {
		return err
	}
	if len(g.active) != 0 {
		return ErrWorkPaused
	}
	g.paused = false
	return nil
}

func (g *WorkGate) Close() {
	g.mu.Lock()
	g.closed = true
	g.cancel()
	g.mu.Unlock()
	_ = g.Drain(context.Background())
}
