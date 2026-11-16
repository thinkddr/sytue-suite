// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

// IngressExecutor adapts the existing MIME ingest WAL. It adds no cipher,
// publication protocol or second copy of the mailbox repository. The shared
// worker supplies authenticated decrypted input and a pinned SDK context.
type IngressExecutor struct{ Repository *Repository }

func (e IngressExecutor) validate(claim ingress.Claim, metadata []byte, size int64) (ingestMessage, error) {
	var message ingestMessage
	if e.Repository == nil || claim.Validate(e.Repository.cfg.Scope, e.Repository.cfg.MigrationState.Epoch) != nil {
		return message, ingress.ErrContract
	}
	return e.validateTask(claim.Task, metadata, size)
}

func (e IngressExecutor) validateTask(task ingress.Task, metadata []byte, size int64) (ingestMessage, error) {
	var message ingestMessage
	if e.Repository == nil || task.Validate(e.Repository.cfg.Scope, e.Repository.cfg.MigrationState.Epoch) != nil || task.Kind != "mail.receive" || json.Unmarshal(metadata, &message) != nil || message.ID == "" || message.MailboxID == "" || message.Size != size {
		return message, ingress.ErrContract
	}
	return message, nil
}

func (e IngressExecutor) Reconcile(ctx context.Context, claim ingress.Claim, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	message, err := e.validate(claim, metadata, size)
	if err != nil {
		return nil, err
	}
	return e.reconcile(ctx, claim.Task, claim.Target, message, metadata, source, size)
}

func (e IngressExecutor) ReconcileHandoff(ctx context.Context, task ingress.Task, previous ingress.Assignment, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	message, err := e.validateTask(task, metadata, size)
	if err != nil {
		return nil, err
	}
	if !previous.Valid() {
		return nil, ingress.ErrClaim
	}
	return e.reconcile(ctx, task, previous.Target, message, metadata, source, size)
}

func (e IngressExecutor) reconcile(ctx context.Context, task ingress.Task, target storage.PublicationTarget, message ingestMessage, metadata []byte, source io.ReaderAt, size int64) (*storage.Reference, error) {
	checksum, err := sourceDigest(ctx, source, size, e.Repository.messageLimit())
	if err != nil {
		return nil, err
	}
	var accepted *storage.Reference
	err = e.Repository.run(ctx, true, func(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) error {
		raw, err := e.Repository.document(ctx, snapshot, "message", message.ID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		defer clear(raw)
		_, document, err := decodeMessage(raw, message.ID)
		defer clear(document.Envelope)
		if err != nil {
			return err
		}
		if document.Operation != task.ID {
			return ErrConflict
		}
		done, err := e.Repository.alreadyStored(ctx, snapshot, message.ID, task.ID, metadata, checksum)
		if err != nil {
			return err
		}
		if !done {
			return ingress.ErrReceipt
		}
		versions := heads(snapshot, objectID("message", message.ID))
		if len(versions) != 1 {
			return ErrConflict
		}
		ref := versions[0].Reference
		if ref.RoomID != target.RoomID {
			return ingress.ErrReceipt
		}
		accepted = &ref
		return nil
	})
	return accepted, err
}

func (e IngressExecutor) Execute(ctx context.Context, claim ingress.Claim, metadata []byte, source io.ReaderAt, size int64) (storage.Reference, error) {
	if _, err := e.validate(claim, metadata, size); err != nil {
		return storage.Reference{}, err
	}
	binding, err := e.Repository.cfg.Crypto.PrepareStorage(ctx, e.Repository.cfg.Scope)
	if err != nil {
		return storage.Reference{}, err
	}
	if !binding.Valid() || binding.Target != claim.Target {
		return storage.Reference{}, ingress.ErrClaim
	}
	if _, err := e.Reconcile(ctx, claim, metadata, source, size); err != nil {
		return storage.Reference{}, err
	}
	if err := e.Repository.StoreMessage(ctx, claim.Task.ID, metadata, source, size); err != nil {
		return storage.Reference{}, err
	}
	accepted, err := e.Reconcile(ctx, claim, metadata, source, size)
	if err != nil {
		return storage.Reference{}, err
	}
	if accepted == nil {
		return storage.Reference{}, ingress.ErrReceipt
	}
	return *accepted, nil
}
