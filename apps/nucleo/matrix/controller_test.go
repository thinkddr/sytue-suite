// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package matrix

import (
	"testing"

	mxcrypto "maunium.net/go/mautrix/crypto"
	"maunium.net/go/mautrix/id"
)

func TestControllerNeverBecomesRecipientEvenAfterRegisteringTrustedDevice(t *testing.T) {
	const controller id.UserID = "@rooms_controller:matrix.test"
	const human id.UserID = "@sytue_person:matrix.test"
	underlying := mxcrypto.NewMemoryStore(nil)
	guarded := controllerStore{Store: underlying, controller: controller}
	// Both devices are deliberately marked as maximally trusted: the controller
	// exclusion must not depend on its current absence or signing state.
	for _, user := range []id.UserID{controller, human} {
		if err := underlying.PutDevice(t.Context(), user, &id.Device{UserID: user, DeviceID: "SIGNED", Trust: id.TrustStateVerified}); err != nil {
			t.Fatal(err)
		}
	}
	blocked, err := guarded.GetDevices(t.Context(), controller)
	if err != nil || blocked == nil || len(blocked) != 0 {
		t.Fatal("controller became a recipient or triggered fallback fetching")
	}
	people, err := guarded.GetDevices(t.Context(), human)
	if err != nil || len(people) != 1 || people["SIGNED"].Trust != id.TrustStateVerified {
		t.Fatal("verified human recipient was filtered")
	}
	if err := underlying.PutDevice(t.Context(), controller, &id.Device{UserID: controller, DeviceID: "FUTURE", Trust: id.TrustStateVerified}); err != nil {
		t.Fatal(err)
	}
	blocked, err = guarded.GetDevices(t.Context(), controller)
	if err != nil || blocked == nil || len(blocked) != 0 {
		t.Fatal("future controller device bypassed exclusion")
	}
}

func TestControllerIdentityIsFixedByMatrixServer(t *testing.T) {
	for _, controller := range []string{"@rooms_controller:matrix.test", "@other:matrix.test", "@rooms_controller:foreign.test", ""} {
		valid := validController(Bootstrap{MatrixUserID: "@sytue_person:matrix.test", ControllerUserID: controller})
		if valid != (controller == "@rooms_controller:matrix.test") {
			t.Fatalf("controller accepted incorrectly: %s", controller)
		}
	}
}
