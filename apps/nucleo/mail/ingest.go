// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"slices"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

const ingestNamespace = "mail-private-ingest-v1"

type ingestIntent struct {
	Version        int                       `json:"v"`
	Scope          storage.Scope             `json:"scope"`
	Target         storage.PublicationTarget `json:"target"`
	ID             string                    `json:"id"`
	Operation      string                    `json:"operation"`
	Envelope       []byte                    `json:"envelope"`
	Size           int64                     `json:"size"`
	SHA256         string                    `json:"sha256"`
	Revision       string                    `json:"revision"`
	Parents        []storage.Reference       `json:"parents"`
	MessageParents []storage.Reference       `json:"message_parents"`
	File           *storage.Reference        `json:"file,omitempty"`
	MIME           *storage.Reference        `json:"mime,omitempty"`
}

type contextReaderAt struct {
	ctx    context.Context
	reader io.ReaderAt
}

func (r contextReaderAt) ReadAt(out []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.ReadAt(out, offset)
}

func sourceDigest(ctx context.Context, source io.ReaderAt, size, limit int64) (string, error) {
	if source == nil || size < 0 || size > limit {
		return "", ErrRecord
	}
	reader := contextReaderAt{ctx: ctx, reader: source}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(reader, 0, size))
	if err != nil {
		return "", err
	}
	if n != size {
		return "", ErrRecord
	}
	var tail [1]byte
	nTail, err := reader.ReadAt(tail[:], size)
	if nTail != 0 || !errors.Is(err, io.EOF) {
		return "", ErrRecord
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func versionReferences(s storage.AtomicCatalogSnapshot, object string) ([]storage.Reference, error) {
	versions := heads(s, object)
	if len(versions) > 1 {
		return nil, ErrConflict
	}
	refs := make([]storage.Reference, len(versions))
	for i, version := range versions {
		refs[i] = version.Reference
	}
	return refs, nil
}

// StoreMessage is the authorized-device ingress operation. operation is a
// stable worker/editor id, supplied before its first attempt. It retains a
// complete encrypted WAL before uploading MIME, uses the original room/device,
// and never calls the plaintext SMTP, JMAP or message-storage service.
type ingestMessage struct {
	ID        string `json:"id"`
	MailboxID string `json:"mailbox_id"`
	Size      int64  `json:"size"`
}

func (r *Repository) StoreMessage(parent context.Context, operation string, envelope []byte, source io.ReaderAt, size int64) error {
	if !storage.Identifier(operation) || len(envelope) > storage.MaxRecordBytes/2 {
		return ErrRecord
	}
	var message ingestMessage
	if json.Unmarshal(envelope, &message) != nil || message.ID == "" || message.MailboxID == "" || message.Size != size {
		return ErrRecord
	}
	envelope = bytes.Clone(envelope)
	defer clear(envelope)
	return r.run(parent, true, func(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) error {
		return r.storeMessage(ctx, snapshot, operation, message, envelope, source, size)
	})
}

// storeMessage requires the repository's admitted work and catalog lock. Editors
// and authorized ingress share the same retained MIME publication protocol.
func (r *Repository) storeMessage(ctx context.Context, snapshot storage.AtomicCatalogSnapshot, operation string, message ingestMessage, envelope []byte, source io.ReaderAt, size int64) error {
	if !storage.Identifier(operation) || len(envelope) > storage.MaxRecordBytes/2 {
		return ErrRecord
	}
	mailbox, err := r.document(ctx, snapshot, "mailbox", message.MailboxID)
	clear(mailbox)
	if err != nil {
		return err
	}
	checksum, err := sourceDigest(ctx, source, size, r.messageLimit())
	if err != nil {
		return err
	}
	owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
	return r.cfg.Journal.Exclusive(ctx, owner, ingestNamespace, operation, func() error {
		work, done, err := r.loadIngestIntent(ctx, snapshot, owner, operation, message.ID, envelope, size, checksum)
		if err != nil || done {
			return err
		}
		defer clear(work.Envelope)
		if err := work.validate(r.cfg.Scope, message.ID, operation, envelope, size, checksum); err != nil {
			return err
		}
		return r.resumeIngest(ctx, owner, &work, source)
	})
}

func (r *Repository) loadIngestIntent(ctx context.Context, snapshot storage.AtomicCatalogSnapshot, owner, operation, id string, envelope []byte, size int64, checksum string) (ingestIntent, bool, error) {
	var work ingestIntent
	found, err := r.cfg.Journal.Read(owner, ingestNamespace, operation, &work)
	if err != nil || found {
		return work, false, err
	}
	done, err := r.alreadyStored(ctx, snapshot, id, operation, envelope, checksum)
	if err != nil || done {
		return work, done, err
	}
	parents, err := versionReferences(snapshot, objectID("mime", id))
	if err != nil {
		return work, false, err
	}
	messageParents, err := versionReferences(snapshot, objectID("message", id))
	if err != nil {
		return work, false, err
	}
	return ingestIntent{Version: 1, Scope: r.cfg.Scope, Target: snapshot.Binding.Target, ID: id, Operation: operation, Envelope: bytes.Clone(envelope), Size: size, SHA256: checksum, Revision: uuid.NewString(), Parents: parents, MessageParents: messageParents}, false, nil
}

func (w ingestIntent) validate(scope storage.Scope, id, operation string, envelope []byte, size int64, checksum string) error {
	if w.Version != 1 || w.Scope != scope || w.ID != id || w.Operation != operation || !bytes.Equal(w.Envelope, envelope) || w.Size != size || w.SHA256 != checksum || !storage.Identifier(w.Revision) || len(w.Parents) > 1 || len(w.MessageParents) > 1 {
		return ErrRecord
	}
	if w.File != nil && (!w.File.Valid() || w.File.RoomID != w.Target.RoomID) || w.MIME != nil && (!w.MIME.Valid() || w.MIME.RoomID != w.Target.RoomID) {
		return ErrRecord
	}
	return nil
}

func (r *Repository) resumeIngest(ctx context.Context, owner string, work *ingestIntent, source io.ReaderAt) error {
	if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
		return err
	}
	if err := r.cfg.Journal.Write(owner, ingestNamespace, work.Operation, work); err != nil {
		return err
	}
	job := "mail_mime_" + digest([]byte(work.Operation))
	if err := r.storeIngestFile(ctx, owner, job, work, source); err != nil {
		return err
	}
	index, err := r.storeIngestMIMEIndex(ctx, owner, job, work)
	if err != nil {
		return err
	}
	if err := r.storeIngestEnvelope(ctx, work, index.Reference); err != nil {
		return err
	}
	if err := r.cfg.Files.Acknowledge(ctx, job, index.Reference); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.cfg.Journal.Delete(owner, ingestNamespace, work.Operation)
}

func (r *Repository) storeIngestFile(ctx context.Context, owner, job string, work *ingestIntent, source io.ReaderAt) error {
	if work.File == nil {
		ref, err := r.cfg.Files.Upload(ctx, job, work.Target, storage.FileRequest{ObjectID: objectID("mime", work.ID), RevisionID: work.Revision, SizeBytes: work.Size, Metadata: []byte(`{"kind":"org.sytue.mail.message.v1","content_type":"message/rfc822"}`)}, source)
		if err != nil {
			return err
		}
		if !ref.Valid() || ref.RoomID != work.Target.RoomID {
			return ErrRecord
		}
		work.File = &ref
		if err := r.cfg.Journal.Write(owner, ingestNamespace, work.Operation, work); err != nil {
			return err
		}
	}
	info, actualHash, err := r.cfg.Files.Commitment(ctx, *work.File, objectID("mime", work.ID), work.Revision)
	if err != nil {
		return err
	}
	if actualHash != work.SHA256 || info.ObjectID != objectID("mime", work.ID) || info.RevisionID != work.Revision || info.SizeBytes != work.Size {
		return ErrRecord
	}
	return nil
}

func (r *Repository) storeIngestMIMEIndex(ctx context.Context, owner, job string, work *ingestIntent) (storage.ObjectVersion, error) {
	metadata, _ := json.Marshal(header{Kind: kind, Type: "mime", ID: work.ID})
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Parents: slices.Clone(work.Parents), Content: &storage.ContentPointer{Kind: "file", RevisionID: work.Revision, Reference: *work.File}}
	index, err := r.cfg.Crypto.PutStorageObjectAt(ctx, work.Target, job+"_index", objectID("mime", work.ID), work.Revision+"_index", commit)
	if err != nil {
		return index, err
	}
	want, _ := json.Marshal(commit)
	actual, _ := json.Marshal(index.Commit)
	if index.ObjectID != objectID("mime", work.ID) || index.RevisionID != work.Revision+"_index" || !index.Reference.Valid() || index.Reference.RoomID != work.Target.RoomID || !bytes.Equal(want, actual) || work.MIME != nil && *work.MIME != index.Reference {
		return index, ErrRecord
	}
	work.MIME = &index.Reference
	return index, r.cfg.Journal.Write(owner, ingestNamespace, work.Operation, work)
}

func (r *Repository) storeIngestEnvelope(ctx context.Context, work *ingestIntent, mime storage.Reference) error {
	current, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	currentParents, err := versionReferences(current, objectID("message", work.ID))
	if err != nil {
		return err
	}
	document := MessageDocument{Kind: messageKind, Envelope: bytes.Clone(work.Envelope), MIME: mime, Operation: work.Operation}
	payload, err := json.Marshal(document)
	clear(document.Envelope)
	if err != nil {
		return err
	}
	defer clear(payload)
	if !slices.Equal(currentParents, work.MessageParents) {
		existing, err := r.document(ctx, current, "message", work.ID)
		if err != nil {
			return err
		}
		equal := bytes.Equal(existing, payload)
		clear(existing)
		if !equal {
			return ErrConflict
		}
	}
	return r.writeDocument(ctx, current, "message", work.ID, payload, "live")
}

func (r *Repository) alreadyStored(ctx context.Context, snapshot storage.AtomicCatalogSnapshot, id, operation string, envelope []byte, checksum string) (bool, error) {
	raw, err := r.document(ctx, snapshot, "message", id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer clear(raw)
	_, document, err := decodeMessage(raw, id)
	defer clear(document.Envelope)
	if err != nil {
		return false, err
	}
	if document.Operation != operation {
		return false, nil
	}
	if !bytes.Equal(document.Envelope, envelope) {
		return false, ErrConflict
	}
	body, err := r.readMIMEVersion(snapshot, document.MIME, id)
	if err != nil {
		return false, err
	}
	pointer := body.Commit.Content
	if pointer == nil || pointer.Kind != "file" {
		return false, ErrRecord
	}
	_, actual, err := r.cfg.Files.Commitment(ctx, pointer.Reference, body.ObjectID, pointer.RevisionID)
	if err != nil {
		return false, err
	}
	if actual != checksum {
		return false, ErrConflict
	}
	return true, nil
}
