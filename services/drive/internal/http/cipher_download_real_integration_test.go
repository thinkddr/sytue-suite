// SPDX-License-Identifier: Apache-2.0
//go:build integration

package drivehttp

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
	"git.sytue.eu/sytue/one/services/drive/internal/testsupport/cipherfixture"
)

// Exercise the actual S3 client, Drive service and download handler. The
// opaque gzip stream makes accidental legacy decompression observable.
func TestCipherDownloadRealS3VerifiesRawObject(t *testing.T) {
	fixture := cipherfixture.New(t)
	tenant := tenancy.Tenant{Slug: "acme"}
	owner := store.WithOwner(tenancy.Into(t.Context(), tenant), "alice")
	scope := storage.Scope{Subject: "alice", Tenant: tenant.Slug, Product: "drive"}
	var compressed bytes.Buffer
	encoder := gzip.NewWriter(&compressed)
	if _, err := encoder.Write([]byte("opaque test bytes; not a private message")); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	opaque := bytes.Clone(compressed.Bytes())
	id := strings.Repeat("A", 26)
	saved, err := fixture.Service.PutCipherBlob(owner, scope.Product, id, int64(len(opaque)), bytes.NewReader(opaque))
	if err != nil {
		t.Fatal("store opaque bytes:", err)
	}
	key := tenant.StoragePrefix() + saved.StorageKey
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{cipher: fixture.Service}
	router := chi.NewRouter()
	router.Get("/cipher/{product}/{id}/content", h.DownloadCipherBlob)
	download := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/cipher/drive/"+id+"/content", http.NoBody).WithContext(owner)
		req.Header.Set(storage.ScopeHeader, header)
		response := httptest.NewRecorder()
		router.ServeHTTP(&cipherDeadlineRecorder{response}, req)
		return response
	}
	put := func(t *testing.T, data []byte, opts minio.PutObjectOptions) {
		t.Helper()
		if _, err := fixture.S3.PutObject(t.Context(), fixture.Bucket, key, bytes.NewReader(data), int64(len(data)), opts); err != nil {
			t.Fatal("replace isolated provider object:", err)
		}
	}
	assertSuccess := func(t *testing.T, response *httptest.ResponseRecorder) {
		t.Helper()
		if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), opaque) || response.Header().Get("Content-Length") != strconv.Itoa(len(opaque)) || response.Header().Get("Content-Type") != "application/octet-stream" || len(response.Header().Values("Content-Encoding")) != 0 {
			t.Fatalf("verified raw download failed: status=%d", response.Code)
		}
	}
	assertRejected := func(t *testing.T, response *httptest.ResponseRecorder, code string) {
		t.Helper()
		if response.Code != http.StatusServiceUnavailable || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") || !json.Valid(response.Body.Bytes()) || !bytes.Contains(response.Body.Bytes(), []byte(code)) || bytes.Contains(response.Body.Bytes(), opaque[:8]) || bytes.Contains(response.Body.Bytes(), []byte("unexpected-suffix")) {
			t.Fatalf("invalid provider object escaped: status=%d", response.Code)
		}
	}
	assertSuccess(t, download(t))

	t.Run("suffix", func(t *testing.T) {
		altered := append(bytes.Clone(opaque), []byte("unexpected-suffix")...)
		put(t, altered, minio.PutObjectOptions{})
		assertRejected(t, download(t), "cipher_blob_corrupt")
	})
	t.Run("gzip metadata", func(t *testing.T) {
		put(t, opaque, minio.PutObjectOptions{UserMetadata: map[string]string{"sytue-codificacion": "gzip"}})
		info, err := fixture.S3.StatObject(t.Context(), fixture.Bucket, key, minio.StatObjectOptions{})
		if err != nil {
			t.Fatal("stat gzip metadata:", err)
		}
		found := false
		for k, v := range info.UserMetadata {
			if strings.EqualFold(strings.TrimPrefix(strings.ToLower(k), "x-amz-meta-"), "sytue-codificacion") && v == "gzip" {
				found = true
			}
		}
		if !found {
			t.Fatal("provider did not retain gzip user metadata")
		}
		assertSuccess(t, download(t))
	})
	t.Run("content encoding", func(t *testing.T) {
		put(t, opaque, minio.PutObjectOptions{ContentEncoding: "gzip"})
		info, err := fixture.S3.StatObject(t.Context(), fixture.Bucket, key, minio.StatObjectOptions{})
		if err != nil || !strings.EqualFold(info.Headers.Get("Content-Encoding"), "gzip") {
			t.Fatal("provider did not retain Content-Encoding gzip:", err)
		}
		assertRejected(t, download(t), "cipher_blob_unavailable")
	})
}
