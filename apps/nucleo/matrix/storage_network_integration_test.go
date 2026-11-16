// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
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

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	privatedrive "git.sytue.eu/sytue/one/apps/nucleo/drive"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

var _ privatedrive.Crypto = (*Client)(nil)

// Drops only the application ACK after the actual SDK/homeserver accepted a root.
// The durable uploader must retain its original Matrix endpoint on retry.
type lostNativeRootACK struct {
	*Client
	dropped   bool
	reference EventRef
}

func (s *lostNativeRootACK) PutStorageRecord(ctx context.Context, txn string, rec storage.Record) (EventRef, error) {
	target, err := s.StoragePublicationTarget()
	if err != nil {
		return EventRef{}, err
	}
	return s.PutStorageRecordAt(ctx, target, txn, rec)
}

func (s *lostNativeRootACK) PutStorageRecordAt(ctx context.Context, target storage.PublicationTarget, txn string, rec storage.Record) (EventRef, error) {
	ref, err := s.Client.PutStorageRecordAt(ctx, target, txn, rec)
	var payload struct {
		Kind string `json:"kind"`
	}
	if err == nil && !s.dropped && json.Unmarshal(rec.Payload, &payload) == nil && payload.Kind == fileKind {
		s.dropped, s.reference = true, ref
		return ref, errors.New("artificial lost root ACK")
	}
	return ref, err
}

// This opt-in fixture uses a disposable, loopback-only Synapse and artificial
// registration. It never takes a production token, password or recovery key.
func TestStorageFileAgainstSynapseSurvivesDeviceRestart(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_STORAGE_SYNAPSE") != "1" {
		t.Skip("requires disposable Synapse on the remote development host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	upstream := httputil.NewSingleHostReverseProxy(target)
	catalogReceipts := make(chan EventRef, 8)
	var lostCatalog atomic.Bool
	upstream.ModifyResponse = func(response *http.Response) error {
		if response.StatusCode != http.StatusOK || !strings.HasSuffix(response.Request.URL.Path, "/sytue-storage-catalog-create") {
			return nil
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if err != nil {
			return err
		}
		var accepted struct {
			EventID string `json:"event_id"`
		}
		if err := json.Unmarshal(raw, &accepted); err != nil {
			return err
		}
		parts := strings.Split(response.Request.URL.Path, "/")
		catalogReceipts <- EventRef{RoomID: parts[5], EventID: accepted.EventID}
		if lostCatalog.CompareAndSwap(false, true) {
			// Synapse accepted the encrypted catalog event. Corrupt only the
			// application response so the SDK cannot expose its receipt.
			response.Body = io.NopCloser(strings.NewReader("{"))
			response.ContentLength = 1
			response.Header.Set("Content-Length", "1")
		} else {
			response.Body = io.NopCloser(bytes.NewReader(raw))
		}
		return nil
	}
	proxy := httptest.NewTLSServer(upstream)
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	client, cli, cfg, room := device.Client, device.Matrix, device.Config, device.Room
	password, recovery := device.Password, device.Recovery
	defer func() { _ = client.Close() }()
	files, gateway, _, _ := fileFixture(t)
	source := patternedSource{size: 16*storage.MaxCipherBlockBytes + 123, pattern: []byte("artificial private source - never on Synapse ")}
	input := fileRequest(source.size)
	dropped := &lostNativeRootACK{Client: client}
	files.scope, files.records, files.closed = client.storageScope(), dropped, client.closed.Load
	if _, err := files.upload(ctx, "real-synapse-file", input, source); err == nil || !dropped.dropped {
		t.Fatal("real lost root ACK did not interrupt publication", err)
	}
	rotated := newPrivateStorageTestRoom(ctx, t, cli)
	next := cfg.Bootstrap
	next.StorageRoomID, next.StorageRoomIDs = rotated.String(), []string{room.String(), rotated.String()}
	if err := client.UpdateBindings(next); err != nil {
		t.Fatal(err)
	}
	root, err := client.UploadStorageFile(ctx, files.gateway, files.journal, "real-synapse-file", input, source)
	if err != nil || root != dropped.reference || gateway.puts != 17 {
		t.Fatal("real endpoint rotation duplicated a publication or ciphertext", err, gateway.puts)
	}
	if err := client.UpdateBindings(cfg.Bootstrap); err != nil {
		t.Fatal(err)
	}
	// Crash after complete file verification but before its application commit.
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal("restart before catalog failed", err)
	}
	go func(c *Client) { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }(client)
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	resumed, err := client.UploadStorageFile(ctx, files.gateway, files.journal, "real-synapse-file", input, source)
	if err != nil || resumed != root || gateway.puts != 17 {
		t.Fatal("restart before catalog duplicated verified file", err, gateway.puts)
	}
	catalogTarget, err := client.StoragePublicationTarget()
	if err != nil {
		t.Fatal(err)
	}
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: input.Metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: input.RevisionID, Reference: root}}
	if _, err := client.PutStorageObjectAt(ctx, catalogTarget, "catalog-create", input.ObjectID, "catalog-r0", commit); err == nil || !lostCatalog.Load() {
		t.Fatal("catalog ACK loss did not interrupt commit", err)
	}
	acceptedCatalog := <-catalogReceipts
	if err := client.UpdateBindings(next); err != nil {
		t.Fatal(err)
	}
	created, err := client.PutStorageObjectAt(ctx, catalogTarget, "catalog-create", input.ObjectID, "catalog-r0", commit)
	if err != nil {
		t.Fatal("private catalog creation failed", err)
	}
	confirmedCatalog := <-catalogReceipts
	if created.Reference != acceptedCatalog || confirmedCatalog != acceptedCatalog {
		t.Fatal("catalog retry after rotation duplicated its accepted commit", created.Reference, acceptedCatalog)
	}
	if err := client.UpdateBindings(cfg.Bootstrap); err != nil {
		t.Fatal(err)
	}
	commit.Parents = []storage.Reference{created.Reference}
	commit.Metadata = []byte(`{"name":"private renamed.pdf","integer":9007199254740993}`)
	edited, err := client.PutStorageObject(ctx, "catalog-edit", input.ObjectID, "catalog-r1", commit)
	if err != nil {
		t.Fatal("private catalog edit failed", err)
	}
	changed := commit
	changed.Metadata = []byte(`{"name":"changed retry must fail"}`)
	if _, err := client.PutStorageObject(ctx, "catalog-edit", input.ObjectID, "catalog-r1", changed); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("changed transaction payload silently reused an old catalog receipt", err)
	}
	removed, err := client.PutStorageObject(ctx, "catalog-delete", input.ObjectID, "catalog-r2", storage.ObjectCommit{Kind: storage.ObjectKind, State: "deleted", Metadata: []byte(`{}`), Parents: []storage.Reference{created.Reference}})
	if err != nil {
		t.Fatal("private logical deletion failed", err)
	}
	if err := client.AcknowledgeStorageFile(ctx, files.journal, "real-synapse-file", removed.Reference); !errors.Is(err, storage.ErrRecord) {
		t.Fatal("tombstone acknowledged unpublished content", err)
	}
	if err := client.AcknowledgeStorageFile(ctx, files.journal, "real-synapse-file", edited.Reference); err != nil {
		t.Fatal("authenticated catalog did not release verified file journal", err)
	}
	if count, err := files.journal.Count(storage.PendingUploadsNamespace); err != nil || count != 0 {
		t.Fatal("catalog acknowledged but journal retained", count, err)
	}
	foreign := commit
	foreign.Parents = nil
	if _, err := client.PutStorageObject(ctx, "catalog-foreign", "foreign-object", "catalog-r3", foreign); err == nil {
		t.Fatal("catalog pointer escaped its object")
	}
	stored, err := cli.GetEvent(ctx, room, id.EventID(root.EventID))
	if err != nil || stored.Type != event.EventEncrypted {
		t.Fatal("Synapse did not store an encrypted event", err)
	}
	wire, err := json.Marshal(stored)
	if err != nil || bytes.Contains(wire, []byte("private name.pdf")) || bytes.Contains(wire, source.pattern) {
		t.Fatal("private metadata/content reached Synapse", err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = Open(ctx, cfg)
	if err != nil {
		t.Fatal("device restart failed", err)
	}
	go func(c *Client) { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }(client)
	if err = client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	info, err := client.DownloadStorageFile(ctx, files.gateway, root, input.ObjectID, input.RevisionID, &patternedSink{source: source})
	if err != nil || !bytes.Equal(info.Metadata, input.Metadata) {
		t.Fatal("authenticated file did not survive restart", err)
	}
	got, err := client.ReadStorageFileRange(ctx, files.gateway, root, input.ObjectID, input.RevisionID, storage.MaxCipherBlockBytes-9, 48)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 48)
	_, _ = source.ReadAt(want, storage.MaxCipherBlockBytes-9)
	if !bytes.Equal(got, want) {
		t.Fatal("range from actual Matrix manifest changed after restart")
	}
	if err = client.Backup(ctx); err != nil {
		t.Fatal("encrypted backup failed", err)
	}
	// A completely new device starts without this account's local keys or cache.
	// An OIDC/Matrix access token alone must not unlock the historical manifest.
	login, err := cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: cli.UserID.String()}, Password: password, DeviceID: "RECOVERED_STORAGE_TEST"})
	if err != nil {
		t.Fatal("artificial second-device login failed", err)
	}
	secondConfig := cfg
	secondConfig.StoreDir = t.TempDir()
	secondConfig.Secrets = &memorySecrets{values: map[string][]byte{}}
	second, err := newClient(secondConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	second.session = credentials{Homeserver: proxy.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant}
	if err = second.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if err = second.initialize(ctx, true); err != nil {
		t.Fatal("second device initialization failed", err)
	}
	go func() { _ = second.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = second.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if data, err := second.ReadStorageFileRange(ctx, files.gateway, root, input.ObjectID, input.RevisionID, 0, 16); !errors.Is(err, ErrUnverified) || len(data) != 0 {
		t.Fatal("access token alone unlocked private storage", err)
	}
	if err = second.Recover(ctx, "invalid recovery key"); err == nil {
		t.Fatal("invalid recovery key unlocked the fresh device")
	}
	if err = second.Recover(ctx, recovery); err != nil {
		t.Fatal("fresh device could not recover authenticated encrypted history", err)
	}
	info, err = second.DownloadStorageFile(ctx, files.gateway, root, input.ObjectID, input.RevisionID, &patternedSink{source: source})
	if err != nil || !bytes.Equal(info.Metadata, input.Metadata) {
		t.Fatal("fresh device did not decrypt the verified replacement", err)
	}
	entries := artificialCatalogHistory(ctx, t, second, room.String())
	heads, err := storage.CatalogHeads(second.storageScope(), entries)
	if err != nil || len(heads) != 2 {
		t.Fatal("fresh device lost a concurrent catalog branch", err)
	}
	seenHeads := map[storage.Reference]bool{}
	for _, head := range heads {
		seenHeads[head.Reference] = true
	}
	if !seenHeads[edited.Reference] || !seenHeads[removed.Reference] {
		t.Fatal("concurrent edit or logical deletion was hidden")
	}
	privateObject, err := second.StorageObject(ctx, edited.Reference, input.ObjectID)
	if err != nil || !bytes.Equal(privateObject.Commit.Metadata, commit.Metadata) || privateObject.Commit.Content == nil || privateObject.Commit.Content.Reference != root {
		t.Fatal("recovered catalog lost exact metadata or immutable file pointer", err)
	}
	t.Run("private Drive imports real PostgreSQL source", func(t *testing.T) {
		exercisePrivateDriveImporter(ctx, t, client, files.gateway, files.journal, next, gateway, "")
	})
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	for _, ciphertext := range gateway.data {
		if bytes.Contains(ciphertext, source.pattern) {
			t.Fatal("object provider received plaintext")
		}
	}
	t.Logf("real Synapse: %d ciphertext blocks, encrypted root %d bytes, restart/range/fresh-device recovery verified", len(gateway.data), len(wire))
}

func artificialCatalogHistory(ctx context.Context, t *testing.T, client *Client, room string) []storage.RecordEntry {
	t.Helper()
	var entries []storage.RecordEntry
	cursor := ""
	seen := map[string]bool{}
	for {
		page, err := client.StorageObjectHistory(ctx, room, cursor, 2)
		if err != nil {
			t.Fatal("authenticated catalog pagination failed", err)
		}
		entries = append(entries, page.Entries...)
		if page.Next == "" {
			return entries
		}
		if seen[page.Next] {
			t.Fatal("catalog pagination looped without completing history")
		}
		seen[page.Next] = true
		cursor = page.Next
	}
}

type patternedSink struct {
	source patternedSource
	offset int64
}

func (s *patternedSink) Write(data []byte) (int, error) {
	want := make([]byte, len(data))
	defer clear(want)
	n, err := s.source.ReadAt(want, s.offset)
	if err != nil && !errors.Is(err, io.EOF) || n != len(data) || !bytes.Equal(data, want) {
		return 0, ErrIdentity
	}
	s.offset += int64(n)
	return n, nil
}

type storageSynapseDevice struct {
	Client   *Client
	Matrix   *mautrix.Client
	Config   Config
	Room     id.RoomID
	Password string
	Recovery string
}

// Reuse actual registration, verification and recovery rather than replacing
// the maintained SDK with a domain mock for focused storage conformance tests.
func newStorageSynapseDevice(ctx context.Context, t *testing.T, baseURL string, httpClient *http.Client) storageSynapseDevice {
	t.Helper()
	return newStorageSynapseDeviceForScope(ctx, t, baseURL, httpClient, storage.Scope{Subject: "artificial", Tenant: "artificial-storage", Product: "drive"}, storage.LegacyIdentity)
}

func newStorageSynapseDeviceForScope(ctx context.Context, t *testing.T, baseURL string, httpClient *http.Client, scope storage.Scope, version storage.IdentityVersion) storageSynapseDevice {
	t.Helper()
	device := newUnverifiedStorageSynapseDevice(ctx, t, baseURL, httpClient, scope, version)
	recovery, err := device.Client.BootstrapRecovery(ctx, storageFixtureUIA(device.Matrix.UserID, device.Password))
	if err != nil {
		t.Fatal("artificial recovery initialization failed", err)
	}
	device.Recovery = recovery
	go func() { _ = device.Client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = device.Client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = device.Client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal("artificial owner acknowledgment failed", err)
	}
	return device
}

func storageFixtureUIA(user id.UserID, password string) mautrix.UIACallback {
	return func(challenge *mautrix.RespUserInteractive) any {
		return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": user.String()}, "password": password, "session": challenge.Session}
	}
}

func newUnverifiedStorageSynapseDevice(ctx context.Context, t *testing.T, baseURL string, httpClient *http.Client, scope storage.Scope, version storage.IdentityVersion) storageSynapseDevice {
	t.Helper()
	cli, err := mautrix.NewClient(baseURL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cli.Client = httpClient
	password := rand.Text()
	username := "artificial_" + strings.ToLower(rand.Text())
	if version == storage.IsolatedIdentity {
		username = "sytue_storage_" + scope.Product + "_" + scope.Tenant + "_" + scope.Subject
	}
	if version == storage.LegacyIdentity && scope.Subject != "artificial" {
		username = "sytue_" + scope.Subject
	}
	registered := matrixprooftest.Register(ctx, t, cli, username, password, "STORAGE_TEST")
	_, serverName, _ := strings.Cut(registered.UserID.String(), ":")
	room := newPrivateStorageTestRoom(ctx, t, cli)
	cfg := Config{Bootstrap: Bootstrap{
		Actor: scope.Subject, Tenant: scope.Tenant, HomeserverURL: baseURL,
		MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:" + serverName, Encryption: string(id.AlgorithmMegolmV1),
		StorageProduct: scope.Product, StorageIdentityVersion: version, StorageRoomID: room.String(), StorageRoomIDs: []string{room.String()},
	}, Subject: scope.Subject, Tenant: scope.Tenant, StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: httpClient}
	client := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: baseURL, UserID: registered.UserID, DeviceID: registered.DeviceID, AccessToken: registered.AccessToken, Subject: scope.Subject, Tenant: scope.Tenant})
	return storageSynapseDevice{Client: client, Matrix: cli, Config: cfg, Room: room, Password: password}
}

func storageSynapseSession(ctx context.Context, t *testing.T, cfg Config, session credentials) *Client {
	t.Helper()
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	client.session = session
	saved, err := json.Marshal(client.session)
	if err != nil {
		t.Fatal(err)
	}
	if err = cfg.Secrets.Write(client.sessionKey(), saved); err != nil {
		t.Fatal(err)
	}
	if err = client.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if err = client.initialize(ctx, true); err != nil {
		t.Fatal("maintained device initialization failed", err)
	}
	return client
}

func newPrivateStorageTestRoom(ctx context.Context, t *testing.T, cli *mautrix.Client) id.RoomID {
	t.Helper()
	room, err := cli.CreateRoom(ctx, &mautrix.ReqCreateRoom{
		Preset: "private_chat", CreationContent: map[string]any{"m.federate": false},
		InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: new(""), Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}},
	})
	if err != nil {
		t.Fatal("private room creation failed", err)
	}
	return room.RoomID
}

func TestPrivateDriveLabelsAgainstSynapse(t *testing.T) {
	exerciseFocusedPrivateDrive(t, "labels")
}

func TestPrivateDriveThumbnailsAgainstSynapse(t *testing.T) {
	exerciseFocusedPrivateDrive(t, "thumbnails")
}

func exerciseFocusedPrivateDrive(t *testing.T, scenario string) {
	t.Helper()
	if os.Getenv("SYTUE_MATRIX_STORAGE_SYNAPSE") != "1" || os.Getenv("SYTUE_DRIVE_PRIVATE_VECTOR") == "" {
		t.Skip("requires disposable remote Synapse and synthetic PostgreSQL export")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	device := newStorageSynapseDevice(ctx, t, proxy.URL, proxy.Client())
	defer func() { _ = device.Client.Close() }()
	rotated := newPrivateStorageTestRoom(ctx, t, device.Matrix)
	next := device.Config.Bootstrap
	next.StorageRoomID, next.StorageRoomIDs = rotated.String(), []string{device.Room.String(), rotated.String()}
	files, gateway, _, _ := fileFixture(t)
	gateway.policyScope = device.Client.storageScope()
	exercisePrivateDriveImporter(ctx, t, device.Client, files.gateway, files.journal, next, gateway, scenario)
}
