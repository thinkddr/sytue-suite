// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package drivehttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type privateSourceFixture struct {
	calls  int
	state  store.PrivateStorageState
	cursor store.PrivateSourceCursor
	err    error
}

func (f *privateSourceFixture) PrivateStorageState(context.Context) (store.PrivateStorageState, error) {
	f.calls++
	return f.state, f.err
}

func (f *privateSourceFixture) BeginPrivateStorageMigration(_ context.Context, epoch uuid.UUID) (store.PrivateStorageState, error) {
	f.calls++
	f.state.Epoch = epoch.String()
	return f.state, f.err
}

func (f *privateSourceFixture) PrivateStorageSourcePage(_ context.Context, _ uuid.UUID, after store.PrivateSourceCursor, _ int) (store.PrivateSourcePage, error) {
	f.calls++
	f.cursor = after
	return store.PrivateSourcePage{Scope: f.state.Scope, Epoch: f.state.Epoch, Rows: []store.PrivateSourceRow{{Kind: "file", ID: "source", Payload: []byte(`{"integer":900719925474099512345}`)}}}, f.err
}

func (f *privateSourceFixture) OpenPrivateStorageSource(context.Context, uuid.UUID, string, string) (store.PrivateSourceBlob, io.ReadCloser, error) {
	f.calls++
	return store.PrivateSourceBlob{StorageKey: "must-never-reach-client", SizeBytes: 8}, io.NopCloser(strings.NewReader("original")), f.err
}

func TestPrivateSourceHTTPRejectsScopeBeforeReadingBodyOrOpeningOriginal(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "drive"}
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	epoch := uuid.New()
	f := &privateSourceFixture{state: store.PrivateStorageState{Scope: scope, Mode: "frozen", Epoch: epoch.String(), StartedAt: &now, Rows: 1, Blobs: 1}}
	h := newHandlers(&fakeSvc{})
	h.migration = f
	router := chi.NewRouter()
	router.Get("/source", h.PrivateStorageState)
	router.Post("/source", h.BeginPrivateStorageMigration)
	router.Get("/source/{epoch}/rows", h.PrivateStorageSourcePage)
	router.Get("/source/{epoch}/blobs/{kind}/{source}", h.PrivateStorageSourceBlob)
	request := func(method, path, body, pin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set(storage.ScopeHeader, pin)
		ctx := store.WithOwner(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}), "alice")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r.WithContext(ctx))
		return w
	}
	for _, operation := range []struct{ method, path, body string }{{"GET", "/source", ""}, {"POST", "/source", `{"epoch":"` + epoch.String() + `"}`}, {"GET", "/source/" + epoch.String() + "/rows", ""}, {"GET", "/source/" + epoch.String() + "/blobs/file/source", ""}} {
		for _, bad := range []string{"", "v1.invalid", header + "," + header} {
			if w := request(operation.method, operation.path, operation.body, bad); w.Code < 400 {
				t.Fatal("invalid scope accepted", w.Code)
			}
		}
		if f.calls != 0 {
			t.Fatal("rejected scope touched source")
		}
	}
	for _, other := range []storage.Scope{{Tenant: "acme", Subject: "bob", Product: "drive"}, {Tenant: "other", Subject: "alice", Product: "drive"}, {Tenant: "acme", Subject: "alice", Product: "mail"}} {
		pin, err := storage.EncodeScopeHeader(other)
		if err != nil {
			t.Fatal(err)
		}
		if w := request("GET", "/source", "", pin); w.Code != 409 {
			t.Fatal("foreign scope accepted", w.Code)
		}
	}
	for _, path := range []string{"/source/bad/rows", "/source/" + epoch.String() + "/rows?after=invalid"} {
		if w := request("GET", path, "", header); w.Code != 400 {
			t.Fatal("bad source cursor accepted", w.Code)
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid request reached source")
	}
	w := request("POST", "/source", `{"epoch":"`+epoch.String()+`"}`, header)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("begin rejected", w.Code, w.Body.String())
	}
	w = request("GET", "/source/"+epoch.String()+"/rows", "", header)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "payload") || strings.Contains(w.Body.String(), "9007199254740995") {
		t.Fatal("raw domain bytes not framed", w.Code, w.Body.String())
	}
	w = request("GET", "/source/"+epoch.String()+"/blobs/file/source", "", header)
	if w.Code != 200 || w.Body.String() != "original" || w.Header().Get("Content-Length") != "8" || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "must-never") {
		t.Fatal("source stream changed", w.Code, w.Body.String())
	}
}

func TestPrivateMigrationHTTPBlocksLegacyWritesBeforeHandlerAndFailsClosed(t *testing.T) {
	f := &privateSourceFixture{state: store.PrivateStorageState{Mode: "frozen"}}
	h := newHandlers(&fakeSvc{})
	h.migration = f
	reached := 0
	handler := h.guardLegacyStorageWrites(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached++; w.WriteHeader(http.StatusNoContent) }))
	for _, path := range []string{"/api/v1/files", "/api/v1/shares", "/api/v1/files/x/content", "/api/v1/folders/x"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader("private body")))
		if w.Code != 409 {
			t.Fatal("legacy write accepted", path, w.Code)
		}
	}
	if reached != 0 {
		t.Fatal("legacy handler read body")
	}
	for _, path := range []string{"/api/v1/cipher/blobs/drive/opaque", "/api/v1/private-storage/migration", "/api/v1/storage/external", "/api/v1/quota"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, http.NoBody))
		if w.Code != 204 {
			t.Fatal("migration transport blocked", path, w.Code)
		}
	}
	f.state.Mode = "legacy"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/files", http.NoBody))
	if w.Code != 204 {
		t.Fatal("unmigrated owner blocked")
	}
	f.err = domain.ErrPermission
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/files", http.NoBody))
	if w.Code < 400 {
		t.Fatal("source state failure fell back to legacy")
	}
}
