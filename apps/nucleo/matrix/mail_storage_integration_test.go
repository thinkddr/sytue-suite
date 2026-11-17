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
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	privateMail "git.sytue.eu/sytue/one/apps/nucleo/mail"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func mailFixtureObject(typ, key string) string { return "mail_" + digestBytes([]byte(typ+"\x00"+key)) }

func seedPrivateMailboxRecord(ctx context.Context, t *testing.T, c *Client, typ, key string, payload []byte) storage.Reference {
	t.Helper()
	object := mailFixtureObject(typ, key)
	revision := uuid.NewString()
	record := storage.Record{Version: 1, Scope: c.storageScope(), ObjectID: object, RevisionID: revision, Role: "content", Payload: payload}
	content, err := writeStorageRecordVerified(ctx, c, "mail-seed-"+revision, record)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]string{"kind": "org.sytue.mail.entry.v1", "type": typ, "id": key})
	if err != nil {
		t.Fatal(err)
	}
	index, err := c.PutStorageObject(ctx, "mail-index-"+revision, object, revision+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "record", RevisionID: revision, Reference: content}})
	if err != nil {
		t.Fatal(err)
	}
	return index.Reference
}

func TestPrivateMailAgainstSynapseStoresMIMEAndRecoversInFreshDevice(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_MAIL") != "1" {
		t.Skip("requires disposable remote Synapse")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	upstreamURL, _ := url.Parse("http://127.0.0.1:18585")
	upstream := httputil.NewSingleHostReverseProxy(upstreamURL)
	var drop atomic.Bool
	upstream.ModifyResponse = func(r *http.Response) error {
		if r.StatusCode == http.StatusOK && r.Request.Method == http.MethodPut && strings.Contains(r.Request.URL.Path, "/send/m.room.encrypted/") && drop.CompareAndSwap(true, false) {
			_ = r.Body.Close()
			r.StatusCode = http.StatusBadRequest
			r.Body = io.NopCloser(strings.NewReader(`{"errcode":"M_UNKNOWN","error":"synthetic lost encrypted Mail acknowledgment"}`))
			r.ContentLength = -1
			r.Header.Del("Content-Length")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(upstream)
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "mail"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	gateway := &fileGatewayFixture{product: "mail"}
	blobServer := httptest.NewServer(gateway)
	t.Cleanup(blobServer.Close)
	blobs := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Date(2026, 10, 30, 0, 1, 2, 123456789, time.UTC)
	epoch := uuid.NewString()
	source := storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: epoch, StartedAt: &started, Rows: 2, Blobs: 1}
	open := func(c *Client, privateJournal *offline.Store) *privateMail.Repository {
		t.Helper()
		r, err := privateMail.New(privateMail.Config{Context: ctx, Scope: scope, Crypto: c, Gateway: blobs, Journal: privateJournal, MigrationState: source})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	repository := open(device.Client, journal)
	defer func() { repository.Close() }()
	if err = repository.Initialize(ctx); !errors.Is(err, privateMail.ErrNotReady) {
		t.Fatal("empty private account activated", err)
	}
	inbox := seedPrivateMailboxRecord(ctx, t, device.Client, "mailbox", "inbox", []byte(`{"id":"inbox","name":"Inbox","role":"inbox","extension":{"integer":900719925474099512345}}`))
	raw := []byte("Subject: Private imported mail\r\nFrom: sender@example.test\r\nTo: owner@example.test\r\n\r\nprivate imported MIME body")
	bodyObject := mailFixtureObject("mime", "baseline")
	fileRequest := storage.FileRequest{ObjectID: bodyObject, RevisionID: "baseline-mime", SizeBytes: int64(len(raw)), Metadata: []byte(`{"type":"message/rfc822"}`)}
	target, err := device.Client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	file, err := device.Client.UploadStorageFileAt(ctx, blobs, journal, "mail-baseline", target, fileRequest, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(map[string]string{"kind": "org.sytue.mail.entry.v1", "type": "mime", "id": "baseline"})
	body, err := device.Client.PutStorageObjectAt(ctx, target, "mail-body-index", bodyObject, "baseline-mime-index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: fileRequest.RevisionID, Reference: file}})
	if err != nil {
		t.Fatal(err)
	}
	if err = device.Client.AcknowledgeStorageFile(ctx, journal, "mail-baseline", body.Reference); err != nil {
		t.Fatal(err)
	}
	envelope, _ := json.Marshal(map[string]any{"id": "baseline", "mailbox_id": "inbox", "thread_id": "thread", "subject": "Reunión privada", "from": "sender@example.test", "to": []string{"owner@example.test"}, "size": len(raw), "received_at": started, "extension": json.RawMessage(`{"precise":900719925474099512345}`)})
	document, _ := json.Marshal(privateMail.MessageDocument{Kind: "org.sytue.mail.message.v1", Envelope: envelope, MIME: body.Reference})
	message := seedPrivateMailboxRecord(ctx, t, device.Client, "message", "baseline", document)
	receipt, _ := json.Marshal(privateMail.ImportReceipt{Kind: "org.sytue.mail.migration.v1", Epoch: epoch, Records: []storage.Reference{inbox, body.Reference, message}})
	seedPrivateMailboxRecord(ctx, t, device.Client, "migration", epoch, receipt)
	if err = repository.Initialize(ctx); err != nil {
		t.Fatal("complete imported mail inventory", err)
	}
	facade, err := api.NewMail(blobServer.URL, nil, blobServer.Client()).WithPrivateData(repository)
	if err != nil {
		t.Fatal(err)
	}
	boxes, err := facade.Mailboxes(ctx)
	if err != nil || len(boxes) != 1 || boxes[0].UnreadEmails != 1 {
		t.Fatal("private mailbox projection", err)
	}
	if err = facade.SetFlagMany(ctx, []string{"baseline"}, api.MailFlagSeen, true); err != nil {
		t.Fatal(err)
	}
	reader, err := facade.Raw(ctx, "baseline")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(data, raw) {
		t.Fatal("private imported MIME changed", err)
	}
	clear(data)
	incoming := []byte("Subject: Later mail\r\nFrom: sender@example.test\r\nTo: owner@example.test\r\n\r\nprivate new MIME through the authorized device")
	incomingEnvelope, _ := json.Marshal(map[string]any{"id": "new-mail", "mailbox_id": "inbox", "thread_id": "new-thread", "subject": "Private new mail", "from": "sender@example.test", "to": []string{"owner@example.test"}, "size": len(incoming), "received_at": started.Add(time.Second)})
	drop.Store(true)
	if err = repository.StoreMessage(ctx, "stable-new-mail", incomingEnvelope, bytes.NewReader(incoming), int64(len(incoming))); err == nil || drop.Load() {
		t.Fatal("fixture did not lose an accepted encrypted Mail write", err)
	}
	repository.Close()
	repository = open(device.Client, journal)
	if err = repository.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if err = repository.StoreMessage(ctx, "stable-new-mail", incomingEnvelope, bytes.NewReader(incoming), int64(len(incoming))); err != nil {
		t.Fatal("MIME WAL did not resume", err)
	}
	puts := gateway.puts
	if err = repository.StoreMessage(ctx, "stable-new-mail", incomingEnvelope, bytes.NewReader(incoming), int64(len(incoming))); err != nil || gateway.puts != puts {
		t.Fatal("completed MIME retry uploaded again", err)
	}
	if err = device.Client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	fresh := freshMailStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)
	freshJournal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	recovered := open(fresh, freshJournal)
	defer recovered.Close()
	if err = recovered.Initialize(ctx); err != nil {
		t.Fatal("fresh mailbox inventory", err)
	}
	reader, err = recovered.Raw(ctx, "new-mail")
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(data, incoming) {
		t.Fatal("fresh device did not recover new MIME", err)
	}
	clear(data)
	boxes, err = recovered.Mailboxes(ctx)
	if err != nil || len(boxes) != 1 || boxes[0].TotalEmails != 2 || boxes[0].UnreadEmails != 1 {
		t.Fatal("fresh private mailbox mutations lost", boxes, err)
	}
	t.Log("real Synapse: final imported inventory, private API projection and seen mutation, SDK-encrypted MIME ingress with lost ACK/WAL retry, no duplicate ciphertext, and fresh isolated-device recovery verified; SMTP/IMAP importer and UI activation remain separate")
}

// Shared real recovery sequence for Mail SDK conformance fixtures.
func freshMailStorageDevice(ctx context.Context, t *testing.T, base string, httpClient *http.Client, original storageSynapseDevice) *Client {
	t.Helper()
	anonymous, err := mautrix.NewClient(base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Client = httpClient
	login, err := anonymous.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: original.Matrix.UserID.String()}, Password: original.Password, DeviceID: "MAIL_FRESH"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := original.Config
	cfg.StoreDir = t.TempDir()
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: base, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant})
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Recover(ctx, original.Recovery); err != nil {
		t.Fatal("fresh private Mail recovery", err)
	}
	return fresh
}
