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
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func fenceClientProof(t *testing.T, phase string) (ingress.ControlProof, ingress.DeviceFence) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "owner", Product: "mail"}
	f := matrixprooftest.New(t, scope, "matrix.test", "B")
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: "B"}
	authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: f.Origin.UserID.String(), RootKey: f.Master.PublicKey().String(), Generation: 3, Status: "active"}
	previous := ingress.Assignment{Target: storage.PublicationTarget{RoomID: target.RoomID, DeviceID: "A"}, Generation: 2, SigningKey: f.Self.PublicKey().String(), ExpiresAt: time.Now().UTC().Add(-time.Minute)}
	request := ingress.ControlRequest{Version: 1, Scope: scope, Epoch: "11111111-1111-4111-8111-111111111111", Kind: "task.handoff", Target: target, Handoff: &ingress.HandoffRequest{ID: "22222222-2222-4222-8222-222222222222", Phase: "begin", TaskID: "task_1", CipherSHA256: strings.Repeat("a", 64), Previous: previous, Token: "synthetic_B_token"}}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	out := ingress.DeviceFence{Version: 1, Scope: scope, Epoch: request.Epoch, ID: request.Handoff.ID, TaskID: request.Handoff.TaskID, CipherSHA256: request.Handoff.CipherSHA256, Previous: previous, Target: target, Authority: authority, RequestSHA256: digest, Status: "confirmed"}
	now := time.Now().UTC()
	out.ConfirmedAt = &now
	if phase == "resolve" {
		request.Handoff.Phase = phase
		request.Handoff.Resolution = "absent"
		request.Handoff.LeaseSeconds = 30
	}
	digest, err = matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	action := matrixproof.Action{Version: 1, Generation: 3, Scope: scope, Epoch: request.Epoch, Kind: request.Kind, Target: target, RootKey: authority.RootKey, RequestSHA256: digest, Nonce: "fence_nonce", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	proof := ingress.ControlProof{Request: request, Proof: matrixproof.SignedAction{Action: action}}
	if proof.Validate(scope, false) != nil || out.ValidateProof(proof) != nil {
		t.Fatal("invalid client fixture")
	}
	return proof, out
}

func TestStorageDeviceFenceInternalBoundaryAndFreshResolveBinding(t *testing.T) {
	for _, phase := range []string{"begin", "resolve"} {
		for _, fault := range []string{"valid", "root", "generation", "target", "digest", "original-key", "timestamp", "status", "redirect"} {
			t.Run(phase+"/"+fault, func(t *testing.T) {
				proof, response := fenceClientProof(t, phase)
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					path := "/internal/storage-authority/device-fence"
					if phase == "resolve" {
						path += "/state"
					}
					if r.Method != http.MethodPost || r.URL.Path != path || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer synthetic-fence-secret" {
						t.Error("fence escaped internal boundary")
					}
					var received ingress.ControlProof
					if err := json.NewDecoder(r.Body).Decode(&received); err != nil || received.Request.Handoff.Token != proof.Request.Handoff.Token || received.Proof.Action != proof.Proof.Action {
						t.Error("changed fresh request", err)
					}
					switch fault {
					case "root":
						response.Authority.RootKey = strings.Repeat("B", 43)
					case "generation":
						response.Authority.Generation++
					case "target":
						response.Target.DeviceID = "OTHER"
					case "digest":
						response.RequestSHA256 = strings.Repeat("b", 64)
					case "original-key":
						response.Previous.SigningKey = strings.Repeat("C", 43)
					case "timestamp":
						response.ConfirmedAt = nil
					case "status":
						response.Status = "unknown"
					case "redirect":
						w.Header().Set("Location", "/must-not-follow")
						w.WriteHeader(http.StatusTemporaryRedirect)
						return
					}
					if err := json.NewEncoder(w).Encode(response); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				client := New(server.URL, "synthetic-fence-secret")
				var err error
				if phase == "begin" {
					_, err = client.FenceStorageDevice(t.Context(), proof)
				} else {
					if _, err := client.FenceStorageDevice(t.Context(), proof); !errors.Is(err, ErrUnavailable) || calls != 0 {
						t.Fatal("resolve performed a destructive request", err, calls)
					}
					_, err = client.ReadStorageDeviceFence(t.Context(), proof)
				}
				if fault == "valid" {
					if err != nil {
						t.Fatal(err)
					}
				} else if !errors.Is(err, ErrUnavailable) {
					t.Fatal("changed fence accepted", err)
				}
				if calls != 1 {
					t.Fatal("redirect followed or request duplicated", calls)
				}
			})
		}
	}
}
