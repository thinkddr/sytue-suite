// SPDX-License-Identifier: Apache-2.0
package cifrado

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type sourceGuard struct {
	ResourceGuard
	scope                          storage.Scope
	registrations, unregistrations int
	denyAt                         int
}

func newSourceGuard(t *testing.T) *sourceGuard {
	t.Helper()
	factory, ctx, _, devices := factoryFixture(t)
	resources, err := factory.Open(ctx, devices)
	if err != nil {
		t.Fatal(err)
	}
	return &sourceGuard{ResourceGuard: resources, scope: resources.PrivateStorageScope()}
}

func (g *sourceGuard) Register(ctx context.Context, _ offline.PrivateWork) (func(), error) {
	g.registrations++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if g.registrations == g.denyAt {
		return nil, errors.New("registration denied")
	}
	return func() { g.unregistrations++ }, nil
}

func TestRetainedSourcesPartialRegistrationAndUnknownWorkKeepEveryReader(t *testing.T) {
	guard := newSourceGuard(t)
	guard.denyAt = 2
	first, second := &cycleRepository{scope: guard.scope, count: 7}, &cycleRepository{scope: guard.scope, count: 11}
	sources, err := NewRetainedSources(guard, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := sources.Register(t.Context()); err == nil {
		t.Fatal("partial registration was called complete")
	}
	if guard.unregistrations != 0 {
		t.Fatal("partial registration was released before draining")
	}
	guard.denyAt = 0
	if err := sources.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	if guard.registrations != 3 {
		t.Fatal("retry registered its first reader twice", guard.registrations)
	}
	second.pendingErr = errors.New("encrypted source cannot be authenticated")
	if err := sources.Retire(t.Context()); err == nil {
		t.Fatal("unknown work permitted source teardown")
	}
	if first.closed.Load()+second.closed.Load() != 0 || guard.unregistrations != 0 {
		t.Fatal("one source was closed before all counts were known")
	}
	second.pendingErr = nil
	if err := sources.Retire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if first.closed.Load() != 1 || second.closed.Load() != 1 || guard.unregistrations != 2 || first.count != 7 || second.count != 11 {
		t.Fatal("retirement lost sources, registrations or retained work")
	}
	if err := sources.Register(t.Context()); !errors.Is(err, offline.ErrWorkClosed) {
		t.Fatal("retired source admitted work", err)
	}
	if err := sources.Retire(t.Context()); err != nil || guard.unregistrations != 2 {
		t.Fatal("retirement was not idempotent", err)
	}
}

func TestRetainedSourcesRejectsScopeOrCountMismatch(t *testing.T) {
	guard := newSourceGuard(t)
	foreign := guard.scope
	foreign.Subject = "other"
	if _, err := NewRetainedSources(guard, &cycleRepository{scope: foreign}); err == nil {
		t.Fatal("foreign source accepted")
	}
	source := &cycleRepository{scope: guard.scope, count: -1}
	sources, err := NewRetainedSources(guard, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := sources.Retire(t.Context()); err == nil || source.closed.Load() != 0 {
		t.Fatal("invalid pending count permitted close", err)
	}
	source.count = 0
	source.scope = foreign
	if err := sources.Retire(t.Context()); err == nil || source.closed.Load() != 0 {
		t.Fatal("foreign pending scope permitted close", err)
	}
}

type composedSource struct {
	*cycleRepository
	paused    bool
	resumeErr error
}

func (s *composedSource) Drain(ctx context.Context) error {
	s.paused = true
	return s.cycleRepository.Drain(ctx)
}

func (s *composedSource) Resume() error {
	if s.resumeErr != nil {
		return s.resumeErr
	}
	s.paused = false
	return nil
}

func TestComposedSourcesKeepOneOwnerRegistrationAndOneSharedJournalInventory(t *testing.T) {
	guard := newSourceGuard(t)
	owner := &cycleRepository{scope: guard.scope}
	unregister, err := guard.Register(t.Context(), owner)
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	first := &composedSource{cycleRepository: &cycleRepository{scope: guard.scope, count: 4}}
	second := &composedSource{cycleRepository: &cycleRepository{scope: guard.scope, count: 4}}
	group, err := NewComposedSources(guard, owner, func(ctx context.Context) (offline.PendingWork, error) {
		return offline.PendingWork{Scope: guard.scope, Count: 4}, ctx.Err()
	}, first)
	if err != nil {
		t.Fatal(err)
	}
	if err := group.Add(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := group.Add(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := group.Add(t.Context(), owner); err == nil {
		t.Fatal("owner composed itself recursively")
	}
	if group.MemberCount() != 2 || guard.registrations != 1 {
		t.Fatal("children were registered twice or duplicated", group.MemberCount(), guard.registrations)
	}
	pending, err := group.Pending(t.Context())
	if err != nil || pending.Scope != guard.scope || pending.Count != 4 {
		t.Fatal("shared journal was counted as independent child jobs", pending, err)
	}
	if err := group.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !first.paused || !second.paused {
		t.Fatal("drain left child work active")
	}
	if err := group.Add(t.Context(), first); !errors.Is(err, offline.ErrWorkPaused) {
		t.Fatal("paused composition admitted work", err)
	}
	if err := group.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	if first.paused || second.paused {
		t.Fatal("aborted shutdown did not resume original children")
	}
	if err := group.Retire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if first.closed.Load() != 1 || second.closed.Load() != 1 || owner.closed.Load() != 0 || guard.unregistrations != 0 {
		t.Fatal("composition closed/unregistered its owner or lost a child")
	}
}

func TestComposedSourcesValidateEveryChildBeforeUsingUniqueInventoryOrClosing(t *testing.T) {
	guard := newSourceGuard(t)
	owner := &cycleRepository{scope: guard.scope}
	first := &cycleRepository{scope: guard.scope, count: 3}
	second := &cycleRepository{scope: guard.scope, count: 3, pendingErr: errors.New("encrypted child inventory is unknown")}
	count := 3
	group, err := NewComposedSources(guard, owner, func(context.Context) (offline.PendingWork, error) {
		return offline.PendingWork{Scope: guard.scope, Count: count}, nil
	}, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if err := group.Retire(t.Context()); err == nil {
		t.Fatal("unique inventory concealed an unknown child")
	}
	if first.closed.Load()+second.closed.Load() != 0 {
		t.Fatal("child closed before every inventory was validated")
	}
	second.pendingErr = nil
	count = 0
	if err := group.Retire(t.Context()); err == nil {
		t.Fatal("incomplete unique inventory hid nonzero child work")
	}
	if first.closed.Load()+second.closed.Load() != 0 {
		t.Fatal("incomplete inventory closed a child")
	}
	count = 3
	if err := group.Retire(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestRetainedSourcesAddPreservesPartialRegistrationAndResumeFailurePausesAll(t *testing.T) {
	guard := newSourceGuard(t)
	first := &composedSource{cycleRepository: &cycleRepository{scope: guard.scope}}
	second := &composedSource{cycleRepository: &cycleRepository{scope: guard.scope}}
	group, err := NewRetainedSources(guard, first)
	if err != nil {
		t.Fatal(err)
	}
	if err := group.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	guard.denyAt = 2
	if err := group.Add(t.Context(), second); err == nil {
		t.Fatal("failed registration was hidden")
	}
	if group.MemberCount() != 2 || guard.unregistrations != 0 {
		t.Fatal("failed registration discarded original membership")
	}
	guard.denyAt = 0
	if err := group.Register(t.Context()); err != nil {
		t.Fatal(err)
	}
	if guard.registrations != 3 {
		t.Fatal("retry registered the original child twice", guard.registrations)
	}
	if err := group.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	second.resumeErr = errors.New("original child session cannot resume")
	if err := group.Resume(t.Context()); err == nil {
		t.Fatal("partial resume was hidden")
	}
	if !first.paused || !second.paused {
		t.Fatal("failed resume left sibling work active")
	}
}

func TestRetainedSourcesAddRetainsOwnedChildWhenItsAdmissionContextWasCanceled(t *testing.T) {
	guard := newSourceGuard(t)
	first := &cycleRepository{scope: guard.scope}
	group, err := NewRetainedSources(guard)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := group.Add(ctx, first); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled child admission was accepted", err)
	}
	if group.MemberCount() != 1 || guard.registrations != 0 {
		t.Fatal("failed admission lost its owned child or registered it anyway")
	}
	if err := group.Retire(t.Context()); err != nil {
		t.Fatal(err)
	}
	if first.closed.Load() != 1 {
		t.Fatal("retirement leaked the child retained before canceled admission")
	}
}
