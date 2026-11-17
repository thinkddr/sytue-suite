// SPDX-License-Identifier: Apache-2.0
// Manager shares the verified host-port lifecycle across private product engines.
package webport

import (
	"context"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ManagedProduct supplies only product behavior. Reservation, construction,
// original leases and retirement remain in this single manager.
type ManagedProduct[C any] interface {
	offline.PrivateWork
	Prepare(context.Context) error
	Initialize(context.Context) error
	Ready(context.Context) error
	Execute(context.Context, C) error
	CloseAndRetain(context.Context) error
}
type ManagedReceiver interface{ RunReceiver(context.Context) error }

// ProductBuildContext contains already opened, same-scope role capabilities.
// Raw ports, source registries and policy registries never leave the manager.
// The constructor receives ctx carrying the original WorkGate lease; product
// API calls must use that ctx. Lifetime is only for ownership after construction.
type ProductBuildContext struct {
	Lifetime                                context.Context
	Isolated, LegacyReader, MigrationWriter *Storage
	Resources                               *Resources
	Journal                                 *offline.Store
}
type ProductBuilder[C any] func(context.Context, ProductBuildContext) (ManagedProduct[C], error)

type ProductPorts struct {
	Isolated        Port
	LegacyReader    Port
	MigrationWriter Port
}

func (p ProductPorts) ids() ([]string, error) {
	if p.Isolated == nil || !ValidID(p.Isolated.ID()) {
		return nil, ErrRejected
	}
	ids := []string{p.Isolated.ID()}
	for _, port := range []Port{p.LegacyReader, p.MigrationWriter} {
		if port == nil {
			continue
		}
		if !ValidID(port.ID()) {
			return nil, ErrRejected
		}
		for _, id := range ids {
			if id == port.ID() {
				return nil, ErrRejected
			}
		}
		ids = append(ids, port.ID())
	}
	if p.MigrationWriter != nil && p.LegacyReader == nil {
		return nil, ErrRejected
	}
	return ids, nil
}

type Manager[C any] struct {
	product   string
	build     ProductBuilder[C]
	mu        sync.Mutex
	sources   *Sources
	policies  *Policies
	sessions  map[string]*ManagedSession[C]
	canonical map[string]*ManagedSession[C]
	retired   map[string]struct{}
}

func NewManager[C any](sources *Sources, policies *Policies, product string, build ProductBuilder[C]) (*Manager[C], error) {
	if sources == nil || policies == nil || !storage.Product(product) || build == nil {
		return nil, ErrRejected
	}
	return &Manager[C]{product: product, build: build, sources: sources, policies: policies, sessions: make(map[string]*ManagedSession[C]), canonical: make(map[string]*ManagedSession[C]), retired: make(map[string]struct{})}, nil
}

type ManagedSession[C any] struct {
	id                       string
	ports                    ProductPorts
	portIDs                  []string
	life                     context.Context
	cancel                   context.CancelFunc
	opened                   chan struct{}
	closeOnce                sync.Once
	transition               sync.Mutex
	done                     chan struct{}
	mu                       sync.Mutex
	closing, retiring, ready bool
	closeErr                 error
	isolated                 *Storage
	resources                *Resources
	application              ManagedProduct[C]
	actions                  *offline.WorkGate
	receiverDone             chan struct{}
	receiverCancel           context.CancelFunc
	receiverErr              error
}

func (m *Manager[C]) Open(ctx context.Context, ports ProductPorts) (*ManagedSession[C], error) {
	if ctx == nil {
		return nil, ErrRejected
	}
	ids, err := ports.ids()
	if err != nil {
		return nil, err
	}
	id, err := NewCapabilityID()
	if err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &ManagedSession[C]{id: id, ports: ports, portIDs: ids, life: life, cancel: cancel, opened: make(chan struct{}), done: make(chan struct{})}
	m.mu.Lock()
	if m.canonical[ports.Isolated.ID()] != nil {
		m.mu.Unlock()
		cancel()
		return nil, ErrRejected
	}
	for _, port := range ids {
		if _, retired := m.retired[port]; retired {
			m.mu.Unlock()
			cancel()
			return nil, ErrRejected
		}
	}
	m.sessions[id] = s
	m.canonical[ports.Isolated.ID()] = s
	m.mu.Unlock()
	construction, stop := context.WithCancel(ctx)
	stopLife := context.AfterFunc(life, stop)
	err = s.prepare(construction, m.sources, m.policies, m.product, m.build)
	stopLife()
	stop()
	s.mu.Lock()
	closing := s.closing
	s.mu.Unlock()
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && closing {
		err = offline.ErrWorkClosed
	}
	close(s.opened)
	if err != nil {
		cleanup := s.shutdown(context.WithoutCancel(ctx), false)
		m.remove(s)
		return nil, errors.Join(err, cleanup)
	}
	return s, nil
}

func (s *ManagedSession[C]) prepare(ctx context.Context, sources *Sources, policies *Policies, product string, build ProductBuilder[C]) error {
	isolated, err := OpenStorage(ctx, s.ports.Isolated, sources, policies)
	if err != nil || isolated.FileScope().Product != product || isolated.StorageIdentityVersion() != storage.IsolatedIdentity {
		return ErrRejected
	}
	resources, err := OpenResourcesForLifetime(ctx, s.life, isolated)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.isolated = isolated
	s.resources = resources
	s.actions = offline.NewWorkGateWithLease(s.life, resources.WorkLease()) //nolint:contextcheck // the work gate is bound to the session lifetime s.life, not to the startup ctx
	retiring := s.retiring
	s.mu.Unlock()
	if retiring {
		resources.RetirePort()
	}
	ctx, release, err := s.actions.Start(ctx)
	if err != nil {
		return err
	}
	defer release()
	journal, identity, err := OpenJournalForLifetime(ctx, s.life, s.ports.Isolated)
	if err != nil || identity.Scope != isolated.FileScope() {
		return ErrRejected
	}
	reader, writer, err := s.openLegacyStorages(ctx, isolated.FileScope(), sources, policies)
	if err != nil {
		return err
	}
	app, err := build(ctx, ProductBuildContext{Lifetime: s.life, Isolated: isolated, LegacyReader: reader, MigrationWriter: writer, Resources: resources, Journal: journal})
	if !validPrivateWork(app) {
		return errors.Join(err, ErrRejected)
	}
	s.mu.Lock()
	s.application = app
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := initializeProduct(ctx, resources, app); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return offline.ErrWorkClosed
	}
	if err := s.startReceiverLocked(app); err != nil { //nolint:contextcheck // the receiver lives for the session lifetime, not for the call ctx
		return err
	}
	s.ready = true
	return nil
}

func (s *ManagedSession[C]) openLegacyStorages(ctx context.Context, scope storage.Scope, sources *Sources, policies *Policies) (reader, writer *Storage, err error) {
	if s.ports.LegacyReader != nil {
		reader, err = OpenStorage(ctx, s.ports.LegacyReader, sources, policies)
		if err != nil || reader.FileScope() != scope || reader.StorageIdentityVersion() != storage.LegacyIdentity {
			return nil, nil, ErrRejected
		}
	}
	if s.ports.MigrationWriter != nil {
		writer, err = OpenStorage(ctx, s.ports.MigrationWriter, sources, policies)
		if err != nil || writer.FileScope() != scope || writer.StorageIdentityVersion() != storage.LegacyIdentity {
			return nil, nil, ErrRejected
		}
	}
	return reader, writer, nil
}

func initializeProduct[C any](ctx context.Context, resources *Resources, app ManagedProduct[C]) error {
	if _, err := resources.Register(ctx, app); err != nil {
		return err
	}
	if err := app.Prepare(ctx); err != nil {
		return err
	}
	if err := app.Initialize(ctx); err != nil {
		return err
	}
	if err := app.Ready(ctx); err != nil {
		return err
	}
	return resources.Revalidate(ctx)
}
func (s *ManagedSession[C]) ID() string { return s.id }
func (s *ManagedSession[C]) Scope() storage.Scope {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isolated == nil {
		return storage.Scope{}
	}
	return s.isolated.FileScope()
}

func (s *ManagedSession[C]) Pending(ctx context.Context) (offline.PendingWork, error) {
	s.mu.Lock()
	app, resources, closing := s.application, s.resources, s.closing
	s.mu.Unlock()
	if app == nil || resources == nil || closing {
		return offline.PendingWork{}, offline.ErrWorkClosed
	}
	if err := resources.Revalidate(ctx); err != nil {
		return offline.PendingWork{}, err
	}
	value, err := app.Pending(ctx)
	if err != nil {
		return offline.PendingWork{}, err
	}
	if value.Scope != resources.PrivateStorageScope() || value.Count < 0 {
		return offline.PendingWork{}, ErrRejected
	}
	if err := resources.Revalidate(ctx); err != nil {
		return offline.PendingWork{}, err
	}
	s.mu.Lock()
	closing = s.closing
	s.mu.Unlock()
	if closing {
		return offline.PendingWork{}, offline.ErrWorkClosed
	}
	return value, nil
}

// Run admits one typed domain command. No repository or dispatcher is returned;
// the product executes and serializes its reply inside this original lease.
func (s *ManagedSession[C]) Run(ctx context.Context, command C) error {
	return s.run(ctx, func(active context.Context, product ManagedProduct[C]) error { return product.Execute(active, command) })
}

func (s *ManagedSession[C]) run(ctx context.Context, fn func(context.Context, ManagedProduct[C]) error) error {
	if ctx == nil || fn == nil {
		return ErrRejected
	}
	s.mu.Lock()
	gate, ready, app, resources := s.actions, s.ready && !s.closing, s.application, s.resources
	s.mu.Unlock()
	if gate == nil || !ready || app == nil || resources == nil {
		return offline.ErrWorkPaused
	}
	active, done, err := gate.Start(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := resources.Revalidate(active); err != nil {
		return err
	}
	if err := app.Ready(active); err != nil {
		return err
	}
	if err := fn(active, app); err != nil {
		return err
	}
	if err := resources.Revalidate(active); err != nil {
		return err
	}
	s.mu.Lock()
	ready = s.ready && !s.closing
	s.mu.Unlock()
	if !ready {
		return offline.ErrWorkPaused
	}
	return active.Err()
}

func (s *ManagedSession[C]) Pause(ctx context.Context) error {
	s.transition.Lock()
	defer s.transition.Unlock()
	s.mu.Lock()
	s.ready = false
	app, gate, stop, receiver := s.application, s.actions, s.receiverCancel, s.receiverDone
	s.mu.Unlock()
	if app == nil || gate == nil {
		return offline.ErrWorkPaused
	}
	if stop != nil {
		stop()
	}
	hooks := PortRevocations{
		func(ctx context.Context, _ string) error { return gate.Drain(ctx) },
		func(ctx context.Context, _ string) error { return app.Drain(ctx) },
	}
	err := hooks.Revoke(ctx, s.ports.Isolated.ID())
	if receiver != nil {
		select {
		case <-receiver:
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	}
	return err
}

func (s *ManagedSession[C]) Resume(ctx context.Context) error {
	s.transition.Lock()
	defer s.transition.Unlock()
	s.mu.Lock()
	app, gate, resources, closing, ready, previousReceiver := s.application, s.actions, s.resources, s.closing, s.ready, s.receiverDone
	s.mu.Unlock()
	if closing || app == nil || gate == nil || resources == nil {
		return offline.ErrWorkClosed
	}
	if ready {
		return resources.Revalidate(ctx)
	}
	if err := resources.Revalidate(ctx); err != nil {
		return err
	}
	// A previous Pause may have exhausted its caller while real IO still joined.
	// Resume must complete that original drain before opening any new admission.
	hooks := PortRevocations{func(ctx context.Context, _ string) error { return gate.Drain(ctx) }, func(ctx context.Context, _ string) error { return app.Drain(ctx) }}
	if err := hooks.Revoke(ctx, s.ports.Isolated.ID()); err != nil {
		return err
	}
	if previousReceiver != nil {
		select {
		case <-previousReceiver:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := resources.Revalidate(ctx); err != nil {
		return err
	}
	if err := app.Resume(); err != nil {
		return err
	}
	if err := gate.Resume(); err != nil {
		return errors.Join(err, app.Drain(ctx))
	}
	if err := resources.Revalidate(ctx); err != nil {
		return errors.Join(err, gate.Drain(context.WithoutCancel(ctx)), app.Drain(context.WithoutCancel(ctx)))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return offline.ErrWorkClosed
	}
	if err := s.startReceiverLocked(app); err != nil { //nolint:contextcheck // the receiver lives for the session lifetime, not for the call ctx
		return err
	}
	s.ready = true
	return nil
}

func (s *ManagedSession[C]) startReceiverLocked(app ManagedProduct[C]) error {
	if s.receiverDone != nil {
		select {
		case <-s.receiverDone:
		default:
			return offline.ErrWorkPaused
		}
	}
	receiver, ok := app.(ManagedReceiver)
	if !ok {
		return nil
	}
	life, cancel := context.WithCancel(s.life)
	done := make(chan struct{})
	s.receiverDone = done
	s.receiverCancel = cancel
	s.receiverErr = nil
	go func() {
		defer cancel()
		err := receiver.RunReceiver(life)
		s.mu.Lock()
		s.receiverErr = err
		s.mu.Unlock()
		close(done)
	}()
	return nil
}

func (s *ManagedSession[C]) shutdown(ctx context.Context, retiring bool) error {
	s.mu.Lock()
	s.closing = true
	s.ready = false
	if retiring {
		s.retiring = true
		if s.resources != nil {
			s.resources.RetirePort()
		}
	}
	s.mu.Unlock()
	s.cancel()
	s.closeOnce.Do(func() {
		defer close(s.done)
		<-s.opened
		s.transition.Lock()
		defer s.transition.Unlock()
		s.mu.Lock()
		app, resources, receiver, gate, stop := s.application, s.resources, s.receiverDone, s.actions, s.receiverCancel
		s.mu.Unlock()
		cleanup := context.WithoutCancel(ctx)
		var failures []error
		if stop != nil {
			stop()
		}
		if gate != nil {
			if err := gate.Drain(cleanup); err != nil {
				failures = append(failures, err)
			}
		}
		if app != nil {
			if err := app.CloseAndRetain(cleanup); err != nil {
				failures = append(failures, err)
			}
		}
		if resources != nil {
			if err := resources.CloseAndDrain(cleanup); err != nil {
				failures = append(failures, err)
			}
		}
		if receiver != nil {
			<-receiver
			s.mu.Lock()
			err := s.receiverErr
			s.mu.Unlock()
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, offline.ErrWorkClosed) && !errors.Is(err, offline.ErrWorkPaused) {
				failures = append(failures, err)
			}
		}
		s.closeErr = errors.Join(failures...)
	})
	<-s.done
	return s.closeErr
}

func (m *Manager[C]) Lookup(ctx context.Context, id, port string) (*ManagedSession[C], error) {
	if !ValidID(id) || !ValidID(port) {
		return nil, ErrRejected
	}
	m.mu.Lock()
	s := m.sessions[id]
	_, retired := m.retired[port]
	m.mu.Unlock()
	if s == nil || retired || s.ports.Isolated.ID() != port {
		return nil, ErrRejected
	}
	s.mu.Lock()
	resources, closing := s.resources, s.closing
	s.mu.Unlock()
	if closing || resources == nil {
		return nil, offline.ErrWorkClosed
	}
	if err := resources.Revalidate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (m *Manager[C]) Get(ctx context.Context, id, port string) (*ManagedSession[C], error) {
	s, err := m.Lookup(ctx, id, port)
	if err != nil {
		return nil, err
	}
	if err := s.run(ctx, func(context.Context, ManagedProduct[C]) error { return nil }); err != nil {
		return nil, err
	}
	return s, nil
}

func (m *Manager[C]) remove(s *ManagedSession[C]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[s.id] == s {
		delete(m.sessions, s.id)
	}
	if m.canonical[s.ports.Isolated.ID()] == s {
		delete(m.canonical, s.ports.Isolated.ID())
	}
}

// ClosePort is registered only on the fixed module lifecycle hook. The host
// factory retires every member of a pair before its first Go acknowledgement.
func (m *Manager[C]) ClosePort(ctx context.Context, port string) error {
	if !ValidID(port) {
		return ErrRejected
	}
	m.mu.Lock()
	m.retired[port] = struct{}{}
	var sessions []*ManagedSession[C]
	for _, s := range m.sessions {
		for _, id := range s.portIDs {
			if id == port {
				sessions = append(sessions, s)
				break
			}
		}
	}
	m.mu.Unlock()
	hooks := make(PortRevocations, 0, len(sessions))
	for _, s := range sessions {
		hooks = append(hooks, func(ctx context.Context, _ string) error { err := s.shutdown(ctx, true); m.remove(s); return err })
	}
	return hooks.Revoke(context.WithoutCancel(ctx), port)
}
