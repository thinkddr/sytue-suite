// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type retainCloseGuard struct {
	scope                    storage.Scope
	registered, unregistered int
}

func (g *retainCloseGuard) PrivateStorageScope() storage.Scope   { return g.scope }
func (g *retainCloseGuard) Check(ctx context.Context) error      { return ctx.Err() }
func (g *retainCloseGuard) Revalidate(ctx context.Context) error { return ctx.Err() }
func (g *retainCloseGuard) Register(_ context.Context, _ PrivateWork) (func(), error) {
	g.registered++
	return func() { g.unregistered++ }, nil
}

type retainCloseWork struct {
	scope                                  storage.Scope
	pendingErr                             error
	entered, finish                        chan struct{}
	drained, closed, resumed, pendingCalls int
}

func (w *retainCloseWork) PrivateStorageScope() storage.Scope { return w.scope }
func (w *retainCloseWork) Pending(context.Context) (PendingWork, error) {
	w.pendingCalls++
	return PendingWork{Scope: w.scope, Count: 1}, w.pendingErr
}

func (w *retainCloseWork) Drain(ctx context.Context) error {
	if w.entered != nil {
		close(w.entered)
		w.entered = nil
	}
	if w.finish != nil {
		select {
		case <-w.finish:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	w.drained++
	return nil
}
func (w *retainCloseWork) Resume() error { w.resumed++; return nil }
func (w *retainCloseWork) Close()        { w.closed++ }
func TestRetainedSourcesCloseRetainingJoinsReadersWithoutDiscardingUnknownWorkOrRegistrations(t *testing.T) {
	scope := storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}
	guard := &retainCloseGuard{scope: scope}
	unknown := errors.New("original source unavailable")
	entered, finish := make(chan struct{}), make(chan struct{})
	first := &retainCloseWork{scope: scope, pendingErr: unknown, entered: entered, finish: finish}
	second := &retainCloseWork{scope: scope, pendingErr: unknown}
	sources, err := NewRetainedSources(guard, ErrWorkLease, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if err = sources.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- sources.CloseRetaining(t.Context()) }()
	<-entered
	// Drain must still be waiting; these fields are touched only after it returns.
	select {
	case err = <-closed:
		t.Fatal("closed before actual reader joined", err)
	default:
	}
	close(finish)
	if err = <-closed; err != nil {
		t.Fatal(err)
	}
	if first.closed != 1 || second.closed != 1 || guard.registered != 2 || guard.unregistered != 0 || first.pendingCalls != 0 || second.pendingCalls != 0 {
		t.Fatal("retained close erased evidence or released original registrations", guard, first, second)
	}
	if err = sources.CloseRetaining(t.Context()); err != nil || first.closed != 1 {
		t.Fatal("retained close not idempotent", err)
	}
	if err = sources.Resume(t.Context()); !errors.Is(err, ErrWorkClosed) || first.resumed != 0 {
		t.Fatal("closed source silently resumed", err)
	}
	if err = sources.Retire(t.Context()); !errors.Is(err, unknown) || guard.unregistered != 0 {
		t.Fatal("unknown pending work authorized destructive retire", err)
	}
	first.pendingErr = nil
	second.pendingErr = nil
	if err = sources.Retire(t.Context()); err != nil || guard.unregistered != 2 {
		t.Fatal("later authenticated retire could not release registration", err, guard.unregistered)
	}
}

func TestRetainedSourcesFailedDrainPreservesEveryOriginalReader(t *testing.T) {
	scope := storage.Scope{Tenant: "tenant", Subject: "owner", Product: "mail"}
	guard := &retainCloseGuard{scope: scope}
	blocked := &retainCloseWork{scope: scope, finish: make(chan struct{})}
	other := &retainCloseWork{scope: scope}
	sources, _ := NewRetainedSources(guard, ErrWorkLease, blocked, other)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sources.CloseRetaining(ctx); !errors.Is(err, context.Canceled) || blocked.closed != 0 || other.closed != 0 || guard.unregistered != 0 {
		t.Fatal("failed drain discarded source", err)
	}
	close(blocked.finish)
	if err := sources.CloseRetaining(t.Context()); err != nil || blocked.closed != 1 || other.closed != 1 {
		t.Fatal("failed drain could not resume actual cleanup", err)
	}
}
