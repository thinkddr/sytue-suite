// SPDX-License-Identifier: Apache-2.0
package drive

import (
	"context"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// observation owns one complete, immutable SDK catalog observation. It is not
// a distributed fence, publication authorization, or a deletion proof.
type observation struct {
	snapshot storage.AtomicCatalogSnapshot
	refs     int
	retired  bool
}

type observationKey struct{ repository *Repository }

// admittedObservations holds observations only for the original work lease.
// Its done callback runs after asynchronous stream workers have stopped.
type admittedObservations struct {
	mu     sync.Mutex
	repo   *Repository
	views  map[*observation]int
	closed bool
}

type observedContext struct {
	admission *admittedObservations
	view      *observation
}

//nolint:nilnil // nil view with nil error means no observation published yet; callers treat it as empty, not as a failure
func (a *admittedObservations) holdCurrent() (*observation, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, ErrClosed
	}
	a.repo.mu.Lock()
	defer a.repo.mu.Unlock()
	view := a.repo.observation
	if a.repo.closed {
		return nil, ErrClosed
	}
	if view == nil {
		return nil, nil
	}
	view.refs++
	a.views[view]++
	return view, nil
}

func (a *admittedObservations) release(view *observation) {
	if view == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.views[view] == 0 {
		return
	}
	a.repo.mu.Lock()
	defer a.repo.mu.Unlock()
	a.views[view]--
	view.refs--
	if a.views[view] == 0 {
		delete(a.views, view)
	}
	if view.refs == 0 && view.retired {
		clearObservation(view)
	}
}

func (a *admittedObservations) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	a.closed = true
	a.repo.mu.Lock()
	defer a.repo.mu.Unlock()
	for view, count := range a.views {
		view.refs -= count
		if view.refs == 0 && view.retired {
			clearObservation(view)
		}
	}
	a.views = nil
}

func (r *Repository) temporaryObservation(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	value, ok := ctx.Value(observationKey{r}).(observedContext)
	if !ok || value.admission == nil {
		return nil, nil, ErrSource
	}
	view, err := value.admission.holdCurrent()
	if err != nil {
		return nil, nil, err
	}
	var once sync.Once
	release := func() { once.Do(func() { value.admission.release(view) }) }
	return context.WithValue(ctx, observationKey{r}, observedContext{admission: value.admission, view: view}), release, nil
}

func clearObservation(view *observation) {
	for _, entry := range view.snapshot.Entries {
		clear(entry.Record.Payload)
	}
	for _, entry := range view.snapshot.Pages {
		clear(entry.Record.Payload)
	}
	for _, version := range view.snapshot.History {
		clear(version.Commit.Metadata)
	}
	for _, version := range view.snapshot.Heads {
		clear(version.Commit.Metadata)
	}
	view.snapshot = storage.AtomicCatalogSnapshot{}
}

func (r *Repository) currentObservation(ctx context.Context) (*observation, error) {
	value, ok := ctx.Value(observationKey{r}).(observedContext)
	if !ok || value.admission == nil {
		return nil, ErrSource
	}
	if value.view == nil {
		return nil, ErrNotReady
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value.admission.mu.Lock()
	active := !value.admission.closed && value.admission.views[value.view] > 0
	value.admission.mu.Unlock()
	if !active {
		return nil, ErrClosed
	}
	return value.view, nil
}

func (r *Repository) checkObservation(ctx context.Context, view *observation) error {
	if view == nil {
		return nil
	}
	current, err := r.currentObservation(ctx)
	if err != nil {
		return err
	}
	if current != view {
		return ErrSource
	}
	binding, err := r.cfg.Crypto.StorageBindings(ctx)
	if err != nil {
		return err
	}
	if !view.snapshot.Binding.Same(binding) {
		return ErrSource
	}
	return ctx.Err()
}

// observeCurrent starts a distinct immutable view without replacing any view
// already held by this admission, so an in-flight reader cannot mix snapshots.
func (r *Repository) observeCurrent(ctx context.Context) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := ctx.Value(observationKey{r}).(observedContext)
	if !ok || value.admission == nil {
		return nil, ErrSource
	}
	view, err := value.admission.holdCurrent()
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, observationKey{r}, observedContext{admission: value.admission, view: view}), nil
}
