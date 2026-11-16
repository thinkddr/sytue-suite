// SPDX-License-Identifier: Apache-2.0
package records

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// SourceAudit is a point-in-time observation, not permission to retire a source.
// Its result does not guarantee that the source or provider remains available.
type SourceAudit struct {
	Scope                    storage.Scope
	Epoch                    string
	Rows, Blobs, SourceBytes int64
	Private                  BinaryAudit
}

// AuditSource verifies the frozen retained source against its existing encrypted
// receipt and reads every private binary revision. It does not modify or delete
// the source, publish a receipt, or create a deletion capability.
func (r *Repository) AuditSource(parent context.Context) (SourceAudit, error) {
	ctx, done, err := r.Start(parent)
	if err != nil {
		return SourceAudit{}, err
	}
	defer done()
	var result SourceAudit
	err = r.cfg.Journal.Exclusive(ctx, r.owner(), r.namespace(), "catalog", func() error {
		var err error
		result, err = r.auditSourceLocked(ctx)
		return err
	})
	if err != nil {
		return SourceAudit{}, err
	}
	return result, nil
}

func (r *Repository) auditSourceLocked(ctx context.Context) (SourceAudit, error) {
	state, before, candidate, receipt, err := r.prepareSourceAudit(ctx)
	if err != nil {
		return SourceAudit{}, err
	}
	result, err := candidate.auditRetainedSource(ctx, state, receipt)
	if err != nil {
		return SourceAudit{}, err
	}
	result.Private, err = candidate.auditBinaryEntries(ctx, before)
	if err != nil {
		return SourceAudit{}, err
	}
	if err := r.finishSourceAudit(ctx, state, before); err != nil {
		return SourceAudit{}, err
	}
	return result, nil
}

func (r *Repository) prepareSourceAudit(ctx context.Context) (SnapshotState, storage.AtomicCatalogSnapshot, *Repository, snapshotReceipt, error) {
	var state SnapshotState
	var before storage.AtomicCatalogSnapshot
	var receipt snapshotReceipt
	if err := r.requireReady(); err != nil {
		return state, before, nil, receipt, err
	}
	state, err := r.cfg.Source.State(ctx, r.cfg.Scope)
	if err != nil {
		return state, before, nil, receipt, err
	}
	if r.validateState(state) != nil || state.Mode != "frozen" {
		return state, before, nil, receipt, ErrSource
	}
	before, err = storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return state, before, nil, receipt, err
	}
	if err := singleAuditHeads(before.Heads); err != nil {
		return state, before, nil, receipt, err
	}
	r.mu.Lock()
	current := r.binding
	r.mu.Unlock()
	if !current.Same(before.Binding) {
		return state, before, nil, receipt, ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(before.Binding.Target); err != nil {
		return state, before, nil, receipt, err
	}
	candidate := &Repository{cfg: r.cfg, gate: r.gate}
	if err := candidate.verifyCompleted(ctx, state); err != nil {
		return state, before, nil, receipt, err
	}
	if !storage.SameAtomicCatalog(before, candidate.catalog) {
		return state, before, nil, receipt, ErrConflict
	}
	receipt, err = candidate.completedReceipt(ctx, state)
	if err != nil {
		return state, before, nil, receipt, err
	}
	return state, before, candidate, receipt, nil
}

func (r *Repository) finishSourceAudit(ctx context.Context, state SnapshotState, before storage.AtomicCatalogSnapshot) error {
	after, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	if !storage.SameAtomicCatalog(before, after) {
		return ErrConflict
	}
	last, err := r.cfg.Source.State(ctx, r.cfg.Scope)
	if err != nil {
		return err
	}
	if r.validateState(last) != nil || !sameSourceState(state, last) {
		return ErrSource
	}
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(after.Binding.Target); err != nil {
		return err
	}
	return ctx.Err()
}

func sameSourceState(a, b SnapshotState) bool {
	if a.StartedAt == nil || b.StartedAt == nil || !a.StartedAt.Equal(*b.StartedAt) {
		return false
	}
	a.StartedAt, b.StartedAt = nil, nil
	return a == b
}

func (r *Repository) auditRetainedSource(ctx context.Context, state SnapshotState, receipt snapshotReceipt) (SourceAudit, error) {
	out := SourceAudit{Scope: r.cfg.Scope, Epoch: state.Epoch}
	inventory := r.newSnapshotInventory(state.Epoch)
	cursor := storage.LegacySourceCursor{}
	for {
		page, err := r.cfg.Source.Page(ctx, r.cfg.Scope, state.Epoch, cursor)
		if err != nil {
			return SourceAudit{}, err
		}
		if page.Validate(r.cfg.Scope, state.Epoch, cursor) != nil {
			return SourceAudit{}, ErrSource
		}
		for _, row := range page.Rows {
			if err := r.auditSourceRow(ctx, state, row, &out); err != nil {
				return SourceAudit{}, err
			}
			_, _ = inventory.Write([]byte(row.Kind + "\x00" + row.ID + "\x00" + row.SHA256 + "\x00"))
			cursor = storage.LegacySourceCursor{Kind: row.Kind, ID: row.ID}
		}
		if page.Next == nil {
			break
		}
	}
	if out.Rows != state.Rows || out.Blobs != state.Blobs || out.Rows != receipt.Rows || out.Blobs != receipt.Blobs || hex.EncodeToString(inventory.Sum(nil)) != receipt.Inventory {
		return SourceAudit{}, ErrSource
	}
	return out, nil
}

func (r *Repository) auditSourceRow(ctx context.Context, state SnapshotState, row SnapshotRow, out *SourceAudit) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	binary := r.binaryKind(row.Kind)
	if out.Rows >= state.Rows || binary && out.Blobs >= state.Blobs || row.Size > (1<<63-1)-out.SourceBytes {
		return ErrSource
	}
	if err := r.verifySourceBytes(ctx, state.Epoch, row); err != nil {
		return err
	}
	out.SourceBytes += row.Size
	out.Rows++
	if binary {
		out.Blobs++
	}
	if out.Rows > state.Rows || out.Blobs > state.Blobs {
		return ErrSource
	}
	return ctx.Err()
}

func (r *Repository) verifySourceBytes(ctx context.Context, epoch string, row SnapshotRow) error {
	body, err := r.cfg.Source.Open(ctx, r.cfg.Scope, epoch, row)
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		return err
	}
	if body == nil {
		return ErrSource
	}
	hash := sha256.New()
	var once sync.Once
	var closeErr error
	closeBody := func() { once.Do(func() { closeErr = body.Close() }) }
	stop := context.AfterFunc(ctx, closeBody)
	n, readErr := io.Copy(hash, io.LimitReader(body, row.Size+1))
	stop()
	closeBody()
	if err := ctx.Err(); err != nil {
		return err
	}
	if readErr != nil || closeErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if n != row.Size || hex.EncodeToString(hash.Sum(nil)) != row.SHA256 {
		return ErrSource
	}
	return nil
}
