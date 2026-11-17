// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"encoding/json"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type BinaryAudit struct {
	Scope   storage.Scope
	Objects int64
	Bytes   int64
}

// AuditBinaries fully downloads and verifies every retained binary revision in
// a stable observed catalog, including old seeds. It is the explicit availability
// check for backup/retirement workflows, not authorization to delete a source.
// No original, server index, backup, or shared ciphertext is removed here.
func (r *Repository) AuditBinaries(parent context.Context) (BinaryAudit, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return BinaryAudit{}, err
	}
	defer done()
	result := BinaryAudit{Scope: r.cfg.Scope}
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		if err := r.requireReady(); err != nil {
			return err
		}
		before, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return err
		}
		if err := singleAuditHeads(before.Heads); err != nil {
			return err
		}
		result, err = r.auditBinaryEntries(ctx, before)
		if err != nil {
			return err
		}
		after, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
		if err != nil {
			return err
		}
		if !storage.SameAtomicCatalog(before, after) {
			return ErrConflict
		}
		return ctx.Err()
	})
	if err != nil {
		return BinaryAudit{}, err
	}
	return result, nil
}

func (r *Repository) auditBinaryEntries(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) (BinaryAudit, error) {
	result := BinaryAudit{Scope: r.cfg.Scope}
	for _, version := range snapshot.History {
		var header struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(version.Commit.Metadata, &header) != nil {
			return BinaryAudit{}, ErrSource
		}
		if header.Kind == r.cfg.ReceiptKind {
			continue
		}
		value, err := r.description(version)
		if err != nil {
			return BinaryAudit{}, err
		}
		if !value.Binary {
			continue
		}
		if _, err := r.readBinary(ctx, version, io.Discard); err != nil {
			return BinaryAudit{}, err
		}
		if value.Size > (1<<63-1)-result.Bytes {
			return BinaryAudit{}, ErrSource
		}
		result.Objects++
		result.Bytes += value.Size
	}
	return result, nil
}

func singleAuditHeads(heads []storage.ObjectVersion) error {
	seen := map[string]bool{}
	for _, head := range heads {
		if seen[head.ObjectID] {
			return ErrConflict
		}
		seen[head.ObjectID] = true
	}
	return nil
}
