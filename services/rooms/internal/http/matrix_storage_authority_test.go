// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
)

type authorityServiceFixture struct {
	calls int
	scope storage.Scope
}

func (f *authorityServiceFixture) MatrixStorageAuthority(_ context.Context, actor, product string) (matrixproof.Authority, error) {
	f.calls++
	f.scope = storage.Scope{Tenant: "acme", Subject: actor, Product: product}
	return matrixproof.Authority{Version: 1, Scope: f.scope}, nil
}

func (f *authorityServiceFixture) MatrixRootChallenge(_ context.Context, actor, product, _ string, _ storage.PublicationTarget) (matrixproof.RootChallenge, error) {
	f.calls++
	f.scope = storage.Scope{Tenant: "acme", Subject: actor, Product: product}
	return matrixproof.RootChallenge{}, nil
}

func (f *authorityServiceFixture) MatrixRootChangeChallenge(context.Context, string, string, string, string, storage.PublicationTarget) (matrixproof.RootChallenge, error) {
	f.calls++
	return matrixproof.RootChallenge{}, nil
}

func (f *authorityServiceFixture) ConfirmMatrixRoot(_ context.Context, _, _ string, _ matrixproof.SignedAction) (matrixproof.Authority, error) {
	f.calls++
	return matrixproof.Authority{}, nil
}

func (f *authorityServiceFixture) VerifyMatrixStorageAction(context.Context, string, string, matrixproof.Action, matrixproof.SignedAction) (matrixproof.Authorization, error) {
	f.calls++
	return matrixproof.Authorization{}, nil
}

func TestStorageAuthorityHTTPUsesVerifiedOwnerAndRejectsDelegation(t *testing.T) {
	for _, fault := range []string{"valid", "delegated", "missing", "foreign-header", "invalid-product"} {
		t.Run(fault, func(t *testing.T) {
			f := &authorityServiceFixture{}
			handler := storageAuthorityHandlers{svc: f}
			router := chi.NewRouter()
			router.Get("/matrix/storage/{product}/authority", handler.get)
			identity := claims()
			identity.Subject = "owner"
			identity.Tenant = "acme"
			path := "/matrix/storage/mail/authority"
			switch fault {
			case "delegated":
				identity.Delegado = true
			case "missing":
				identity = nil
			case "invalid-product":
				path = "/matrix/storage/unknown/authority"
			}
			request := req(http.MethodGet, path, "", identity, nil)
			request = request.WithContext(tenancy.Into(request.Context(), tenancy.Tenant{Slug: "acme"}))
			header, err := storage.EncodeScopeHeader(storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"})
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set(storage.ScopeHeader, header)
			if fault == "foreign-header" {
				scope := storage.Scope{Tenant: "acme", Subject: "other", Product: "mail"}
				header, err := storage.EncodeScopeHeader(scope)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set(storage.ScopeHeader, header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if fault == "valid" {
				if response.Code != http.StatusOK || f.calls != 1 || f.scope.Subject != "owner" || f.scope.Product != "mail" {
					t.Fatal("lost caller identity", response.Code, f.calls)
				}
			} else if response.Code < 400 || f.calls != 0 {
				t.Fatal("unverified identity reached authority", fault, response.Code, f.calls)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("authority response can be cached")
			}
		})
	}
}

func TestInternalStorageAuthorityRouteRequiresExistingSharedSecret(t *testing.T) {
	for _, tc := range []struct {
		configured, auth string
		want             int
	}{{"", "", http.StatusNotFound}, {"synthetic-internal", "", http.StatusUnauthorized}, {"synthetic-internal", "Bearer wrong", http.StatusUnauthorized}, {"synthetic-internal", "Bearer synthetic-internal", http.StatusBadRequest}} {
		for _, path := range []string{"/internal/storage-authority/verify", "/internal/storage-authority/state", "/internal/storage-authority/receipt", "/internal/storage-authority/device-fence", "/internal/storage-authority/device-fence/state"} {
			router := NewRouter(Deps{Cfg: config.Config{InternalSecret: tc.configured}})
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", tc.auth)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatal("internal authority gate", response.Code, tc.want)
			}
		}
	}
}
