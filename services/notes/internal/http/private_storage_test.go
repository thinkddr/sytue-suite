// SPDX-License-Identifier: Apache-2.0
package noteshttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/store"
)

var privateNotesHTTPScope = storage.Scope{Tenant: "acme", Subject: "alice", Product: "notes"}

type notesSourceHTTPFixture struct {
	calls   int
	state   storage.AppMigrationState
	body    []byte
	failure error
}

func (f *notesSourceHTTPFixture) PrivateStorageState(_ context.Context, owner string) (storage.AppMigrationState, error) {
	f.calls++
	if owner != "alice" {
		panic("wrong owner")
	}
	return f.state, f.failure
}

func (f *notesSourceHTTPFixture) BeginPrivateStorageMigration(_ context.Context, owner string, epoch uuid.UUID) (storage.AppMigrationState, error) {
	f.calls++
	if owner != "alice" {
		panic("wrong owner")
	}
	now := time.Now().UTC()
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: privateNotesHTTPScope, Mode: "frozen", Epoch: epoch.String(), StartedAt: &now, Rows: 1}}, f.failure
}

func (f *notesSourceHTTPFixture) PrivateStorageSourcePage(_ context.Context, owner string, epoch uuid.UUID, _ storage.LegacySourceCursor, _ int) (storage.StreamSourcePage, error) {
	f.calls++
	if owner != "alice" {
		panic("wrong owner")
	}
	sum := sha256.Sum256(f.body)
	return storage.StreamSourcePage{Scope: privateNotesHTTPScope, Epoch: epoch.String(), Rows: []storage.StreamSourceRow{{Kind: "audit", ID: "42", Size: int64(len(f.body)), SHA256: hex.EncodeToString(sum[:])}}}, f.failure
}

func (f *notesSourceHTTPFixture) PrivateStorageSourceBody(_ context.Context, owner string, _ uuid.UUID, _, _ string) (store.PrivateSourceBody, error) {
	f.calls++
	if owner != "alice" {
		panic("wrong owner")
	}
	sum := sha256.Sum256(f.body)
	return store.PrivateSourceBody{Data: []byte(string(f.body)), Size: int64(len(f.body)), SHA256: hex.EncodeToString(sum[:])}, f.failure
}

func notesSourceRequest(t *testing.T, method, path, body string, scope *storage.Scope) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	ctx := tenancy.Into(request.Context(), tenancy.Tenant{Slug: "acme"})
	ctx = authn.Into(ctx, authn.Claims{Subject: "alice"})
	request = request.WithContext(ctx)
	if scope != nil {
		header, err := storage.EncodeScopeHeader(*scope)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set(storage.ScopeHeader, header)
	}
	return request
}

func TestPrivateNotesSourceScopeChecksBeforeServiceIO(t *testing.T) {
	for _, name := range []string{"missing", "other-subject", "other-tenant", "other-product", "duplicate", "anonymous"} {
		t.Run(name, func(t *testing.T) {
			fixture := &notesSourceHTTPFixture{}
			handler := &Handlers{migration: fixture}
			scope := privateNotesHTTPScope
			switch name {
			case "other-subject":
				scope.Subject = "bob"
			case "other-tenant":
				scope.Tenant = "other"
			case "other-product":
				scope.Product = "drive"
			}
			request := notesSourceRequest(t, http.MethodGet, "/migration", "", &scope)
			if name == "missing" {
				request.Header.Del(storage.ScopeHeader)
			}
			if name == "duplicate" {
				request.Header.Add(storage.ScopeHeader, request.Header.Get(storage.ScopeHeader))
			}
			if name == "anonymous" {
				request = request.WithContext(tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"}))
			}
			out := httptest.NewRecorder()
			handler.PrivateStorageState(out, request)
			if out.Code == http.StatusOK || fixture.calls != 0 {
				t.Fatal("scope reached source", out.Code, fixture.calls)
			}
		})
	}
}

func TestPrivateNotesSourceVersionedStatePreflightAndExactBody(t *testing.T) {
	fixture := &notesSourceHTTPFixture{state: storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: privateNotesHTTPScope, Mode: "legacy"}}, body: []byte(`{"id":42,"future":{"n":900719925474099312345}}`)}
	handler := &Handlers{migration: fixture}
	out := httptest.NewRecorder()
	handler.PrivateStorageState(out, notesSourceRequest(t, http.MethodGet, "/migration", "", &privateNotesHTTPScope))
	var state storage.AppMigrationState
	if out.Code != http.StatusOK || json.Unmarshal(out.Body.Bytes(), &state) != nil || state.Validate(privateNotesHTTPScope) != nil || out.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid source state", out.Code, out.Body.String())
	}
	epoch := uuid.NewString()
	fixture.failure = store.ErrPrivateStorageAttachment
	out = httptest.NewRecorder()
	handler.BeginPrivateStorage(out, notesSourceRequest(t, http.MethodPost, "/migration", `{"epoch":"`+epoch+`"}`, &privateNotesHTTPScope))
	if out.Code != http.StatusConflict || !strings.Contains(out.Body.String(), "private_notes_attachment_import_required") {
		t.Fatal("unsupported source did not report safe preflight", out.Code, out.Body.String())
	}
	fixture.failure = nil
	router := chi.NewRouter()
	router.Get("/migration/{epoch}/source/{kind}/{source}", handler.PrivateStorageBody)
	out = httptest.NewRecorder()
	router.ServeHTTP(out, notesSourceRequest(t, http.MethodGet, "/migration/"+epoch+"/source/audit/42", "", &privateNotesHTTPScope))
	sum := sha256.Sum256(fixture.body)
	if out.Code != http.StatusOK || out.Body.String() != string(fixture.body) || out.Header().Get("Content-Type") != "application/octet-stream" || out.Header().Get("X-Sytue-Source-SHA256") != hex.EncodeToString(sum[:]) || out.Header().Get("Cache-Control") != "no-store, no-transform" {
		t.Fatal("source body changed", out.Code, out.Body.String())
	}
}
