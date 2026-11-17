// SPDX-License-Identifier: Apache-2.0
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func TestIngressChallengeChecksClientRetainedIntentAndAuthority(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "matrix.test", RootKey: strings.Repeat("A", 43), Generation: 2, Status: "active"}
	request := ingress.ControlRequest{Version: 1, Scope: scope, Epoch: epoch, Kind: "task.claim", Target: storage.PublicationTarget{RoomID: "!mail:matrix.test", DeviceID: "DEVICE"}, Claim: &ingress.ClaimRequest{TaskID: "task_01", Token: "claim_01", LeaseSeconds: 300}}
	intent, err := matrixproof.IntentFor(scope, authority.Generation, epoch, request.Kind, request.Target, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"valid", "generation", "root", "digest", "target", "scope", "expired"} {
		t.Run(fault, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				got, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || got != scope || r.Method != http.MethodPost || r.URL.Path != "/api/v1/private-storage/ingress/challenge" {
					t.Error("ingress scope or route changed", err)
				}
				var body ingress.ControlRequest
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Validate(scope, epoch) != nil || body.Claim == nil || *body.Claim != *request.Claim {
					t.Error("queue intent changed on wire", err)
				}
				now := time.Now()
				action := matrixproof.Action{Version: 1, Generation: authority.Generation, Scope: scope, Epoch: epoch, Kind: request.Kind, Target: request.Target, RootKey: authority.RootKey, RequestSHA256: intent.RequestSHA256, Nonce: "nonce_01", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
				switch fault {
				case "generation":
					action.Generation++
				case "root":
					action.RootKey = strings.Repeat("B", 43)
				case "digest":
					action.RequestSHA256 = strings.Repeat("b", 64)
				case "target":
					action.Target.DeviceID = "OTHER"
				case "scope":
					action.Scope.Subject = "mallory"
				case "expired":
					action.IssuedAt = now.Add(-2 * time.Minute).UnixMilli()
					action.ExpiresAt = now.Add(-time.Minute).UnixMilli()
				}
				_ = json.NewEncoder(w).Encode(action)
			}))
			defer server.Close()
			client, err := NewPrivateIngress(server.URL, "mail", &sesionFalsa{tokens: []string{"synthetic"}}, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Challenge(t.Context(), request, authority)
			if fault == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ingress.ErrRevoked) {
				t.Fatal("server substituted client-retained authorization", fault, err)
			}
			foreign := request
			foreign.Scope.Product = "calendar"
			if _, err := client.Challenge(t.Context(), foreign, authority); err == nil || calls != 1 {
				t.Fatal("foreign product performed HTTP", err, calls)
			}
		})
	}
}
