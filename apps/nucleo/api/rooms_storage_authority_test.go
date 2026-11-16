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
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func TestStorageRootClientRejectsSubstitutedConsentBeforeSigning(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	fixture := matrixprooftest.New(t, scope, "matrix.test", "DEVICE")
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}
	expected := matrixproof.RootEnrollment{Version: 1, Scope: scope, UserID: fixture.Origin.UserID.String(), Target: target, RootKey: fixture.Master.PublicKey().String(), Generation: 1}
	const epoch = "11111111-1111-4111-8111-111111111111"
	for _, fault := range []string{"valid", "root", "owner", "generation", "request", "purpose", "expired"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				routed, err := storage.DecodeScopeHeader(r.Header.Get(storage.ScopeHeader))
				if err != nil || routed != scope || r.URL.Path != "/api/v1/matrix/storage/mail/authority/challenge" || r.Method != http.MethodPost {
					t.Error("lost original caller routing")
				}
				var body struct {
					Kind, Epoch string
					Target      storage.PublicationTarget
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Kind != "root.enroll" || body.Epoch != epoch || body.Target != target {
					t.Error("changed local consent")
				}
				digest, err := matrixproof.RequestDigest(expected)
				if err != nil {
					t.Error(err)
					return
				}
				now := time.Now()
				challenge := matrixproof.RootChallenge{Request: expected, Action: matrixproof.Action{Version: 1, Generation: 1, Scope: scope, Epoch: epoch, Kind: "root.enroll", Target: target, RootKey: expected.RootKey, RequestSHA256: digest, Nonce: "root_consent", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}}
				switch fault {
				case "root":
					challenge.Action.RootKey = strings.Repeat("A", 43)
				case "owner":
					challenge.Action.Scope.Subject = "other"
				case "generation":
					challenge.Action.Generation = 2
				case "request":
					challenge.Request.Target.RoomID = "!other:matrix.test"
				case "purpose":
					challenge.Action.Kind = "key.enroll"
				case "expired":
					challenge.Action.ExpiresAt = now.Add(-time.Second).UnixMilli()
				}
				if err := json.NewEncoder(w).Encode(challenge); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			_, err := NewRooms(server.URL, nil, server.Client()).MatrixStorageRootChallenge(t.Context(), scope, epoch, "root.enroll", expected)
			if fault == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, matrixproof.ErrUnverified) {
				t.Fatal("substituted consent accepted", fault, err)
			}
		})
	}
}
