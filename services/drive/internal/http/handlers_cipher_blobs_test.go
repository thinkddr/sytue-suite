// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package drivehttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

type cipherHTTPFixture struct {
	blob  domain.CipherBlob
	data  string
	calls int
	err   error
}

func (f *cipherHTTPFixture) PutCipherBlob(_ context.Context, product, id string, size int64, body io.Reader) (domain.CipherBlob, error) {
	f.calls++
	if f.err != nil {
		return domain.CipherBlob{}, f.err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return domain.CipherBlob{}, err
	}
	sum := sha256.Sum256(data)
	f.data = string(data)
	f.blob = domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: id, Product: product, SizeBytes: size, SHA256: hex.EncodeToString(sum[:])}, OwnerSub: "private-owner", StorageKey: "private-internal-key"}
	return f.blob, nil
}

func (f *cipherHTTPFixture) CipherBlob(context.Context, string, string) (domain.CipherBlob, error) {
	return f.blob, nil
}

func (f *cipherHTTPFixture) OpenCipherBlob(context.Context, string, string) (domain.CipherBlob, io.ReadCloser, error) {
	return f.blob, io.NopCloser(strings.NewReader(f.data)), nil
}
func (f *cipherHTTPFixture) DeleteCipherBlob(context.Context, string, string) error { return nil }

func TestCipherGatewayExposesOnlyOpaqueProtocol(t *testing.T) {
	fixture := &cipherHTTPFixture{}
	h := newHandlers(&fakeSvc{})
	h.cipher = fixture
	router := chi.NewRouter()
	router.Put("/api/v1/cipher/blobs/{product}/{id}", h.PutCipherBlob)
	router.Get("/api/v1/cipher/blobs/{product}/{id}", h.GetCipherBlob)
	router.Get("/api/v1/cipher/blobs/{product}/{id}/content", h.DownloadCipherBlob)
	router.Delete("/api/v1/cipher/blobs/{product}/{id}", h.DeleteCipherBlob)
	server := httptest.NewServer(router)
	defer server.Close()
	id := strings.Repeat("A", 26)
	target := server.URL + "/api/v1/cipher/blobs/mail/" + id
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, target, strings.NewReader("opaque"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var receipt storage.BlobReceipt
	err = json.NewDecoder(resp.Body).Decode(&receipt)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusCreated || !receipt.Valid("mail", id) {
		t.Fatal("invalid HTTP receipt", err, resp.StatusCode)
	}
	for _, suffix := range []string{"", "/content"} {
		req, err = http.NewRequestWithContext(t.Context(), http.MethodGet, target+suffix, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}
		resp, err = server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatal("download failed or became cacheable", err)
		}
		if suffix == "/content" {
			if string(raw) != "opaque" || resp.Header.Get("Content-Type") != "application/octet-stream" {
				t.Fatal("content changed")
			}
		} else if strings.Contains(string(raw), "private-") {
			t.Fatal("storage key or owner leaked")
		}
	}
	req, err = http.NewRequestWithContext(t.Context(), http.MethodDelete, target, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatal("delete", resp.StatusCode)
	}
}

func TestCipherGatewayRejectsEncodedUnknownAndOversizedBodiesBeforeService(t *testing.T) {
	for _, scenario := range []string{"mime", "encoding", "unknown", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := &cipherHTTPFixture{}
			h := newHandlers(&fakeSvc{})
			h.cipher = fixture
			req := mkReq(http.MethodPut, "/", "abc", nil)
			req.Header.Set("Content-Type", "application/octet-stream")
			switch scenario {
			case "mime":
				req.Header.Set("Content-Type", "text/plain")
			case "encoding":
				req.Header.Set("Content-Encoding", "gzip")
			case "unknown":
				req.ContentLength = -1
			case "oversized":
				req.ContentLength = storage.MaxCipherBlockBytes + 1
			}
			rec := httptest.NewRecorder()
			h.PutCipherBlob(rec, req)
			if rec.Code != http.StatusBadRequest || fixture.calls != 0 {
				t.Fatal("invalid body reached storage", rec.Code)
			}
		})
	}
	h := newHandlers(&fakeSvc{})
	h.cipher = &cipherHTTPFixture{err: service.ErrUploadSizeMismatch}
	req := mkReq(http.MethodPut, "/", "abc", nil)
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	h.PutCipherBlob(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatal("size mismatch was not a client error", rec.Code)
	}
}
