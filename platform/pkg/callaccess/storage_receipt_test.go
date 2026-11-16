// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestStorageReceiptUsesDedicatedAuthenticatedEndpointAndRejectsRedirects(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "calendar"}
	target := storage.PublicationTarget{RoomID: "!calendar:matrix.test", DeviceID: "DEVICE"}
	receipt := storage.Reference{RoomID: target.RoomID, EventID: "$encrypted"}
	for _, status := range []int{http.StatusNoContent, http.StatusOK, http.StatusTemporaryRedirect, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodPost || r.URL.Path != "/internal/storage-authority/receipt" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer synthetic-receipt-secret" {
					t.Error("receipt escaped dedicated internal boundary")
				}
				var request struct {
					Scope   storage.Scope
					Target  storage.PublicationTarget
					Receipt storage.Reference
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Scope != scope || request.Target != target || request.Receipt != receipt {
					t.Error("receipt binding changed", err)
				}
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(status)
			}))
			defer server.Close()
			err := New(server.URL, "synthetic-receipt-secret").VerifyStorageReceipt(t.Context(), scope, target, receipt)
			if status == http.StatusNoContent {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnavailable) {
				t.Fatal("unacknowledged receipt accepted", err)
			}
			if calls != 1 {
				t.Fatal("internal receipt followed a redirect", calls)
			}
			other := receipt
			other.RoomID = "!other:matrix.test"
			if err := New(server.URL, "synthetic-receipt-secret").VerifyStorageReceipt(t.Context(), scope, target, other); !errors.Is(err, ErrUnavailable) || calls != 1 {
				t.Fatal("foreign room performed an authority request", err)
			}
		})
	}
}
