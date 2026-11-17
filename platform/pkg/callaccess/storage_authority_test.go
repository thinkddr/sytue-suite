// SPDX-License-Identifier: Apache-2.0
package callaccess

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

func TestStorageAuthorityInternalClientRejectsChangedProofAndKeepsSecretOffURL(t *testing.T) {
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	f := matrixprooftest.New(t, scope, "matrix.test", "DEVICE")
	now := time.Now()
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "DEVICE"}
	authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: f.Origin.UserID.String(), RootKey: f.Master.PublicKey().String(), Generation: 3, Status: "active"}
	action := matrixproof.Action{Version: 1, Generation: 3, Scope: scope, Epoch: "11111111-1111-4111-8111-111111111111", Kind: "task.claim", Target: target, RootKey: authority.RootKey, RequestSHA256: strings.Repeat("a", 64), Nonce: "action_nonce", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	proof := matrixproof.SignedAction{Action: action}
	for _, fault := range []string{"valid", "generation", "root", "target", "nonce", "digest", "signing"} {
		t.Run(fault, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer synthetic-authority" || r.URL.RawQuery != "" {
					t.Error("credential outside existing header")
				}
				if r.URL.Path == "/internal/storage-authority/state" {
					if err := json.NewEncoder(w).Encode(authority); err != nil {
						t.Error(err)
					}
					return
				}
				var body struct {
					Scope    storage.Scope
					Expected matrixproof.Action
					Proof    matrixproof.SignedAction
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Scope != scope || body.Expected != action || body.Proof.Action != action {
					t.Error("changed domain transcript")
				}
				out := matrixproof.Authorization{Version: 1, Authority: authority, Target: target, SigningKey: f.Device.PublicKey().String(), Nonce: action.Nonce, RequestSHA256: action.RequestSHA256}
				switch fault {
				case "generation":
					out.Authority.Generation++
				case "root":
					out.Authority.RootKey = strings.Repeat("A", 43)
				case "target":
					out.Target.DeviceID = "OTHER"
				case "nonce":
					out.Nonce = "other_nonce"
				case "digest":
					out.RequestSHA256 = strings.Repeat("b", 64)
				case "signing":
					out.SigningKey = "invalid"
				}
				if err := json.NewEncoder(w).Encode(out); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			client := New(server.URL, "synthetic-authority")
			if _, err := client.ReadStorageAuthority(t.Context(), scope); err != nil {
				t.Fatal(err)
			}
			_, err := client.VerifyStorageAction(t.Context(), action, proof)
			if fault == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnavailable) {
				t.Fatal("changed authority accepted", fault, err)
			}
		})
	}
}
