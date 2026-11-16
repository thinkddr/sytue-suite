// SPDX-License-Identifier: Apache-2.0
package records

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// Entry describes an authenticated current domain revision. Version contains no
// plaintext body; ReadJSON verifies that body through the maintained adapter.
type Entry struct {
	Kind    string
	ID      string
	State   string
	Binary  bool
	Version storage.ObjectVersion
}

// Catalog lets a domain validate the whole graph before completion. It exposes
// no writes and works before the ordinary ready gate is opened.
type Catalog interface {
	Entries() ([]Entry, error)
	ReadJSON(context.Context, Entry) ([]byte, error)
}

// BinaryCatalog is optional so JSON-only domains and adapters keep their
// contract. The engine verifies the complete bytes before accepting a snapshot.
type BinaryCatalog interface {
	Catalog
	BinaryInfo(context.Context, Entry) (SnapshotRow, error)
}

// BinaryContentCatalog lets a domain verify bounded binary structures such as
// playlist references during the engine's final validation. Public views keep
// their ordinary readiness and session-lease gates.
type BinaryContentCatalog interface {
	BinaryCatalog
	ReadBinary(context.Context, Entry, io.Writer) (SnapshotRow, error)
}

type catalogView struct {
	r            *Repository
	requireReady bool
}

func (v catalogView) Entries() ([]Entry, error) {
	v.r.mu.Lock()
	defer v.r.mu.Unlock()
	if v.r.closed {
		return nil, ErrClosed
	}
	if v.requireReady && !v.r.ready {
		return nil, ErrNotReady
	}
	entries := []Entry{}
	for _, heads := range v.r.heads {
		if len(heads) != 1 {
			return nil, ErrConflict
		}
		version := heads[0]
		var header struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(version.Commit.Metadata, &header) != nil {
			return nil, ErrSource
		}
		if header.Kind == v.r.cfg.ReceiptKind {
			continue
		}
		summary, err := v.r.description(version)
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Kind: summary.Entity, ID: summary.ID, State: version.Commit.State, Binary: summary.Binary, Version: cloneVersion(version)})
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.Kind < b.Kind {
			return -1
		}
		if a.Kind > b.Kind {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return entries, nil
}

func (v catalogView) ReadJSON(ctx context.Context, entry Entry) ([]byte, error) {
	if v.requireReady {
		if err := v.r.requireReady(); err != nil {
			return nil, err
		}
	}
	summary, err := v.r.description(entry.Version)
	if err != nil {
		return nil, err
	}
	if summary.Entity != entry.Kind || summary.ID != entry.ID || entry.State != entry.Version.Commit.State || entry.Binary != summary.Binary {
		return nil, ErrSource
	}
	if v.requireReady {
		return v.r.ReadJSON(ctx, entry.Version)
	}
	return v.r.readJSON(ctx, entry.Version)
}

func (v catalogView) BinaryInfo(ctx context.Context, entry Entry) (SnapshotRow, error) {
	return v.readBinaryEntry(ctx, entry, v.r.binaryInfo)
}

func (v catalogView) ReadBinary(ctx context.Context, entry Entry, out io.Writer) (SnapshotRow, error) {
	return v.readBinaryEntry(ctx, entry, func(ctx context.Context, version storage.ObjectVersion) (SnapshotRow, error) {
		return v.r.readBinary(ctx, version, out)
	})
}

func (v catalogView) readBinaryEntry(ctx context.Context, entry Entry, read func(context.Context, storage.ObjectVersion) (SnapshotRow, error)) (SnapshotRow, error) {
	if v.requireReady {
		work, done, err := v.r.Start(ctx)
		if err != nil {
			return SnapshotRow{}, err
		}
		defer done()
		ctx = work
		if err := v.r.requireReady(); err != nil {
			return SnapshotRow{}, err
		}
	}
	version, err := v.binaryEntryVersion(entry)
	if err != nil {
		return SnapshotRow{}, err
	}
	return read(ctx, version)
}

func (v catalogView) binaryEntryVersion(entry Entry) (storage.ObjectVersion, error) {
	summary, err := v.r.description(entry.Version)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	if !entry.Binary || !summary.Binary || summary.Entity != entry.Kind || summary.ID != entry.ID || entry.State != entry.Version.Commit.State {
		return storage.ObjectVersion{}, ErrSource
	}
	v.r.mu.Lock()
	defer v.r.mu.Unlock()
	if v.r.closed {
		return storage.ObjectVersion{}, ErrClosed
	}
	heads := v.r.heads[entry.Version.ObjectID]
	if len(heads) != 1 {
		return storage.ObjectVersion{}, ErrConflict
	}
	wanted, _ := json.Marshal(heads[0])
	actual, _ := json.Marshal(entry.Version)
	if !bytes.Equal(wanted, actual) {
		return storage.ObjectVersion{}, ErrSource
	}
	return cloneVersion(heads[0]), nil
}

// Catalog exposes current authenticated identities only after complete import.
// The returned view keeps checking readiness when it is used later.
func (r *Repository) Catalog() (Catalog, error) {
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	return catalogView{r: r, requireReady: true}, nil
}

func (r *Repository) requireReady() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	if !r.ready {
		return ErrNotReady
	}
	return nil
}

// ReadJSON reads one selected authenticated revision on a ready repository.
func (r *Repository) ReadJSON(parent context.Context, version storage.ObjectVersion) ([]byte, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return nil, err
	}
	defer done()
	if err := r.requireReady(); err != nil {
		return nil, err
	}
	return r.readJSON(ctx, version)
}
