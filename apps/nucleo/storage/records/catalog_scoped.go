// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

// ErrCatalogExpired identifies a projection catalog used after its callback.
var ErrCatalogExpired = errors.New("records: projection catalog has expired")

type scopedCatalog struct {
	r       *Repository
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	entries []Entry
	byKey   map[scopedEntryKey]Entry
	buffers [][]byte
}

type scopedEntryKey struct{ kind, id string }

func copyCatalogEntry(entry Entry) Entry {
	entry.Version = cloneVersion(entry.Version)
	return entry
}

func (v *scopedCatalog) current() error {
	if v.closed {
		return ErrCatalogExpired
	}
	if err := v.ctx.Err(); err != nil {
		return err
	}
	return v.r.requireReady()
}

func (v *scopedCatalog) Entries() ([]Entry, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.current(); err != nil {
		return nil, err
	}
	entries := make([]Entry, len(v.entries))
	for i, entry := range v.entries {
		entries[i] = copyCatalogEntry(entry)
	}
	return entries, nil
}

func (v *scopedCatalog) ReadJSON(parent context.Context, entry Entry) ([]byte, error) {
	if parent == nil {
		return nil, ErrSource
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.current(); err != nil {
		return nil, err
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	selected, ok := v.byKey[scopedEntryKey{entry.Kind, entry.ID}]
	if !ok || !reflect.DeepEqual(selected, entry) {
		return nil, ErrSource
	}
	// Preserve the original admitted context and its session binding; the
	// caller's context can narrow the lifetime but cannot extend it.
	ctx, cancel := context.WithCancel(v.ctx)
	stop := context.AfterFunc(parent, cancel)
	defer func() { stop(); cancel() }()
	if err := parent.Err(); err != nil {
		return nil, err
	}
	raw, err := v.r.readJSON(ctx, selected.Version) //nolint:contextcheck // ctx derives from the view lifetime v.ctx and is cancelled by parent via AfterFunc
	if err == nil {
		err = parent.Err()
	}
	if err == nil {
		err = v.current()
	}
	if err != nil {
		clear(raw)
		return nil, err
	}
	v.buffers = append(v.buffers, raw)
	return raw, nil
}

func (v *scopedCatalog) close() {
	// Interrupt an escaped read before waiting for it. A provider that ignores
	// cancellation can delay shutdown, just like any other admitted work.
	v.cancel()
	v.mu.Lock()
	defer v.mu.Unlock()
	v.closed = true
	for _, raw := range v.buffers {
		clear(raw)
	}
	v.buffers = nil
	for i := range v.entries {
		clear(v.entries[i].Version.Commit.Metadata)
	}
	v.entries = nil
	v.byKey = nil
}

// ProjectCatalog runs a read-only projection under one admitted work lease.
// Entries and JSON are tied to one captured catalog; the Catalog becomes
// unusable and its returned JSON buffers are cleared when project returns.
// A callback must not use those buffers concurrently with or after its return.
// The callback must not call back into the repository, synchronously close it,
// or expose a result until this method returns successfully. It must respect
// ctx cancellation; Close/Drain must wait if it or a provider ignores
// cancellation and does not return. Copies and strings made by the callback
// remain the callback's responsibility.
func (r *Repository) ProjectCatalog(parent context.Context, project func(context.Context, Catalog) error) error {
	if parent == nil || project == nil {
		return ErrSource
	}
	ctx, done, err := r.Start(parent)
	if err != nil {
		return err
	}
	defer done()
	entries, err := (catalogView{r: r, requireReady: true}).Entries()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	life, cancel := context.WithCancel(ctx)
	view := &scopedCatalog{r: r, ctx: life, cancel: cancel, entries: entries, byKey: make(map[scopedEntryKey]Entry, len(entries))}
	for _, entry := range entries {
		view.byKey[scopedEntryKey{entry.Kind, entry.ID}] = entry
	}
	defer view.close()
	err = project(life, view)
	view.close()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.requireReady()
}
