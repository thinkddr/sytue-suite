// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// contentReference makes a bounded reference copy, not a transfer or a promise
// about current provider availability. Both private manifest commitments must
// agree. Shared ciphertext remains reachable through every surviving copy and
// version, even after the source item's logical deletion.
func (r *Repository) contentReference(ctx context.Context, view *observation, item summary, pointer bodyPointer) (bodyPointer, string, error) {
	pointer, _, sum, err := r.contentCommitment(ctx, view, item, pointer)
	return pointer, sum, err
}

func (r *Repository) contentCommitment(ctx context.Context, view *observation, item summary, pointer bodyPointer) (bodyPointer, storage.ObjectVersion, string, error) {
	source, err := pointer.source(item, "binary")
	if err != nil {
		return bodyPointer{}, storage.ObjectVersion{}, "", err
	}
	description, version, err := r.bodyVersion(ctx, view, pointer, item, "binary")
	if err != nil {
		return bodyPointer{}, storage.ObjectVersion{}, "", err
	}
	content := version.Commit.Content
	details, sum, err := r.cfg.Crypto.StorageFileCommitment(ctx, content.Reference, pointer.ObjectID, content.RevisionID)
	if err != nil {
		return bodyPointer{}, storage.ObjectVersion{}, "", err
	}
	if details.Validate(r.cfg.Scope) != nil || details.ObjectID != pointer.ObjectID || details.RevisionID != content.RevisionID || details.SizeBytes != item.SizeBytes || sum != description.SHA256 {
		return bodyPointer{}, storage.ObjectVersion{}, "", ErrSource
	}
	if err := r.checkObservation(ctx, view); err != nil {
		return bodyPointer{}, storage.ObjectVersion{}, "", err
	}
	pointer.Origin = bodyOrigin{Kind: source.SourceKind, ID: source.SourceID, Epoch: source.Epoch}
	return pointer, version, sum, nil
}

func validCopyOrigin(reference storage.Reference, object string) bool {
	return reference.Valid() && storage.Identifier(object)
}
