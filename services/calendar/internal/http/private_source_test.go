// SPDX-License-Identifier: Apache-2.0
package calendarhttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/calendar/internal/store"
)

type calendarSourceFixture struct {
	calls int
	owner string
	scope storage.Scope
}

func (s *calendarSourceFixture) PrivateStorageState(_ context.Context, owner string) (storage.AppMigrationState, error) {
	s.calls++
	s.owner = owner
	return storage.AppMigrationState{Version: 1, LegacyMigrationState: storage.LegacyMigrationState{Scope: s.scope, Mode: "legacy"}}, nil
}

func (s *calendarSourceFixture) BeginPrivateStorageMigration(_ context.Context, owner string, _ uuid.UUID) (storage.AppMigrationState, error) {
	s.calls++
	s.owner = owner
	return storage.AppMigrationState{}, store.ErrPrivateStorageIngress
}

func (s *calendarSourceFixture) PrivateStorageSourcePage(context.Context, string, uuid.UUID, storage.LegacySourceCursor, int) (storage.StreamSourcePage, error) {
	s.calls++
	return storage.StreamSourcePage{}, store.ErrPrivateStorageEpoch
}

func (s *calendarSourceFixture) OpenPrivateStorageSource(context.Context, string, uuid.UUID, string, string) (storage.StreamSourceRow, io.ReadCloser, error) {
	s.calls++
	return storage.StreamSourceRow{}, nil, store.ErrPrivateStorageEpoch
}

func calendarSourceRequest(t *testing.T, scope storage.Scope, method, path, body string) *http.Request {
	t.Helper()
	ctx := authn.Into(tenancy.Into(context.Background(), tenancy.Tenant{Slug: scope.Tenant}), authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject, Email: "alice@example.invalid"})
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	value, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(storage.ScopeHeader, value)
	return r
}

func TestCalendarPrivateSourceMountKeepsBeginClosedAndUsesVerifiedOwner(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	fixture := &calendarSourceFixture{scope: scope}
	router := chi.NewRouter()
	mountCalendarPrivateSource(router, fixture)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, calendarSourceRequest(t, scope, http.MethodGet, "/private-storage/migration?subject=bob&tenant=foreign", ""))
	if w.Code != http.StatusOK || fixture.owner != "alice" {
		t.Fatal("source route used URL identity", w.Code, fixture.owner)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, calendarSourceRequest(t, scope, http.MethodPost, "/private-storage/migration", `{"epoch":"12345678-1234-1234-1234-123456789abc"}`))
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "private_calendar_ingress_required") {
		t.Fatal("Calendar Begin opened without continuing ingress", w.Code, w.Body.String())
	}
	before := fixture.calls
	r := calendarSourceRequest(t, scope, http.MethodGet, "/private-storage/migration", "")
	r = r.WithContext(authn.Into(r.Context(), authn.Claims{Tenant: "acme", Subject: "alice", Delegado: true}))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || fixture.calls != before {
		t.Fatal("delegated source read reached Calendar store", w.Code, fixture.calls)
	}
	foreign := scope
	foreign.Subject = "bob"
	value, _ := storage.EncodeScopeHeader(foreign)
	r = calendarSourceRequest(t, scope, http.MethodGet, "/private-storage/migration", "")
	r.Header.Set(storage.ScopeHeader, value)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusConflict || fixture.calls != before {
		t.Fatal("forged source header reached Calendar store", w.Code, fixture.calls)
	}
}

func TestCalendarPrivateSourceMissingStoreNeverMeansEmptyReadyAccount(t *testing.T) {
	router := chi.NewRouter()
	mountCalendarPrivateSource(router, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, calendarSourceRequest(t, storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}, http.MethodGet, "/private-storage/migration", ""))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("missing source became a ready empty calendar", w.Code)
	}
}

func TestCalendarPrivateSourcePortRejectsForeignProductOrTenantBeforeStore(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	fixture := &calendarSourceFixture{scope: scope}
	port := calendarPrivateSource{store: fixture}
	ctx := tenancy.Into(context.Background(), tenancy.Tenant{Slug: "acme"})
	for _, foreign := range []storage.Scope{{Tenant: "foreign", Subject: "alice", Product: "calendar"}, {Tenant: "acme", Subject: "alice", Product: "contacts"}} {
		if _, err := port.State(ctx, foreign); !errors.Is(err, pg.ErrPrivateStorageScope) || fixture.calls != 0 {
			t.Fatal("foreign port scope reached Calendar store", err, fixture.calls)
		}
	}
}
