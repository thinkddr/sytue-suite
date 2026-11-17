// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maunium.net/go/mautrix"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func TestHybridStorageAgainstSynapseResumesFragmentPrefixAndFreshRecovery(t *testing.T) {
	origin := os.Getenv("SYTUE_MATRIX_PQ_SYNAPSE_URL")
	if origin == "" {
		t.Skip("requires isolated remote Synapse")
	}
	upstreamURL, err := matrixprooftest.DisposableHTTPOrigin(origin)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	upstream := httputil.NewSingleHostReverseProxy(upstreamURL)
	var loseFragment atomic.Bool
	var acceptedFragments atomic.Int32
	upstream.ModifyResponse = func(response *http.Response) error {
		if response.StatusCode == http.StatusOK && strings.HasSuffix(response.Request.URL.Path, "sytue-storage-pq-crash_pq_fragment_0") {
			acceptedFragments.Add(1)
			if loseFragment.CompareAndSwap(true, false) {
				_ = response.Body.Close()
				response.Body = io.NopCloser(strings.NewReader("{"))
				response.ContentLength = 1
				response.Header.Set("Content-Length", "1")
			}
		}
		return nil
	}
	proxy := httptest.NewTLSServer(upstream)
	defer proxy.Close()
	scope := storage.Scope{Tenant: "pq", Subject: "hybrid", Product: "drive"}
	device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
	client := device.Client
	defer func() { _ = client.Close() }()
	if _, err := client.PrepareStorage(ctx, scope); err != nil {
		t.Fatal("hybrid recovery was not ready before writes", err)
	}
	originalID := client.DeviceID()
	record := maximumHybridStorageRecord(t, scope)
	loseFragment.Store(true)
	if _, err := client.PutStorageRecord(ctx, "pq-crash", record); err == nil || acceptedFragments.Load() == 0 || loseFragment.Load() {
		t.Fatal("partial fragment crash did not follow an actual accepted event", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	client, err = Open(ctx, device.Config)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = client.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := client.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if client.DeviceID() != originalID {
		t.Fatal("original publication identity was not resumed")
	}
	ref, err := client.PutStorageRecord(ctx, "pq-crash", record)
	if err != nil {
		t.Fatal("accepted prefix could not be resumed", err)
	}
	outer, err := client.Read(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	wire, found, err := decodeStoragePQWire(outer, client.session.UserID.String())
	if err != nil || !found || wire.Kind != "fragment-root" || wire.Envelope.Profile != pqc.HybridProfile {
		t.Fatal("maximum record did not use the protected paged root", err)
	}
	if bytes.Contains(outer.Content, []byte("org.sytue.storage_auth")) || bytes.Contains(outer.Content, []byte("high_entropy")) {
		t.Fatal("classical transport decryption revealed protected application metadata")
	}
	opened, err := client.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role)
	if err != nil || !bytes.Equal(opened.Payload, record.Payload) {
		t.Fatal("resumed record changed the exact source", err)
	}
	clear(opened.Payload)
	commit := storage.ObjectCommit{Kind: storage.ObjectKind, State: "live", Metadata: []byte("{\"name\":\"private hybrid index\",\"precise\":900719925474099312345}"), Content: &storage.ContentPointer{Kind: "record", RevisionID: record.RevisionID, Reference: ref}}
	index, err := client.PutStorageObject(ctx, "pq-index", record.ObjectID, "index-revision", commit)
	if err != nil {
		t.Fatal("hybrid private index failed", err)
	}
	files, gateway, _, _ := fileFixture(t)
	gateway.policyScope = scope
	filePayload := []byte(strings.Repeat("private binary attachment ", 32))
	fileInput := fileRequest(int64(len(filePayload)))
	fileRef, err := client.UploadStorageFile(ctx, files.gateway, files.journal, "pq-file", fileInput, bytes.NewReader(filePayload))
	if err != nil {
		t.Fatal("hybrid file-key publication", err)
	}
	fileOuter, err := client.Read(ctx, fileRef)
	if err != nil || bytes.Contains(fileOuter.Content, []byte("\"key\":")) || bytes.Contains(fileOuter.Content, fileInput.Metadata) {
		t.Fatal("outer Matrix keys revealed file-key metadata", err)
	}
	if err := client.Backup(ctx); err != nil {
		t.Fatal(err)
	}
	proxyB := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstreamURL))
	defer proxyB.Close()
	login, err := device.Matrix.Login(ctx, &mautrix.ReqLogin{Type: mautrix.AuthTypePassword, Identifier: mautrix.UserIdentifier{Type: mautrix.IdentifierTypeUser, User: device.Matrix.UserID.String()}, Password: device.Password, DeviceID: "RECOVERED_PQ_STORAGE"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := device.Config
	cfg.StoreDir = t.TempDir()
	cfg.Secrets = &memorySecrets{values: map[string][]byte{}}
	cfg.Bootstrap.HomeserverURL = proxyB.URL
	cfg.HTTPClient = proxyB.Client()
	fresh := storageSynapseSession(ctx, t, cfg, credentials{Homeserver: proxyB.URL, UserID: login.UserID, DeviceID: login.DeviceID, AccessToken: login.AccessToken, Subject: cfg.Subject, Tenant: cfg.Tenant})
	defer func() { _ = fresh.Close() }()
	go func() { _ = fresh.Sync(ctx, func(DecryptedEvent, error) {}) }()
	if err := fresh.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if value, err := fresh.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role); err == nil || len(value.Payload) != 0 {
		t.Fatal("a token alone opened hybrid data")
	}
	if err := fresh.Recover(ctx, device.Recovery); err != nil {
		t.Fatal("fresh SSSS recovery", err)
	}
	recovered, err := fresh.ReadStorageRecord(ctx, ref, record.ObjectID, record.Role)
	if err != nil || !bytes.Equal(recovered.Payload, record.Payload) {
		t.Fatal("fresh device lost the hybrid private identity or exact JSON", err)
	}
	clear(recovered.Payload)
	var binary bytes.Buffer
	fileInfo, err := fresh.DownloadStorageFile(ctx, files.gateway, fileRef, fileInput.ObjectID, fileInput.RevisionID, &binary)
	if err != nil || !bytes.Equal(binary.Bytes(), filePayload) || !bytes.Equal(fileInfo.Metadata, fileInput.Metadata) {
		t.Fatal("fresh recovery lost hybrid-protected file keys", err)
	}
	if fresh.DeviceID() == originalID {
		t.Fatal("fresh recovery reused the original Matrix device")
	}
	page, err := fresh.StorageObjectHistory(ctx, device.Room.String(), "", 100)
	if err != nil {
		t.Fatal("fragment events broke the authenticated catalog", err)
	}
	foundIndex := false
	for _, entry := range page.Entries {
		foundIndex = foundIndex || entry.Reference == index.Reference
	}
	if !foundIndex {
		t.Fatal("hybrid index disappeared on fresh recovery")
	}
	t.Log("HYBRID_STORAGE_REAL_SDK_PREFIX_RESTART_SSSS_FRESH_GREEN")
}

func maximumHybridStorageRecord(t *testing.T, scope storage.Scope) storage.Record {
	t.Helper()
	data := make([]byte, 40<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	defer clear(data)
	encoded := base64.RawStdEncoding.EncodeToString(data)
	record := storage.Record{Version: 1, Scope: scope, ObjectID: "maximum_pq_record", RevisionID: "private_revision", Role: "content"}
	best := 0
	for low, high := 1, len(encoded); low <= high; {
		mid := low + (high-low)/2
		record.Payload = []byte("{ \"high_entropy\":\"" + encoded[:mid] + "\", \"precise\":900719925474099312345 }")
		if record.Validate(scope) == nil {
			best = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	record.Payload = []byte("{ \"high_entropy\":\"" + encoded[:best] + "\", \"precise\":900719925474099312345 }")
	raw, err := json.Marshal(record)
	if err != nil || record.Validate(scope) != nil || len(raw) < storage.MaxRecordBytes-4 {
		t.Fatal("maximum high-entropy record was not exercised", len(raw), err)
	}
	return record
}
