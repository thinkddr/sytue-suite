// SPDX-License-Identifier: Apache-2.0
package jmap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

type routingScopeStore struct {
	fakeStore
	reads int
}

func (f *routingScopeStore) AccountBySubject(ctx context.Context, subject string) (domain.Account, error) {
	f.reads++
	return f.fakeStore.AccountBySubject(ctx, subject)
}

func TestPrivateRoutingMetadataScopeFenceRunsBeforeAnyAccountIO(t *testing.T) {
	for _, operation := range []string{"aliases", "create", "delete", "quota"} {
		t.Run(operation, func(t *testing.T) {
			for _, scenario := range []string{"match", "legacy", "tenant", "subject", "product", "empty", "malformed", "duplicate"} {
				t.Run(scenario, func(t *testing.T) {
					store := &routingScopeStore{fakeStore: fakeStore{acc: domain.Account{Address: "ada@sytue.eu"}}}
					a := newAPI(store, nil, nil)
					method, target, body := http.MethodGet, "/api/v1/aliases", ""
					if operation == "create" {
						method = http.MethodPost
						body = `{"address":"alias@sytue.eu"}`
					}
					if operation == "delete" {
						method = http.MethodDelete
						target += "/11111111-1111-4111-8111-111111111111"
					}
					if operation == "quota" {
						target = "/api/v1/quota"
					}
					request := req(method, target, body, true, map[string]string{"aliasID": "11111111-1111-4111-8111-111111111111"})
					scope := storage.Scope{Tenant: "acme", Subject: "user-1", Product: "mail"}
					switch scenario {
					case "tenant":
						scope.Tenant = "other"
					case "subject":
						scope.Subject = "other"
					case "product":
						scope.Product = "drive"
					}
					encoded, err := storage.EncodeScopeHeader(scope)
					if err != nil {
						t.Fatal(err)
					}
					if scenario != "legacy" {
						request.Header.Set(storage.ScopeHeader, encoded)
					}
					switch scenario {
					case "empty":
						request.Header.Set(storage.ScopeHeader, "")
					case "malformed":
						request.Header.Set(storage.ScopeHeader, "invalid")
					case "duplicate":
						request.Header.Add(storage.ScopeHeader, encoded)
					}
					response := httptest.NewRecorder()
					switch operation {
					case "aliases":
						a.listAliases(response, request)
					case "create":
						a.createAlias(response, request)
					case "delete":
						a.deleteAlias(response, request)
					case "quota":
						a.getQuota(response, request)
					}
					if scenario == "match" || scenario == "legacy" {
						if response.Code >= 400 || store.reads != 1 {
							t.Fatal("existing authenticated routing rejected", response.Code, store.reads)
						}
					} else if response.Code < 400 || store.reads != 0 {
						t.Fatal("changed routing account reached store", response.Code, store.reads)
					}
				})
			}
		})
	}
}
