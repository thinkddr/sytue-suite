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
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// All credentials and recovery material in this test belong to a disposable,
// loopback-only Synapse. The fixture is private, temporary and never logged.
func TestStorageNativeBrowserBackupTrustBoundary(t *testing.T) {
	storageNativeBrowserBackupTrustBoundary(t, storage.LegacyIdentity)
}

func TestIsolatedStorageNativeBrowserBackupTrustBoundary(t *testing.T) {
	storageNativeBrowserBackupTrustBoundary(t, storage.IsolatedIdentity)
}

func TestIsolatedStorageMobileNativeBrowserBackupTrustBoundary(t *testing.T) {
	t.Setenv("SYTUE_MATRIX_BROWSER_PROFILE", "mobile")
	storageNativeBrowserBackupTrustBoundary(t, storage.IsolatedIdentity)
}

func TestIsolatedStorageCoreIngressNativeBrowserInterop(t *testing.T) {
	t.Setenv("SYTUE_MATRIX_CORE_INGRESS_BROWSER", "1")
	storageNativeBrowserBackupTrustBoundary(t, storage.IsolatedIdentity)
}

func storageNativeBrowserBackupTrustBoundary(t *testing.T, version storage.IdentityVersion) {
	t.Helper()
	if os.Getenv("SYTUE_MATRIX_STORAGE_BROWSER") != "1" {
		t.Skip("requires disposable Synapse and Chromium on the remote QA host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	target, _ := url.Parse("http://127.0.0.1:18585")
	listener, err := net.Listen("tcp", "127.0.0.1:18586")
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
	if err := proxy.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	proxy.Config.ErrorLog = log.New(io.Discard, "", 0)
	proxy.Listener = listener
	proxy.StartTLS()
	defer proxy.Close()
	const home = "https://storage.test:18586"
	transport := proxy.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = transport.TLSClientConfig.Clone()
	transport.TLSClientConfig.ServerName = "example.com"
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "storage.test:18586" {
			return nil, ErrIdentity
		}
		return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:18586")
	}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport}
	cli, err := mautrix.NewClient(home, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cli.Client = httpClient
	const actor = "00000000-0000-4000-8000-000000000001"
	password := rand.Text()
	product := "notes"
	if os.Getenv("SYTUE_MATRIX_CORE_INGRESS_BROWSER") == "1" {
		product = "mail"
	}
	username := "sytue_" + actor
	if version == storage.IsolatedIdentity {
		username = strings.TrimSuffix(strings.TrimPrefix(storage.IsolatedMatrixUserPrefix(storage.Scope{Subject: actor, Tenant: "artificial-browser-storage", Product: product}), "@"), ":")
	}
	request := &mautrix.ReqRegister[any]{Username: username, Password: password, DeviceID: "ARTIFICIAL_NATIVE"}
	registered, challenge, err := cli.Register(ctx, request)
	if err == nil && challenge != nil {
		request.Auth = map[string]any{"type": "m.login.dummy", "session": challenge.Session}
		registered, _, err = cli.Register(ctx, request)
	}
	if err != nil || registered == nil {
		t.Fatal("disposable registration failed", err)
	}
	cli.UserID, cli.DeviceID, cli.AccessToken = registered.UserID, registered.DeviceID, registered.AccessToken
	room, err := cli.CreateRoom(ctx, &mautrix.ReqCreateRoom{Preset: "private_chat", CreationContent: map[string]any{"m.federate": false}, InitialState: []*event.Event{{Type: event.StateEncryption, StateKey: new(""), Content: event.Content{Parsed: &event.EncryptionEventContent{Algorithm: id.AlgorithmMegolmV1}}}}})
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := Bootstrap{Actor: actor, Tenant: "artificial-browser-storage", HomeserverURL: home, SSOProviderID: "artificial", MatrixUserID: registered.UserID.String(), ControllerUserID: "@rooms_controller:storage.test:18586", Encryption: string(id.AlgorithmMegolmV1), Channels: []RoomBinding{}, StorageProduct: product, StorageIdentityVersion: version, StorageRoomID: room.RoomID.String(), StorageRoomIDs: []string{room.RoomID.String()}}
	cfg := Config{Bootstrap: bootstrap, Subject: actor, Tenant: bootstrap.Tenant, StoreDir: t.TempDir(), Secrets: &memorySecrets{values: map[string][]byte{}}, HTTPClient: httpClient}
	client, err := newClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	client.session = credentials{Homeserver: home, UserID: registered.UserID, DeviceID: registered.DeviceID, AccessToken: registered.AccessToken, Subject: actor, Tenant: cfg.Tenant}
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
		t.Fatal(err)
	}
	recovery, err := client.BootstrapRecovery(ctx, func(challenge *mautrix.RespUserInteractive) any {
		return map[string]any{"type": "m.login.password", "identifier": map[string]string{"type": "m.id.user", "user": registered.UserID.String()}, "password": password, "session": challenge.Session}
	})
	if err != nil {
		t.Fatal("disposable recovery setup failed", err)
	}
	go func(current *Client) { _ = current.Sync(ctx, func(DecryptedEvent, error) {}) }(client)
	if err = client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = client.AcknowledgeRecoverySetup(ctx); err != nil {
		t.Fatal("artificial owner acknowledgment failed", err)
	}
	payload := []byte(`{"text":"artificial private native to browser","integer":9007199254740993}`)
	record := storage.Record{Version: 1, Scope: client.storageScope(), ObjectID: "artificial-object", RevisionID: "native-revision", Role: "content", Payload: payload}
	ref, err := writeStorageRecordVerified(ctx, client, "artificial-native-content", record)
	if err != nil {
		t.Fatal("native publication failed", err)
	}
	catalog, err := client.PutStorageObject(ctx, "artificial-native-catalog", record.ObjectID, "native-catalog", storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte(`{"title":"artificial private title"}`), Content: &storage.ContentPointer{Kind: "record", RevisionID: record.RevisionID, Reference: ref}})
	if err != nil {
		t.Fatal("native catalog failed", err)
	}
	stored, err := cli.GetEvent(ctx, room.RoomID, id.EventID(ref.EventID))
	if err != nil || stored.Type != event.EventEncrypted {
		t.Fatal("native content was not encrypted", err)
	}
	wire, err := json.Marshal(stored)
	if err != nil || bytes.Contains(wire, payload) || bytes.Contains(wire, []byte("artificial private title")) {
		t.Fatal("private data reached Synapse", err)
	}
	const coreIngressEpoch = "00000000-0000-4000-8000-000000000003"
	var nativeIngressPlan *ingress.KeyPlan
	var nativeIngressTask ingress.Task
	var nativeIngressCipher, nativeIngressMetadata, nativeIngressBody []byte
	if os.Getenv("SYTUE_MATRIX_CORE_INGRESS_BROWSER") == "1" {
		plan, keyErr := client.CreateIngressKey(ctx, client.storageScope(), coreIngressEpoch, 1)
		if keyErr != nil {
			t.Fatal("native ingress SSSS setup failed", keyErr)
		}
		nativeIngressPlan = &plan
		nativeIngressMetadata = []byte(`{"subject":"native ingress exact original","integer":900719925474099312345}`)
		nativeIngressBody = bytes.Repeat([]byte("MIME exact original\r\nBcc: private@example.test\r\n"), 2048)
		var output bytes.Buffer
		nativeIngressTask, keyErr = agecipher.Seal(ctx, &output, plan, "browser_core_ingress", "mail.receive", nativeIngressMetadata, bytes.NewReader(nativeIngressBody), int64(len(nativeIngressBody)))
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		nativeIngressCipher = bytes.Clone(output.Bytes())
		clear(output.Bytes())
	}
	if err = client.Backup(ctx); err != nil {
		t.Fatal("native encrypted backup failed", err)
	}
	// A separate public-only backup session is not owner-attested. A raw SDK
	// event in the already-attested native session would be legitimate legacy.
	legacyCipher, legacyRecord := anonymousStorageBackupWithProof(ctx, t, cli, room.RoomID, record, false)
	forgedCipher, forgedRecord := anonymousStorageBackup(ctx, t, cli, room.RoomID, record)
	login, err := cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: registered.UserID.String()}, Password: password, DeviceID: "ARTIFICIAL_BROWSER"})
	if err != nil {
		t.Fatal("disposable browser login failed", err)
	}
	rootKeys, err := client.crypto.Machine().GetOwnCrossSigningPublicKeys(ctx)
	if err != nil || rootKeys == nil {
		t.Fatal("independent native trust root missing", err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	fixtureDir := t.TempDir()
	fixture := map[string]any{"artificial": true, "bootstrap": bootstrap, "session": map[string]any{"user_id": login.UserID, "device_id": login.DeviceID, "access_token": login.AccessToken, "storage_scope": record.Scope}, "recovery": recovery, "record": record, "reference": ref, "catalog": catalog, "legacy_ciphertext": legacyCipher, "legacy_record": legacyRecord, "forged_ciphertext": forgedCipher, "forged_record": forgedRecord}
	if nativeIngressPlan != nil {
		fixture["core_ingress"] = map[string]any{"plan": nativeIngressPlan, "task": nativeIngressTask, "cipher": nativeIngressCipher, "metadata": nativeIngressMetadata, "body": nativeIngressBody}
	}
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureDir, "fixture.json")
	if err = os.WriteFile(fixturePath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	clear(encoded)
	resultPath := filepath.Join(fixtureDir, "result.json")
	runner := filepath.Join("..", "..", "..", "test", "e2e", "matrix-storage-recovery.mjs")
	command := exec.CommandContext(ctx, "bun", runner, fixturePath, resultPath)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser recovery probe failed: %v\n%s", err, output)
	}
	resultBytes, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		WebIngressPlan    *ingress.KeyPlan            `json:"core_ingress_plan"`
		RootProof         *matrixproof.SignedAction   `json:"root_proof"`
		RootRequest       *matrixproof.RootEnrollment `json:"root_request"`
		Reference         storage.Reference           `json:"reference"`
		Payload           []byte                      `json:"payload"`
		LegacyReference   storage.Reference           `json:"legacy_reference"`
		InvalidReference  storage.Reference           `json:"invalid_reference"`
		ForgedReference   storage.Reference           `json:"forged_reference"`
		CoreCopyReference storage.Reference           `json:"core_copy_reference"`
		CoreCopyRequest   StorageFileRequest          `json:"core_copy_request"`
		FileReference     storage.Reference           `json:"file_reference"`
		FileRequest       StorageFileRequest          `json:"file_request"`
		FileBlobs         map[string][]byte           `json:"file_blobs"`
	}
	if json.Unmarshal(resultBytes, &result) != nil || !result.Reference.Valid() {
		t.Fatal("invalid browser receipt")
	}
	if version == storage.IsolatedIdentity {
		verifyBrowserStorageRootConsent(ctx, t, cli, rootKeys.MasterKey, record.Scope, storage.PublicationTarget{RoomID: room.RoomID.String(), DeviceID: login.DeviceID.String()}, result.RootRequest, result.RootProof)
	}
	reopened, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	client = reopened
	go func(current *Client) { _ = current.Sync(ctx, func(DecryptedEvent, error) {}) }(client)
	if err = client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := client.ReadStorageRecord(ctx, result.Reference, record.ObjectID, "content")
	if err != nil || !bytes.Equal(got.Payload, result.Payload) {
		t.Fatal("native could not authenticate browser revision", err)
	}
	stored, err = cli.GetEvent(ctx, room.RoomID, id.EventID(result.Reference.EventID))
	if err != nil || stored.Type != event.EventEncrypted {
		t.Fatal("browser content was not encrypted", err)
	}
	wire, err = json.Marshal(stored)
	if err != nil || bytes.Contains(wire, result.Payload) {
		t.Fatal("browser private data reached Synapse", err)
	}
	if _, err := client.ReadStorageRecord(ctx, result.InvalidReference, record.ObjectID, "content"); !errors.Is(err, ErrUnverified) {
		t.Fatal("live verified sender bypassed an invalid owner proof", err)
	}
	if err = client.Backup(ctx); err != nil {
		t.Fatal("browser session backup failed", err)
	}
	loginFresh, err := cli.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: registered.UserID.String()}, Password: password, DeviceID: "ARTIFICIAL_FRESH_NATIVE"})
	if err != nil {
		t.Fatal(err)
	}
	freshCfg := cfg
	freshCfg.StoreDir = t.TempDir()
	freshCfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	fresh, err := newClient(freshCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fresh.Close() }()
	fresh.session = credentials{Homeserver: home, UserID: loginFresh.UserID, DeviceID: loginFresh.DeviceID, AccessToken: loginFresh.AccessToken, Subject: actor, Tenant: cfg.Tenant}
	if err = fresh.loadMaster(true); err != nil {
		t.Fatal(err)
	}
	if err = fresh.initialize(ctx, true); err != nil {
		t.Fatal(err)
	}
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err = fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err = fresh.Recover(ctx, recovery); err != nil {
		t.Fatal("fresh native recovery failed", err)
	}
	if nativeIngressPlan != nil {
		if result.WebIngressPlan == nil {
			t.Fatal("browser ingress enrollment missing")
		}
		restored, keyErr := fresh.RecoverIngressEnrollment(ctx, *result.WebIngressPlan)
		if keyErr != nil || restored != *result.WebIngressPlan || restored.Profile != ingress.HybridProfile || restored.Generation != 2 {
			t.Fatal("native could not recover exact browser ingress SSSS", keyErr)
		}
		var output bytes.Buffer
		task, keyErr := agecipher.Seal(ctx, &output, restored, "fresh_native_ingress", "mail.receive", nativeIngressMetadata, bytes.NewReader(nativeIngressBody), int64(len(nativeIngressBody)))
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		metadata, body, keyErr := fresh.OpenIngressBody(ctx, task, bytes.NewReader(output.Bytes()))
		if keyErr != nil {
			t.Fatal("fresh native could not open browser-created ingress key", keyErr)
		}
		actual, readErr := io.ReadAll(body)
		if readErr != nil || !bytes.Equal(metadata, nativeIngressMetadata) || !bytes.Equal(actual, nativeIngressBody) {
			t.Fatal("recovered browser key changed original", readErr)
		}
		clear(actual)
		clear(metadata)
		clear(output.Bytes())
		t.Log("NATIVE_WASM_NATIVE_SSSS_HYBRID_INGRESS_EXACT_GREEN")
	}
	recovered, err := fresh.ReadStorageRecord(ctx, ref, record.ObjectID, "content")
	if err != nil || !bytes.Equal(recovered.Payload, record.Payload) {
		t.Fatal("fresh native owner attestation failed", err)
	}
	recoveredWeb, err := fresh.ReadStorageRecord(ctx, result.Reference, record.ObjectID, "content")
	if err != nil || !bytes.Equal(recoveredWeb.Payload, result.Payload) {
		t.Fatal("fresh native could not authenticate recovered browser data", err)
	}
	if !result.FileReference.Valid() || result.FileRequest.ObjectID != "browser-file" || result.FileRequest.SizeBytes != 73 || len(result.FileBlobs) != 2 {
		t.Fatal("invalid artificial browser file receipt")
	}
	files, gateway, _, _ := fileFixture(t)
	files.scope, files.records = record.Scope, fresh
	gateway.data, gateway.product = result.FileBlobs, record.Scope.Product
	var clearFile bytes.Buffer
	fileInfo, err := files.download(ctx, result.FileReference, result.FileRequest.ObjectID, result.FileRequest.RevisionID, &clearFile)
	if err != nil || !bytes.Equal(fileInfo.Metadata, result.FileRequest.Metadata) || !bytes.Equal(clearFile.Bytes(), bytes.Repeat([]byte{9}, 73)) {
		t.Fatal("fresh native device could not verify browser file after recovery", err)
	}
	clear(clearFile.Bytes())
	if !result.CoreCopyReference.Valid() || result.CoreCopyRequest.ObjectID != "go-port-copy" || result.CoreCopyRequest.SizeBytes != 73 {
		t.Fatal("missing Go port file receipt")
	}
	var copiedFile bytes.Buffer
	copiedInfo, err := files.download(ctx, result.CoreCopyReference, result.CoreCopyRequest.ObjectID, result.CoreCopyRequest.RevisionID, &copiedFile)
	if err != nil || !bytes.Equal(copiedInfo.Metadata, result.CoreCopyRequest.Metadata) || !bytes.Equal(copiedFile.Bytes(), bytes.Repeat([]byte{9}, 73)) {
		t.Fatal("fresh native could not authenticate Go-Wasm port file copy", err)
	}
	clear(copiedFile.Bytes())
	encryptedFile, err := cli.GetEvent(ctx, room.RoomID, id.EventID(result.FileReference.EventID))
	if err != nil || encryptedFile.Type != event.EventEncrypted {
		t.Fatal("browser manifest was not encrypted", err)
	}
	fileWire, err := json.Marshal(encryptedFile)
	if err != nil || bytes.Contains(fileWire, []byte("private browser upload")) || bytes.Contains(fileWire, []byte("org.sytue.file.v1")) {
		t.Fatal("private browser manifest reached Synapse", err)
	}
	rawForgery, err := fresh.Read(ctx, result.ForgedReference)
	if err != nil {
		t.Fatal("public-only backup fixture was not decryptable; review SDK trust boundary", err)
	}
	decodedForgery, err := decodePrivateRecord(rawForgery, record.Scope, registered.UserID.String())
	if err != nil || !bytes.Equal(decodedForgery.Payload, forgedRecord.Payload) {
		t.Fatal("public-only backup fixture changed", err)
	}
	if _, err = fresh.ReadStorageRecord(ctx, result.ForgedReference, record.ObjectID, "content"); !errors.Is(err, ErrUnverified) {
		t.Fatal("anonymous public-key backup writer forged owner attestation", err)
	}
	if _, err = fresh.ReadStorageRecord(ctx, result.LegacyReference, record.ObjectID, "content"); !errors.Is(err, ErrUnverified) {
		t.Fatal("unsealed anonymous backup was accepted by native storage", err)
	}
	t.Log("real Synapse: owner-attested native/browser recovery and exact writes verified; token-only, wrong key, unsealed backup and invalid proofs denied")
}

// Native trust was captured before the browser ran; queried keys cannot choose
// a replacement root. This checks real Rust SDK signatures and canonical JSON,
// independently from the server's separately tested nonce/consent transaction.
func verifyBrowserStorageRootConsent(ctx context.Context, t *testing.T, client *mautrix.Client, root id.Ed25519, scope storage.Scope, target storage.PublicationTarget, request *matrixproof.RootEnrollment, proof *matrixproof.SignedAction) {
	t.Helper()
	want := matrixproof.RootEnrollment{Version: 1, Scope: scope, UserID: client.UserID.String(), Target: target, RootKey: root.String(), Generation: 1}
	if request == nil || proof == nil || *request != want {
		t.Fatal("browser root request changed the captured SDK identity")
	}
	intent, err := matrixproof.IntentFor(scope, 1, proof.Epoch, "root.enroll", target, *request)
	if err != nil || proof.Intent() != intent || proof.RootKey != root.String() {
		t.Fatal("browser signed different public intent", err)
	}
	keys, err := client.QueryKeys(ctx, &mautrix.ReqQueryKeys{DeviceKeys: mautrix.DeviceKeysRequest{client.UserID: mautrix.DeviceIDList{id.DeviceID(target.DeviceID)}}})
	if err != nil {
		t.Fatal(err)
	}
	_, server, _ := strings.Cut(client.UserID.String(), ":")
	verified, err := matrixproof.Verify(ctx, matrixproof.Origin{Scope: scope, ServerName: server, UserID: client.UserID, DeviceID: id.DeviceID(target.DeviceID), AccountActive: true, DeviceActive: true, Keys: *keys}, root)
	if err != nil {
		t.Fatal("browser public device chain rejected", err)
	}
	if err := verified.VerifyAction(ctx, *proof, proof.Action, time.Now()); err != nil {
		t.Fatal("Rust SDK control signature rejected by native SDK", err)
	}
	altered := *proof
	altered.Generation++
	if err := verified.VerifyAction(ctx, altered, altered.Action, time.Now()); err == nil {
		t.Fatal("changed signed generation was accepted")
	}
	t.Log("BROWSER_RUST_CONTROL_SIGNATURE_NATIVE_VERIFICATION_PASS")
}
