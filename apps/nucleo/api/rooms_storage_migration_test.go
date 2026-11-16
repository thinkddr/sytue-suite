// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMatrixStorageMigrationClientPinsAllProductsAndValidatesReceipt(t *testing.T) {
	epoch := "df75d053-b8c8-4eb3-8454-235ef9c532c4"
	receipt := storage.Reference{RoomID: "!isolated:matrix.test", EventID: "$receipt"}
	for _, product := range storage.Products() {
		t.Run(product, func(t *testing.T) {
			scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: product}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || got != scope {
					t.Error("request lost original scope", got, err)
				}
				if !strings.HasPrefix(r.URL.Path, "/api/v1/matrix/storage/"+product+"/migration") {
					t.Error("wrong product route", r.URL.Path)
				}
				state := storage.IdentityCutoverState{Version: 1, Scope: scope, Phase: "legacy", SourceRooms: []string{"!source:matrix.test"}}
				if r.Method == http.MethodPost {
					var request storageCutoverRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if request.Epoch != epoch {
						t.Error("epoch changed")
					}
					state.Epoch = epoch
					state.Phase = "frozen"
					if request.Receipt != nil {
						state.Phase = "isolated"
						state.Receipt = request.Receipt
						if *request.Receipt != receipt {
							t.Error("receipt changed")
						}
					}
				}
				if err := json.NewEncoder(w).Encode(state); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := NewRooms(server.URL, nil, server.Client())
			if _, err := client.MatrixStorageMigration(t.Context(), scope); err != nil {
				t.Fatal(err)
			}
			if _, err := client.BeginMatrixStorageMigration(t.Context(), scope, epoch); err != nil {
				t.Fatal(err)
			}
			if _, err := client.CompleteMatrixStorageMigration(t.Context(), scope, epoch, receipt); err != nil {
				t.Fatal(err)
			}
			if calls != 3 {
				t.Fatal("missing request", calls)
			}
		})
	}
}

func TestMatrixStorageMigrationClientRejectsForeignOrPrematureState(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "drive"}
	epoch := "df75d053-b8c8-4eb3-8454-235ef9c532c4"
	receipt := storage.Reference{RoomID: "!isolated:matrix.test", EventID: "$receipt"}
	for _, fault := range []string{"owner", "tenant", "product", "version", "epoch", "premature", "receipt"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				value := receipt
				state := storage.IdentityCutoverState{Version: 1, Scope: scope, Epoch: epoch, Phase: "isolated", Receipt: &value}
				switch fault {
				case "owner":
					state.Scope.Subject = "foreign"
				case "tenant":
					state.Scope.Tenant = "other"
				case "product":
					state.Scope.Product = "mail"
				case "version":
					state.Version = 0
				case "epoch":
					state.Epoch = "ad75d053-b8c8-4eb3-8454-235ef9c532c4"
				case "premature":
					state.Phase = "freezing"
					state.Receipt = nil
				case "receipt":
					state.Receipt.EventID = "$different"
				}
				if err := json.NewEncoder(w).Encode(state); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			if _, err := NewRooms(server.URL, nil, server.Client()).CompleteMatrixStorageMigration(t.Context(), scope, epoch, receipt); !errors.Is(err, storage.ErrRecord) {
				t.Fatal("foreign or incomplete cutover response accepted", fault, err)
			}
		})
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer server.Close()
	client := NewRooms(server.URL, nil, server.Client())
	if _, err := client.BeginMatrixStorageMigration(t.Context(), scope, "invalid"); !errors.Is(err, storage.ErrRecord) {
		t.Fatal(err)
	}
	if _, err := client.CompleteMatrixStorageMigration(t.Context(), scope, epoch, storage.Reference{}); !errors.Is(err, storage.ErrRecord) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("malformed cutover reached network", calls)
	}
}
