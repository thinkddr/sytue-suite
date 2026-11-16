// SPDX-License-Identifier: Apache-2.0
package notas

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ErrLocalAdmission rejects a new edit before its original session acquired a lease.
var ErrLocalAdmission = errors.New("notes: local edit was not admitted by its original session")

// LocalWork owns only Notes captures/editor drafts, never the records engine's
// publication or identity namespaces. Register it before mounting a presenter.
type LocalWork struct {
	scope             storage.Scope
	journal           *offline.Store
	gate              *offline.WorkGate
	acquire           offline.WorkLease
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	writes            sync.Mutex
	retainedMu        sync.Mutex
	queueMu           sync.Mutex
	editors           map[string]*editorSlot
	editorOrder       uint64
	editorWriter      func(context.Context, string, EditorDraft) error
	editorBeforeWrite func()
	phase             context.Context
	cancelPhase       context.CancelFunc
	generation        uint64
	closed, paused    bool
	retained          map[localKey]json.RawMessage
	retainedLeases    map[localKey]func()
	resume            func()
}
type localKey struct{ namespace, id string }

func NewLocalWork(parent context.Context, scope storage.Scope, journal *offline.Store, acquire offline.WorkLease) (*LocalWork, error) {
	if acquire == nil {
		return nil, offline.ErrWorkLease
	}
	if parent == nil || !scope.Valid() || scope.Product != "notes" || !journal.Encrypted() {
		return nil, offline.ErrEncryptedStore
	}
	ctx, cancel := context.WithCancel(parent)
	phase, cancelPhase := context.WithCancel(ctx)
	return &LocalWork{scope: scope, journal: journal, acquire: acquire, ctx: ctx, cancel: cancel, phase: phase, cancelPhase: cancelPhase, gate: offline.NewWorkGate(ctx), retained: map[localKey]json.RawMessage{}, retainedLeases: map[localKey]func(){}, editors: map[string]*editorSlot{}}, nil
}

func (w *LocalWork) PrivateStorageScope() storage.Scope { return w.scope }

func (w *LocalWork) Journal() *offline.Store { return w.journal }

func (w *LocalWork) Owner() string { return w.scope.Subject + "/" + w.scope.Tenant }

func (w *LocalWork) Context() (context.Context, uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.phase, w.generation
}

func (w *LocalWork) Start() (context.Context, func(), error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gate.Start(w.phase)
}

func (w *LocalWork) OnResume(fn func()) { w.mu.Lock(); defer w.mu.Unlock(); w.resume = fn }

// Write is invoked only by admitted UI work. Failed durable writes stay owned in
// memory; Drain must persist them before releasing any registration or device.
func (w *LocalWork) Write(ctx context.Context, namespace, id string, value any) error {
	if (namespace != EspacioCapturas && namespace != EspacioBorradores) || id == "" {
		return errors.New("notes: invalid local work namespace")
	}
	var raw json.RawMessage
	if value != nil {
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		raw = b
	}
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return errors.Join(ErrLocalAdmission, offline.ErrWorkClosed)
	}
	w.writes.Lock()
	defer w.writes.Unlock()
	key := localKey{namespace: namespace, id: id}
	release, err := w.acquire(ctx)
	if err == nil && release == nil {
		err = offline.ErrWorkLease
	}
	if err != nil {
		return errors.Join(ErrLocalAdmission, err)
	}
	if err := w.write(ctx, key, raw); err != nil {
		if namespace == EspacioBorradores {
			w.retainedMu.Lock()
			w.retained[key] = raw
			if w.retainedLeases[key] != nil {
				release()
			} else {
				w.retainedLeases[key] = release
			}
			w.retainedMu.Unlock()
		} else {
			release()
		}
		return err
	}
	release()
	w.retainedMu.Lock()
	delete(w.retained, key)
	w.releaseRetained(key)
	w.retainedMu.Unlock()
	return nil
}

func (w *LocalWork) write(ctx context.Context, key localKey, raw json.RawMessage) error {
	return w.journal.Exclusive(ctx, w.Owner(), key.namespace, key.id, func() error {
		if raw == nil {
			return w.journal.Delete(w.Owner(), key.namespace, key.id)
		}
		return w.journal.Write(w.Owner(), key.namespace, key.id, raw)
	})
}

func (w *LocalWork) flush(ctx context.Context) error {
	w.writes.Lock()
	defer w.writes.Unlock()
	w.retainedMu.Lock()
	defer w.retainedMu.Unlock()
	for key, raw := range w.retained {
		if err := w.write(ctx, key, raw); err != nil {
			return err
		}
		delete(w.retained, key)
		w.releaseRetained(key)
	}
	return nil
}

// A failed accepted write keeps its existing native dirty lease visible to
// other apps until the exact retained source is durable. No new lease is taken
// while draining, and the lease never surrounds normal UI or network work.
func (w *LocalWork) releaseRetained(key localKey) {
	if release := w.retainedLeases[key]; release != nil {
		release()
		delete(w.retainedLeases, key)
	}
}

func (w *LocalWork) Pending(ctx context.Context) (offline.PendingWork, error) {
	result := offline.PendingWork{Scope: w.scope}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// Serialize inventory with the RAM → durable handoff. Otherwise a writer
	// could finish between List and the RAM scan and make accepted work invisible.
	w.writes.Lock()
	defer w.writes.Unlock()
	ids := map[localKey]struct{}{}
	for _, namespace := range []string{EspacioCapturas, EspacioBorradores} {
		records, err := w.journal.List(w.Owner(), namespace)
		if err != nil {
			return result, err
		}
		for _, record := range records {
			ids[localKey{namespace, record.ID}] = struct{}{}
		}
	}
	w.retainedMu.Lock()
	for key := range w.retained {
		ids[key] = struct{}{}
	}
	w.retainedMu.Unlock()
	w.queueMu.Lock()
	for id := range w.editors {
		ids[localKey{EspacioBorradores, id}] = struct{}{}
	}
	w.queueMu.Unlock()
	result.Count = len(ids)
	return result, ctx.Err()
}

func (w *LocalWork) Drain(ctx context.Context) error {
	w.mu.Lock()
	w.paused = true
	w.generation++
	w.cancelPhase()
	w.mu.Unlock()
	if err := w.gate.Drain(ctx); err != nil {
		return err
	}
	if err := w.flush(ctx); err != nil {
		return err
	}
	return w.flushEditors(ctx)
}

func (w *LocalWork) Resume() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return offline.ErrWorkClosed
	}
	if err := w.ctx.Err(); err != nil {
		w.mu.Unlock()
		return err
	}
	if !w.paused {
		w.mu.Unlock()
		return nil
	}
	if err := w.gate.Resume(); err != nil {
		w.mu.Unlock()
		return err
	}
	w.phase, w.cancelPhase = context.WithCancel(w.ctx)
	w.generation++
	w.paused = false
	fn := w.resume
	w.mu.Unlock()
	if fn != nil {
		fn()
	}
	return nil
}

func (w *LocalWork) Stop() { w.mu.Lock(); w.closed = true; w.cancelPhase(); w.cancel(); w.mu.Unlock() }

func (w *LocalWork) Close() { w.Stop(); w.gate.Close() }
