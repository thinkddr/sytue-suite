// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageFileMigrationAcrossIdentitiesSurvivesRestartAndFreshRecovery(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_FILE_MIGRATION") != "1" {
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
			r.Body = io.NopCloser(strings.NewReader(`{"errcode":"M_UNKNOWN","error":"synthetic lost file migration acknowledgment after commit"}`))
			r.ContentLength = -1
			r.Header.Del("Content-Length")
		}
		return nil
	}
	proxy := httptest.NewTLSServer(upstream)
	t.Cleanup(proxy.Close)
	scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "drive"}
	old := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.LegacyIdentity)
	target := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	seed, gateway, _, _ := fileFixture(t)
	secrets := &authtest.Secrets{}
	dir := t.TempDir()
	journal, err := offline.OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal(err)
	}
	original := patternedSource{size: storage.MaxCipherBlockBytes*fileFanout + 123, pattern: []byte("retained private version and content ")}
	request := fileRequest(original.size)
	before, err := old.Client.UploadStorageFile(ctx, seed.gateway, journal, "retained-original", request, original)
	if err != nil {
		t.Fatal("original SDK upload failed", err)
	}
	sourceIO := storageFileIO{scope: scope, gateway: seed.gateway, records: old.Client}
	from, err := sourceIO.root(ctx, before, request.ObjectID, request.RevisionID)
	if err != nil {
		t.Fatal(err)
	}
	drop.Store(true)
	if _, err = old.Client.MigrateStorageFileContent(ctx, target.Client, seed.gateway, journal, "isolated-migration", before, request.ObjectID, request.RevisionID); err == nil || drop.Load() {
		t.Fatal("fixture failed to lose an accepted migration event", err)
	}
	if err = target.Client.Close(); err != nil {
		t.Fatal(err)
	}
	replacement, err := Open(ctx, target.Config)
	if err != nil {
		t.Fatal("destination did not reopen", err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	go func() { _ = replacement.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = replacement.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	journal, err = offline.OpenWithSecrets(dir, secrets)
	if err != nil {
		t.Fatal("encrypted migration journal did not reopen", err)
	}
	after, err := old.Client.MigrateStorageFileContent(ctx, replacement, seed.gateway, journal, "isolated-migration", before, request.ObjectID, request.RevisionID)
	if err != nil {
		t.Fatal("migration did not resume", err)
	}
	if after.RoomID != target.Room.String() || gateway.puts != 2*(fileFanout+1) {
		t.Fatal("migration duplicated ciphertext or changed destination", gateway.puts)
	}
	destinationIO := storageFileIO{scope: scope, gateway: seed.gateway, records: replacement}
	to, err := destinationIO.root(ctx, after, request.ObjectID, request.RevisionID)
	if err != nil || to.SHA256 != from.SHA256 || !bytes.Equal(to.Request.Metadata, request.Metadata) {
		t.Fatal("migration lost precise metadata or commitment", err)
	}
	originalLeaf, err := sourceIO.leafAt(ctx, from, 0, map[EventRef]storageFilePage{})
	if err != nil {
		t.Fatal(err)
	}
	migratedLeaf, err := destinationIO.leafAt(ctx, to, 0, map[EventRef]storageFilePage{})
	if err != nil || originalLeaf.File.Key.Key == migratedLeaf.File.Key.Key || originalLeaf.Blob.ID == migratedLeaf.Blob.ID {
		t.Fatal("old file keys or ciphertext were reused", err)
	}
	wire, err := target.Matrix.GetEvent(ctx, target.Room, id.EventID(after.EventID))
	encoded, encodeErr := json.Marshal(wire)
	if err != nil || encodeErr != nil || wire.Type != event.EventEncrypted || bytes.Contains(encoded, request.Metadata) || bytes.Contains(encoded, []byte("private name.pdf")) {
		t.Fatal("destination exposed file descriptors or metadata", err, encodeErr)
	}
	var pending storageFileUpload
	found, err := journal.Read(destinationIO.journalOwner(), storage.PendingUploadsNamespace, "isolated-migration", &pending)
	if err != nil || !found || !pending.Verified {
		t.Fatal("migration discarded its intent before catalog publication", err)
	}
	catalog, err := replacement.PutStorageObject(ctx, "migrated-catalog", request.ObjectID, "migrated-index", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"kind":"synthetic migrated historical body"}`), Content: &storage.ContentPointer{Kind: "file", RevisionID: request.RevisionID, Reference: after}})
	if err != nil {
		t.Fatal("migrated body could not join the new catalog", err)
	}
	if err = replacement.AcknowledgeStorageFile(ctx, journal, "isolated-migration", catalog.Reference); err != nil {
		t.Fatal("catalog receipt did not retire the migration upload", err)
	}
	if found, err = journal.Read(destinationIO.journalOwner(), storage.PendingUploadsNamespace, "isolated-migration", &pending); err != nil || found {
		t.Fatal("acknowledged migration upload remains", err)
	}
	if err = old.Client.Close(); err != nil {
		t.Fatal(err)
	}
	if err = replacement.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	anonymous, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	anonymous.Client = proxy.Client()
	login, err := anonymous.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: target.Matrix.UserID.String()}, Password: target.Password, DeviceID: "MIGRATED_FRESH"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := target.Config
	cfg.StoreDir, cfg.Secrets = t.TempDir(), &memorySecrets{values: map[string][]byte{}}
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = fresh.StorageFileInfo(ctx, after, request.ObjectID, request.RevisionID); err == nil {
		t.Fatal("fresh access token opened migrated file")
	}
	if err = fresh.Recover(ctx, old.Recovery); err == nil {
		t.Fatal("old account recovery unlocked isolated file")
	}
	if err = fresh.Recover(ctx, target.Recovery); err != nil {
		t.Fatal("isolated recovery failed", err)
	}
	want, got := sha256.New(), sha256.New()
	if _, err = io.Copy(want, io.NewSectionReader(original, 0, original.size)); err != nil {
		t.Fatal(err)
	}
	recovered, err := fresh.DownloadStorageFile(ctx, seed.gateway, after, request.ObjectID, request.RevisionID, got)
	if err != nil || !bytes.Equal(want.Sum(nil), got.Sum(nil)) || !bytes.Equal(recovered.Metadata, request.Metadata) {
		t.Fatal("fresh isolated device did not recover exact migrated file", err)
	}
	if _, err = fresh.ReadStorageFileRange(ctx, seed.gateway, before, request.ObjectID, request.RevisionID, 0, 1); err == nil {
		t.Fatal("isolated device could read original account manifest")
	}
	if _, err = old.Matrix.GetEvent(ctx, old.Room, id.EventID(before.EventID)); err != nil {
		t.Fatal("migration retired the original prematurely", err)
	}
	t.Log("real Synapse: 17-block legacy-to-isolated migration, fresh ciphertext, lost event ACK and SDK/journal restart, catalog acknowledgment, source retention and independent fresh-device recovery verified")
}
