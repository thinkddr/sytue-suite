// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type photoControlFixture struct {
	photoPrivateSource
	scope storage.Scope
	calls int
	err   error
	wrong bool
}

func (f *photoControlFixture) PlanRouting(_ context.Context, scope storage.Scope) (model.SourcePlanRouting, error) {
	f.calls++
	if scope != f.scope {
		return model.SourcePlanRouting{}, errors.New("wrong identity")
	}
	if f.wrong {
		scope.Subject = "other"
	}
	return model.SourcePlanRouting{Scope: scope}, f.err
}

func (f *photoControlFixture) Preflight(_ context.Context, scope storage.Scope, epoch string) (model.SourcePreflight, error) {
	f.calls++
	if scope != f.scope {
		return model.SourcePreflight{}, errors.New("wrong identity")
	}
	return model.SourcePreflight{Scope: scope, Epoch: epoch, CAS: "12345678-1234-1234-1234-123456789abc"}, f.err
}

func (f *photoControlFixture) AcceptPlan(_ context.Context, scope storage.Scope, epoch, _ string, link storage.Reference) (model.SourcePlanReference, error) {
	f.calls++
	if scope != f.scope {
		return model.SourcePlanReference{}, errors.New("wrong identity")
	}
	if f.wrong {
		link.EventID = "$other"
	}
	return model.SourcePlanReference{Epoch: epoch, Link: link}, f.err
}

func (f *photoControlFixture) StartPreflight(_ context.Context, scope storage.Scope, epoch string) (model.SourcePreflightInfo, error) {
	f.calls++
	if scope != f.scope {
		return model.SourcePreflightInfo{}, errors.New("wrong identity")
	}
	if f.wrong {
		scope.Subject = "other"
	}
	return model.SourcePreflightInfo{Scope: scope, Epoch: epoch, CAS: "12345678-1234-1234-1234-123456789abc", SHA256: strings.Repeat("1", 64)}, f.err
}

func photoControlRequest(t *testing.T, scope storage.Scope, method, path, body string) *http.Request {
	t.Helper()
	ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: scope.Tenant})
	ctx = authn.Into(ctx, authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject})
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	header, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set(storage.ScopeHeader, header)
	return r
}

func TestPhotosControlPlaneUsesSharedVerifiedScopeAndSanitizesFailures(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/private-storage/plan?subject=bob&tenant=other", ""},
		{http.MethodPost, "/private-storage/preflight", `{"epoch":"12345678-1234-1234-1234-123456789abc"}`},
		{http.MethodPost, "/private-storage/preflight/paged", `{"epoch":"12345678-1234-1234-1234-123456789abc"}`},
		{http.MethodPost, "/private-storage/plan", `{"epoch":"12345678-1234-1234-1234-123456789abc","cas":"12345678-1234-1234-1234-123456789abc","link":{"room_id":"!photos:test","event_id":"$plan"}}`},
	} {
		for _, change := range []string{"valid", "delegated", "foreign", "cancelled", "unavailable", "provider error", "wrong response"} {
			t.Run(route.method+route.path+change, func(t *testing.T) {
				fixture := &photoControlFixture{scope: scope}
				var port photoPrivateSource = fixture
				r := photoControlRequest(t, scope, route.method, route.path, route.body)
				want := http.StatusOK
				switch change {
				case "delegated":
					r = r.WithContext(authn.Into(r.Context(), authn.Claims{Tenant: scope.Tenant, Subject: scope.Subject, Delegado: true}))
					want = http.StatusForbidden
				case "foreign":
					foreign := scope
					foreign.Subject = "bob"
					header, _ := storage.EncodeScopeHeader(foreign)
					r.Header.Set(storage.ScopeHeader, header)
					want = http.StatusConflict
				case "cancelled":
					ctx, cancel := context.WithCancel(r.Context())
					cancel()
					r = r.WithContext(ctx)
					want = http.StatusServiceUnavailable
				case "unavailable":
					port = nil
					want = http.StatusServiceUnavailable
				case "provider error":
					fixture.err = errors.New("PRIVATE_PROVIDER_BODY_AND_KEY")
					want = http.StatusInternalServerError
				case "wrong response":
					fixture.wrong = true
					if route.path == "/private-storage/preflight" {
						return
					}
					want = http.StatusInternalServerError
				}
				router := chi.NewRouter()
				mountPhotosPrivateSource(router, port)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != want || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "PRIVATE_PROVIDER") {
					t.Fatal("unsafe control response", w.Code, w.Body.String())
				}
				if (change == "delegated" || change == "foreign" || change == "cancelled" || change == "unavailable") && fixture.calls != 0 {
					t.Fatal("unauthorized request reached source")
				}
			})
		}
	}
}
