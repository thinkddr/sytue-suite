// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type cutoverHTTPService struct {
	fakeService
	scope   storage.Scope
	epoch   string
	receipt storage.Reference
	calls   int
}

func (s *cutoverHTTPService) result(ctx context.Context, actor, product, phase string) storage.IdentityCutoverState {
	s.calls++
	s.scope = storage.Scope{Tenant: tenancy.MustFrom(ctx).Slug, Subject: actor, Product: product}
	state := storage.IdentityCutoverState{Version: 1, Scope: s.scope, Phase: phase, SourceRooms: []string{}}
	if phase != "legacy" {
		state.Epoch = s.epoch
	}
	if phase == "isolated" {
		state.Receipt = &s.receipt
	}
	return state
}

func (s *cutoverHTTPService) MatrixStorageMigration(ctx context.Context, actor, product string) (storage.IdentityCutoverState, error) {
	return s.result(ctx, actor, product, "legacy"), nil
}

func (s *cutoverHTTPService) BeginMatrixStorageMigration(ctx context.Context, actor, product, epoch string) (storage.IdentityCutoverState, error) {
	s.epoch = epoch
	return s.result(ctx, actor, product, "frozen"), nil
}

func (s *cutoverHTTPService) CompleteMatrixStorageMigration(ctx context.Context, actor, product, epoch string, receipt storage.Reference) (storage.IdentityCutoverState, error) {
	s.epoch, s.receipt = epoch, receipt
	return s.result(ctx, actor, product, "isolated"), nil
}

func TestMatrixStorageCutoverHTTPPinsAuthenticatedScopeBeforeAnyEffect(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: claims().Subject, Product: "drive"}
	pin, err := storage.EncodeScopeHeader(scope)
	if err != nil {
		t.Fatal(err)
	}
	epoch := "df75d053-b8c8-4eb3-8454-235ef9c532c4"
	for _, method := range []string{"status", "begin", "complete"} {
		for _, fault := range []string{"valid", "missing", "repeated", "casing", "owner", "tenant", "product", "body-owner", "epoch", "receipt"} {
			if method == "status" && (fault == "body-owner" || fault == "epoch" || fault == "receipt") {
				continue
			}
			if method == "begin" && fault == "receipt" {
				continue
			}
			t.Run(method+"/"+fault, func(t *testing.T) {
				service := &cutoverHTTPService{}
				handler := &Handlers{svc: service}
				body := `{"epoch":"` + epoch + `"}`
				if method == "complete" {
					body = `{"epoch":"` + epoch + `","receipt":{"room_id":"!target:matrix.test","event_id":"$receipt"}}`
				}
				if fault == "body-owner" {
					body = `{"epoch":"` + epoch + `","subject":"another"}`
				}
				if fault == "epoch" {
					body = `{"epoch":"invalid"}`
				}
				if fault == "receipt" {
					body = `{"epoch":"` + epoch + `","receipt":{"room_id":"!target:matrix.test","event_id":"invalid"}}`
				}
				request := req(http.MethodPost, "/", body, claims(), map[string]string{"product": "drive"})
				request = request.WithContext(tenancy.Into(request.Context(), tenancy.Tenant{Slug: "acme"}))
				request.Header.Set(storage.ScopeHeader, pin)
				switch fault {
				case "missing":
					request.Header.Del(storage.ScopeHeader)
				case "repeated":
					request.Header.Add(storage.ScopeHeader, pin)
				case "casing":
					request.Header["x-sytue-storage-scope"] = []string{pin}
				case "owner", "tenant", "product":
					other := scope
					if fault == "owner" {
						other.Subject = "other"
					}
					if fault == "tenant" {
						other.Tenant = "other"
					}
					if fault == "product" {
						other.Product = "mail"
					}
					value, _ := storage.EncodeScopeHeader(other)
					request.Header.Set(storage.ScopeHeader, value)
				}
				response := httptest.NewRecorder()
				switch method {
				case "status":
					handler.MatrixStorageMigration(response, request)
				case "begin":
					handler.BeginMatrixStorageMigration(response, request)
				case "complete":
					handler.CompleteMatrixStorageMigration(response, request)
				}
				if fault == "valid" {
					if response.Code != http.StatusOK || service.calls != 1 || service.scope != scope || response.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("valid scope not preserved", response.Code, service.calls, service.scope)
					}
				} else if response.Code == http.StatusOK || service.calls != 0 {
					t.Fatal("invalid scope/body reached storage", response.Code, service.calls)
				}
			})
		}
	}
}
