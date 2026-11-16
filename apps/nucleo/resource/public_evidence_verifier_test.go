// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

// These public constructors must continue requiring a LIVE concrete Device.
var (
	_ func(context.Context, SignedEpoch, VerifiedRoster, matrixproof.Device, Anchor) (VerifiedEpoch, error)  = VerifyInitialEpoch
	_ func(context.Context, SignedCapability, VerifiedEpoch, matrixproof.Device) (VerifiedCapability, error) = VerifyCapability
	_ func(context.Context, SignedRecipient, matrixproof.Device) (VerifiedRecipient, error)                  = VerifyAccountRecipient
)

func TestPublicMetadataCollectorRetainsExactHistoricalTranscriptWithoutLiveDevice(t *testing.T) {
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "docs"}
	f := matrixprooftest.NewUser(t, scope, "matrix.test", "@sytue_owner:matrix.test", "RETIRED_POLICY_DEVICE")
	live, err := matrixproof.VerifyUser(t.Context(), f.Origin, f.Origin.UserID, f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	value := map[string]any{"domain": "org.sytue.resource.accepted-public-metadata.test", "reference": "opaque-accepted-reference"}
	value["signatures"] = matrixprooftest.Sign(t, f.Device, value, f.Origin.UserID, f.Origin.DeviceID.String())
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		t.Fatal(err)
	}
	original := recordCollector{}
	expected, err := original.statement(raw, live)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := live.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	f.Origin.AccountActive = false
	f.Origin.DeviceActive = false
	if _, err := matrixproof.VerifyUser(t.Context(), f.Origin, f.Origin.UserID, f.Master.PublicKey()); err == nil {
		t.Fatal("retired producer regained a live device")
	}
	historical, err := matrixproof.VerifyHistoricalDevice(t.Context(), evidence, live.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var verifier publicEvidenceVerifier = historical
	if err := verifier.VerifyJSON(t.Context(), scope, storage.PublicationTarget{RoomID: "!accepted:matrix.test", DeviceID: f.Origin.DeviceID.String()}, value); err != nil {
		t.Fatal(err)
	}
	restored := recordCollector{}
	actual, err := restored.statement(raw, verifier)
	if err != nil || actual != expected || len(restored) != len(original) {
		t.Fatal("historical metadata changed captured transcript", err)
	}
	for hash, want := range original {
		if !bytes.Equal(restored[hash], want) {
			t.Fatal("historical evidence wire changed", hash)
		}
	}
	if _, err := restored.signer(nil); err == nil {
		t.Fatal("nil metadata verifier captured evidence")
	}
	if _, ok := any(verifier).(interface {
		SigningKey(storage.Scope, storage.PublicationTarget) (string, error)
	}); ok {
		t.Fatal("metadata verifier exposed live publication authority")
	}
}
