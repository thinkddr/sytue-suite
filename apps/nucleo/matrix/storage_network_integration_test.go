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
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// This opt-in fixture uses a disposable, loopback-only Synapse and artificial
// registration. It never takes a production token, password or recovery key.
func TestStorageFileAgainstSynapseSurvivesDeviceRestart(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_STORAGE_SYNAPSE") != "1" {
		t.Skip("requires disposable Synapse on the remote development host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(target))
	defer proxy.Close()
	cli, err := mautrix.NewClient(proxy.URL, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cli.Client = proxy.Client()
	password := rand.Text()
	request := &mautrix.ReqRegister[any]{Username: "artificial_" + strings.ToLower(rand.Text()), Password: password, DeviceID: "STORAGE_TEST"}
	registered, challenge, err := cli.Register(ctx, request)
	if err == nil && challenge != nil {
		request.Auth = map[string]any{"type": "m.login.dummy", "session": challenge.Session}
		registered, _, err = cli.Register(ctx, request)
	}
	if err != nil || registered == nil {
		t.Fatal("artificial registration failed", err)
	}
	cli.UserID, cli.DeviceID, cli.AccessToken = registered.UserID, registered.DeviceID, registered.AccessToken
	room, err := cli.CreateRoom(ctx, &mautrix.ReqCreateRoom{
		Preset: "private_chat", CreationContent: map[string]any{"m.federate": false},
		InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: new(""), Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}},
	})
	if err != nil {
		t.Fatal("private room creation failed", err)
	}
	cfg := Config{Bootstrap: Bootstrap{
		Actor: "artificial", Tenant: "artificial-storage", HomeserverURL: proxy.URL,
		MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:storage.test", Encryption: string(id.AlgorithmMegolmV1),
		StorageProduct: "drive", StorageRoomID: room.RoomID.String(), StorageRoomIDs: []string{room.RoomID.String()},
	}, Subject: "artificial", Tenant: "artificial-storage", StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: proxy.Client()}
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	client.session = credentials{Homeserver: proxy.URL, UserID: registered.UserID, DeviceID: registered.DeviceID, AccessToken: registered.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant}
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
	recovery, err := client.BootstrapRecovery(ctx, func(challenge *mautrix.RespUserInteractive) any {
		return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": registered.UserID.String()}, "password": password, "session": challenge.Session}
	})
	if err != nil {
		t.Fatal("artificial recovery initialization failed", err)
	}
	go func(c *Client) { _ = c.Sync(ctx, func(DecryptedEvent, error) {}) }(client)
	if err = client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	files, gateway, _, _ := fileFixture(t)
	source := patternedSource{size: 16*storage.MaxCipherBlockBytes + 123, pattern: []byte("artificial private source - never on Synapse ")}
	input := fileRequest(source.size)
	root, err := client.UploadStorageFile(ctx, files.gateway, files.journal, "real-synapse-file", input, source)
	if err != nil {
		t.Fatal("real encrypted upload failed", err)
	}
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: input.Metadata, Content: &storage.ContentPointer{Kind: "file", RevisionID: input.RevisionID, Reference: root}}
	created, err := client.PutStorageObject(ctx, "catalog-create", input.ObjectID, "catalog-r0", commit)
	if err != nil {
		t.Fatal("private catalog creation failed", err)
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
	foreign := commit
	foreign.Parents = nil
	if _, err := client.PutStorageObject(ctx, "catalog-foreign", "foreign-object", "catalog-r3", foreign); err == nil {
		t.Fatal("catalog pointer escaped its object")
	}
	stored, err := cli.GetEvent(ctx, room.RoomID, id.EventID(root.EventID))
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
	login, err := cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: registered.UserID.String()}, Password: password, DeviceID: "RECOVERED_STORAGE_TEST"})
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
	entries := artificialCatalogHistory(ctx, t, second, room.RoomID.String())
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
