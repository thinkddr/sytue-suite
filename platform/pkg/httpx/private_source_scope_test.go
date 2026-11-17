// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func TestAuthenticatedSourceScopeUsesExactlyTheExistingRequestGuard(t *testing.T) {
	want := storage.Scope{Tenant: "acme", Subject: "alice", Product: "photos"}
	for _, change := range []string{"valid", "identity", "tenant", "delegated", "header", "malformed", "cancelled", "no port"} {
		t.Run(change, func(t *testing.T) {
			identity := PrivateSourceIdentity{Tenant: want.Tenant, Subject: want.Subject}
			known := true
			var port PrivateSourcePort = struct{ PrivateSourcePort }{}
			ctx := tenancy.Into(t.Context(), tenancy.Tenant{Slug: want.Tenant})
			request := httptest.NewRequest(http.MethodPost, "/private-storage/plan", http.NoBody)
			encoded, err := storage.EncodeScopeHeader(want)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set(storage.ScopeHeader, encoded)
			switch change {
			case "identity":
				known = false
			case "tenant":
				identity.Tenant = "other"
			case "delegated":
				identity.Delegado = true
			case "header":
				other := want
				other.Subject = "other"
				encoded, _ := storage.EncodeScopeHeader(other)
				request.Header.Set(storage.ScopeHeader, encoded)
			case "malformed":
				request.Header.Set(storage.ScopeHeader, "invalid")
			case "cancelled":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			case "no port":
				port = nil
			}
			request = request.WithContext(ctx)
			h, err := NewPrivateSourceHandlers(PrivateSourceHTTPConfig{Product: "photos", Port: port, MaxBodyBytes: 1024, Kind: func(string) bool { return true }, MapError: func(error) *Error { return ErrInternal }, Identity: func(context.Context) (PrivateSourceIdentity, bool) { return identity, known }})
			if err != nil {
				t.Fatal(err)
			}
			old, newResponse := httptest.NewRecorder(), httptest.NewRecorder()
			oldScope, oldOK := h.scope(old, request)
			newScope, newOK := h.AuthenticatedScope(newResponse, request)
			if oldScope != newScope || oldOK != newOK || old.Code != newResponse.Code || old.Body.String() != newResponse.Body.String() || old.Header().Get("Cache-Control") != newResponse.Header().Get("Cache-Control") {
				t.Fatal("domain control plane changed the source authorization guard")
			}
			if change == "valid" {
				if !newOK || newScope != want {
					t.Fatal("valid authenticated scope rejected")
				}
			} else if newOK || newScope != (storage.Scope{}) {
				t.Fatal("invalid control-plane request authorized")
			}
		})
	}
}
