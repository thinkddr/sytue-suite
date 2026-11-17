// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

func TestMailRoutingMetadataUsesOnlyOriginalDeliveryEndpointsAndExactCapturedScope(t *testing.T) {
	scope := storage.Scope{Tenant: "synthetic", Subject: "owner", Product: "mail"}
	id := "11111111-1111-4111-8111-111111111111"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		bound, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
		if err != nil || bound != scope {
			t.Error("routing metadata lost exact captured storage scope")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/aliases":
			_ = json.NewEncoder(w).Encode(MailRoutingSnapshot{Primary: "owner@example.test", Aliases: []MailAlias{{ID: id, Address: "old@example.test"}}})
		case "POST /api/v1/aliases":
			var input map[string]string
			if json.NewDecoder(r.Body).Decode(&input) != nil || len(input) != 1 || input["address"] != "new@example.test" {
				t.Error("routing alias request changed original public-address contract")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(MailAlias{ID: id, Address: input["address"]})
		case "DELETE /api/v1/aliases/" + id:
			w.WriteHeader(http.StatusNoContent)
		case "GET /api/v1/quota":
			_ = json.NewEncoder(w).Encode(MailQuota{UsedBytes: 123, QuotaBytes: 456})
		default:
			t.Error("routing metadata escaped fixed endpoints")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := NewMail(server.URL, nil, server.Client())
	snapshot, err := client.RoutingMetadata(t.Context(), scope)
	if err != nil || snapshot.Primary != "owner@example.test" || len(snapshot.Aliases) != 1 {
		t.Fatal("original address snapshot failed", err)
	}
	alias, err := client.CreateRoutingAlias(t.Context(), scope, "new@example.test")
	if err != nil || alias.ID != id {
		t.Fatal("original alias creation failed", err)
	}
	if err = client.DeleteRoutingAlias(t.Context(), scope, id); err != nil {
		t.Fatal(err)
	}
	quota, err := client.RoutingQuota(t.Context(), scope)
	if err != nil || quota.UsedBytes != 123 || quota.QuotaBytes != 456 || calls != 4 {
		t.Fatal("original quota metadata failed", err, calls)
	}
	wrong := scope
	wrong.Product = "drive"
	if _, err = client.RoutingMetadata(t.Context(), wrong); err == nil {
		t.Fatal("foreign product metadata request admitted")
	}
	if _, err = client.CreateRoutingAlias(t.Context(), wrong, "new@example.test"); err == nil {
		t.Fatal("foreign product metadata mutation admitted")
	}
	if err = client.DeleteRoutingAlias(t.Context(), scope, "../messages"); err == nil {
		t.Fatal("unlisted endpoint admitted through alias ID")
	}
	if _, err = client.RoutingQuota(t.Context(), storage.Scope{}); err == nil {
		t.Fatal("empty routing owner admitted")
	}
	if calls != 4 {
		t.Fatal("invalid metadata operation reached HTTP")
	}
}
