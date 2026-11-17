// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type cycleRepository struct {
	scope                               storage.Scope
	mu                                  sync.Mutex
	count                               int
	pendingErr, drainErr, initializeErr error
	pendingScope                        storage.Scope
	initialized, closed                 atomic.Int32
	onInitialize                        func(context.Context) error
}

func (r *cycleRepository) PrivateStorageScope() storage.Scope { return r.scope }
func (r *cycleRepository) Initialize(ctx context.Context) error {
	r.initialized.Add(1)
	if r.onInitialize != nil {
		return r.onInitialize(ctx)
	}
	return r.initializeErr
}

func (r *cycleRepository) Pending(ctx context.Context) (offline.PendingWork, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	scope := r.scope
	if r.pendingScope.Valid() {
		scope = r.pendingScope
	}
	if err := ctx.Err(); err != nil {
		return offline.PendingWork{}, err
	}
	return offline.PendingWork{Scope: scope, Count: r.count}, r.pendingErr
}

func (r *cycleRepository) Drain(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.drainErr
}
func (r *cycleRepository) Resume() error { return nil }
func (r *cycleRepository) Close()        { r.closed.Add(1) }

func cycleFixture(t *testing.T, cfg RepositoryConfig[*cycleRepository]) (*RepositoryCycle[*cycleRepository], *cycleRepository, *auth.Session, Devices[*factoryDevice]) {
	t.Helper()
	f, ctx, session, devices := factoryFixture(t)
	resources, err := f.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	repo := &cycleRepository{scope: resources.PrivateStorageScope()}
	if cfg.Create == nil {
		cfg.Create = func(context.Context) (*cycleRepository, error) { return repo, nil }
	}
	cycle, err := NewRepositoryCycle(resources, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cycle, repo, session, devices
}

func TestRepositoryCyclePreparesBeforeInitializeAndRetainsFailedMigration(t *testing.T) {
	var events []string
	created := 0
	var lifetime context.Context
	cycle, repo, _, devices := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
	cycle.cfg.Create = func(ctx context.Context) (*cycleRepository, error) {
		created++
		lifetime = ctx
		events = append(events, "create")
		return repo, nil
	}
	failure := errors.New("retained source import has not completed")
	cycle.cfg.Prepare = func(context.Context, *cycleRepository) error {
		if devices.Legacy.preparations < 2 || devices.Isolated.preparations < 2 {
			t.Error("freeze before owner keys were prepared")
		}
		events = append(events, "prepare")
		return nil
	}
	repo.onInitialize = func(context.Context) error { events = append(events, "initialize"); return failure }
	if err := cycle.Open(t.Context()); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	mounted := 0
	cycle.Dispatch(func(f func()) { f() }, func(*cycleRepository) { mounted++ })
	if mounted != 0 {
		t.Fatal("partial import mounted an empty private account")
	}
	failure = nil
	if err := cycle.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	cycle.Dispatch(func(f func()) { f() }, func(*cycleRepository) { mounted++ })
	if mounted != 1 || created != 1 || lifetime.Err() != nil || !reflect.DeepEqual(events, []string{"create", "prepare", "initialize", "prepare", "initialize"}) {
		t.Fatal("retry replaced retained repository or used a request lifetime", created, mounted, events, lifetime.Err())
	}
	if err := cycle.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if repo.closed.Load() != 1 || devices.Legacy.closes+devices.Isolated.closes != 0 {
		t.Fatal("repository close revoked borrowed clients")
	}
}

func TestRepositoryCycleQueuedUIAndWorkRejectOldSessionIncludingABA(t *testing.T) {
	cycle, _, session, _ := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
	if err := cycle.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	var queued func()
	called := 0
	cycle.Dispatch(func(f func()) { queued = f }, func(*cycleRepository) { called++ })
	bound, ok := cycle.resources.(*Resources[*factoryDevice])
	if !ok {
		t.Fatal("missing private resource fixture")
	}
	factoryAdopt(t, bound.session.witness, session, factoryTokens("bob"))
	factoryAdopt(t, bound.session.witness, session, factoryTokens("alice"))
	queued()
	if called != 0 {
		t.Fatal("queued UI action adopted later login")
	}
	err := cycle.Run(t.Context(), func(context.Context, *cycleRepository) error { called++; return nil })
	if err == nil || called != 0 {
		t.Fatal("worker adopted later login", err, called)
	}
	if err := cycle.Resume(t.Context()); err == nil {
		t.Fatal("resume revived cancelled owner", err)
	}
}

func TestRepositoryCycleClosesOnlyAfterValidEmptyPendingAndDrain(t *testing.T) {
	for _, test := range []string{"pending", "pending error", "foreign scope", "negative count", "drain error"} {
		t.Run(test, func(t *testing.T) {
			cycle, repo, _, devices := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
			if err := cycle.Open(t.Context()); err != nil {
				t.Fatal(err)
			}
			switch test {
			case "pending":
				repo.count = 2
			case "pending error":
				repo.pendingErr = offline.ErrEncryptedStore
			case "foreign scope":
				repo.pendingScope = storage.Scope{Tenant: "acme", Subject: "bob", Product: "mail"}
			case "negative count":
				repo.count = -1
			case "drain error":
				repo.drainErr = offline.ErrWorkPaused
			}
			if err := cycle.Close(t.Context()); err == nil {
				t.Fatal("incomplete drain authorized closing repository")
			}
			if repo.closed.Load() != 0 || devices.Legacy.closes+devices.Isolated.closes != 0 {
				t.Fatal("pending/error discarded keys or repository")
			}
			called := false
			if err := cycle.Run(t.Context(), func(context.Context, *cycleRepository) error { called = true; return nil }); err == nil || called {
				t.Fatal("drained cycle admitted new work")
			}
			repo.count, repo.pendingErr, repo.drainErr, repo.pendingScope = 0, nil, nil, storage.Scope{}
			if err := cycle.Resume(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := cycle.Run(t.Context(), func(context.Context, *cycleRepository) error { called = true; return nil }); err != nil || !called {
				t.Fatal("aborted shutdown could not resume", err)
			}
			if err := cycle.Close(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := cycle.Close(t.Context()); err != nil {
				t.Fatal("close is not idempotent", err)
			}
			if repo.closed.Load() != 1 {
				t.Fatal("repository closed more than once")
			}
		})
	}
}

func TestRepositoryCycleDrainCancelsAndWaitsForCleanupBeforePending(t *testing.T) {
	cycle, repo, _, _ := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
	if err := cycle.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	started, cancelled, release, workDone := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		workDone <- cycle.Run(t.Context(), func(ctx context.Context, _ *cycleRepository) error {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-release
			repo.mu.Lock()
			repo.count = 3
			repo.mu.Unlock()
			return ctx.Err()
		})
	}()
	<-started
	type result struct {
		pending offline.PendingWork
		err     error
	}
	drained := make(chan result, 1)
	go func() { p, e := cycle.Drain(t.Context()); drained <- result{p, e} }()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("drain did not cancel active work")
	}
	select {
	case <-drained:
		t.Fatal("pending counted before publication cleanup")
	default:
	}
	close(release)
	if err := <-workDone; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r := <-drained
	if r.err != nil || r.pending.Count != 3 || r.pending.Scope != repo.scope {
		t.Fatal("cleanup WAL was not counted", r)
	}
	if err := cycle.Close(t.Context()); !errors.Is(err, ErrPrivatePending) {
		t.Fatal("retained cleanup discarded", err)
	}
}

func TestRepositoryCycleResourceChangeDuringPreparePreventsInitialize(t *testing.T) {
	cycle, repo, _, devices := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
	cycle.cfg.Prepare = func(context.Context, *cycleRepository) error {
		devices.Isolated.binding.Target.RoomID = "!changed:rooms.example.test"
		devices.Isolated.binding.Rooms = append(devices.Isolated.binding.Rooms, devices.Isolated.binding.Target.RoomID)
		return nil
	}
	if err := cycle.Open(t.Context()); !errors.Is(err, ErrPrivateResources) {
		t.Fatal("changed target accepted", err)
	}
	if repo.initialized.Load() != 0 {
		t.Fatal("repository initialized with a changed target")
	}
	cycle.Dispatch(func(f func()) { f() }, func(*cycleRepository) { t.Fatal("changed target mounted") })
}

func TestRepositoryCycleDrainWaitsForCancelledPreparationWithoutMount(t *testing.T) {
	cycle, repo, _, _ := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
	started, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cycle.cfg.Prepare = func(ctx context.Context, _ *cycleRepository) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return ctx.Err()
	}
	opened := make(chan error, 1)
	go func() { opened <- cycle.Open(t.Context()) }()
	<-started
	drained := make(chan error, 1)
	go func() { _, err := cycle.Drain(t.Context()); drained <- err }()
	<-cancelled
	select {
	case <-drained:
		t.Fatal("drain returned before preparation ended")
	default:
	}
	close(release)
	if err := <-opened; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if repo.initialized.Load() != 0 {
		t.Fatal("cancelled migration initialized")
	}
	cycle.Dispatch(func(f func()) { f() }, func(*cycleRepository) { t.Fatal("cancelled migration mounted") })
}

func TestRepositoryCycleRegistersAndHoldsLeaseAcrossPrepareThenRetiresPreservingWork(t *testing.T) {
	cycle, repo, _, devices := cycleFixture(t, RepositoryConfig[*cycleRepository]{})
	bound, ok := cycle.resources.(*Resources[*factoryDevice])
	if !ok {
		t.Fatal("missing resource fixture")
	}
	registered, removed := 0, 0
	bound.session.register = func(work offline.PrivateWork, _ auth.TestigoTrabajo) (func(), error) {
		if work != repo {
			t.Fatal("registered a later repository")
		}
		registered++
		return func() { removed++ }, nil
	}
	cycle.cfg.Prepare = func(ctx context.Context, _ *cycleRepository) error {
		if registered != 1 {
			t.Fatal("Prepare ran before suite registration")
		}
		short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		if release, err := bound.session.witness.Exclusivo(short); err == nil {
			release()
			t.Fatal("Prepare did not retain the shared session lease")
		}
		return nil
	}
	if err := cycle.Open(t.Context()); err != nil {
		t.Fatal(err)
	}
	repo.count = 2
	pending, err := cycle.RetirePreservingWork(t.Context())
	if err != nil || pending.Count != 2 || repo.closed.Load() != 1 || removed != 1 {
		t.Fatal("ordinary view retirement discarded or failed to retain work", pending, err, removed)
	}
	if devices.Legacy.closes+devices.Isolated.closes != 0 {
		t.Fatal("repository retirement revoked SDK credentials")
	}
}
