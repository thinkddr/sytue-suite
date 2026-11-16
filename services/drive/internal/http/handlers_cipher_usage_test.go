// SPDX-License-Identifier: Apache-2.0
package drivehttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

type cipherUsageHTTPFixture struct {
	cipherHTTPFixture
	usage storage.CipherUsage
}

func (f *cipherUsageHTTPFixture) CipherUsage(context.Context, string) (storage.CipherUsage, error) {
	f.calls++
	return f.usage, nil
}

func TestCipherUsagePinsScopeBeforeAggregationAndDeniesForeignResult(t *testing.T) {
	for _, scenario := range []string{"match", "tenant", "subject", "product", "missing", "foreign_result"} {
		t.Run(scenario, func(t *testing.T) {
			scope := storage.Scope{Tenant: "current", Subject: "private-owner", Product: "mail"}
			f := &cipherUsageHTTPFixture{usage: storage.CipherUsage{Version: 1, Scope: scope}}
			h := newHandlers(&fakeSvc{})
			h.cipher = f
			ctx := store.WithOwner(tenancy.Into(t.Context(), tenancy.Tenant{Slug: "current"}), "private-owner")
			switch scenario {
			case "tenant":
				scope.Tenant = "other"
			case "subject":
				scope.Subject = "other"
			case "product":
				scope.Product = "drive"
			case "foreign_result":
				f.usage.Scope.Subject = "other"
			}
			request := httptest.NewRequest(http.MethodGet, "/cipher/usage/mail", http.NoBody).WithContext(ctx)
			encoded, err := storage.EncodeScopeHeader(scope)
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "missing" {
				request.Header.Set(storage.ScopeHeader, encoded)
			}
			router := chi.NewRouter()
			router.Get("/cipher/usage/{product}", h.CipherUsage)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			switch scenario {
			case "match":
				if response.Code != http.StatusOK || f.calls != 1 || response.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("matching usage failed", response.Code)
				}
			case "foreign_result":
				if response.Code < 400 || f.calls != 1 {
					t.Fatal("foreign service result escaped", response.Code)
				}
			default:
				if response.Code < 400 || f.calls != 0 {
					t.Fatal("scope mismatch reached aggregate", response.Code, f.calls)
				}
			}
		})
	}
}
