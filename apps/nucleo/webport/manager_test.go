// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type managerTestPort struct {
	mu                                 sync.Mutex
	id                                 string
	identity                           Identity
	current, calls, acquired, released int
	held                               bool
}

func newManagerTestPort(product string) *managerTestPort {
	scope := storage.Scope{Tenant: "tenant", Subject: "owner", Product: product}
	return &managerTestPort{id: strings.Repeat("a", 64), identity: Identity{Version: Version, Scope: scope, Binding: Binding{MatrixUserID: storage.IsolatedMatrixUserPrefix(scope) + ":matrix.test", DeviceID: "DEVICE"}}}
}
func (p *managerTestPort) ID() string { return p.id }
func (p *managerTestPort) Current(ctx context.Context) (Identity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	return p.identity, ctx.Err()
}

func (p *managerTestPort) Call(ctx context.Context, _ storage.Scope, method Method, _ []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	switch method {
	case Authorize:
		return nil, ctx.Err()
	case LeaseAcquire:
		p.acquired++
		if p.held {
			return nil, offline.ErrWorkLease
		}
		p.held = true
		return json.Marshal(map[string]string{"lease": strings.Repeat("b", 64)})
	case LeaseRelease:
		p.held = false
		p.released++
		return nil, nil
	default:
		return nil, ErrRejected
	}
}

func (p *managerTestPort) state() (current, acquired, released int, held bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current, p.acquired, p.released, p.held
}

func (p *managerTestPort) replaceSubject(subject string) {
	p.mu.Lock()
	p.identity.Scope.Subject = subject
	p.mu.Unlock()
}

type (
	managerTestCommand func(context.Context) error
	managerTestProduct struct {
		mu              sync.Mutex
		scope           storage.Scope
		executed        int
		pending         offline.PendingWork
		pendingErr      error
		onPrepare       func(context.Context) error
		onResume        func()
		closed, drained int
	}
)

func (p *managerTestProduct) PrivateStorageScope() storage.Scope { return p.scope }
func (p *managerTestProduct) Prepare(ctx context.Context) error {
	if p.onPrepare != nil {
		return p.onPrepare(ctx)
	}
	return ctx.Err()
}
func (p *managerTestProduct) Initialize(ctx context.Context) error { return ctx.Err() }
func (p *managerTestProduct) Ready(ctx context.Context) error      { return ctx.Err() }
func (p *managerTestProduct) Execute(ctx context.Context, command managerTestCommand) error {
	p.mu.Lock()
	p.executed++
	p.mu.Unlock()
	if command == nil {
		return ErrRejected
	}
	return command(ctx)
}

func (p *managerTestProduct) Pending(ctx context.Context) (offline.PendingWork, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pendingErr != nil {
		return offline.PendingWork{}, p.pendingErr
	}
	return p.pending, ctx.Err()
}

func (p *managerTestProduct) Drain(context.Context) error {
	p.mu.Lock()
	p.drained++
	p.mu.Unlock()
	return nil
}

func (p *managerTestProduct) Resume() error {
	if p.onResume != nil {
		p.onResume()
	}
	return nil
}
func (p *managerTestProduct) Close() {}
func (p *managerTestProduct) CloseAndRetain(ctx context.Context) error {
	p.mu.Lock()
	p.closed++
	p.mu.Unlock()
	return p.Drain(ctx)
}

func managerFixture(t *testing.T, product string) (*Manager[managerTestCommand], *managerTestPort, *managerTestProduct) {
	t.Helper()
	port := newManagerTestPort(product)
	work := &managerTestProduct{scope: port.identity.Scope, pending: offline.PendingWork{Scope: port.identity.Scope, Count: 1}}
	manager, err := NewManager[managerTestCommand](NewSources(), NewPolicies(), product, func(ctx context.Context, input ProductBuildContext) (ManagedProduct[managerTestCommand], error) {
		_, acquired, _, held := port.state()
		if !held || acquired != 1 {
			return nil, errors.New("constructor did not retain original work witness")
		}
		if input.Isolated.FileScope() != work.scope || input.Resources.PrivateStorageScope() != work.scope || input.Journal == nil || input.LegacyReader != nil || input.MigrationWriter != nil {
			return nil, ErrRejected
		}
		release, e := input.Resources.WorkLease()(ctx)
		if e != nil {
			return nil, e
		}
		release()
		return work, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager, port, work
}

func TestProductManagerUsesTheOriginalConstructionAndActionLeaseAcrossProducts(t *testing.T) {
	for _, product := range []string{"mail", "contacts"} {
		t.Run(product, func(t *testing.T) {
			m, p, _ := managerFixture(t, product)
			s, err := m.Open(t.Context(), ProductPorts{Isolated: p})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = m.ClosePort(context.WithoutCancel(t.Context()), p.ID()) }()
			_, acquired, released, held := p.state()
			if acquired != 1 || released != 1 || held {
				t.Fatal("constructor witness was duplicated or retained", acquired, released, held)
			}
			calls := 0
			err = s.Run(t.Context(), func(ctx context.Context) error {
				calls++
				_, _, _, held := p.state()
				if !held {
					return errors.New("action lost original witness")
				}
				release, e := s.resources.WorkLease()(ctx)
				if e != nil {
					return e
				}
				release()
				return nil
			})
			if err != nil || calls != 1 {
				t.Fatal("domain action did not use one bound admission", err, calls)
			}
			_, acquired, released, held = p.state()
			if acquired != 2 || released != 2 || held {
				t.Fatal("nested action acquired another host lock", acquired, released, held)
			}
			err = s.Run(t.Context(), func(context.Context) error { p.replaceSubject("foreign"); return nil })
			if !errors.Is(err, ErrRejected) {
				t.Fatal("POST accepted a changed original owner", err)
			}
			p.replaceSubject("owner")
		})
	}
}

func TestProductManagerDoesNotFabricateEmptyPendingAndRetiredCanonicalCannotReopen(t *testing.T) {
	m, p, work := managerFixture(t, "contacts")
	if _, err := m.Lookup(t.Context(), strings.Repeat("c", 64), p.ID()); !errors.Is(err, ErrRejected) {
		t.Fatal("unknown handle admitted", err)
	}
	if _, err := m.Open(t.Context(), ProductPorts{Isolated: p, LegacyReader: p}); !errors.Is(err, ErrRejected) {
		t.Fatal("role alias admitted", err)
	}
	s, err := m.Open(t.Context(), ProductPorts{Isolated: p})
	if err != nil {
		t.Fatal(err)
	}
	before, _, _, _ := p.state()
	if _, err = m.Open(t.Context(), ProductPorts{Isolated: p}); !errors.Is(err, ErrRejected) {
		t.Fatal("two canonical constructors admitted", err)
	}
	after, _, _, _ := p.state()
	if after != before {
		t.Fatal("duplicate reservation reached host")
	}
	pendingErr := errors.New("original journal unavailable")
	work.pendingErr = pendingErr
	if value, err := s.Pending(t.Context()); !errors.Is(err, pendingErr) || value.Count != 0 {
		t.Fatal("failed pending inventory became success", value, err)
	}
	work.pendingErr = nil
	if err = s.Pause(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = s.Run(t.Context(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("paused engine executed domain command")
	}
	if value, err := s.Pending(t.Context()); err != nil || value.Count != 1 || value.Scope != work.scope {
		t.Fatal("paused retained inventory was lost", value, err)
	}
	if err = m.ClosePort(t.Context(), p.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Pending(t.Context()); err == nil {
		t.Fatal("retired session returned pending success")
	}
	before, _, _, _ = p.state()
	if _, err = m.Open(t.Context(), ProductPorts{Isolated: p}); !errors.Is(err, ErrRejected) {
		t.Fatal("retired canonical reopened", err)
	}
	after, _, _, _ = p.state()
	if after != before {
		t.Fatal("retired ID reached SDK")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) != 0 || len(m.canonical) != 0 {
		t.Fatal("closed session retained registration")
	}
}

type managerConstructionPort struct {
	id                          string
	entered, cancelled, release chan struct{}
	current                     atomic.Int64
}

func (p *managerConstructionPort) ID() string { return p.id }
func (p *managerConstructionPort) Current(ctx context.Context) (Identity, error) {
	p.current.Add(1)
	close(p.entered)
	<-ctx.Done()
	close(p.cancelled)
	<-p.release
	return Identity{}, ctx.Err()
}

func (p *managerConstructionPort) Call(context.Context, storage.Scope, Method, []byte) ([]byte, error) {
	return nil, ErrRejected
}

func TestProductManagerRetirementJoinsCancelledConstructionBeforeAcknowledgement(t *testing.T) {
	m, err := NewManager[managerTestCommand](NewSources(), NewPolicies(), "contacts", func(context.Context, ProductBuildContext) (ManagedProduct[managerTestCommand], error) {
		return nil, ErrRejected
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &managerConstructionPort{id: strings.Repeat("d", 64), entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(p.release) })
	opened := make(chan error, 1)
	go func() { _, e := m.Open(t.Context(), ProductPorts{Isolated: p}); opened <- e }()
	<-p.entered
	closed := make(chan error, 1)
	go func() { closed <- m.ClosePort(t.Context(), p.ID()) }()
	<-p.cancelled
	select {
	case e := <-closed:
		t.Fatal("retirement acknowledged unresolved constructor IO", e)
	default:
	}
	release.Do(func() { close(p.release) })
	if <-opened == nil {
		t.Fatal("cancelled constructor returned an engine")
	}
	if err = <-closed; err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sessions) != 0 || len(m.canonical) != 0 {
		t.Fatal("cancelled constructor retained reservation")
	}
}

type managerTestReceiver struct {
	*managerTestProduct
	entered, cancelled, release chan struct{}
}

func (p *managerTestReceiver) RunReceiver(ctx context.Context) error {
	close(p.entered)
	<-ctx.Done()
	close(p.cancelled)
	<-p.release
	return ctx.Err()
}

func TestProductManagerRetirementJoinsOriginalActionAndReceiverBeforeAcknowledgement(t *testing.T) {
	p := newManagerTestPort("contacts")
	work := &managerTestReceiver{managerTestProduct: &managerTestProduct{scope: p.identity.Scope, pending: offline.PendingWork{Scope: p.identity.Scope, Count: 1}}, entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	m, err := NewManager[managerTestCommand](NewSources(), NewPolicies(), "contacts", func(context.Context, ProductBuildContext) (ManagedProduct[managerTestCommand], error) {
		return work, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Open(t.Context(), ProductPorts{Isolated: p})
	if err != nil {
		t.Fatal(err)
	}
	<-work.entered
	actionEntered, actionCancelled, actionRelease := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseAction, releaseReceiver sync.Once
	defer releaseAction.Do(func() { close(actionRelease) })
	defer releaseReceiver.Do(func() { close(work.release) })
	action := make(chan error, 1)
	go func() {
		action <- s.Run(t.Context(), func(ctx context.Context) error {
			close(actionEntered)
			<-ctx.Done()
			close(actionCancelled)
			<-actionRelease
			return ctx.Err()
		})
	}()
	<-actionEntered
	retired := make(chan error, 1)
	go func() { retired <- m.ClosePort(t.Context(), p.ID()) }()
	<-actionCancelled
	<-work.cancelled
	select {
	case e := <-retired:
		t.Fatal("retired original action before IO joined", e)
	default:
	}
	releaseAction.Do(func() { close(actionRelease) })
	if <-action == nil {
		t.Fatal("cancelled original action succeeded")
	}
	select {
	case e := <-retired:
		t.Fatal("retired original receiver before IO joined", e)
	default:
	}
	releaseReceiver.Do(func() { close(work.release) })
	if err = <-retired; err != nil {
		t.Fatal(err)
	}
	if work.closed != 1 {
		t.Fatal("retained close did not run exactly once", work.closed)
	}
}

func TestProductManagerResumeAndPendingRequireTheCapturedOwnerAndScope(t *testing.T) {
	m, p, work := managerFixture(t, "contacts")
	s, err := m.Open(t.Context(), ProductPorts{Isolated: p})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.ClosePort(context.WithoutCancel(t.Context()), p.ID()) }()
	if err = s.Pause(t.Context()); err != nil {
		t.Fatal(err)
	}
	work.onResume = func() { p.replaceSubject("foreign") }
	if err = s.Resume(t.Context()); err == nil {
		t.Fatal("resume lacked fresh POST owner check")
	}
	p.replaceSubject("owner")
	work.onResume = nil
	if err = s.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	work.pending.Scope.Subject = "foreign"
	if _, err = s.Pending(t.Context()); !errors.Is(err, ErrRejected) {
		t.Fatal("foreign pending scope accepted", err)
	}
	work.pending.Scope = work.scope
	work.pending.Count = -1
	if _, err = s.Pending(t.Context()); !errors.Is(err, ErrRejected) {
		t.Fatal("invalid pending count accepted", err)
	}
}

func TestProductManagerRetainedAdmissionCannotReviveItsContextOrBorrowItsRetiredLease(t *testing.T) {
	m, p, _ := managerFixture(t, "contacts")
	s, err := m.Open(t.Context(), ProductPorts{Isolated: p})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.ClosePort(context.WithoutCancel(t.Context()), p.ID()) }()
	for _, method := range []string{"Value", "Repository", "Product", "Resources", "Ports"} {
		if _, exists := reflect.TypeOf(s).MethodByName(method); exists {
			t.Fatal("raw capability getter returned to host API", method)
		}
	}
	var retained context.Context
	if err = s.Run(t.Context(), func(ctx context.Context) error { retained = ctx; return nil }); err != nil {
		t.Fatal(err)
	}
	if retained == nil || retained.Err() == nil {
		t.Fatal("original action context survived Run")
	}
	_, before, _, _ := p.state()
	type retainedContextKey struct{}
	for _, ctx := range []context.Context{retained, context.WithoutCancel(retained), context.WithValue(context.WithoutCancel(retained), retainedContextKey{}, true)} {
		release, e := s.resources.WorkLease()(ctx)
		if release != nil {
			release()
		}
		if e == nil {
			t.Fatal("retained original admission revived outside Run")
		}
	}
	_, after, _, _ := p.state()
	if after != before {
		t.Fatal("retired operation fell back to a new host lease", before, after)
	}
	if err = s.Pause(t.Context()); err != nil {
		t.Fatal(err)
	}
	executed := false
	if err = s.Run(t.Context(), func(context.Context) error { executed = true; return nil }); err == nil || executed {
		t.Fatal("paused command dispatched without fresh admission", err)
	}
	if err = m.ClosePort(t.Context(), p.ID()); err != nil {
		t.Fatal(err)
	}
	if err = s.Run(t.Context(), func(context.Context) error { executed = true; return nil }); err == nil || executed {
		t.Fatal("retired command dispatched with retained session", err)
	}
}

type managerTimedReceiver struct {
	*managerTestProduct
	muRun                        sync.Mutex
	starts, active, maximum      int
	entered                      chan int
	firstCancelled, releaseFirst chan struct{}
}

func (p *managerTimedReceiver) RunReceiver(ctx context.Context) error {
	p.muRun.Lock()
	p.starts++
	index := p.starts
	p.active++
	if p.active > p.maximum {
		p.maximum = p.active
	}
	p.muRun.Unlock()
	defer func() { p.muRun.Lock(); p.active--; p.muRun.Unlock() }()
	p.entered <- index
	<-ctx.Done()
	if index == 1 {
		close(p.firstCancelled)
		<-p.releaseFirst
	}
	return ctx.Err()
}

func TestProductManagerResumeCannotReplaceAReceiverWhosePauseTimedOut(t *testing.T) {
	port := newManagerTestPort("contacts")
	work := &managerTimedReceiver{managerTestProduct: &managerTestProduct{scope: port.identity.Scope, pending: offline.PendingWork{Scope: port.identity.Scope}}, entered: make(chan int, 3), firstCancelled: make(chan struct{}), releaseFirst: make(chan struct{})}
	m, err := NewManager[managerTestCommand](NewSources(), NewPolicies(), "contacts", func(context.Context, ProductBuildContext) (ManagedProduct[managerTestCommand], error) {
		return work, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Open(t.Context(), ProductPorts{Isolated: port})
	if err != nil {
		t.Fatal(err)
	}
	if <-work.entered != 1 {
		t.Fatal("initial receiver missing")
	}
	var release sync.Once
	defer release.Do(func() { close(work.releaseFirst) })
	defer func() {
		release.Do(func() { close(work.releaseFirst) })
		_ = m.ClosePort(context.WithoutCancel(t.Context()), port.ID())
	}()
	paused, cancel := context.WithCancel(t.Context())
	cancel()
	if err = s.Pause(paused); err == nil {
		t.Fatal("exhausted Pause context became completed drain")
	}
	<-work.firstCancelled
	retry, stop := context.WithTimeout(t.Context(), 100*time.Millisecond)
	err = s.Resume(retry)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Resume did not join previous receiver before admitting replacement", err)
	}
	work.muRun.Lock()
	starts, maximum := work.starts, work.maximum
	work.muRun.Unlock()
	if starts != 1 || maximum != 1 {
		t.Fatal("two receivers overlapped after timed out Pause", starts, maximum)
	}
	release.Do(func() { close(work.releaseFirst) })
	if err = s.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	if <-work.entered != 2 {
		t.Fatal("joined receiver was not replaced once")
	}
	work.muRun.Lock()
	maximum = work.maximum
	work.muRun.Unlock()
	if maximum != 1 {
		t.Fatal("replacement started before previous IO ended", maximum)
	}
}
