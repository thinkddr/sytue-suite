// SPDX-License-Identifier: Apache-2.0
package offline

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestReentrantWorkLeaseThreeGatesPreserveExactSourceWithWriterWaiting(t *testing.T) {
	var lock sync.RWMutex
	var acquisitions, releases atomic.Int64
	var writerPending atomic.Bool
	source := func(ctx context.Context) (func(), error) {
		acquisitions.Add(1)
		// A second physical read acquisition while a writer waits is the regression.
		if writerPending.Load() {
			return nil, ErrWorkLease
		}
		lock.RLock()
		return func() { releases.Add(1); lock.RUnlock() }, ctx.Err()
	}
	lease := ReentrantWorkLease(source)
	root := NewWorkGateWithLease(t.Context(), lease)
	defer root.Close()
	ctx, finish, err := root.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	done := func() { once.Do(finish) }
	defer done()
	writerAcquired, allowWriter := make(chan struct{}), make(chan struct{})
	go func() { lock.Lock(); close(writerAcquired); <-allowWriter; lock.Unlock() }()
	// TryRLock=false proves a writer is pending: our original read lock is held.
	deadline := time.Now().Add(time.Second)
	for lock.TryRLock() {
		lock.RUnlock()
		if time.Now().After(deadline) {
			done()
			close(allowWriter)
			t.Fatal("writer did not wait behind original lease")
		}
		runtime.Gosched()
	}
	writerPending.Store(true)
	second := NewWorkGateWithLease(ctx, lease)
	defer second.Close()
	ctx2, done2, err := second.Start(ctx)
	if err != nil {
		done()
		close(allowWriter)
		t.Fatal("nested gate reacquired physical read lock", err)
	}
	third := NewWorkGateWithLease(ctx2, lease)
	defer third.Close()
	ctx3, done3, err := third.StartBound(ctx2, func(ctx context.Context) (context.Context, error) {
		return context.WithValue(ctx, boundWorkAdapterKey{}, "native signer"), nil
	})
	if err != nil {
		done2()
		done()
		close(allowWriter)
		t.Fatal(err)
	}
	release, err := lease(context.WithValue(ctx3, boundWorkCallerKey{}, "derived callback"))
	if err != nil || acquisitions.Load() != 1 {
		done3()
		done2()
		done()
		close(allowWriter)
		t.Fatal("same source did not lend exact held lease", err, acquisitions.Load())
	}
	release()
	release()
	done3()
	done2()
	done()
	<-writerAcquired
	close(allowWriter)
	if acquisitions.Load() != 1 || releases.Load() != 1 {
		t.Fatal("nested scopes changed physical lease lifetime", acquisitions.Load(), releases.Load())
	}
}

func TestReentrantWorkLeaseRetiresAndDrainsBorrowersBeforeActualRelease(t *testing.T) {
	var acquisitions atomic.Int64
	physicalRelease := make(chan struct{})
	lease := ReentrantWorkLease(func(context.Context) (func(), error) {
		acquisitions.Add(1)
		return func() { close(physicalRelease) }, nil
	})
	gate := NewWorkGateWithLease(t.Context(), lease)
	defer gate.Close()
	ctx, done, err := gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release, err := lease(ctx)
	if err != nil {
		done()
		t.Fatal(err)
	}
	finished := make(chan struct{})
	go func() { done(); close(finished) }()
	<-ctx.Done()
	// Removing cancellation from a stale context cannot create a fresh operation.
	if _, err := lease(context.WithoutCancel(ctx)); err == nil {
		release()
		<-finished
		t.Fatal("retired lease fell back to another acquisition")
	}
	releasedEarly := false
	select {
	case <-physicalRelease:
		releasedEarly = true
	case <-time.After(20 * time.Millisecond):
	}
	release()
	<-finished
	if releasedEarly || acquisitions.Load() != 1 {
		t.Fatal("root released actual source before borrowed SDK cleanup", acquisitions.Load())
	}
	if err := gate.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestReentrantWorkLeaseUsesWrapperIdentityNotFunctionCodeAndNeverRevivesToken(t *testing.T) {
	acquisitions, releases := 0, 0
	source := func(context.Context) (func(), error) { acquisitions++; return func() { releases++ }, nil }
	first, other := ReentrantWorkLease(source), ReentrantWorkLease(source)
	gate := NewWorkGateWithLease(t.Context(), first)
	defer gate.Close()
	ctx, done, err := gate.Start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	release, err := other(ctx)
	if err != nil || acquisitions != 2 {
		if release != nil {
			release()
		}
		done()
		t.Fatal("different wrapper inherited another source token", err, acquisitions)
	}
	release()
	if _, err := other(ctx); !errors.Is(err, ErrWorkLease) || acquisitions != 2 {
		done()
		t.Fatal("retired token reacquired a fresh source", err, acquisitions)
	}
	borrowed, err := first(ctx)
	if err != nil || acquisitions != 2 {
		done()
		t.Fatal(err)
	}
	borrowed()
	done()
	if releases != 2 {
		t.Fatal("physical releases changed", releases)
	}
	// A genuinely new operation gets a new holder and may acquire its own lease.
	fresh := NewWorkGateWithLease(t.Context(), first)
	defer fresh.Close()
	_, finish, err := fresh.Start(t.Context())
	if err != nil || acquisitions != 3 {
		t.Fatal("new operation could not acquire current source", err)
	}
	finish()
	if releases != 3 {
		t.Fatal("new operation leaked its lease")
	}
}

func TestReentrantWorkLeaseOutsideGateIsOrdinaryAndMissingSourceFails(t *testing.T) {
	calls := 0
	lease := ReentrantWorkLease(func(context.Context) (func(), error) { calls++; return func() {}, nil })
	for range 2 {
		release, err := lease(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if calls != 2 {
		t.Fatal("unbound context borrowed an unrelated lease")
	}
	if _, err := ReentrantWorkLease(nil)(t.Context()); !errors.Is(err, ErrWorkLease) {
		t.Fatal("nil source became a lease", err)
	}
	if _, err := ReentrantWorkLease(func(context.Context) (func(), error) { return nil, nil })(t.Context()); !errors.Is(err, ErrWorkLease) { //nolint:nilnil // test double: deliberately returns a nil release to prove ReentrantWorkLease rejects it
		t.Fatal("missing release became a lease", err)
	}
}
