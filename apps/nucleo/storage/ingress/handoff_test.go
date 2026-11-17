// SPDX-License-Identifier: Apache-2.0
package ingress

import (
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func handoffFixture(t *testing.T) (ControlRequest, Task, matrixproof.Authority, DeviceFence) {
	t.Helper()
	scope := storage.Scope{Tenant: "acme", Subject: "alice", Product: "mail"}
	epoch := "11111111-1111-4111-8111-111111111111"
	task := Task{Version: 1, Scope: scope, Epoch: epoch, ID: "task_01", Kind: "mail.receive", KeyID: strings.Repeat("b", 64), KeyGeneration: 1, Cipher: storage.StreamSourceRow{Kind: "ingress", ID: "task_01", Size: 1000, SHA256: strings.Repeat("a", 64)}}
	previous := Assignment{Target: storage.PublicationTarget{RoomID: "!original:test", DeviceID: "A"}, Generation: 1, SigningKey: strings.Repeat("A", 43), ExpiresAt: time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)}
	request := ControlRequest{Version: 1, Scope: scope, Epoch: epoch, Kind: "task.handoff", Target: storage.PublicationTarget{RoomID: "!original:test", DeviceID: "B"}, Handoff: &HandoffRequest{ID: "22222222-2222-4222-8222-222222222222", Phase: "begin", TaskID: task.ID, CipherSHA256: task.Cipher.SHA256, Previous: previous, Token: "new_B_claim"}}
	authority := matrixproof.Authority{Version: 1, Scope: scope, UserID: storage.IsolatedMatrixUserPrefix(scope) + "test", RootKey: strings.Repeat("A", 43), Generation: 4, Status: "active"}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	fence := DeviceFence{Version: 1, Scope: scope, Epoch: epoch, ID: request.Handoff.ID, TaskID: task.ID, CipherSHA256: task.Cipher.SHA256, Previous: previous, Target: request.Target, Authority: authority, RequestSHA256: digest, Status: "planned"}
	return request, task, authority, fence
}

func TestHandoffFenceBindsFullOriginalIntentWithoutOriginalToken(t *testing.T) {
	request, _, authority, fence := handoffFixture(t)
	if err := request.Validate(request.Scope, request.Epoch); err != nil {
		t.Fatal(err)
	}
	if err := fence.Validate(request, authority); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"scope", "epoch", "task", "cipher", "original-key", "original-device", "original-generation", "destination", "authority", "digest", "confirmed-without-readback"} {
		t.Run(fault, func(t *testing.T) {
			changed := fence
			switch fault {
			case "scope":
				changed.Scope.Subject = "mallory"
			case "epoch":
				changed.Epoch = "33333333-3333-4333-8333-333333333333"
			case "task":
				changed.TaskID = "different"
			case "cipher":
				changed.CipherSHA256 = strings.Repeat("b", 64)
			case "original-key":
				changed.Previous.SigningKey = strings.Repeat("B", 43)
			case "original-device":
				changed.Previous.Target.DeviceID = "C"
			case "original-generation":
				changed.Previous.Generation++
			case "destination":
				changed.Target.DeviceID = "C"
			case "authority":
				changed.Authority.Generation++
			case "digest":
				changed.RequestSHA256 = strings.Repeat("c", 64)
			case "confirmed-without-readback":
				changed.Status = "confirmed"
			}
			if err := changed.Validate(request, authority); err == nil {
				t.Fatal("changed fence accepted", fault)
			}
		})
	}
}

func TestHandoffBeginCannotGrantPublicationOrSkipSDKReconciliation(t *testing.T) {
	request, task, authority, fence := handoffFixture(t)
	now := time.Now()
	fence.Status = "confirmed"
	fence.ConfirmedAt = &now
	claim := Claim{Version: 1, Task: task, Target: request.Target, Token: request.Handoff.Token, Generation: request.Handoff.Previous.Generation + 1, ExpiresAt: now.Add(time.Minute)}
	view := HandoffView{Version: 1, ID: request.Handoff.ID, Task: task, Previous: request.Handoff.Previous, Target: request.Target, Status: "claimed", Fence: &fence, Claim: &claim}
	if err := view.Validate(request, authority); err == nil {
		t.Fatal("begin granted a new publisher before SDK reconciliation")
	}
	resolved := request
	resolution := *request.Handoff
	resolution.Phase = "resolve"
	resolution.Resolution = "absent"
	resolution.LeaseSeconds = 60
	resolved.Handoff = &resolution
	if err := view.Validate(resolved, authority); err != nil {
		t.Fatal("fenced absence reconciliation did not permit original CAS", err)
	}
	reconstructed, err := resolved.BeginHandoffRequest()
	if err != nil {
		t.Fatal(err)
	}
	initialDigest, _ := matrixproof.RequestDigest(request)
	reconstructedDigest, _ := matrixproof.RequestDigest(reconstructed)
	if initialDigest != reconstructedDigest {
		t.Fatal("resolve changed the retained initial intent")
	}
	changed := resolved
	substitution := *resolved.Handoff
	substitution.Token = "substituted_B"
	changed.Handoff = &substitution
	if err := fence.Validate(changed, authority); err == nil {
		t.Fatal("resolve substituted the new token behind the same fence")
	}
	view.Fence = nil
	if err := view.Validate(resolved, authority); err == nil {
		t.Fatal("expiry alone granted another publisher")
	}
}

func TestHandoffMixedPurposeAndChangedReceiptAreRejected(t *testing.T) {
	request, task, authority, _ := handoffFixture(t)
	mixed := request
	mixed.Claim = &ClaimRequest{TaskID: task.ID, Token: "hidden", LeaseSeconds: 60}
	if err := mixed.Validate(request.Scope, request.Epoch); err == nil {
		t.Fatal("handoff hid a second operation")
	}
	receipt := Receipt{Version: 1, Scope: task.Scope, Epoch: task.Epoch, TaskID: task.ID, CipherSHA256: task.Cipher.SHA256, ClaimGeneration: request.Handoff.Previous.Generation, Commit: storage.Reference{RoomID: request.Handoff.Previous.Target.RoomID, EventID: "$accepted"}}
	resolution := *request.Handoff
	resolution.Phase = "resolve"
	resolution.Resolution = "committed"
	resolution.Receipt = &receipt
	request.Handoff = &resolution
	view := HandoffView{Version: 1, ID: resolution.ID, Task: task, Previous: resolution.Previous, Target: request.Target, Status: "acknowledged", Receipt: &receipt}
	if err := view.Validate(request, authority); err != nil {
		t.Fatal(err)
	}
	changed := receipt
	changed.Commit.EventID = "$different"
	view.Receipt = &changed
	if err := view.Validate(request, authority); err == nil {
		t.Fatal("server replaced the SDK-reconciled receipt")
	}
}
