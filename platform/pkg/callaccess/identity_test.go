// SPDX-License-Identifier: Apache-2.0
package callaccess

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallIdentityMetadataRemainsActorAndTenantBound(t *testing.T) {
	for _, wrong := range []string{"actor", "tenant", "version"} {
		t.Run(wrong, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/internal/people/call-access" || r.Header.Get("Authorization") != "Bearer identity-only" {
					t.Error("unexpected private identity request")
				}
				out := IdentityProof{Subject: "alice", Tenant: "acme", Active: true, Member: true, Version: 7}
				switch wrong {
				case "actor":
					out.Subject = "bob"
				case "tenant":
					out.Tenant = "other"
				case "version":
					out.Version = -1
				}
				_ = json.NewEncoder(w).Encode(out)
			}))
			defer srv.Close()
			if _, err := New(srv.URL, "identity-only").ReadCallIdentity(context.Background(), "alice", "acme"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("unbound identity accepted: %v", err)
			}
		})
	}
}
