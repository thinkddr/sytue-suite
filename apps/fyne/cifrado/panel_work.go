// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cifrado

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	mx "git.sytue.eu/sytue/one/apps/nucleo/matrix"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// PanelWorkers protects key creation/recovery before an application repository
// exists. Finite crypto work holds the session's shared lease; sync is separately
// joined so it cannot hold that lease indefinitely while waiting for messages.
type PanelWorkers struct {
	session        *ApplicationSession
	finite, syncer *offline.WorkGate
	mu             sync.Mutex
	clients        map[*mx.Client]bool
	unregister     func()
	closed         bool
	paused         atomic.Bool
	era            atomic.Uint64
	restart        func()
	status         func(context.Context, *mx.Client) (mx.VerificationStatus, error)
}

var _ offline.PrivateWork = (*PanelWorkers)(nil)

type (
	panelTicketKey struct{}
	panelTicket    struct {
		life       context.Context
		generation uint64
		worker     *PanelWorkers
		era        uint64
		cancelled  atomic.Bool
	}
)

//nolint:contextcheck // Gates retain the presenter lifetime; registration uses the caller operation context.
func newPanelWorkers(ctx context.Context, session *ApplicationSession) (*PanelWorkers, error) {
	if session == nil {
		return nil, ErrPrivateResources
	}
	if err := session.Check(ctx); err != nil {
		return nil, err
	}
	w := &PanelWorkers{session: session, finite: offline.NewWorkGateWithLease(session.Context(), session.WorkLease()), syncer: offline.NewWorkGate(session.Context()), clients: map[*mx.Client]bool{}, status: func(ctx context.Context, c *mx.Client) (mx.VerificationStatus, error) {
		return c.VerificationStatus(ctx)
	}}
	unregister, err := session.Register(ctx, w)
	if err != nil {
		w.finite.Close()
		w.syncer.Close()
		return nil, err
	}
	w.unregister = unregister
	return w, nil
}

func (w *PanelWorkers) PrivateStorageScope() storage.Scope { return w.session.PrivateStorageScope() }

func (w *PanelWorkers) start(ctx context.Context, background bool) (context.Context, func(), error) {
	if bound, _ := ctx.Value(applicationSessionKey{}).(*ApplicationSession); bound != w.session {
		return nil, nil, auth.ErrAuthorizationChanged
	}
	if w.paused.Load() {
		return nil, nil, offline.ErrWorkPaused
	}
	if err := w.session.Check(ctx); err != nil {
		return nil, nil, err
	}
	gate := w.finite
	if background {
		gate = w.syncer
	}
	return gate.Start(ctx)
}

func (w *PanelWorkers) Drain(ctx context.Context) error {
	w.paused.Store(true)
	w.era.Add(1)
	return errors.Join(w.finite.Drain(ctx), w.syncer.Drain(ctx))
}

func (w *PanelWorkers) Pending(ctx context.Context) (offline.PendingWork, error) {
	result := offline.PendingWork{Scope: w.PrivateStorageScope()}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	w.mu.Lock()
	clients := make([]*mx.Client, 0, len(w.clients))
	for c := range w.clients {
		clients = append(clients, c)
	}
	w.mu.Unlock()
	for _, client := range clients {
		status, err := w.status(ctx, client)
		if err != nil {
			return result, err
		}
		if status.RecoverySetupPending {
			result.Count++
		}
	}
	return result, nil
}

func (w *PanelWorkers) Resume() error {
	if err := w.session.Check(w.session.Context()); err != nil {
		return err
	}
	if err := w.finite.Resume(); err != nil {
		return err
	}
	if err := w.syncer.Resume(); err != nil {
		return err
	}
	w.paused.Store(false)
	w.mu.Lock()
	restart := w.restart
	w.mu.Unlock()
	if restart != nil {
		restart()
	}
	return nil
}

func (w *PanelWorkers) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	unregister := w.unregister
	w.unregister = nil
	w.mu.Unlock()
	w.paused.Store(true)
	w.era.Add(1)
	w.finite.Close()
	w.syncer.Close()
	if unregister != nil {
		unregister()
	}
}

func (w *PanelWorkers) retain(client *mx.Client) {
	if client != nil {
		w.mu.Lock()
		w.clients[client] = true
		w.mu.Unlock()
	}
}

func (w *PanelWorkers) release(client *mx.Client) {
	w.mu.Lock()
	delete(w.clients, client)
	w.mu.Unlock()
}

func (c *Panel) beginWork(ctx context.Context, background bool) (context.Context, func(), *PanelWorkers, error) {
	worker := c.workers.Load()
	if worker != nil {
		job, done, err := worker.start(ctx, background)
		return job, done, worker, err
	}
	c.mu.Lock()
	if c.localWork == nil {
		c.localWork = offline.NewWorkGate(ctx)
	}
	gate := c.localWork
	c.mu.Unlock()
	job, done, err := gate.Start(ctx)
	return job, done, nil, err
}

func (c *Panel) runWork(ctx context.Context, generation uint64, background bool, fn func(context.Context, *PanelWorkers)) {
	go func() {
		job, done, worker, err := c.beginWork(ctx, background)
		if err != nil {
			if !background {
				fyne.Do(func() {
					if generation == c.generation && ctx.Err() == nil && c.workers.Load() == worker {
						c.busy = false
						c.fallo()
						c.pintar() //nolint:contextcheck // UI rendering retains future actions on the panel lifetime.
					}
				})
			}
			return
		}
		life := ctx //nolint:contextcheck // The UI ticket retains the original lifetime, not the gate request context cancelled at completion.
		if inherited, _ := ctx.Value(panelTicketKey{}).(*panelTicket); inherited != nil {
			life, generation = inherited.life, inherited.generation
		}
		ticket := &panelTicket{life: life, generation: generation, worker: worker}
		if worker != nil {
			ticket.era = worker.era.Load()
		}
		defer func() { ticket.cancelled.Store(job.Err() != nil); done() }()
		fn(context.WithValue(job, panelTicketKey{}, ticket), worker)
	}()
}

func (c *Panel) callback(ctx context.Context) (context.Context, uint64, *PanelWorkers, bool) {
	if ticket, _ := ctx.Value(panelTicketKey{}).(*panelTicket); ticket != nil {
		valid := !ticket.cancelled.Load() && c.workCurrent(ticket.life, ticket.worker)
		if ticket.worker != nil && ticket.era != ticket.worker.era.Load() {
			valid = false
		}
		return ticket.life, ticket.generation, ticket.worker, valid
	}
	worker := c.workers.Load()
	return ctx, 0, worker, c.workCurrent(ctx, worker)
}

func (c *Panel) workCurrent(ctx context.Context, worker *PanelWorkers) bool {
	if ctx.Err() != nil || c.workers.Load() != worker {
		return false
	}
	if worker == nil {
		return true
	}
	if worker.paused.Load() {
		return false
	}
	return worker.session.Check(ctx) == nil
}

func (c *Panel) remember(client *mx.Client, worker *PanelWorkers) {
	if client == nil {
		return
	}
	c.mu.Lock()
	if c.clients == nil {
		c.clients = map[*mx.Client]bool{}
	}
	c.clients[client] = true
	c.mu.Unlock()
	if worker != nil {
		worker.retain(client)
	}
}

func (c *Panel) releaseClient(client *mx.Client, worker *PanelWorkers) {
	if client == nil {
		return
	}
	_ = client.Close()
	c.mu.Lock()
	delete(c.clients, client)
	c.mu.Unlock()
	if worker != nil {
		worker.release(client)
	}
}

// BindWorkers is opt-in and runs on the UI thread. Existing device keys stay
// open while pre-binding jobs are cancelled/joined and the same client is adopted.
// The callback belongs only to this generation and receives no private details.
func (c *Panel) BindWorkers(session *ApplicationSession, ready func(error)) {
	if session == nil || ready == nil {
		return
	}
	if current := c.workers.Load(); current != nil {
		if current.session == session {
			ready(nil)
		} else {
			ready(ErrPrivateResources)
		}
		return
	}
	if c.Base.Subject != "" && c.Base.Subject != session.identity.Subject || c.Base.Tenant != "" && c.Base.Tenant != session.scope.Tenant {
		ready(ErrPrivateResources)
		return
	}
	c.mu.RLock()
	oldGate := c.localWork
	c.mu.RUnlock()
	generation := c.generation
	c.busy = true
	go func() {
		work, err := preparePanelWorkers(session, oldGate)
		fyne.Do(func() { c.finishWorkerBinding(session, work, generation, err, ready) })
	}()
}

func preparePanelWorkers(session *ApplicationSession, oldGate *offline.WorkGate) (*PanelWorkers, error) {
	release, err := session.WorkLease()(session.Context())
	if err != nil {
		return nil, err
	}
	defer release()
	work, err := newPanelWorkers(session.Context(), session)
	if err == nil && oldGate != nil {
		err = oldGate.Drain(session.Context())
	}
	if err == nil {
		err = session.Check(session.Context())
	}
	return work, err
}

func (c *Panel) finishWorkerBinding(session *ApplicationSession, work *PanelWorkers, generation uint64, err error, ready func(error)) {
	if generation != c.generation || session.Check(session.Context()) != nil {
		if work != nil {
			go work.Close()
		}
		return
	}
	c.busy = false
	if err != nil {
		if work != nil {
			go work.Close()
		}
		ready(ErrPrivateResources)
		return
	}
	c.generation++
	if c.nativeRecovery {
		c.Base.NativeRecovery = &mx.NativeRecoveryConfig{Binding: session.witness.Acceso.Revision, Secrets: nativeRecoverySecrets{}, Authorize: session.Check}
	}
	c.workers.Store(work)
	c.mu.RLock()
	client := c.client
	for retained := range c.clients {
		work.retain(retained)
	}
	c.mu.RUnlock()
	if client == nil {
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
		c.ctx = nil
	} else if c.ctx != nil {
		c.ctx = context.WithValue(auth.WithoutAuthorizationIntent(c.ctx), applicationSessionKey{}, session)
	}
	c.workerResumeHook(work)
	if client != nil {
		c.instalar(client)
	}
	ready(nil)
}

func (c *Panel) workerResumeHook(work *PanelWorkers) {
	work.mu.Lock()
	work.restart = func() {
		fyne.Do(func() {
			if c.workers.Load() != work || work.session.Check(work.session.Context()) != nil {
				return
			}
			if current, err := c.Cliente(work.session.Context()); err == nil {
				c.instalar(current)
			} else {
				c.busy = false
				c.pintar()
			}
		})
	}
	work.mu.Unlock()
}
