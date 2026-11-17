// SPDX-License-Identifier: Apache-2.0
package contactshttp

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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/contacts/internal/store"
)

type contactsSourceFixture struct {
	calls      int
	owner      string
	state      store.PrivateStorageState
	cursor     storage.LegacySourceCursor
	raw        string
	body       *contactsSourceBody
	descriptor storage.StreamSourceRow
	err        error
}

func (f *contactsSourceFixture) PrivateStorageState(_ context.Context, owner string) (store.PrivateStorageState, error) {
	f.calls++
	f.owner = owner
	return f.state, f.err
}

func (f *contactsSourceFixture) BeginPrivateStorageMigration(_ context.Context, owner string, _ uuid.UUID) (store.PrivateStorageState, error) {
	f.calls++
	f.owner = owner
	return f.state, f.err
}

func (f *contactsSourceFixture) PrivateStorageSourcePage(_ context.Context, owner string, _ uuid.UUID, cursor storage.LegacySourceCursor) (storage.StreamSourcePage, error) {
	f.calls++
	f.owner = owner
	f.cursor = cursor
	return storage.StreamSourcePage{Scope: f.state.Scope, Epoch: f.state.Epoch, Rows: []storage.StreamSourceRow{f.descriptor}}, f.err
}

func (f *contactsSourceFixture) OpenPrivateStorageSource(_ context.Context, owner string, _ uuid.UUID, _, _ string) (storage.StreamSourceRow, io.ReadCloser, error) {
	f.calls++
	f.owner = owner
	f.body = &contactsSourceBody{Reader: strings.NewReader(f.raw)}
	return f.descriptor, f.body, f.err
}

type contactsSourceBody struct {
	*strings.Reader
	closed bool
}

func (b *contactsSourceBody) Close() error { b.closed = true; return nil }

type unreadableSourceBody struct{ read bool }

func (b *unreadableSourceBody) Read([]byte) (int, error) { b.read = true; return 0, io.EOF }
func (b *unreadableSourceBody) Close() error             { return nil }

func sourceHTTPFixture(t *testing.T) (*contactsSourceFixture, http.Handler, string) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "user-1", Product: "contacts"}
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	raw := `{"note":"private fixture","integer":900719925474099512345}`
	digest := sha256.Sum256([]byte(raw))
	f := &contactsSourceFixture{state: store.PrivateStorageState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: scope, Mode: "frozen", Epoch: uuid.NewString(), Rows: 1, StartedAt: &now}}, raw: raw, descriptor: storage.StreamSourceRow{Kind: "card", ID: uuid.NewString(), Size: int64(len(raw)), SHA256: hex.EncodeToString(digest[:])}}
	h := &Handlers{source: f}
	router := chi.NewRouter()
	router.Get("/source", h.PrivateStorageState)
	router.Post("/source", h.BeginPrivateStorageMigration)
	router.Get("/source/{epoch}/rows", h.PrivateStorageSourcePage)
	router.Get("/source/{epoch}/blobs/{kind}/{id}", h.OpenPrivateStorageSource)
	return f, router, header
}

func sourceRequest(router http.Handler, method, path, body, header string) *httptest.ResponseRecorder {
	r := req(method, path, body, nil)
	r.Header.Set(storage.ScopeHeader, header)
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestPrivateContactsSourceRejectsForeignIdentityBeforeReadingOrOpening(t *testing.T) {
	f, router, header := sourceHTTPFixture(t)
	for _, operation := range []struct{ method, path string }{
		{"GET", "/source"}, {"POST", "/source"}, {"GET", "/source/" + f.state.Epoch + "/rows"}, {"GET", "/source/" + f.state.Epoch + "/blobs/card/" + f.descriptor.ID},
	} {
		for _, pin := range []string{"", "v1.invalid", header + "," + header} {
			w := sourceRequest(router, operation.method, operation.path, "{}", pin)
			if w.Code != http.StatusBadRequest || f.calls != 0 {
				t.Fatal("invalid identity touched source", w.Code, f.calls)
			}
		}
	}
	for _, other := range []storage.Scope{{Tenant: "acme", Subject: "another", Product: "contacts"}, {Tenant: "other", Subject: "user-1", Product: "contacts"}, {Tenant: "acme", Subject: "user-1", Product: "mail"}} {
		pin, err := storage.EncodeScopeHeader(other)
		if err != nil {
			t.Fatal(err)
		}
		if w := sourceRequest(router, "GET", "/source", "", pin); w.Code != http.StatusConflict || f.calls != 0 {
			t.Fatal("foreign identity touched source", w.Code)
		}
	}
	body := &unreadableSourceBody{}
	r := req("POST", "/source", "", nil)
	r.Body = body
	r = r.WithContext(tenancy.Into(r.Context(), tenancy.Tenant{Slug: "acme"}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if body.read || f.calls != 0 {
		t.Fatal("read rejected request body")
	}
}

func TestPrivateContactsSourceStreamsExactBytesAndPinsPages(t *testing.T) {
	f, router, header := sourceHTTPFixture(t)
	w := sourceRequest(router, "POST", "/source", `{"epoch":"`+f.state.Epoch+`"}`, header)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || f.owner != "user-1" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = sourceRequest(router, "GET", "/source/"+f.state.Epoch+"/rows", "", header)
	var page storage.StreamSourcePage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Rows) != 1 || strings.Contains(w.Body.String(), "private fixture") {
		t.Fatal("invalid descriptor page", w.Code)
	}
	w = sourceRequest(router, "GET", "/source/"+f.state.Epoch+"/blobs/card/"+f.descriptor.ID, "", header)
	if w.Code != 200 || w.Body.String() != f.raw || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("X-Sytue-Source-SHA256") != f.descriptor.SHA256 || f.body == nil || !f.body.closed {
		t.Fatal("unbound/unclosed source stream", w.Code, w.Body.String())
	}
	f.descriptor.ID = uuid.NewString()
	w = sourceRequest(router, "GET", "/source/"+f.state.Epoch+"/blobs/card/"+uuid.NewString(), "", header)
	if w.Code != 500 || !f.body.closed {
		t.Fatal("accepted wrong descriptor or left body open", w.Code)
	}
	f.state.Scope.Subject = "other"
	w = sourceRequest(router, "GET", "/source/"+f.state.Epoch+"/rows", "", header)
	if w.Code != 500 {
		t.Fatal("accepted foreign page", w.Code)
	}
}

func TestPrivateContactsSourceRejectsMalformedEpochBodyAndCursor(t *testing.T) {
	f, router, header := sourceHTTPFixture(t)
	for _, path := range []string{"/source/bad/rows", "/source/" + f.state.Epoch + "/rows?after=bad"} {
		if w := sourceRequest(router, "GET", path, "", header); w.Code != 400 || f.calls != 0 {
			t.Fatal("invalid cursor reached source", w.Code)
		}
	}
	for _, body := range []string{`{}`, `{"epoch":"bad"}`, `{"epoch":"` + f.state.Epoch + `","unknown":true}`} {
		if w := sourceRequest(router, "POST", "/source", body, header); w.Code != 400 || f.calls != 0 {
			t.Fatal("invalid begin reached source", w.Code)
		}
	}
}
