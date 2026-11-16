// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// FilePin fixes an authenticated file/version index and its complete content
// commitment. It contains no key or credential, and belongs only in the calling
// device or an encrypted application record. It grants no access by itself.
type FilePin struct {
	Scope           storage.Scope     `json:"scope"`
	SourceKind      string            `json:"source_kind"`
	SourceID        string            `json:"source_id"`
	Epoch           string            `json:"epoch,omitempty"`
	IndexRef        storage.Reference `json:"index_ref"`
	ObjectID        string            `json:"object_id"`
	ContentRevision string            `json:"content_revision"`
	SizeBytes       int64             `json:"size_bytes"`
	SHA256          string            `json:"sha256"`
}

func (pin FilePin) Valid() bool {
	return pin.Scope.Valid() && pin.Scope.Product == "drive" && binaryKind(pin.SourceKind) && storage.SourceCursorValid(storage.LegacySourceCursor{Kind: pin.SourceKind, ID: pin.SourceID}) && (pin.Epoch == "" || storage.MigrationEpoch(pin.Epoch)) && pin.IndexRef.Valid() && storage.Identifier(pin.ObjectID) && storage.Identifier(pin.ContentRevision) && pin.SizeBytes >= 0 && pin.SizeBytes <= storage.MaxFileBytes && validHash(pin.SHA256)
}

// PinRetainedOriginal selects the unique current index once, including trash
// and retained version objects. A later replacement or metadata edit does not
// redirect the returned pin. Conflicts must be resolved before selecting one.
func (r *Repository) PinRetainedOriginal(parent context.Context, kind, id string) (FilePin, error) {
	if !binaryKind(kind) || !storage.SourceCursorValid(storage.LegacySourceCursor{Kind: kind, ID: id}) {
		return FilePin{}, ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return FilePin{}, err
	}
	defer done()
	index, err := r.version(ctx, kind, id)
	if err != nil {
		return FilePin{}, err
	}
	pin, _, _, _, err := r.originalPin(ctx, index)
	return pin, err
}

func (r *Repository) originalPin(ctx context.Context, index storage.ObjectVersion) (FilePin, summary, bodyPointer, *observation, error) {
	item, data, raw, err := r.readEntry(ctx, index)
	defer clear(raw)
	if err != nil {
		return FilePin{}, summary{}, bodyPointer{}, nil, err
	}
	if !binaryKind(item.SourceKind) || data.Binary == nil {
		return FilePin{}, summary{}, bodyPointer{}, nil, ErrSource
	}
	pointer, body, sum, err := r.contentCommitment(ctx, data.observation, item, *data.Binary)
	if err != nil {
		return FilePin{}, summary{}, bodyPointer{}, nil, err
	}
	pin := FilePin{Scope: r.cfg.Scope, SourceKind: item.SourceKind, SourceID: item.SourceID, Epoch: item.Epoch, IndexRef: index.Reference, ObjectID: pointer.ObjectID, ContentRevision: body.Commit.Content.RevisionID, SizeBytes: item.SizeBytes, SHA256: sum}
	if !pin.Valid() {
		return FilePin{}, summary{}, bodyPointer{}, nil, ErrSource
	}
	return pin, item, pointer, data.observation, nil
}

// WritePinnedOriginal reauthorizes the originating repository and opens exactly
// the authenticated index, even after its head changes. It never reads SQL or a
// legacy HTTP body. Output is incomplete until the full digest is verified and
// this method returns successfully; availability is checked by the actual read.
func (r *Repository) WritePinnedOriginal(parent context.Context, pin FilePin, output io.Writer) error {
	if output == nil || !pin.Valid() || pin.Scope != r.cfg.Scope {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	item, pointer, view, err := r.validatePinnedOriginal(ctx, pin)
	if err != nil {
		return err
	}
	return r.readBodyWithPin(ctx, view, pointer, item, "binary", output, &pin)
}

// ValidatePinnedOriginal reauthorizes the exact retained index and complete
// authenticated content commitment without choosing a current head or reading
// media. It does not assert provider availability; import and retirement still
// require WritePinnedOriginal to verify every byte.
func (r *Repository) ValidatePinnedOriginal(parent context.Context, pin FilePin) error {
	if !pin.Valid() || pin.Scope != r.cfg.Scope {
		return ErrSource
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	_, _, _, err = r.validatePinnedOriginal(ctx, pin)
	return err
}

func (r *Repository) validatePinnedOriginal(ctx context.Context, pin FilePin) (summary, bodyPointer, *observation, error) {
	index, err := r.pinnedIndex(ctx, pin)
	if err != nil {
		return summary{}, bodyPointer{}, nil, err
	}
	actual, item, pointer, view, err := r.originalPin(ctx, index)
	if err != nil {
		return summary{}, bodyPointer{}, nil, err
	}
	if actual != pin {
		return summary{}, bodyPointer{}, nil, ErrSource
	}
	return item, pointer, view, nil
}

func (r *Repository) pinnedIndex(ctx context.Context, pin FilePin) (storage.ObjectVersion, error) {
	view, err := r.currentObservation(ctx)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return storage.ObjectVersion{}, ErrClosed
	}
	if !r.ready {
		return storage.ObjectVersion{}, ErrNotReady
	}
	if _, found := view.snapshot.Entries[pin.IndexRef]; !found {
		return storage.ObjectVersion{}, ErrNotFound
	}
	index, found, err := storage.AcceptedAtomicObject(view.snapshot, r.cfg.Scope, objectID(pin.SourceKind, pin.SourceID), "", &pin.IndexRef)
	if err != nil {
		return storage.ObjectVersion{}, ErrSource
	}
	if !found {
		return storage.ObjectVersion{}, ErrNotFound
	}
	if index.Reference != pin.IndexRef || index.ObjectID != objectID(pin.SourceKind, pin.SourceID) || index.Commit.State != "live" && index.Commit.State != "trash" {
		return storage.ObjectVersion{}, ErrSource
	}
	return index, nil
}
