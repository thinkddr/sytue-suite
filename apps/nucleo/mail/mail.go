// SPDX-License-Identifier: Apache-2.0
// Package mail owns private mailbox data in verified Matrix clients. It never
// reads message contents from a plaintext Mail API or manufactures an empty
// private account when the retained source has not been completely imported.
package mail

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sync"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/limits"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
)

var (
	ErrNotReady = errors.New("mail: private source import is not complete")
	ErrRecord   = errors.New("mail: invalid private mailbox record")
	ErrConflict = errors.New("mail: concurrent private revisions require resolution")
	ErrClosed   = errors.New("mail: private repository is closed")
	ErrNotFound = errors.New("mail: private mailbox resource not found")
)

const (
	kind            = "org.sytue.mail.entry.v1"
	migrationKind   = "org.sytue.mail.migration.v1"
	writesNamespace = "mail-private-writes-v1"
)

// Crypto is the existing maintained SDK, not a cipher implementation.
type Crypto interface {
	storage.RecordStore
	StorageIdentityVersion() storage.IdentityVersion
}

type Config struct {
	Metadata RoutingMetadata
	Usage    CipherUsageReader
	// BindPublicationContext is captured by the native session owner. Every
	// admitted operation applies it to its actual WorkGate context, never life-only Values.
	BindPublicationContext func(context.Context) (context.Context, error)
	WorkLease              offline.WorkLease
	Context                context.Context
	Scope                  storage.Scope
	Crypto                 Crypto
	Gateway                *api.Drive
	Files                  storage.BoundFiles // Explicitly bound SDK/gateway/journal; web never supplies a dummy Drive.
	Journal                *offline.Store
	// MigrationState comes from the retained, fenced server source. A matching
	// private receipt must reference every imported index, including MIME bodies.
	MigrationState      storage.LegacyMigrationState
	MaxMessageBytes     int64 // authenticated capability; historical import may need a larger explicit limit
	Submission          api.PrivateMailSubmissionTransport
	AuthorizeSubmission func(context.Context) error
}

type Repository struct {
	cfg           Config
	ctx           context.Context
	cancel        context.CancelFunc
	gate          *offline.WorkGate
	mu            sync.Mutex
	closed, ready bool
	readers       map[*bodyReader]struct{}
}

type header struct {
	Kind string `json:"kind"`
	Type string `json:"type"`
	ID   string `json:"id"`
}

// ImportReceipt is published LAST by the source importer. Every listed index
// must exist in complete authenticated history before any mailbox is exposed.
// Large inventories may use the SDK's encrypted file as their content pointer.
type ImportReceipt struct {
	Kind    string              `json:"kind"`
	Epoch   string              `json:"epoch"`
	Records []storage.Reference `json:"records"`
	Sources []ImportedSource    `json:"source_records,omitempty"`
}

func New(cfg Config) (*Repository, error) {
	return newRepository(cfg, storage.IsolatedIdentity)
}

func newRepository(cfg Config, identity storage.IdentityVersion) (*Repository, error) {
	maximum, err := limits.MessageBytes(cfg.MaxMessageBytes)
	if err != nil {
		return nil, ErrRecord
	}
	cfg.MaxMessageBytes = maximum
	if cfg.Context == nil || !cfg.Scope.Valid() || cfg.Scope.Product != "mail" || cfg.Crypto == nil || cfg.Crypto.StorageIdentityVersion() != identity || cfg.Journal == nil || !cfg.Journal.Encrypted() || cfg.MigrationState.Validate(cfg.Scope) != nil || cfg.MigrationState.Mode == "legacy" {
		return nil, ErrRecord
	}
	if cfg.Files == nil {
		sdk, ok := cfg.Crypto.(filesdk.SDK)
		if !ok {
			return nil, ErrRecord
		}
		files, err := filesdk.New(cfg.Scope, sdk, cfg.Gateway, cfg.Journal)
		if err != nil {
			return nil, err
		}
		cfg.Files = files
	}
	if cfg.Files.FileScope() != cfg.Scope {
		return nil, ErrRecord
	}
	ctx, cancel := context.WithCancel(cfg.Context)
	return &Repository{cfg: cfg, ctx: ctx, cancel: cancel, gate: offline.NewWorkGateWithLease(ctx, cfg.WorkLease), readers: map[*bodyReader]struct{}{}}, nil
}
func (r *Repository) PrivateStorageScope() storage.Scope { return r.cfg.Scope }
func (r *Repository) Close() {
	r.cancel()
	r.mu.Lock()
	r.closed, r.ready = true, false
	readers := r.readers
	r.readers = nil
	r.mu.Unlock()
	for reader := range readers {
		_ = reader.Close()
	}
	r.gate.Close()
}

func (r *Repository) Initialize(parent context.Context) error {
	return r.run(parent, false, func(ctx context.Context, s storage.AtomicCatalogSnapshot) error {
		r.ready = false
		if _, err := r.cfg.Crypto.PrepareStorage(ctx, r.cfg.Scope); err != nil {
			return err
		}
		if err := r.cfg.Files.Prepare(ctx); err != nil {
			return err
		}
		receipt, err := r.completedReceipt(ctx, s)
		if err != nil {
			return err
		}
		if err := r.materializeSettings(ctx, s, receipt); err != nil {
			return err
		}
		r.ready = true
		return nil
	})
}

// completedReceipt is a read-only inventory check, including original MIME
// bytes. Frozen legacy catalogs must never materialize settings while checked.
func (r *Repository) completedReceipt(ctx context.Context, snapshot storage.AtomicCatalogSnapshot) (ImportReceipt, error) {
	receipt, err := r.loadImportReceipt(ctx, snapshot)
	if err != nil {
		return receipt, err
	}
	return receipt, r.verifyImportReceipt(ctx, snapshot, receipt)
}

func (r *Repository) loadImportReceipt(ctx context.Context, s storage.AtomicCatalogSnapshot) (ImportReceipt, error) {
	versions := heads(s, objectID("migration", r.cfg.MigrationState.Epoch))
	if len(versions) == 0 {
		return ImportReceipt{}, ErrNotReady
	}
	if len(versions) != 1 {
		return ImportReceipt{}, ErrConflict
	}
	value, err := r.readDocument(ctx, versions[0])
	if err != nil {
		return ImportReceipt{}, err
	}
	defer clear(value)
	var receipt ImportReceipt
	if json.Unmarshal(value, &receipt) != nil || (receipt.Kind != migrationKind && receipt.Kind != migrationKindV2) || receipt.Epoch != r.cfg.MigrationState.Epoch || len(receipt.Records) == 0 {
		return receipt, ErrNotReady
	}
	return receipt, nil
}

func (r *Repository) receiptInventory(s storage.AtomicCatalogSnapshot, receipt ImportReceipt) ([]storage.ObjectVersion, map[storage.Reference]bool, map[string]bool, error) {
	versions := make([]storage.ObjectVersion, 0, len(receipt.Records))
	seen := map[storage.Reference]bool{}
	mailboxes := map[string]bool{}
	for _, ref := range receipt.Records {
		if !ref.Valid() || seen[ref] {
			return nil, nil, nil, ErrRecord
		}
		seen[ref] = true
		entry, found := s.Entries[ref]
		if !found {
			return nil, nil, nil, ErrNotReady
		}
		version, found, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, entry.Record.ObjectID, "", &ref)
		if err != nil || !found {
			return nil, nil, nil, ErrRecord
		}
		h, err := decodeHeader(version)
		if err != nil || h.Type == "migration" {
			return nil, nil, nil, ErrRecord
		}
		if h.Type == "mailbox" {
			mailboxes[h.ID] = true
		}
		versions = append(versions, version)
	}
	return versions, seen, mailboxes, nil
}

func (r *Repository) verifyImportReceipt(ctx context.Context, s storage.AtomicCatalogSnapshot, receipt ImportReceipt) error {
	versions, seen, mailboxes, err := r.receiptInventory(s, receipt)
	if err != nil {
		return err
	}
	if receipt.Kind == migrationKindV2 {
		if len(mailboxes) == 0 {
			return ErrNotReady
		}
		return r.validateSourceReceipt(ctx, s, receipt)
	}
	if len(receipt.Sources) != 0 {
		return ErrRecord
	}
	var rows, bodies int64
	for _, version := range versions {
		h, err := decodeHeader(version)
		if err != nil {
			return err
		}
		if h.Type == "mime" {
			bodies++
		} else {
			rows++
		}
		content, err := r.readDocument(ctx, version)
		if err != nil {
			return err
		}
		err = r.validateImportedDocument(s, h, content, seen, mailboxes)
		clear(content)
		if err != nil {
			return err
		}
	}
	if len(mailboxes) == 0 || rows != r.cfg.MigrationState.Rows || bodies != r.cfg.MigrationState.Blobs {
		return ErrNotReady
	}
	return nil
}

func (r *Repository) run(parent context.Context, requireReady bool, fn func(context.Context, storage.AtomicCatalogSnapshot) error) error {
	ctx, done, err := r.start(parent)
	if err != nil {
		return err
	}
	defer done()
	return r.runAdmitted(ctx, requireReady, fn)
}

// runAdmitted requires the caller to retain its real WorkGate permission.
// Raw keeps that same permission until its MIME reader finishes cleanup.
func (r *Repository) runAdmitted(ctx context.Context, requireReady bool, fn func(context.Context, storage.AtomicCatalogSnapshot) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrClosed
	}
	if requireReady && !r.ready {
		return ErrNotReady
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	snapshot, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	if err := r.recoverAcceptedDocumentIntents(ctx, snapshot); err != nil {
		return err
	}
	return fn(ctx, snapshot)
}
func objectID(typ, id string) string { return "mail_" + digest([]byte(typ+"\x00"+id)) }
func digest(data []byte) string      { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func heads(s storage.AtomicCatalogSnapshot, object string) []storage.ObjectVersion {
	var out []storage.ObjectVersion
	for _, version := range s.Heads {
		if version.ObjectID == object {
			out = append(out, version)
		}
	}
	return out
}

func decodeHeader(version storage.ObjectVersion) (header, error) {
	var h header
	if json.Unmarshal(version.Commit.Metadata, &h) != nil || h.Kind != kind || h.ID == "" || len(h.ID) > 255 || version.ObjectID != objectID(h.Type, h.ID) {
		return h, ErrRecord
	}
	switch h.Type {
	case "mailbox", "message", "label", "signature", "templates", "migration", "mime", "source", "filter", "vacation", "outbound", "routing":
	default:
		return h, ErrRecord
	}
	return h, nil
}

func (r *Repository) readDocument(ctx context.Context, version storage.ObjectVersion) ([]byte, error) {
	if _, err := decodeHeader(version); err != nil {
		return nil, err
	}
	pointer := version.Commit.Content
	if version.Commit.State != "live" || pointer == nil {
		return nil, ErrNotFound
	}
	if pointer.Kind == "record" {
		record, err := r.cfg.Crypto.ReadStorageRecord(ctx, pointer.Reference, version.ObjectID, "content")
		if err != nil {
			return nil, err
		}
		if record.Validate(r.cfg.Scope) != nil || record.RevisionID != pointer.RevisionID || record.ObjectID != version.ObjectID || record.Role != "content" {
			clear(record.Payload)
			return nil, ErrRecord
		}
		return record.Payload, nil
	}
	if pointer.Kind != "file" {
		return nil, ErrRecord
	}
	info, err := r.cfg.Files.Info(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID)
	if err != nil {
		return nil, err
	}
	if info.Validate(r.cfg.Scope) != nil || info.ObjectID != version.ObjectID || info.RevisionID != pointer.RevisionID {
		return nil, ErrRecord
	}
	if info.SizeBytes > r.messageLimit() {
		return nil, api.ErrMailTooLarge
	}
	var out bytes.Buffer
	actual, err := r.cfg.Files.Download(ctx, pointer.Reference, version.ObjectID, pointer.RevisionID, limitedBodyWriter{buffer: &out, limit: info.SizeBytes})
	before, _ := json.Marshal(info)
	after, _ := json.Marshal(actual)
	if err != nil {
		clear(out.Bytes())
		return nil, err
	}
	if int64(out.Len()) != info.SizeBytes || !bytes.Equal(before, after) {
		clear(out.Bytes())
		return nil, ErrRecord
	}
	return out.Bytes(), nil
}

func (r *Repository) document(ctx context.Context, s storage.AtomicCatalogSnapshot, typ, id string) ([]byte, error) {
	versions := heads(s, objectID(typ, id))
	if len(versions) == 0 {
		return nil, ErrNotFound
	}
	if len(versions) != 1 {
		return nil, ErrConflict
	}
	h, err := decodeHeader(versions[0])
	if err != nil || h.Type != typ || h.ID != id {
		return nil, ErrRecord
	}
	data, err := r.readDocument(ctx, versions[0])
	return data, err
}

func (r *Repository) list(ctx context.Context, s storage.AtomicCatalogSnapshot, typ string) (map[string][]byte, error) {
	out := map[string][]byte{}
	seen := map[string]bool{}
	for _, version := range s.Heads {
		h, err := decodeHeader(version)
		if err != nil {
			return nil, err
		}
		if h.Type != typ {
			continue
		}
		if seen[h.ID] {
			return nil, ErrConflict
		}
		seen[h.ID] = true
		if version.Commit.State == "deleted" {
			continue
		}
		data, err := r.readDocument(ctx, version)
		if err != nil {
			return nil, err
		}
		out[h.ID] = data
	}
	return out, nil
}

func clearDocuments(documents map[string][]byte) {
	for _, value := range documents {
		clear(value)
	}
}

type writeIntent struct {
	OperationID string                    `json:"operation_id,omitempty"`
	State       string                    `json:"state,omitempty"`
	Version     int                       `json:"v"`
	Scope       storage.Scope             `json:"scope"`
	Target      storage.PublicationTarget `json:"target"`
	Header      header                    `json:"header"`
	Revision    string                    `json:"revision"`
	Payload     []byte                    `json:"payload"`
	Parents     []storage.Reference       `json:"parents"`
	Content     *storage.Reference        `json:"content,omitempty"`
	Index       *storage.Reference        `json:"index,omitempty"`
	ForceFile   bool                      `json:"force_file,omitempty"`
}

func (r *Repository) writeDocument(ctx context.Context, s storage.AtomicCatalogSnapshot, typ, id string, payload []byte, state string) error {
	if !json.Valid(payload) {
		return ErrRecord
	}
	return r.writeContent(ctx, s, typ, id, payload, state, false)
}

func (r *Repository) writeContent(ctx context.Context, s storage.AtomicCatalogSnapshot, typ, id string, payload []byte, state string, forceFile bool) error {
	if int64(len(payload)) > r.messageLimit() || state != "live" && state != "deleted" {
		return ErrRecord
	}
	desired := header{Kind: kind, Type: typ, ID: id}
	raw, _ := json.Marshal(desired)
	key := documentIntentKey(raw, payload, state, documentOperation(ctx))
	owner := r.cfg.Scope.Tenant + "\x00" + r.cfg.Scope.Subject + "\x00mail"
	return r.cfg.Journal.Exclusive(ctx, owner, writesNamespace, key, func() error {
		if done, err := r.recoverDocumentHistory(ctx, s, owner, key, desired, payload, state, forceFile); err != nil || done {
			return err
		}
		work, done, err := r.loadWriteIntent(ctx, s, owner, key, desired, payload, state, forceFile)
		if err != nil || done {
			return err
		}
		defer clear(work.Payload)
		if err := work.validate(r.cfg.Scope, desired, payload, forceFile); err != nil {
			return err
		}
		if work.State != "" && work.State != state {
			return ErrRecord
		}
		object := objectID(typ, id)
		accepted, err := r.completeDocumentWrite(ctx, s, owner, key, object, &work, state)
		if accepted || err != nil {
			return err
		}
		if err := r.cfg.Crypto.CheckStoragePublicationTarget(work.Target); err != nil {
			return err
		}
		if err := r.cfg.Journal.Write(owner, writesNamespace, key, work); err != nil {
			return err
		}
		if state == "live" {
			if err := r.storeWriteContent(ctx, owner, key, object, payload, &work); err != nil {
				return err
			}
		}
		version, err := r.storeWriteIndex(ctx, owner, key, object, raw, state, &work)
		if err != nil {
			return err
		}
		return r.finishDocumentWrite(ctx, owner, key, object, &work, state, version)
	})
}

func (r *Repository) finishDocumentWrite(ctx context.Context, owner, key, object string, work *writeIntent, state string, version storage.ObjectVersion) error {
	if work.Index == nil || *work.Index != version.Reference {
		return ErrRecord
	}
	current, err := storage.LoadAtomicCatalog(ctx, r.cfg.Crypto, r.cfg.Scope)
	if err != nil {
		return err
	}
	accepted, err := r.completeDocumentWrite(ctx, current, owner, key, object, work, state)
	if err != nil {
		return err
	}
	if !accepted {
		return ErrConflict
	}
	return nil
}

// Reconcile the exact accepted revision before requiring the original device's
// publication target. A newer head cannot turn cleanup into another write.
func (r *Repository) completeDocumentWrite(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, key, object string, work *writeIntent, state string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	version, found, err := storage.AcceptedAtomicObject(s, r.cfg.Scope, object, work.Revision+"_index", work.Index)
	if err != nil || !found {
		return false, err
	}
	if err := verifyAcceptedDocumentCommit(version, work, state); err != nil {
		return false, err
	}
	if state == "live" {
		if err := r.verifyAcceptedDocumentContent(ctx, key, work, version); err != nil {
			return false, err
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := r.retainAcceptedDocumentIntent(ctx, owner, key, state, work, version); err != nil {
		return false, err
	}
	return true, r.cfg.Journal.Delete(owner, writesNamespace, key)
}

func verifyAcceptedDocumentCommit(version storage.ObjectVersion, work *writeIntent, state string) error {
	if version.Reference.RoomID != work.Target.RoomID {
		return ErrRecord
	}
	acceptedHeader, err := decodeHeader(version)
	if err != nil || acceptedHeader != work.Header {
		return ErrRecord
	}
	expected := storage.ObjectCommit{Kind: storage.ObjectKind, State: state, Parents: slices.Clone(work.Parents)}
	expected.Metadata, _ = json.Marshal(work.Header)
	if state == "live" {
		pointer := version.Commit.Content
		contentKind := "record"
		if work.useFile() {
			contentKind = "file"
		}
		if pointer == nil || pointer.Kind != contentKind || pointer.RevisionID != work.Revision || work.Content != nil && *work.Content != pointer.Reference {
			return ErrRecord
		}
		expected.Content = &storage.ContentPointer{Kind: contentKind, RevisionID: work.Revision, Reference: pointer.Reference}
	}
	before, _ := json.Marshal(expected)
	after, _ := json.Marshal(version.Commit)
	if !bytes.Equal(before, after) {
		return ErrRecord
	}
	return nil
}

func (r *Repository) verifyAcceptedDocumentContent(ctx context.Context, key string, work *writeIntent, version storage.ObjectVersion) error {
	payload, err := r.readDocument(ctx, version)
	if err != nil {
		return err
	}
	equal := bytes.Equal(payload, work.Payload)
	clear(payload)
	if !equal {
		return ErrRecord
	}
	if work.useFile() {
		return r.ackWriteFile(ctx, key, work, version.Reference)
	}
	return nil
}

func (r *Repository) loadWriteIntent(ctx context.Context, s storage.AtomicCatalogSnapshot, owner, key string, desired header, payload []byte, state string, forceFile bool) (writeIntent, bool, error) {
	var work writeIntent
	found, err := r.cfg.Journal.Read(owner, writesNamespace, key, &work)
	if err != nil || found {
		return work, false, err
	}
	parents := heads(s, objectID(desired.Type, desired.ID))
	if len(parents) > 1 {
		return work, false, ErrConflict
	}
	if len(parents) == 1 && parents[0].Commit.State == state && documentOperation(ctx) == "" {
		if state == "deleted" {
			return work, true, nil
		}
		prior, err := r.readDocument(ctx, parents[0])
		if err != nil {
			return work, false, err
		}
		same := bytes.Equal(prior, payload)
		clear(prior)
		if same {
			return work, true, nil
		}
	}
	refs := make([]storage.Reference, len(parents))
	for i, parent := range parents {
		refs[i] = parent.Reference
	}
	return writeIntent{Version: 1, OperationID: documentOperation(ctx), State: state, Scope: r.cfg.Scope, Target: s.Binding.Target, Header: desired, Revision: uuid.NewString(), Payload: bytes.Clone(payload), Parents: refs, ForceFile: forceFile}, false, nil
}

func (w writeIntent) validate(scope storage.Scope, desired header, payload []byte, forceFile bool) error {
	if w.Version != 1 || w.Scope != scope || w.ForceFile != forceFile || w.Header != desired || !storage.Identifier(w.Revision) || !bytes.Equal(w.Payload, payload) || len(w.Parents) > 1 {
		return ErrRecord
	}
	return nil
}

func (r *Repository) storeWriteRecord(ctx context.Context, owner, key, object string, payload []byte, work *writeIntent) error {
	if work.Content == nil {
		record := storage.Record{Version: 1, Scope: r.cfg.Scope, ObjectID: object, RevisionID: work.Revision, Role: "content", Payload: bytes.Clone(payload)}
		if record.Validate(r.cfg.Scope) != nil {
			clear(record.Payload)
			return ErrRecord
		}
		ref, err := r.cfg.Crypto.PutStorageRecordAt(ctx, work.Target, key+"_content_"+work.Revision, record)
		clear(record.Payload)
		if err != nil {
			return err
		}
		if !ref.Valid() || ref.RoomID != work.Target.RoomID {
			return ErrRecord
		}
		actual, err := r.cfg.Crypto.ReadStorageRecord(ctx, ref, object, "content")
		if err != nil {
			return err
		}
		equal := actual.Validate(r.cfg.Scope) == nil && actual.RevisionID == work.Revision && bytes.Equal(actual.Payload, payload)
		clear(actual.Payload)
		if !equal {
			return ErrRecord
		}
		work.Content = &ref
		if err := r.cfg.Journal.Write(owner, writesNamespace, key, work); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) storeWriteIndex(ctx context.Context, owner, key, object string, raw []byte, state string, work *writeIntent) (storage.ObjectVersion, error) {
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: state, Metadata: raw, Parents: slices.Clone(work.Parents)}
	if state == "live" {
		contentKind := "record"
		if work.useFile() {
			contentKind = "file"
		}
		commit.Content = &storage.ContentPointer{Kind: contentKind, RevisionID: work.Revision, Reference: *work.Content}
	}
	version, err := r.cfg.Crypto.PutStorageObjectAt(ctx, work.Target, key+"_index_"+work.Revision, object, work.Revision+"_index", commit)
	if err != nil {
		return version, err
	}
	before, _ := json.Marshal(commit)
	after, _ := json.Marshal(version.Commit)
	if version.ObjectID != object || version.RevisionID != work.Revision+"_index" || !version.Reference.Valid() || version.Reference.RoomID != work.Target.RoomID || !bytes.Equal(before, after) {
		return version, ErrRecord
	}
	work.Index = &version.Reference
	return version, r.cfg.Journal.Write(owner, writesNamespace, key, work)
}

type limitedBodyWriter struct {
	buffer *bytes.Buffer
	limit  int64
}

func (w limitedBodyWriter) Write(data []byte) (int, error) {
	if int64(w.buffer.Len()) > w.limit-int64(len(data)) {
		return 0, api.ErrMailTooLarge
	}
	return w.buffer.Write(data)
}

func (r *Repository) validateImportedDocument(snapshot storage.AtomicCatalogSnapshot, h header, payload []byte, refs map[storage.Reference]bool, mailboxes map[string]bool) error {
	switch h.Type {
	case "message":
		return r.validateImportedMessage(snapshot, h, payload, refs, mailboxes)
	case "mailbox":
		var value api.Mailbox
		if json.Unmarshal(payload, &value) != nil || value.ID != h.ID || !validName(value.Name) {
			return ErrRecord
		}
	case "label":
		var value api.MailLabel
		if json.Unmarshal(payload, &value) != nil || value.ID != h.ID || !validName(value.Name) {
			return ErrRecord
		}
	case "signature":
		var value string
		if json.Unmarshal(payload, &value) != nil {
			return ErrRecord
		}
	case "mime", "source", "filter", "vacation":
	default:
		return ErrRecord
	}
	return nil
}

func (r *Repository) validateImportedMessage(snapshot storage.AtomicCatalogSnapshot, h header, payload []byte, refs map[storage.Reference]bool, mailboxes map[string]bool) error {
	message, doc, err := decodeMessage(payload, h.ID)
	defer clear(doc.Envelope)
	if err != nil {
		return err
	}
	if !mailboxes[message.MailboxID] || !refs[doc.MIME] {
		return ErrNotReady
	}
	entry, ok := snapshot.Entries[doc.MIME]
	if !ok {
		return ErrNotReady
	}
	version, found, err := storage.AcceptedAtomicObject(snapshot, r.cfg.Scope, entry.Record.ObjectID, "", &doc.MIME)
	if err != nil || !found {
		return ErrRecord
	}
	body, err := decodeHeader(version)
	if err != nil || body.Type != "mime" || body.ID != h.ID {
		return ErrRecord
	}
	return nil
}

func (w writeIntent) useFile() bool { return w.ForceFile || len(w.Payload) > storage.MaxRecordBytes/2 }

func (r *Repository) storeWriteContent(ctx context.Context, owner, key, object string, payload []byte, work *writeIntent) error {
	if !work.useFile() {
		return r.storeWriteRecord(ctx, owner, key, object, payload, work)
	}
	job := "mail_document_" + key + "_" + work.Revision
	if work.Content == nil {
		ref, err := r.cfg.Files.Upload(ctx, job, work.Target, storage.FileRequest{ObjectID: object, RevisionID: work.Revision, SizeBytes: int64(len(payload)), Metadata: []byte(`{"kind":"org.sytue.mail.document.v1"}`)}, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		if !ref.Valid() || ref.RoomID != work.Target.RoomID {
			return ErrRecord
		}
		work.Content = &ref
		if err := r.cfg.Journal.Write(owner, writesNamespace, key, work); err != nil {
			return err
		}
	}
	info, sum, err := r.cfg.Files.Commitment(ctx, *work.Content, object, work.Revision)
	if err != nil {
		return err
	}
	if info.Validate(r.cfg.Scope) != nil || info.ObjectID != object || info.RevisionID != work.Revision || info.SizeBytes != int64(len(payload)) || sum != digest(payload) {
		return ErrRecord
	}
	return nil
}

func (r *Repository) ackWriteFile(ctx context.Context, key string, work *writeIntent, index storage.Reference) error {
	job := "mail_document_" + key + "_" + work.Revision
	if err := r.cfg.Files.Acknowledge(ctx, job, index); err != nil && !errors.Is(err, storage.ErrUploadMissing) {
		return err
	}
	return nil
}

func (r *Repository) messageLimit() int64 { return r.cfg.MaxMessageBytes }

// MessageByteLimit is the original normalized capability retained by this repository.
// A transport may frame MIME with it; it must never choose a smaller silent limit.
func (r *Repository) MessageByteLimit() int64 { return r.messageLimit() }
