// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

type CopyPrivateFile func(context.Context, storage.PublicationTarget, string, storage.Reference, string, string) (storage.Reference, error)

type mailTransfer struct {
	source, target *Repository
	retained       Source
	lookup         transfer.Lookup
}

// MigratePrivateCatalog retains all authenticated revisions and exact original
// MIME/source archives. Message and receipt documents rewrite only SDK refs.
// Neither the ordinary repository nor its UI can use the historical identity.
func (r *Repository) MigratePrivateCatalog(parent context.Context, source *HistoricalSource, copyFile CopyPrivateFile) error {
	if source == nil || source.repository == nil || copyFile == nil {
		return ErrRecord
	}
	previous := source.repository
	if previous == r || previous.cfg.Scope != r.cfg.Scope || !sameSource(previous.cfg.MigrationState, r.cfg.MigrationState) {
		return ErrRecord
	}
	proof, ok := previous.cfg.Crypto.(frozenMailSource)
	if !ok {
		return ErrRecord
	}
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	ctx, sourceDone, err := previous.start(ctx)
	if err != nil {
		return err
	}
	defer sourceDone()
	previous.mu.Lock()
	defer previous.mu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || previous.closed {
		return ErrClosed
	}
	if r.ready {
		return ErrRecord
	}
	owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
	return r.cfg.Journal.Exclusive(ctx, owner, storage.IdentityJournalNamespace("mail"), "catalog", func() error {
		destination, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope)
		if err != nil {
			return err
		}
		frozen, err := proof.FrozenStorageBindings(ctx)
		if err != nil {
			return err
		}
		rules := mailTransfer{source: previous, target: r, retained: source.source}
		err = transfer.Run(ctx, transfer.Config{
			Scope: r.cfg.Scope, Owner: owner, Namespace: storage.IdentityJournalNamespace("mail"), ReceiptKind: kind, ReceiptType: "migration", Journal: r.cfg.Journal,
			Source: previous.cfg.Crypto, Target: r.cfg.Crypto, FrozenBindings: proof.FrozenStorageBindings, Rules: &rules,
			PutObject: r.cfg.Crypto.PutStorageObjectAt, PutRecord: r.cfg.Crypto.PutStorageRecordAt, CopyFile: copyFile,
			SourceFileCommitment: previous.cfg.Files.Commitment, TargetFileCommitment: r.cfg.Files.Commitment,
			VerifyFile: func(ctx context.Context, ref storage.Reference, object, revision string) error {
				_, err := r.cfg.Files.Download(ctx, ref, object, revision, io.Discard)
				return err
			},
			AcknowledgeFile: func(ctx context.Context, job string, ref storage.Reference) error {
				return r.cfg.Files.Acknowledge(ctx, job, ref)
			},
			PrepareTarget: func(ctx context.Context) error { return ctx.Err() },
			MarkReady: func(ctx context.Context) error {
				if err := rules.TargetComplete(ctx); err != nil {
					return err
				}
				r.ready = true
				return nil
			},
			ReadFileDocument: func(ctx context.Context, version storage.ObjectVersion, size int64) ([]byte, error) {
				if size > previous.messageLimit() {
					return nil, ErrRecord
				}
				return previous.readDocument(ctx, version)
			},
			PutFileDocument: func(ctx context.Context, target storage.PublicationTarget, job string, request storage.FileRequest, body io.ReaderAt) (storage.Reference, error) {
				return r.cfg.Files.Upload(ctx, job, target, request, body)
			},
		}, frozen, destination)
		if errors.Is(err, transfer.ErrSource) {
			return ErrRecord
		}
		return err
	})
}

func (t *mailTransfer) SourceComplete(ctx context.Context) error {
	if err := t.source.checkSourceState(ctx, t.retained); err != nil {
		return err
	}
	snapshot, err := storage.LoadAtomicCatalog(ctx, t.source.cfg.Crypto, t.source.cfg.Scope)
	if err != nil {
		return err
	}
	_, err = t.source.completedReceipt(ctx, snapshot)
	return err
}

func (t *mailTransfer) TargetComplete(ctx context.Context) error {
	if err := t.source.checkSourceState(ctx, t.retained); err != nil {
		return err
	}
	snapshot, err := storage.LoadAtomicCatalog(ctx, t.target.cfg.Crypto, t.target.cfg.Scope)
	if err != nil {
		return err
	}
	_, err = t.target.completedReceipt(ctx, snapshot)
	return err
}

func (t *mailTransfer) Classify(ctx context.Context, version storage.ObjectVersion, lookup transfer.Lookup) (transfer.Class, error) {
	t.lookup = lookup
	h, err := decodeHeader(version)
	if err != nil {
		return transfer.Class{}, err
	}
	switch h.Type {
	case "mime", "source":
		if version.Commit.State != "live" || version.Commit.Content == nil || version.Commit.Content.Kind != "file" {
			return transfer.Class{}, ErrRecord
		}
		_, _, err := t.source.cfg.Files.Commitment(ctx, version.Commit.Content.Reference, version.ObjectID, version.Commit.Content.RevisionID)
		return transfer.Class{Order: 0}, err
	case "mailbox", "label", "signature", "templates", "filter", "vacation", "message", "migration":
		if version.Commit.State == "deleted" && version.Commit.Content == nil && h.Type != "migration" {
			return transfer.Class{Order: 1}, nil
		}
		raw, err := t.source.readDocument(ctx, version)
		if err != nil {
			return transfer.Class{}, err
		}
		defer clear(raw)
		if h.Type == "message" {
			_, doc, err := decodeMessage(raw, h.ID)
			clear(doc.Envelope)
			return transfer.Class{Order: 2}, err
		}
		if h.Type == "migration" {
			var receipt ImportReceipt
			if json.Unmarshal(raw, &receipt) != nil || (receipt.Kind != migrationKind && receipt.Kind != migrationKindV2) || receipt.Epoch != h.ID {
				return transfer.Class{}, ErrRecord
			}
			return transfer.Class{Order: 3}, nil
		}
		if !json.Valid(raw) {
			return transfer.Class{}, ErrRecord
		}
		return transfer.Class{Order: 1}, nil
	default:
		return transfer.Class{}, ErrRecord
	}
}

func (t *mailTransfer) Rewrite(_ context.Context, version storage.ObjectVersion, record storage.Record, resolve transfer.Resolve) ([]byte, error) {
	return t.rewrite(version, record.Payload, resolve)
}

func (t *mailTransfer) FileDocumentLimit(_ context.Context, version storage.ObjectVersion) (int64, error) {
	h, err := decodeHeader(version)
	if err != nil {
		return 0, err
	}
	if version.Commit.Content != nil && version.Commit.Content.Kind == "file" && h.Type != "mime" && h.Type != "source" {
		return t.source.messageLimit(), nil
	}
	return 0, nil
}

func (t *mailTransfer) RewriteFileDocument(_ context.Context, version storage.ObjectVersion, payload []byte, resolve transfer.Resolve) ([]byte, error) {
	return t.rewrite(version, payload, resolve)
}

func (t *mailTransfer) rewrite(version storage.ObjectVersion, payload []byte, resolve transfer.Resolve) ([]byte, error) {
	h, err := decodeHeader(version)
	if err != nil {
		return nil, err
	}
	if h.Type != "message" && h.Type != "migration" {
		if !json.Valid(payload) {
			return nil, ErrRecord
		}
		return bytes.Clone(payload), nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return nil, ErrRecord
	}
	if h.Type == "message" {
		_, doc, err := decodeMessage(payload, h.ID)
		defer clear(doc.Envelope)
		if err != nil {
			return nil, err
		}
		next, err := resolve(doc.MIME, objectID("mime", h.ID))
		if err != nil {
			return nil, err
		}
		fields["mime"], err = json.Marshal(next)
		if err != nil {
			return nil, err
		}
	} else if err := t.rewriteReceipt(fields, h, payload, resolve); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func (t *mailTransfer) resolveReference(ref storage.Reference, resolve transfer.Resolve) (storage.Reference, error) {
	if t.lookup == nil {
		return storage.Reference{}, ErrRecord
	}
	original, ok := t.lookup(ref)
	if !ok {
		return storage.Reference{}, ErrRecord
	}
	return resolve(ref, original.ObjectID)
}

func (t *mailTransfer) rewriteReceipt(fields map[string]json.RawMessage, h header, payload []byte, resolve transfer.Resolve) error {
	var receipt ImportReceipt
	if json.Unmarshal(payload, &receipt) != nil || (receipt.Kind != migrationKind && receipt.Kind != migrationKindV2) || receipt.Epoch != h.ID {
		return ErrRecord
	}
	for i, ref := range receipt.Records {
		next, err := t.resolveReference(ref, resolve)
		if err != nil {
			return err
		}
		receipt.Records[i] = next
	}
	var err error
	fields["records"], err = json.Marshal(receipt.Records)
	if err != nil {
		return err
	}
	if raw, ok := fields["source_records"]; ok {
		fields["source_records"], err = t.rewriteReceiptSources(raw, receipt.Sources, resolve)
	}
	return err
}

func (t *mailTransfer) rewriteReceiptSources(raw []byte, original []ImportedSource, resolve transfer.Resolve) ([]byte, error) {
	var sources []map[string]json.RawMessage
	if json.Unmarshal(raw, &sources) != nil || len(sources) != len(original) {
		return nil, ErrRecord
	}
	for i, item := range original {
		if sources[i] == nil {
			return nil, ErrRecord
		}
		next, err := t.resolveReference(item.Archive, resolve)
		if err != nil {
			return nil, err
		}
		sources[i]["archive"], err = json.Marshal(next)
		if err != nil {
			return nil, err
		}
		if item.Projection != nil {
			next, err = t.resolveReference(*item.Projection, resolve)
			if err != nil {
				return nil, err
			}
			sources[i]["projection"], err = json.Marshal(next)
			if err != nil {
				return nil, err
			}
		}
	}
	return json.Marshal(sources)
}

func (t *mailTransfer) CheckPending(ctx context.Context, source storage.FrozenBinding, destination storage.Binding) error {
	owner := t.source.cfg.Scope.Tenant + "\x00" + t.source.cfg.Scope.Subject + "\x00mail"
	for _, namespace := range []string{writesNamespace, importNamespace, ingestNamespace} {
		ids, err := t.source.cfg.Journal.RecordIDs(owner, namespace)
		if err != nil {
			return err
		}
		if len(ids) != 0 {
			return ErrRecord
		}
	}
	ids, err := t.source.cfg.Journal.RecordIDs(owner, storage.PendingUploadsNamespace)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		var work struct {
			Target storage.PublicationTarget `json:"target"`
		}
		found, err := t.source.cfg.Journal.Read(owner, storage.PendingUploadsNamespace, id, &work)
		if err != nil {
			return err
		}
		if !found || !work.Target.Valid() || slices.Contains(source.Binding.Rooms, work.Target.RoomID) || !slices.Contains(destination.Rooms, work.Target.RoomID) {
			return ErrRecord
		}
	}
	return nil
}
