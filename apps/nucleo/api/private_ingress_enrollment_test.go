// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

func TestIngressEnrollmentNeedsExplicitValidatedAbsence(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	for _, fault := range []string{"absent", "404", "409", "foreign", "malformed", "revoked"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || got != scope || r.URL.Path != "/api/v1/private-storage/ingress/enrollment" || r.Method != http.MethodPost {
					t.Error("enrollment request binding changed", err)
				}
				var request ingress.EnrollmentRequest
				if json.NewDecoder(r.Body).Decode(&request) != nil || request.Epoch != epoch {
					t.Error("epoch missing")
				}
				state := ingress.EnrollmentState{Version: 1, Scope: scope, Epoch: epoch, Status: "absent"}
				switch fault {
				case "404":
					w.WriteHeader(http.StatusNotFound)
				case "409":
					w.WriteHeader(http.StatusConflict)
				case "foreign":
					state.Scope.Subject = "bob"
				case "malformed":
					state.Generation = 1
				case "revoked":
					state.Status = "revoked"
					state.Generation = 2
				}
				_ = json.NewEncoder(w).Encode(state)
			}))
			defer server.Close()
			client, err := NewPrivateIngress(server.URL, "mail", &sesionFalsa{tokens: []string{"synthetic-enrollment"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			state, err := client.Enrollment(t.Context(), scope, epoch)
			if fault == "absent" || fault == "revoked" {
				if err != nil || state.Validate(scope, epoch) != nil {
					t.Fatal(state, err)
				}
			} else if err == nil || state.Status != "" {
				t.Fatal("ambiguous response became absence", state, err)
			}
		})
	}
}
