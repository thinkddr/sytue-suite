// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/filesdk"
)

const testEpoch = "c2236155-0c17-4d4f-ad5b-6b657d44286a"

var testScope = storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}

type fileFixture struct {
	request storage.FileRequest
	data    []byte
}
type cryptoFixture struct {
	binding      storage.Binding
	records      map[storage.Reference]storage.Record
	transactions map[string]storage.Reference
	files        map[storage.Reference]fileFixture
	loseIndex    bool
	puts         int
	identity     storage.IdentityVersion
}

func newCryptoFixture() *cryptoFixture {
	return &cryptoFixture{binding: storage.Binding{Scope: testScope, Target: storage.PublicationTarget{RoomID: "!mail:test", DeviceID: "MAIL"}, Rooms: []string{"!mail:test"}}, records: map[storage.Reference]storage.Record{}, transactions: map[string]storage.Reference{}, files: map[storage.Reference]fileFixture{}, identity: storage.IsolatedIdentity}
}
func (c *cryptoFixture) StorageIdentityVersion() storage.IdentityVersion { return c.identity }
func (c *cryptoFixture) PrepareStorage(_ context.Context, scope storage.Scope) (storage.Binding, error) {
	if scope != c.binding.Scope {
		return storage.Binding{}, ErrRecord
	}
	return c.binding, nil
}

func (c *cryptoFixture) StorageBindings(context.Context) (storage.Binding, error) {
	return c.binding, nil
}

func (c *cryptoFixture) StorageObjectHistory(_ context.Context, room, cursor string, _ int) (storage.ObjectPage, error) {
	if room != c.binding.Target.RoomID || cursor != "" {
		return storage.ObjectPage{}, ErrRecord
	}
	out := storage.ObjectPage{}
	for ref, record := range c.records {
		if record.Role == "index" {
			record.Payload = bytes.Clone(record.Payload)
			record.Previous = slices.Clone(record.Previous)
			out.Entries = append(out.Entries, storage.RecordEntry{Reference: ref, Record: record})
		}
	}
	return out, nil
}

func (c *cryptoFixture) CheckStoragePublicationTarget(target storage.PublicationTarget) error {
	if target != c.binding.Target {
		return ErrRecord
	}
	return nil
}

func (c *cryptoFixture) ReadStorageRecord(_ context.Context, ref storage.Reference, object, role string) (storage.Record, error) {
	record, ok := c.records[ref]
	if !ok || record.ObjectID != object || record.Role != role {
		return storage.Record{}, ErrRecord
	}
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	return record, nil
}

func (c *cryptoFixture) PutStorageRecordAt(_ context.Context, target storage.PublicationTarget, transaction string, record storage.Record) (storage.Reference, error) {
	if err := c.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	if record.Validate(testScope) != nil {
		return storage.Reference{}, ErrRecord
	}
	c.puts++
	if ref, found := c.transactions[transaction]; found {
		before, _ := json.Marshal(c.records[ref])
		after, _ := json.Marshal(record)
		if !bytes.Equal(before, after) {
			return storage.Reference{}, ErrRecord
		}
		return ref, nil
	}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$mail" + strconv.Itoa(len(c.records)+1)}
	record.Payload = bytes.Clone(record.Payload)
	record.Previous = slices.Clone(record.Previous)
	c.records[ref] = record
	c.transactions[transaction] = ref
	return ref, nil
}

func (c *cryptoFixture) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, transaction, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	payload, _ := json.Marshal(commit)
	previous := make([]string, len(commit.Parents))
	for i, parent := range commit.Parents {
		previous[i] = parent.EventID
	}
	record := storage.Record{Version: 1, Scope: testScope, ObjectID: object, RevisionID: revision, Role: "index", Payload: payload, Previous: previous}
	ref, err := c.PutStorageRecordAt(ctx, target, transaction, record)
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	version, err := storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, testScope)
	if err == nil && c.loseIndex {
		c.loseIndex = false
		return version, errors.New("synthetic lost index acknowledgment")
	}
	return version, err
}

func (c *cryptoFixture) StorageFileInfo(_ context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, error) {
	value, ok := c.files[ref]
	if !ok || value.request.ObjectID != object || value.request.RevisionID != revision {
		return storage.FileRequest{}, ErrRecord
	}
	return value.request, nil
}

func (c *cryptoFixture) DownloadStorageFile(ctx context.Context, _ *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	if err != nil {
		return request, err
	}
	_, err = out.Write(c.files[ref].data)
	return request, err
}

func (c *cryptoFixture) seed(t *testing.T, typ, id string, value any, parents ...storage.Reference) storage.Reference {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	object := objectID(typ, id)
	revision := "seed" + strconv.Itoa(len(c.records))
	content, err := c.PutStorageRecordAt(t.Context(), c.binding.Target, revision+"_content", storage.Record{Version: 1, Scope: testScope, ObjectID: object, RevisionID: revision, Role: "content", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(header{Kind: kind, Type: typ, ID: id})
	index, err := c.PutStorageObjectAt(t.Context(), c.binding.Target, revision+"_index", object, revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: content}, Parents: parents})
	if err != nil {
		t.Fatal(err)
	}
	return index.Reference
}

type fixture struct {
	repo    *Repository
	crypto  *cryptoFixture
	source  storage.LegacyMigrationState
	raw     []byte
	message storage.Reference
	mailbox storage.Reference
	body    storage.Reference
}

func setup(t *testing.T, receipt bool) *fixture {
	t.Helper()
	c := newCryptoFixture()
	started := time.Date(2026, 10, 29, 10, 0, 0, 123456789, time.UTC)
	source := storage.LegacyMigrationState{Scope: testScope, Mode: "frozen", Epoch: testEpoch, StartedAt: &started, Rows: 2, Blobs: 1}
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := New(Config{Context: t.Context(), Scope: testScope, Crypto: c, Gateway: api.NewDrive("https://unused.invalid", nil, nil), Journal: journal, MigrationState: source})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(repo.Close)
	f := &fixture{repo: repo, crypto: c, source: source, raw: []byte("Subject: Private mail\r\n\r\nprivate MIME body")}
	f.mailbox = c.seed(t, "mailbox", "inbox", map[string]any{"id": "inbox", "name": "Inbox", "role": "inbox", "extension": json.RawMessage(`{"integer":900719925474099512345}`)})
	file := storage.Reference{RoomID: c.binding.Target.RoomID, EventID: "$opaque-file"}
	request := storage.FileRequest{ObjectID: objectID("mime", "message"), RevisionID: "raw-revision", SizeBytes: int64(len(f.raw)), Metadata: []byte(`{"type":"message/rfc822"}`)}
	c.files[file] = fileFixture{request: request, data: bytes.Clone(f.raw)}
	metadata, _ := json.Marshal(header{Kind: kind, Type: "mime", ID: "message"})
	body, err := c.PutStorageObjectAt(t.Context(), c.binding.Target, "body-index", request.ObjectID, "body-index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: file}})
	if err != nil {
		t.Fatal(err)
	}
	f.body = body.Reference
	envelope := []byte(`{"id":"message","mailbox_id":"inbox","thread_id":"thread","subject":"Reunión privada","from":"sender@example.test","to":["alice@example.test"],"size":44,"received_at":"2026-10-29T10:00:00.123456789Z","extension":{"precise":900719925474099512345}}`)
	envelope, err = patchField(envelope, "size", len(f.raw))
	if err != nil {
		t.Fatal(err)
	}
	f.message = c.seed(t, "message", "message", MessageDocument{Kind: messageKind, Envelope: envelope, MIME: f.body})
	if receipt {
		f.receipt(t, []storage.Reference{f.mailbox, f.body, f.message})
	}
	return f
}

func (f *fixture) receipt(t *testing.T, refs []storage.Reference) {
	t.Helper()
	f.crypto.seed(t, "migration", testEpoch, ImportReceipt{Kind: migrationKind, Epoch: testEpoch, Records: refs})
}

func (f *fixture) open(t *testing.T) {
	t.Helper()
	if err := f.repo.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateMailRequiresCompleteImportedInventoryBeforeAnyContent(t *testing.T) {
	for _, fault := range []string{"absent", "early-marker", "missing-body", "duplicate-row"} {
		t.Run(fault, func(t *testing.T) {
			f := setup(t, false)
			switch fault {
			case "early-marker":
				f.receipt(t, []storage.Reference{f.mailbox})
			case "missing-body":
				f.receipt(t, []storage.Reference{f.mailbox, f.message})
			case "duplicate-row":
				f.receipt(t, []storage.Reference{f.mailbox, f.message, f.message})
			}
			if err := f.repo.Initialize(t.Context()); err == nil {
				t.Fatal("incomplete import marker activated private mailbox")
			}
			if _, err := f.repo.Mailboxes(t.Context()); !errors.Is(err, ErrNotReady) {
				t.Fatal("uninitialized account exposed mailbox", err)
			}
		})
	}
}

func TestPrivateMailReadsMessagesMIMEAndNanosecondCursorWithoutPlaintextAPI(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	boxes, err := f.repo.Mailboxes(t.Context())
	if err != nil || len(boxes) != 1 || boxes[0].UnreadEmails != 1 {
		t.Fatal("private inbox projection failed", err)
	}
	messages, next, err := f.repo.Messages(t.Context(), "inbox", "", 1)
	if err != nil || len(messages) != 1 || next != "" || messages[0].ReceivedAt.Nanosecond() != 123456789 {
		t.Fatal("private message metadata changed", err)
	}
	found, err := f.repo.Search(t.Context(), "reunion", 0, 50)
	if err != nil || len(found) != 1 {
		t.Fatal("private accent-insensitive search failed", err)
	}
	body, err := f.repo.Raw(t.Context(), "message")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(actual, f.raw) {
		t.Fatal("MIME body changed", err)
	}
}

func TestPrivateMailFlagAndFolderChangesRetainExactUnknownEnvelopeJSON(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	if err := f.repo.SetFlagMany(t.Context(), []string{"message"}, api.MailFlagSeen, true); err != nil {
		t.Fatal(err)
	}
	messages, _, err := f.repo.Messages(t.Context(), "inbox", "", 50)
	if err != nil || len(messages) != 1 || !messages[0].Has(api.MailFlagSeen) {
		t.Fatal("private flag did not update", err)
	}
	if err := f.repo.MoveMany(t.Context(), []string{"message"}, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal("message moved into nonexistent folder", err)
	}
	box, err := f.repo.CreateMailbox(t.Context(), "Private folder")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.MoveMany(t.Context(), []string{"message"}, box.ID); err != nil {
		t.Fatal(err)
	}
	if result, err := f.repo.RenameMailbox(t.Context(), box.ID, "Renamed"); err != nil || result != "" {
		t.Fatal(err, result)
	}
	s, err := storage.LoadAtomicCatalog(t.Context(), f.crypto, testScope)
	if err != nil {
		t.Fatal(err)
	}
	data, err := f.repo.document(t.Context(), s, "message", "message")
	if err != nil {
		t.Fatal(err)
	}
	defer clear(data)
	_, document, err := decodeMessage(data, "message")
	defer clear(document.Envelope)
	if err != nil || !bytes.Contains(document.Envelope, []byte(`900719925474099512345`)) {
		t.Fatal("private mutation rounded imported JSON", err)
	}
	if err := f.repo.SetFlagMany(t.Context(), []string{"message", "other"}, api.MailFlagSeen, true); !errors.Is(err, api.ErrPrivateMailOperation) {
		t.Fatal("batch was partially applied", err)
	}
}

func TestPrivateMailMIMEReaderIsClearedSynchronouslyWhenAccountCloses(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	body, err := f.repo.Raw(t.Context(), "message")
	if err != nil {
		t.Fatal(err)
	}
	reader := body.(*bodyReader)
	retained := reader.data
	f.repo.Close()
	if _, err := reader.Read(make([]byte, 1)); !errors.Is(err, ErrClosed) {
		t.Fatal("retired reader retained access", err)
	}
	if !bytes.Equal(retained, make([]byte, len(retained))) {
		t.Fatal("MIME copy survived account close")
	}
}

func TestPrivateMailLostIndexACKResumesWithoutDuplicatePublication(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	f.crypto.loseIndex = true
	if err := f.repo.SetSignature(t.Context(), "Private signature"); err == nil {
		t.Fatal("missing index acknowledgment accepted")
	}
	count := len(f.crypto.records)
	if err := f.repo.SetSignature(t.Context(), "Private signature"); err != nil {
		t.Fatal(err)
	}
	if len(f.crypto.records) != count {
		t.Fatal("retry duplicated encrypted signature revision")
	}
	signature, err := f.repo.Signature(t.Context())
	if err != nil || signature != "Private signature" {
		t.Fatal(err)
	}
}

func TestPrivateMailRejectsSharedIdentityAndMismatchedScope(t *testing.T) {
	f := setup(t, true)
	cfg := f.repo.cfg
	f.crypto.identity = storage.LegacyIdentity
	if _, err := New(cfg); !errors.Is(err, ErrRecord) {
		t.Fatal("legacy shared identity accepted", err)
	}
	f.crypto.identity = storage.IsolatedIdentity
	cfg.Scope.Product = "drive"
	if _, err := New(cfg); !errors.Is(err, ErrRecord) {
		t.Fatal("another product scope accepted", err)
	}
}

func TestPrivateMailRepeatedValuesAndReturningToAnOldValueUseFreshTransactions(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	for _, value := range []string{"first", "first", "second", "first"} {
		if err := f.repo.SetSignature(t.Context(), value); err != nil {
			t.Fatal("ordinary edit reused an obsolete Matrix transaction", err)
		}
	}
	actual, err := f.repo.Signature(t.Context())
	if err != nil || actual != "first" {
		t.Fatal("private edit reverted to an obsolete revision", err)
	}
}

func TestPrivateMailConcurrentDeleteAndEditNeverBecomeAChosenVisibleWinner(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	before := f.crypto.records[f.message]
	version, err := storage.DecodeObject(storage.RecordEntry{Reference: f.message, Record: before}, testScope)
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.crypto.ReadStorageRecord(t.Context(), version.Commit.Content.Reference, version.ObjectID, "content")
	if err != nil {
		t.Fatal(err)
	}
	var value MessageDocument
	if json.Unmarshal(content.Payload, &value) != nil {
		t.Fatal("invalid fixture message")
	}
	f.crypto.seed(t, "message", "message", value, f.message)
	_, err = f.crypto.PutStorageObjectAt(t.Context(), f.crypto.binding.Target, "concurrent-delete", version.ObjectID, "concurrent-delete", storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: version.Commit.Metadata, Parents: []storage.Reference{f.message}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.repo.Messages(t.Context(), "inbox", "", 50); !errors.Is(err, ErrConflict) {
		t.Fatal("deletion/edit conflict silently chose a branch", err)
	}
}

func (c *cryptoFixture) StorageFileCommitment(ctx context.Context, ref storage.Reference, object, revision string) (storage.FileRequest, string, error) {
	request, err := c.StorageFileInfo(ctx, ref, object, revision)
	if err != nil {
		return request, "", err
	}
	return request, digest(c.files[ref].data), nil
}

func (c *cryptoFixture) UploadStorageFileAt(_ context.Context, _ *api.Drive, _ *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, source io.ReaderAt) (storage.Reference, error) {
	if err := c.CheckStoragePublicationTarget(target); err != nil {
		return storage.Reference{}, err
	}
	data, err := io.ReadAll(io.NewSectionReader(source, 0, request.SizeBytes))
	if err != nil {
		return storage.Reference{}, err
	}
	if int64(len(data)) != request.SizeBytes {
		return storage.Reference{}, ErrRecord
	}
	key := job + "_file"
	if ref, found := c.transactions[key]; found {
		old := c.files[ref]
		if !bytes.Equal(old.data, data) {
			return storage.Reference{}, ErrRecord
		}
		return ref, nil
	}
	ref := storage.Reference{RoomID: target.RoomID, EventID: "$file-" + job}
	c.files[ref] = fileFixture{request: request, data: bytes.Clone(data)}
	c.transactions[key] = ref
	return ref, nil
}

func (c *cryptoFixture) AcknowledgeStorageFile(_ context.Context, _ *offline.Store, _ string, catalog storage.Reference) error {
	record, found := c.records[catalog]
	if !found {
		return ErrRecord
	}
	version, err := storage.DecodeObject(storage.RecordEntry{Reference: catalog, Record: record}, testScope)
	if err != nil {
		return err
	}
	if version.Commit.Content == nil || version.Commit.Content.Kind != "file" {
		return ErrRecord
	}
	return nil
}

func TestPrivateMailStoresMIMEAndEnvelopeThroughMaintainedSDKPorts(t *testing.T) {
	f := setup(t, true)
	f.open(t)
	raw := []byte("Subject: new private message\r\n\r\nprivate authorized-device ingress")
	value := api.MailMessage{ID: "received", MailboxID: "inbox", ThreadID: "received-thread", Subject: "Private subject", From: "external@example.test", To: []string{"alice@example.test"}, Size: int64(len(raw)), ReceivedAt: time.Date(2026, 10, 30, 0, 0, 0, 123456789, time.UTC)}
	envelope, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	f.crypto.loseIndex = true
	if err := f.repo.StoreMessage(t.Context(), "ingress-operation", envelope, bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("lost MIME catalog ACK accepted")
	}
	count := len(f.crypto.files)
	if err := f.repo.StoreMessage(t.Context(), "ingress-operation", envelope, bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatal(err)
	}
	if len(f.crypto.files) != count {
		t.Fatal("resumed MIME transaction duplicated ciphertext source")
	}
	if err := f.repo.StoreMessage(t.Context(), "ingress-operation", envelope, bytes.NewReader(raw), int64(len(raw))); err != nil {
		t.Fatal("completed ingress operation did not remain idempotent", err)
	}
	if len(f.crypto.files) != count {
		t.Fatal("completed ingress operation uploaded another MIME copy")
	}
	messages, _, err := f.repo.Messages(t.Context(), "inbox", "", 50)
	if err != nil || len(messages) != 2 {
		t.Fatal("private ingress did not appear in mailbox", err)
	}
	body, err := f.repo.Raw(t.Context(), "received")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(actual, raw) {
		t.Fatal("stored MIME did not roundtrip", err)
	}
	changed := bytes.Clone(raw)
	changed[0] ^= 1
	f.crypto.loseIndex = true
	if err := f.repo.StoreMessage(t.Context(), "another-operation", envelope, bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("fixture did not interrupt")
	}
	if err := f.repo.StoreMessage(t.Context(), "another-operation", envelope, bytes.NewReader(changed), int64(len(changed))); err == nil {
		t.Fatal("changed MIME resumed old operation")
	}
}

func (c *cryptoFixture) StorageObject(ctx context.Context, ref storage.Reference, object string) (storage.ObjectVersion, error) {
	record, err := c.ReadStorageRecord(ctx, ref, object, "index")
	if err != nil {
		return storage.ObjectVersion{}, err
	}
	return storage.DecodeObject(storage.RecordEntry{Reference: ref, Record: record}, testScope)
}

func replaceTestCrypto(t *testing.T, repository *Repository, crypto Crypto) {
	t.Helper()
	sdk, ok := crypto.(filesdk.SDK)
	if !ok {
		t.Fatal("fixture lacks the real file SDK surface")
	}
	files, err := filesdk.New(repository.cfg.Scope, sdk, repository.cfg.Gateway, repository.cfg.Journal)
	if err != nil {
		t.Fatal(err)
	}
	repository.cfg.Crypto = crypto
	repository.cfg.Files = files
}
