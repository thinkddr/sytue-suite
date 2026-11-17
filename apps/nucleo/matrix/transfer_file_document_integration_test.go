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
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/transfer"
)

const (
	transferDocumentKind = "org.sytue.mail.entry.v1"
	transferDocumentMIME = "From: sender@example.invalid\r\nSubject: PRIVATE-TRANSFER-MIME\r\n\r\nOriginal bytes / 漢字\r\n"
)

type documentTransferRules struct {
	source, target *Client
	scope          storage.Scope
	lookup         transfer.Lookup
	verified       map[string]bool
	ready          bool
}

func (r *documentTransferRules) SourceComplete(ctx context.Context) error {
	snapshot, err := storage.LoadCatalog(ctx, r.source, r.scope)
	if err != nil {
		return err
	}
	if len(snapshot.Heads) != 3 {
		return transfer.ErrSource
	}
	return nil
}

func (r *documentTransferRules) TargetComplete(ctx context.Context) error {
	snapshot, err := storage.LoadCatalog(ctx, r.target, r.scope)
	if err != nil {
		return err
	}
	if len(snapshot.Heads) != 3 || !r.verified["mime"] || !r.verified["message"] || !r.verified["receipt"] {
		return transfer.ErrSource
	}
	return nil
}

func (r *documentTransferRules) Classify(_ context.Context, version storage.ObjectVersion, lookup transfer.Lookup) (transfer.Class, error) {
	r.lookup = lookup
	var header struct{ Kind, Type string }
	if json.Unmarshal(version.Commit.Metadata, &header) != nil || header.Kind != transferDocumentKind {
		return transfer.Class{}, transfer.ErrSource
	}
	switch header.Type {
	case "mime":
		return transfer.Class{Order: 0}, nil
	case "message":
		return transfer.Class{Order: 1}, nil
	case "migration":
		return transfer.Class{Order: 2}, nil
	default:
		return transfer.Class{}, transfer.ErrSource
	}
}

func (*documentTransferRules) Rewrite(context.Context, storage.ObjectVersion, storage.Record, transfer.Resolve) ([]byte, error) {
	return nil, transfer.ErrSource
}

func (*documentTransferRules) CheckPending(context.Context, storage.FrozenBinding, storage.Binding) error {
	return nil
}

func (*documentTransferRules) FileDocumentLimit(_ context.Context, version storage.ObjectVersion) (int64, error) {
	if version.ObjectID == "message" || version.ObjectID == "receipt" {
		return 4 << 20, nil
	}
	return 0, nil
}

func (r *documentTransferRules) RewriteFileDocument(_ context.Context, version storage.ObjectVersion, raw []byte, resolve transfer.Resolve) ([]byte, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, transfer.ErrSource
	}
	switch version.ObjectID {
	case "message":
		var old storage.Reference
		if json.Unmarshal(fields["mime"], &old) != nil {
			return nil, transfer.ErrSource
		}
		next, err := resolve(old, "mime")
		if err != nil {
			return nil, err
		}
		fields["mime"], err = json.Marshal(next)
		if err != nil {
			return nil, err
		}
	case "receipt":
		var refs []storage.Reference
		if json.Unmarshal(fields["records"], &refs) != nil || len(refs) != 2 {
			return nil, transfer.ErrSource
		}
		for i, ref := range refs {
			original, ok := r.lookup(ref)
			if !ok {
				return nil, transfer.ErrSource
			}
			next, err := resolve(ref, original.ObjectID)
			if err != nil {
				return nil, err
			}
			refs[i] = next
		}
		var err error
		fields["records"], err = json.Marshal(refs)
		if err != nil {
			return nil, err
		}
	default:
		return nil, transfer.ErrSource
	}
	return json.Marshal(fields)
}

type documentTransferBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *documentTransferBuffer) Write(raw []byte) (int, error) {
	if int64(len(raw)) > b.limit-int64(b.Len()) {
		return 0, transfer.ErrSource
	}
	return b.Buffer.Write(raw)
}

func TestTransferStructuredFilesRewritesReferencesBeforeReceiptAndRecoversAfterLostACK(t *testing.T) {
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
			response.Body = io.NopCloser(strings.NewReader(`{"errcode":"M_UNKNOWN","error":"synthetic accepted document reply lost"}`))
			response.ContentLength = -1
			response.Header.Del("Content-Length")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(forward)
	defer func() { cancel(); proxy.Close() }()
	scope := storage.Scope{Tenant: "document-proof", Subject: uuid.NewString(), Product: "mail"}
	source, controller := controlledMigrationSource(ctx, t, proxy.URL, proxy.Client(), scope)
	destination := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	gateway := &fileGatewayFixture{product: "mail"}
	blobServer := httptest.NewServer(gateway)
	defer blobServer.Close()
	files := api.NewDrive(blobServer.URL, nil, blobServer.Client())
	secrets := &authtest.Secrets{}
	dir := t.TempDir()
	journal, err := offline.OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	seed := func(object, typ string, payload []byte) storage.Reference {
		t.Helper()
		ref, err := source.Client.UploadStorageFile(ctx, files, journal, "source_"+object, storage.FileRequest{ObjectID: object, RevisionID: object + "_body", SizeBytes: int64(len(payload)), Metadata: []byte(`{"kind":"private document"}`)}, bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		metadata, _ := json.Marshal(map[string]string{"kind": transferDocumentKind, "type": typ})
		version, err := source.Client.PutStorageObject(ctx, "source_index_"+object, object, object+"_index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: object + "_body", Reference: ref}})
		if err != nil {
			t.Fatal(err)
		}
		if err := source.Client.AcknowledgeStorageFile(ctx, journal, "source_"+object, version.Reference); err != nil {
			t.Fatal(err)
		}
		return version.Reference
	}
	mime := seed("mime", "mime", []byte(transferDocumentMIME))
	envelope := []byte(`{"subject":"private subject","future":9007199254740993}`)
	messageRaw, err := json.Marshal(map[string]any{"mime": mime, "envelope": envelope, "padding": strings.Repeat("x", storage.MaxRecordBytes+128), "extension": json.RawMessage(`{"precise":900719925474099512345}`)})
	if err != nil {
		t.Fatal(err)
	}
	message := seed("message", "message", messageRaw)
	receiptRaw, _ := json.Marshal(map[string]any{"records": []storage.Reference{mime, message}, "future": json.RawMessage(`900719925474099512345`), "padding": strings.Repeat("r", storage.MaxRecordBytes+128)})
	_ = seed("receipt", "migration", receiptRaw)
	if _, err := controller.SendStateEvent(ctx, source.Room, event.StatePowerLevels, "", migrationFixturePowers(true)); err != nil {
		t.Fatal(err)
	}
	frozen, err := source.Client.FrozenStorageBindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := destination.Client.PrepareStorage(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	rules := &documentTransferRules{source: source.Client, target: destination.Client, scope: scope, verified: map[string]bool{}}
	putDocuments := 0
	makeConfig := func() transfer.Config {
		return transfer.Config{
			Scope: scope, Owner: scope.Tenant + "\x00" + scope.Subject + "\x00mail", Namespace: "mail-document-transfer-proof-v1", ReceiptKind: transferDocumentKind, ReceiptType: "migration", Journal: journal, Source: source.Client, Target: destination.Client, FrozenBindings: source.Client.FrozenStorageBindings, Rules: rules,
			PutRecord: func(context.Context, storage.PublicationTarget, string, storage.Record) (storage.Reference, error) {
				return storage.Reference{}, transfer.ErrSource
			},
			PutObject: func(ctx context.Context, target storage.PublicationTarget, job, object, revision string, commit storage.ObjectCommit) (storage.ObjectVersion, error) {
				if object == "receipt" && (!rules.verified["mime"] || !rules.verified["message"]) {
					return storage.ObjectVersion{}, errors.New("receipt appeared before data and MIME readback")
				}
				return destination.Client.PutStorageObjectAt(ctx, target, job, object, revision, commit)
			},
			CopyFile: func(ctx context.Context, target storage.PublicationTarget, job string, ref storage.Reference, object, revision string) (storage.Reference, error) {
				return source.Client.MigrateStorageFileContentAt(ctx, destination.Client, files, journal, job, target, ref, object, revision)
			},
			SourceFileCommitment: source.Client.StorageFileCommitment, TargetFileCommitment: destination.Client.StorageFileCommitment,
			VerifyFile: func(ctx context.Context, ref storage.Reference, object, revision string) error {
				_, err := destination.Client.DownloadStorageFile(ctx, files, ref, object, revision, io.Discard)
				if err == nil {
					rules.verified[object] = true
				}
				return err
			},
			AcknowledgeFile: func(ctx context.Context, job string, ref storage.Reference) error {
				return destination.Client.AcknowledgeStorageFile(ctx, journal, job, ref)
			},
			PrepareTarget: func(context.Context) error { return nil }, MarkReady: func(context.Context) error { rules.ready = true; return nil },
			ReadFileDocument: func(ctx context.Context, version storage.ObjectVersion, limit int64) ([]byte, error) {
				var out documentTransferBuffer
				out.limit = limit
				_, err := source.Client.DownloadStorageFile(ctx, files, version.Commit.Content.Reference, version.ObjectID, version.Commit.Content.RevisionID, &out)
				if err != nil {
					clear(out.Bytes())
					return nil, err
				}
				return out.Bytes(), nil
			},
			PutFileDocument: func(ctx context.Context, target storage.PublicationTarget, job string, request storage.FileRequest, payload io.ReaderAt) (storage.Reference, error) {
				putDocuments++
				if putDocuments == 1 {
					drop.Store(true)
				}
				return destination.Client.UploadStorageFileAt(ctx, files, journal, job, target, request, payload)
			},
		}
	}
	if err := transfer.Run(ctx, makeConfig(), frozen, bindings); err == nil || drop.Load() || rules.ready {
		t.Fatal("accepted document ACK loss did not retain a partial unready transfer", err)
	}
	partial, err := storage.LoadCatalog(ctx, destination.Client, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, head := range partial.Heads {
		if head.ObjectID == "receipt" {
			t.Fatal("partial transfer exposed its completion receipt")
		}
	}
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
	rules.target = reopened
	journal, err = offline.OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	if err := transfer.Run(ctx, makeConfig(), frozen, bindings); err != nil {
		t.Fatal("structured transfer did not resume its original SDK/WAL jobs", err)
	}
	if !rules.ready {
		t.Fatal("complete transfer was not marked ready")
	}
	assertTransferredDocuments(ctx, t, destination.Client, files, scope, envelope)
	if err := source.Client.Close(); err != nil {
		t.Fatal(err)
	}
	fresh := freshDocumentDevice(ctx, t, proxy, destination, scope)
	assertTransferredDocuments(ctx, t, fresh, files, scope, envelope)
	assertDocumentGatewayOpaque(t, gateway)
	if gateway.puts != 6 {
		t.Fatal("document retry duplicated accepted ciphertext uploads", gateway.puts)
	}
	if _, err := source.Matrix.GetEvent(ctx, source.Room, id.EventID(mime.EventID)); err != nil {
		t.Fatal("legacy original was removed", err)
	}
	t.Log("TRANSFER_DOCUMENT_FILE_SDK_LOST_ACK_FRESH_RECOVERY_PASS; actual SDK, kind+type receipt, rewritten file refs, original bytes and independent recovery; MailRepository adapter remains separate")
}

func assertDocumentGatewayOpaque(t *testing.T, gateway *fileGatewayFixture) {
	t.Helper()
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	for _, raw := range gateway.data {
		for _, marker := range []string{"PRIVATE-TRANSFER-MIME", "private subject", "900719925474099512345"} {
			if bytes.Contains(raw, []byte(marker)) {
				t.Fatal("provider stored plaintext document content")
			}
		}
	}
}

func assertTransferredDocuments(ctx context.Context, t *testing.T, client *Client, files *api.Drive, scope storage.Scope, envelope []byte) {
	t.Helper()
	snapshot, err := storage.LoadCatalog(ctx, client, scope)
	if err != nil || len(snapshot.Heads) != 3 {
		t.Fatal("transferred catalog is incomplete", err)
	}
	refs := map[string]storage.Reference{}
	for _, head := range snapshot.Heads {
		refs[head.ObjectID] = head.Reference
	}
	for _, head := range snapshot.Heads {
		var out bytes.Buffer
		if _, err := client.DownloadStorageFile(ctx, files, head.Commit.Content.Reference, head.ObjectID, head.Commit.Content.RevisionID, &out); err != nil {
			t.Fatal(err)
		}
		if head.ObjectID == "mime" {
			if !bytes.Equal(out.Bytes(), []byte(transferDocumentMIME)) {
				t.Fatal("MIME bytes changed")
			}
			clear(out.Bytes())
			continue
		}
		if !bytes.Contains(out.Bytes(), []byte("900719925474099512345")) {
			t.Fatal("extension integer was rounded or dropped")
		}
		if head.ObjectID == "message" {
			var value struct {
				MIME     storage.Reference
				Envelope []byte
			}
			if json.Unmarshal(out.Bytes(), &value) != nil || value.MIME != refs["mime"] || !bytes.Equal(value.Envelope, envelope) {
				t.Fatal("message file kept legacy refs or changed envelope")
			}
		} else {
			var value struct{ Records []storage.Reference }
			if json.Unmarshal(out.Bytes(), &value) != nil || len(value.Records) != 2 || value.Records[0] != refs["mime"] || value.Records[1] != refs["message"] {
				t.Fatal("receipt file kept legacy references")
			}
		}
		clear(out.Bytes())
	}
}

func freshDocumentDevice(ctx context.Context, t *testing.T, proxy *httptest.Server, destination storageSynapseDevice, scope storage.Scope) *Client {
	t.Helper()
	anonymous, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Client = proxy.Client()
	login, err := anonymous.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: destination.Matrix.UserID.String()}, Password: destination.Password, DeviceID: "DOCUMENT_FRESH"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := destination.Config
	cfg.StoreDir, cfg.Secrets = t.TempDir(), &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.LoadCatalog(ctx, fresh, scope); err == nil {
		t.Fatal("fresh token-only device opened private transferred content")
	}
	if err := fresh.Recover(ctx, destination.Recovery); err != nil {
		t.Fatal(err)
	}
	return fresh
}
