// SPDX-License-Identifier: Apache-2.0
//go:build integration && cgo

package matrix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth/authtest"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	photosCostCrypto struct {
		*Client
		calls, plainBytes atomic.Int64
	}
	photosCostWriter struct {
		io.Writer
		bytes *atomic.Int64
	}
)

func (w photosCostWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.bytes.Add(int64(n))
	return n, err
}

func (c *photosCostCrypto) DownloadStorageFile(ctx context.Context, gateway *api.Drive, ref storage.Reference, object, revision string, out io.Writer) (storage.FileRequest, error) {
	c.calls.Add(1)
	return c.Client.DownloadStorageFile(ctx, gateway, ref, object, revision, photosCostWriter{Writer: out, bytes: &c.plainBytes})
}

type (
	photosCostGateway struct {
		handler     http.Handler
		gets, bytes atomic.Int64
	}
	photosCostResponse struct {
		http.ResponseWriter
		bytes *atomic.Int64
	}
)

func (w photosCostResponse) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.bytes.Add(int64(n))
	return n, err
}

func (g *photosCostGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		g.gets.Add(1)
		g.handler.ServeHTTP(photosCostResponse{ResponseWriter: w, bytes: &g.bytes}, r)
		return
	}
	g.handler.ServeHTTP(w, r)
}

func TestPrivatePhotosMeasureBinaryReadsOnReopenAndFreshRecovery(t *testing.T) {
	if os.Getenv("SYTUE_MATRIX_PRIVATE_PHOTOS_COST") != "1" {
		t.Skip("requires isolated remote Synapse and explicit cost measurement")
	}
	for _, size := range []int{1 << 20, 8 << 20} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
			defer cancel()
			upstream, err := url.Parse(os.Getenv("SYTUE_MATRIX_PRIVATE_PHOTOS_URL"))
			if err != nil || upstream.Host == "" {
				t.Fatal("disposable Synapse URL required")
			}
			proxy := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(upstream))
			t.Cleanup(proxy.Close)
			scope := storage.Scope{Tenant: "acme", Subject: uuid.NewString(), Product: "photos"}
			device := newStorageSynapseDeviceForScope(ctx, t, proxy.URL, proxy.Client(), scope, storage.IsolatedIdentity)
			crypto := &photosCostCrypto{Client: device.Client}
			opaque := &fileGatewayFixture{product: "photos"}
			gateway := &photosCostGateway{handler: opaque}
			server := httptest.NewServer(gateway)
			t.Cleanup(server.Close)
			blobs := api.NewDrive(server.URL, nil, server.Client())
			photoID := uuid.NewString()
			body := []byte(strings.Repeat("p", size))
			media := model.MediaRow{ID: photoID, Tenant: scope.Tenant, Owner: scope.Subject, AssetID: photoID, Role: "original", Size: int64(size), SHA256: digestBytes(body), ContentType: "image/jpeg"}
			asset := []byte(`{"id":"` + photoID + `","tenant":"acme","owner_sub":"` + scope.Subject + `","drive_file_id":"retained-cost-original","name":"cost.jpg","media_type":"image/jpeg","bytes":` + strconv.Itoa(size) + `,"width":1,"height":1,"uploaded_at":"2026-10-30T12:00:00Z"}`)
			source := &photosRetainedSourceFixture{notesRetainedSourceFixture: notesRetainedSourceFixture{state: privatePhotos.SnapshotState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "legacy"}}, rows: map[string][]byte{"asset/" + photoID: asset, "original/" + photoID: body}}, blobs: 1}
			source.rows["media/"+photoID], _ = json.Marshal(media)
			httpSource := httptest.NewServer(source.handler(t))
			t.Cleanup(httpSource.Close)
			port, err := privatePhotos.NewHTTPSource(api.NewPhotos(httpSource.URL, nil, httpSource.Client()))
			if err != nil {
				t.Fatal(err)
			}
			open := func(client privatePhotos.Crypto) *privatePhotos.Repository {
				t.Helper()
				journal, err := offline.OpenWithSecrets(t.TempDir(), &authtest.Secrets{})
				if err != nil {
					t.Fatal(err)
				}
				r, err := privatePhotos.New(privatePhotos.Config{Context: ctx, Scope: scope, Crypto: client, Gateway: blobs, Journal: journal, Source: port})
				if err != nil {
					t.Fatal(err)
				}
				return r
			}
			r := open(crypto)
			defer r.Close()
			reset := func(client *photosCostCrypto) {
				client.calls.Store(0)
				client.plainBytes.Store(0)
				gateway.gets.Store(0)
				gateway.bytes.Store(0)
			}
			report := func(phase string, client *photosCostCrypto) {
				t.Logf("COST size=%d phase=%s sdk_downloads=%d plaintext_bytes=%d cipher_gets=%d cipher_bytes=%d", size, phase, client.calls.Load(), client.plainBytes.Load(), gateway.gets.Load(), gateway.bytes.Load())
			}
			reset(crypto)
			if err := r.ImportLegacySnapshot(ctx); err != nil {
				t.Fatal(err)
			}
			report("import", crypto)
			for _, phase := range []string{"reopen", "second_reopen"} {
				reset(crypto)
				if err := r.Initialize(ctx); err != nil {
					t.Fatal(err)
				}
				report(phase, crypto)
				if crypto.calls.Load() != 0 || crypto.plainBytes.Load() != 0 || gateway.bytes.Load() != 0 || gateway.gets.Load() != 0 {
					t.Fatal("metadata reopen fetched media bytes")
				}
			}
			if err := device.Client.Backup(ctx); err != nil {
				t.Fatal(err)
			}
			fresh := &photosCostCrypto{Client: freshNotesStorageDevice(ctx, t, proxy.URL, proxy.Client(), device)}
			recovered := open(fresh)
			defer recovered.Close()
			reset(fresh)
			if err := recovered.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			report("fresh", fresh)
			if fresh.calls.Load() != 0 || fresh.plainBytes.Load() != 0 || gateway.bytes.Load() != 0 || gateway.gets.Load() != 0 {
				t.Fatal("fresh metadata recovery fetched media bytes")
			}
			reset(fresh)
			if err := recovered.Original(ctx, photoID, io.Discard); err != nil {
				t.Fatal("fresh authorized device could not read actual original", err)
			}
			report("fresh_read", fresh)
			if fresh.calls.Load() != 1 || fresh.plainBytes.Load() != int64(size) {
				t.Fatal("fresh recovery skipped actual byte verification")
			}
			opaque.mu.Lock()
			opaque.tamper = true
			opaque.mu.Unlock()
			if err := recovered.Original(ctx, photoID, io.Discard); err == nil {
				t.Fatal("corrupt provider content accepted by read")
			}
			if _, err := recovered.AuditBinaries(ctx); err == nil {
				t.Fatal("corrupt provider content accepted by availability audit")
			}
			opaque.mu.Lock()
			opaque.tamper = false
			for key := range opaque.data {
				delete(opaque.data, key)
			}
			opaque.mu.Unlock()
			reset(fresh)
			if err := recovered.Initialize(ctx); err != nil {
				t.Fatal("authenticated commitment unexpectedly depended on provider availability", err)
			}
			if gateway.bytes.Load() != 0 || fresh.calls.Load() != 0 {
				t.Fatal("metadata recovery fetched absent provider content")
			}
			if err := recovered.Original(ctx, photoID, io.Discard); err == nil {
				t.Fatal("missing provider body accepted by read")
			}
			if _, err := recovered.AuditBinaries(ctx); err == nil {
				t.Fatal("missing provider body accepted by audit")
			}
			if source.opensCount() != 3 {
				t.Fatal("cost measurement unexpectedly reimported retained source")
			}
		})
	}
}
