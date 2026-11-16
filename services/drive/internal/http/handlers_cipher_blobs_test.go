// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package drivehttp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type cipherHTTPFixture struct {
	blob     domain.CipherBlob
	data     string
	calls    int
	err      error
	openBody io.ReadCloser
	openErr  error
}

func TestCipherGatewayRetryFailuresRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{service.ErrCipherBlobMissing, "cipher_blob_missing"},
		{service.ErrCipherBlobCorrupt, "cipher_blob_corrupt"},
		{service.ErrCipherBlobProvider, "cipher_blob_unavailable"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			h := newHandlers(&fakeSvc{})
			h.cipher = &cipherHTTPFixture{err: tc.err}
			router := chi.NewRouter()
			router.Put("/api/v1/cipher/blobs/{product}/{id}", h.PutCipherBlob)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/cipher/blobs/mail/"+strings.Repeat("A", 26), strings.NewReader("abc"))
			req.Header.Set("Content-Type", "application/octet-stream")
			header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "mail"})
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(storage.ScopeHeader, header)
			ctx := store.WithOwner(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req.WithContext(ctx))
			if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), tc.code) {
				t.Fatal("retry failure lost its reason", response.Code, response.Body.String())
			}
		})
	}
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
	f.calls++
	return f.blob, nil
}

func (f *cipherHTTPFixture) OpenCipherBlob(context.Context, string, string) (domain.CipherBlob, io.ReadCloser, error) {
	f.calls++
	if f.openBody != nil || f.openErr != nil {
		return f.blob, f.openBody, f.openErr
	}
	return f.blob, io.NopCloser(strings.NewReader(f.data)), nil
}

func (f *cipherHTTPFixture) DeleteCipherBlob(context.Context, string, string) error {
	f.calls++
	return nil
}

func TestCipherGatewayExposesOnlyOpaqueProtocol(t *testing.T) {
	fixture := &cipherHTTPFixture{}
	h := newHandlers(&fakeSvc{})
	h.cipher = fixture
	router := chi.NewRouter()
	router.Put("/api/v1/cipher/blobs/{product}/{id}", h.PutCipherBlob)
	router.Get("/api/v1/cipher/blobs/{product}/{id}", h.GetCipherBlob)
	router.Get("/api/v1/cipher/blobs/{product}/{id}/content", h.DownloadCipherBlob)
	router.Delete("/api/v1/cipher/blobs/{product}/{id}", h.DeleteCipherBlob)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := store.WithOwner(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
		router.ServeHTTP(w, r.WithContext(ctx))
	}))
	scopeHeader, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "mail"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	id := strings.Repeat("A", 26)
	target := server.URL + "/api/v1/cipher/blobs/mail/" + id
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, target, strings.NewReader("opaque"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set(storage.ScopeHeader, scopeHeader)
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
		req.Header.Set(storage.ScopeHeader, scopeHeader)
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
	req.Header.Set(storage.ScopeHeader, scopeHeader)
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
	ctx := store.WithOwner(tenancy.Into(req.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
	req = req.WithContext(ctx)
	header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(storage.ScopeHeader, header)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("product", "drive")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	rec := httptest.NewRecorder()
	h.PutCipherBlob(rec, req)
	if rec.Code != http.StatusBadRequest || h.cipher.(*cipherHTTPFixture).calls != 1 {
		t.Fatal("size mismatch was not a client error", rec.Code)
	}
}

func TestCipherScopePinnedBeforeService(t *testing.T) {
	for _, operation := range []string{"put", "get", "content", "delete"} {
		t.Run(operation, func(t *testing.T) {
			for _, scenario := range []string{"match", "tenant", "subject", "product", "missing", "malformed", "duplicate", "unauthenticated"} {
				t.Run(scenario, func(t *testing.T) {
					emptyHash := sha256.Sum256(nil)
					fixture := &cipherHTTPFixture{blob: domain.CipherBlob{BlobReceipt: storage.BlobReceipt{ID: strings.Repeat("A", 26), Product: "drive", SHA256: hex.EncodeToString(emptyHash[:])}}}
					h := newHandlers(&fakeSvc{})
					h.cipher = fixture
					router := chi.NewRouter()
					path := "/cipher/blobs/{product}/{id}"
					router.Put(path, h.PutCipherBlob)
					router.Get(path, h.GetCipherBlob)
					router.Get(path+"/content", h.DownloadCipherBlob)
					router.Delete(path, h.DeleteCipherBlob)
					ctx := t.Context()
					if scenario != "unauthenticated" {
						ctx = store.WithOwner(tenancy.Into(ctx, tenancy.Tenant{Slug: "current"}), "private-owner")
					}
					expected := storage.Scope{Tenant: "current", Subject: "private-owner", Product: "drive"}
					if scenario == "tenant" {
						expected.Tenant = "original"
					}
					if scenario == "subject" {
						expected.Subject = "old-owner"
					}
					if scenario == "product" {
						expected.Product = "mail"
					}
					payload, err := json.Marshal(expected)
					if err != nil {
						t.Fatal(err)
					}
					method, target := http.MethodGet, "/cipher/blobs/drive/"+strings.Repeat("A", 26)
					switch operation {
					case "put":
						method = http.MethodPut
					case "delete":
						method = http.MethodDelete
					case "content":
						target += "/content"
					}
					req := httptest.NewRequest(method, target, strings.NewReader("opaque")).WithContext(ctx)
					req.Header.Set("Content-Type", "application/octet-stream")
					value := "v1." + base64.RawURLEncoding.EncodeToString(payload)
					if scenario != "missing" {
						req.Header.Set(storage.ScopeHeader, value)
					}
					if scenario == "malformed" {
						req.Header.Set(storage.ScopeHeader, "v1.invalid")
					}
					if scenario == "duplicate" {
						req.Header.Add(storage.ScopeHeader, value)
					}
					response := httptest.NewRecorder()
					router.ServeHTTP(&cipherDeadlineRecorder{response}, req)
					if scenario == "match" {
						if response.Code < 200 || response.Code >= 300 || fixture.calls != 1 {
							t.Fatal("matching account scope rejected", response.Code, fixture.calls)
						}
					} else if response.Code < 400 || fixture.calls != 0 {
						t.Fatal("cipher operation escaped the original account scope", response.Code, fixture.calls)
					}
				})
			}
		})
	}
}
