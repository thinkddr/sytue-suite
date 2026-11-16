// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

// This source deliberately exposes no SQL bodies. The test migrates a complete
// actual legacy SDK catalog through the production Mail domain adapter.
type mailIdentitySource struct{ state storage.AppMigrationState }

func (s mailIdentitySource) PrivateStorageCapabilities(context.Context, storage.Scope) (storage.AppMigrationState, error) {
	return s.state, nil
}

func (s mailIdentitySource) PrivateStorageState(context.Context, storage.Scope) (storage.LegacyMigrationState, error) {
	return s.state.LegacyMigrationState, nil
}

func (mailIdentitySource) PrivateStorageSourcePage(context.Context, storage.Scope, string, storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	return storage.StreamSourcePage{}, errors.New("SQL pages must never replace the E2E catalog")
}

func (mailIdentitySource) OpenPrivateStorageSource(context.Context, storage.Scope, string, storage.StreamSourceRow) (io.ReadCloser, error) {
	return nil, errors.New("SQL originals must never replace the E2E catalog")
}

type mailIdentityTransferDevice struct {
	*Client
	drop          *atomic.Bool
	firstDocument bool
	beforeReceipt func(context.Context)
}

func (c *mailIdentityTransferDevice) UploadStorageFileAt(ctx context.Context, files *api.Drive, journal *offline.Store, job string, target storage.PublicationTarget, request storage.FileRequest, body io.ReaderAt) (storage.Reference, error) {
	if !c.firstDocument {
		c.firstDocument = true
		c.drop.Store(true)
	}
	return c.Client.UploadStorageFileAt(ctx, files, journal, job, target, request, body)
}

func (c *mailIdentityTransferDevice) PutStorageObjectAt(ctx context.Context, target storage.PublicationTarget, job, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
	var h struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(commit.Metadata, &h) == nil && h.Type == "migration" && c.beforeReceipt != nil {
		c.beforeReceipt(ctx)
	}
	return c.Client.PutStorageObjectAt(ctx, target, job, object, revision, commit)
}

type mailCutoverReplies struct {
	*driveCutoverRouting
	lost bool
}

func (r *mailCutoverReplies) CompleteMatrixStorageMigration(ctx context.Context, scope storage.Scope, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	accepted, err := r.driveCutoverRouting.CompleteMatrixStorageMigration(ctx, scope, epoch, receipt)
	if err == nil && !r.lost {
		r.lost = true
		return storage.IdentityCutoverState{}, errors.New("accepted final routing reply lost")
	}
	return accepted, err
}

func TestMailIdentityTransferUsesRepositoryAndFreshRecoveryAfterStructuredFileACKLoss(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_TRANSFER_DOCUMENT_PROOF") != "1" {
		t.Skip("requires disposable Synapse")
	}
	endpoint, err := matrixprooftest.DisposableHTTPOrigin(os.Getenv("SYTUE_MATRIX_TRANSFER_DOCUMENT_URL"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	forward := httputil.NewSingleHostReverseProxy(endpoint)
	var drop atomic.Bool
	forward.ModifyResponse = func(response *http.Response) error {
		if response.StatusCode == http.StatusOK && response.Request.Method == http.MethodPut && strings.Contains(response.Request.URL.Path, "/send/m.room.encrypted/") && drop.CompareAndSwap(true, false) {
			_ = response.Body.Close()
			response.StatusCode = http.StatusBadRequest
			response.Body = io.NopCloser(strings.NewReader(`{"errcode":"M_UNKNOWN","error":"synthetic accepted Mail document reply lost"}`))
			response.ContentLength = -1
			response.Header.Del("Content-Length")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(forward)
	defer func() { cancel(); proxy.Close() }()
	scope := storage.Scope{Tenant: "mail-transfer-proof", Subject: uuid.NewString(), Product: "mail"}
	previous, controller := controlledMigrationSource(ctx, t, proxy.URL, proxy.Client(), scope)
	destination := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	gateway := &fileGatewayFixture{product: "mail"}
	server := httptest.NewServer(gateway)
	defer server.Close()
	files := api.NewDrive(server.URL, nil, server.Client())
	oldJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	inbox := seedPrivateMailboxRecord(ctx, t, previous.Client, "mailbox", "inbox", []byte(`{"id":"inbox","name":"Inbox","role":"inbox"}`))
	seedFile := func(typ, key string, body []byte) storage.Reference {
		t.Helper()
		object := mailFixtureObject(typ, key)
		revision := uuid.NewString()
		job := "mail_source_" + revision
		ref, err := previous.Client.UploadStorageFile(ctx, files, oldJournal, job, storage.FileRequest{ObjectID: object, RevisionID: revision, SizeBytes: int64(len(body)), Metadata: []byte(`{"kind":"org.sytue.mail.document.v1"}`)}, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		metadata, _ := json.Marshal(map[string]string{"kind": "org.sytue.mail.entry.v1", "type": typ, "id": key})
		version, err := previous.Client.PutStorageObject(ctx, job+"_index", object, revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: revision, Reference: ref}})
		if err != nil {
			t.Fatal(err)
		}
		if err := previous.Client.AcknowledgeStorageFile(ctx, oldJournal, job, version.Reference); err != nil {
			t.Fatal(err)
		}
		return version.Reference
	}
	mime := seedFile("mime", "baseline", []byte(transferDocumentMIME))
	envelope, _ := json.Marshal(map[string]any{"id": "baseline", "mailbox_id": "inbox", "thread_id": "thread", "subject": "private subject", "size": len(transferDocumentMIME), "received_at": "2026-10-30T00:00:00.123456789Z", "future": json.RawMessage(`{"precise":900719925474099512345}`), "padding": strings.Repeat("e", storage.MaxRecordBytes+128)})
	document, _ := json.Marshal(privateMail.MessageDocument{Kind: "org.sytue.mail.message.v1", Envelope: envelope, MIME: mime})
	message := seedFile("message", "baseline", document)
	epoch := uuid.NewString()
	receipt, _ := json.Marshal(map[string]any{"kind": "org.sytue.mail.migration.v1", "epoch": epoch, "records": []storage.Reference{inbox, mime, message}, "future": json.RawMessage(`900719925474099512345`), "padding": strings.Repeat("r", storage.MaxRecordBytes+128)})
	originalReceipt := seedFile("migration", epoch, receipt)
	started := time.Now().UTC()
	state := storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &started, Rows: 2, Blobs: 1}
	retained := mailIdentitySource{state: storage.AppMigrationState{Version: 1, LegacyMigrationState: state, MaxBodyBytes: 50 << 20, PrivateIngressReady: true}}
	lease := func(ctx context.Context) (func(), error) { return func() {}, ctx.Err() } // Synthetic session witness; SDK identities/recovery are real.
	source, err := privateMail.NewHistoricalSource(privateMail.Config{Context: ctx, WorkLease: lease, Scope: scope, Crypto: previous.Client, Gateway: files, Journal: oldJournal, MigrationState: state}, retained)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := source.Prepare(ctx, storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{previous.Room.String()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.SendStateEvent(ctx, previous.Room, event.StatePowerLevels, "", migrationFixturePowers(true)); err != nil {
		t.Fatal(err)
	}
	if _, err := previous.Client.FrozenStorageBindings(ctx); err != nil {
		t.Fatal(err)
	}
	secrets := &authtest.Secrets{}
	directory := t.TempDir()
	journal, err := offline.OpenWithSecrets(directory, secrets)
	if err != nil {
		t.Fatal(err)
	}
	device := &mailIdentityTransferDevice{Client: destination.Client, drop: &drop}
	config := privateMail.Config{Context: ctx, WorkLease: lease, Scope: scope, Crypto: device, Gateway: files, Journal: journal, MigrationState: state}
	repository, err := privateMail.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { repository.Close() }()
	receiptChecks := 0
	device.beforeReceipt = func(ctx context.Context) {
		receiptChecks++
		assertMailTransferData(ctx, t, device.Client, files, scope, envelope)
	}
	copyFile := func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
		return previous.Client.MigrateStorageFileContentAt(ctx, device.Client, files, journal, job, target, ref, object, revision)
	}
	routing := &mailCutoverReplies{driveCutoverRouting: &driveCutoverRouting{state: storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "frozen", Epoch: uuid.NewString(), SourceRooms: []string{previous.Room.String()}}}}
	newCutover := func() *transfer.Cutover {
		cutover, err := repository.IdentityCutover(privateMail.IdentityCutoverConfig{Scope: scope, Service: routing, Legacy: source, CopyFile: copyFile, Authorize: func(ctx context.Context) error { return ctx.Err() }})
		if err != nil {
			t.Fatal(err)
		}
		return cutover
	}
	cutover := newCutover()
	if err := cutover.Prepare(ctx); err == nil || drop.Load() {
		t.Fatal("structured document ACK was not lost", err)
	}
	if receiptChecks != 0 {
		t.Fatal("partial document transfer published a receipt")
	}
	if _, err := repository.Mailboxes(ctx); !errors.Is(err, privateMail.ErrNotReady) {
		t.Fatal("partial transfer exposed Mail data", err)
	}
	repository.Close()
	if err := destination.Client.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, destination.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	go func() { _ = reopened.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := reopened.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	destination.Client = reopened
	device.Client = reopened
	journal, err = offline.OpenWithSecrets(directory, secrets)
	if err != nil {
		t.Fatal(err)
	}
	config.Journal = journal
	repository, err = privateMail.New(config)
	if err != nil {
		t.Fatal(err)
	}
	cutover = newCutover()
	if err := cutover.Prepare(ctx); err == nil || !routing.lost || routing.state.Phase != "isolated" {
		t.Fatal("accepted final routing reply was not lost", err)
	}
	pending, err := repository.Pending(ctx)
	if err != nil || pending.Count == 0 {
		t.Fatal("uncertain final routing erased the original transfer WAL", pending, err)
	}
	if err := cutover.Prepare(ctx); err != nil {
		t.Fatal("production Mail cutover did not reconcile accepted routing against full SDK receipt", err)
	}
	pending, err = repository.Pending(ctx)
	if err != nil || pending.Count != 0 {
		t.Fatal("acknowledged cutover kept active transfer markers", pending, err)
	}
	archived, err := journal.CountOwner(scope.Tenant+"\x00"+scope.Subject+"\x00mail", storage.IdentityHistoryNamespace("mail"))
	if err != nil || archived == 0 {
		t.Fatal("cutover retired original evidence without encrypted archive", archived, err)
	}
	if receiptChecks == 0 {
		t.Fatal("fixture never checked final receipt ordering")
	}
	assertMailRepositoryContent(ctx, t, repository, envelope)
	if gateway.puts != 6 {
		t.Fatal("accepted structured document upload was duplicated", gateway.puts)
	}
	if err := previous.Client.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := freshDocumentDevice(ctx, t, proxy, destination, scope)
	defer fresh.Close()
	freshJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	config.Crypto = fresh
	config.Journal = freshJournal
	freshRepository, err := privateMail.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer freshRepository.Close()
	if err := freshRepository.Initialize(ctx); err != nil {
		t.Fatal("fresh SSSS-recovered device did not validate transferred Mail receipt", err)
	}
	assertMailRepositoryContent(ctx, t, freshRepository, envelope)
	assertDocumentGatewayOpaque(t, gateway)
	if _, err := previous.Matrix.GetEvent(ctx, previous.Room, id.EventID(originalReceipt.EventID)); err != nil {
		t.Fatal("source receipt was removed", err)
	}
	t.Log("MAIL_REPOSITORY_TRANSFER_SDK_FILE_ACKLOSS_FRESH_SSSS_PASS; production Mail adapter+shared cutover, exact envelope/MIME, large receipt, final routing ACK loss, encrypted evidence retained, no SQL fallback")
}

func assertMailTransferData(ctx context.Context, t *testing.T, client *Client, files *api.Drive, scope storage.Scope, envelope []byte) {
	t.Helper()
	snapshot, err := storage.LoadCatalog(ctx, client, scope)
	if err != nil {
		t.Fatal(err)
	}
	var version storage.ObjectVersion
	for _, head := range snapshot.Heads {
		if head.ObjectID == mailFixtureObject("message", "baseline") {
			version = head
		}
	}
	if !version.Reference.Valid() || version.Commit.Content == nil || version.Commit.Content.Kind != "file" {
		t.Fatal("receipt preceded large Mail message file")
	}
	var out documentTransferBuffer
	out.limit = 1 << 20
	_, err = client.DownloadStorageFile(ctx, files, version.Commit.Content.Reference, version.ObjectID, version.Commit.Content.RevisionID, &out)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(out.Bytes())
	var message privateMail.MessageDocument
	if json.Unmarshal(out.Bytes(), &message) != nil || !bytes.Equal(message.Envelope, envelope) || message.MIME.RoomID != version.Reference.RoomID {
		t.Fatal("message envelope changed or MIME remained on source")
	}
	defer clear(message.Envelope)
	entry, ok := snapshot.Entries[message.MIME]
	if !ok {
		t.Fatal("receipt preceded MIME index")
	}
	body, err := storage.DecodeObject(entry, scope)
	if err != nil {
		t.Fatal(err)
	}
	var mime documentTransferBuffer
	mime.limit = 1 << 20
	_, err = client.DownloadStorageFile(ctx, files, body.Commit.Content.Reference, body.ObjectID, body.Commit.Content.RevisionID, &mime)
	if err != nil || mime.String() != transferDocumentMIME {
		t.Fatal("receipt preceded exact MIME bytes", err)
	}
	clear(mime.Bytes())
}

func assertMailRepositoryContent(ctx context.Context, t *testing.T, repository *privateMail.Repository, envelope []byte) {
	t.Helper()
	boxes, err := repository.Mailboxes(ctx)
	if err != nil || len(boxes) != 1 {
		t.Fatal("transferred repository lacks mailbox", boxes, err)
	}
	messages, _, err := repository.Messages(ctx, "inbox", "", 50)
	if err != nil || len(messages) != 1 || messages[0].ID != "baseline" {
		t.Fatal("transferred repository lacks message", messages, err)
	}
	body, err := repository.Raw(ctx, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(body)
	_ = body.Close()
	defer clear(raw)
	if err != nil || string(raw) != transferDocumentMIME {
		t.Fatal("Mail facade changed original MIME", err)
	}
	var expected api.MailMessage
	if json.Unmarshal(envelope, &expected) != nil || messages[0].Subject != expected.Subject || messages[0].ReceivedAt != expected.ReceivedAt {
		t.Fatal("Mail facade changed original metadata")
	}
}
