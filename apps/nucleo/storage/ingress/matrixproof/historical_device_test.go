// SPDX-License-Identifier: Apache-2.0
package matrixproof_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func historicalFixture(t *testing.T) (*matrixprooftest.Fixture, matrixproof.Device) {
	t.Helper()
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "rooms"}
	f := matrixprooftest.NewUser(t, scope, "matrix.test", "@human:matrix.test", "RETIRED_DEVICE")
	d, err := matrixproof.VerifyUser(t.Context(), f.Origin, f.Origin.UserID, f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return f, d
}

func TestHistoricalDeviceKeepsRetiredEvidenceSeparateFromLiveAuthority(t *testing.T) {
	f, live := historicalFixture(t)
	evidence, err := live.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var restored matrixproof.DeviceEvidence
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	f.Origin.AccountActive = false
	f.Origin.DeviceActive = false
	if _, err = matrixproof.VerifyUser(t.Context(), f.Origin, f.Origin.UserID, f.Master.PublicKey()); err == nil {
		t.Fatal("retired device regained live authority")
	}
	historical, err := matrixproof.VerifyHistoricalDevice(t.Context(), restored, live.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if historical.Identity() != live.Identity() {
		t.Fatal("historical pin changed")
	}
	if _, ok := any(historical).(interface {
		SigningKey(storage.Scope, storage.PublicationTarget) (string, error)
	}); ok {
		t.Fatal("historical evidence exposes live publication authority")
	}
	target := storage.PublicationTarget{RoomID: "!accepted:matrix.test", DeviceID: f.Origin.DeviceID.String()}
	value := map[string]any{"accepted_ref": "opaque-accepted-reference"}
	value["signatures"] = matrixprooftest.Sign(t, f.Device, value, f.Origin.UserID, f.Origin.DeviceID.String())
	if err = historical.VerifyJSON(t.Context(), f.Origin.Scope, target, value); err != nil {
		t.Fatal(err)
	}
	value["accepted_ref"] = "another-reference"
	if err = historical.VerifyJSON(t.Context(), f.Origin.Scope, target, value); err == nil {
		t.Fatal("changed accepted transcript verified")
	}
	value["accepted_ref"] = "opaque-accepted-reference"
	wrongTarget := target
	wrongTarget.DeviceID = "OTHER"
	if err = historical.VerifyJSON(t.Context(), f.Origin.Scope, wrongTarget, value); err == nil {
		t.Fatal("other producer target verified")
	}
	wrongScope := f.Origin.Scope
	wrongScope.Product = "mail"
	if err = historical.VerifyJSON(t.Context(), wrongScope, target, value); err == nil {
		t.Fatal("other product verified")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = historical.VerifyJSON(ctx, f.Origin.Scope, target, value); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = matrixproof.VerifyHistoricalDevice(ctx, restored, live.Identity()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = (matrixproof.Device{}).PublicEvidence(); err == nil {
		t.Fatal("zero live device created historical evidence")
	}
	if err = (matrixproof.HistoricalDevice{}).VerifyJSON(t.Context(), f.Origin.Scope, target, value); err == nil {
		t.Fatal("zero historical device verified transcript")
	}
}

func TestHistoricalDeviceRequiresExactIndependentPinAndMaintainedSDKChain(t *testing.T) {
	_, live := historicalFixture(t)
	for _, fault := range []string{"version", "server", "scope", "user", "device", "master", "self-signature", "device-signature", "own-signature", "unsupported-algorithm", "dehydrated", "pin-scope", "pin-user", "pin-device", "pin-root", "pin-signing", "pin-encryption"} {
		t.Run(fault, func(t *testing.T) {
			evidence, err := live.PublicEvidence()
			if err != nil {
				t.Fatal(err)
			}
			pin := live.Identity()
			switch fault {
			case "version":
				evidence.Version++
			case "server":
				evidence.ServerName = "other.test"
			case "scope":
				evidence.Scope.Product = "mail"
			case "user":
				evidence.UserID = "@other:matrix.test"
			case "device":
				evidence.DeviceID = "OTHER"
			case "master":
				evidence.Master.Keys = nil
			case "self-signature":
				evidence.Self.Signatures = nil
			case "device-signature":
				evidence.Device.Signatures = nil
			case "own-signature":
				delete(evidence.Device.Signatures[evidence.UserID], id.NewKeyID(id.KeyAlgorithmEd25519, evidence.DeviceID.String()))
			case "unsupported-algorithm":
				evidence.Device.Algorithms = []id.Algorithm{id.AlgorithmOlmV1}
			case "dehydrated":
				evidence.Device.Dehydrated = true
			case "pin-scope":
				pin.Scope.Product = "mail"
			case "pin-user":
				pin.UserID = "@other:matrix.test"
			case "pin-device":
				pin.DeviceID = "OTHER"
			case "pin-root":
				pin.RootKey = matrixprooftest.NewSigning(t).PublicKey()
			case "pin-signing":
				pin.SigningKey = matrixprooftest.NewSigning(t).PublicKey()
			case "pin-encryption":
				pin.EncryptionKey = id.Curve25519(matrixprooftest.NewSigning(t).PublicKey())
			}
			if _, err := matrixproof.VerifyHistoricalDevice(t.Context(), evidence, pin); err == nil {
				t.Fatal("untrusted historical chain or independent pin accepted", fault)
			}
		})
	}
}

func TestDeviceEvidenceCannotBeMutatedAfterLiveVerification(t *testing.T) {
	f, live := historicalFixture(t)
	public, err := live.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	public.Self.Signatures = nil
	public.Device.Keys[id.NewDeviceKeyID(id.KeyAlgorithmEd25519, f.Origin.DeviceID)] = "invalid"
	original := f.Origin.Keys.DeviceKeys[f.Origin.UserID][f.Origin.DeviceID]
	original.Keys[id.NewDeviceKeyID(id.KeyAlgorithmCurve25519, f.Origin.DeviceID)] = "also-invalid"
	f.Origin.Keys.DeviceKeys[f.Origin.UserID][f.Origin.DeviceID] = original
	f.Origin.Keys.SelfSigningKeys[f.Origin.UserID] = public.Self
	captured, err := live.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = matrixproof.VerifyHistoricalDevice(t.Context(), captured, live.Identity()); err != nil {
		t.Fatal("caller mutated verified public snapshot", err)
	}
}

func TestHistoricalDevicePublicEvidenceIsAnIndependentImmutableCopy(t *testing.T) {
	f, live := historicalFixture(t)
	original, err := live.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	historical, err := matrixproof.VerifyHistoricalDevice(t.Context(), original, live.Identity())
	if err != nil {
		t.Fatal(err)
	}
	f.Origin.AccountActive = false
	f.Origin.DeviceActive = false
	for key := range original.Device.Keys {
		original.Device.Keys[key] = "mutated-caller-key"
		break
	}
	original.Master.Keys = nil
	original.Self.Signatures = nil
	first, err := historical.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(first)
	if err != nil || !bytes.Equal(raw, expected) {
		t.Fatal("historical caller mutation changed captured evidence", err)
	}
	for key := range first.Device.Keys {
		first.Device.Keys[key] = "mutated-exported-key"
		break
	}
	first.Master.Keys = nil
	first.Self.Signatures = nil
	second, err := historical.PublicEvidence()
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(second)
	if err != nil || !bytes.Equal(raw, expected) {
		t.Fatal("historical exported copy changed retained evidence", err)
	}
	if _, err = (matrixproof.HistoricalDevice{}).PublicEvidence(); err == nil {
		t.Fatal("zero historical device exported evidence")
	}
	if _, ok := any(historical).(interface {
		SigningKey(storage.Scope, storage.PublicationTarget) (string, error)
	}); ok {
		t.Fatal("public evidence exported live signing authority")
	}
}
