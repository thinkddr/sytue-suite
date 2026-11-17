// SPDX-License-Identifier: Apache-2.0
package matrixproof_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"maunium.net/go/mautrix/crypto/goolm/pk"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

func signingFixture(t *testing.T) *pk.Signing { t.Helper(); return matrixprooftest.NewSigning(t) }
func sdkSign(t *testing.T, key *pk.Signing, object any, user id.UserID, name string) signatures.Signatures {
	t.Helper()
	return matrixprooftest.Sign(t, key, object, user, name)
}

func newChain(t *testing.T) *matrixprooftest.Fixture {
	t.Helper()
	return matrixprooftest.New(t, storage.Scope{Tenant: "fixture", Subject: "owner", Product: "mail"}, "matrix.test", id.DeviceID("VERIFIED_DEVICE"))
}

func TestDeviceProofRequiresIndependentRootAndCompleteCrossSigningChain(t *testing.T) {
	f := newChain(t)
	verified, err := matrixproof.Verify(t.Context(), f.Origin, f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	target := storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: f.Origin.DeviceID.String()}
	if key, err := verified.SigningKey(f.Origin.Scope, target); err != nil || key != f.Device.PublicKey().String() {
		t.Fatal("verified device changed", err)
	}
	for _, fault := range []string{"missing-root", "changed-root", "scope", "server", "account", "device-inactive", "query-failure", "self-signature", "device-signature", "own-signature", "device-owner", "malformed-key", "unsupported-algorithm", "dehydrated"} {
		t.Run(fault, func(t *testing.T) {
			f := newChain(t)
			origin := f.Origin
			root := f.Master.PublicKey()
			switch fault {
			case "missing-root":
				root = ""
			case "changed-root":
				root = signingFixture(t).PublicKey()
			case "scope":
				origin.Scope.Product = "calendar"
			case "server":
				origin.ServerName = "attacker.test"
			case "account":
				origin.AccountActive = false
			case "device-inactive":
				origin.DeviceActive = false
			case "query-failure":
				origin.Keys.Failures = map[string]any{"matrix.test": map[string]any{"errcode": "M_UNAVAILABLE"}}
			case "self-signature":
				v := origin.Keys.SelfSigningKeys[origin.UserID]
				v.Signatures = nil
				origin.Keys.SelfSigningKeys[origin.UserID] = v
			default:
				v := origin.Keys.DeviceKeys[origin.UserID][origin.DeviceID]
				switch fault {
				case "device-signature":
					delete(v.Signatures[origin.UserID], id.NewKeyID(id.KeyAlgorithmEd25519, f.Self.PublicKey().String()))
				case "own-signature":
					delete(v.Signatures[origin.UserID], id.NewKeyID(id.KeyAlgorithmEd25519, origin.DeviceID.String()))
				case "device-owner":
					v.UserID = "@other:matrix.test"
				case "malformed-key":
					v.Keys[id.NewDeviceKeyID(id.KeyAlgorithmEd25519, origin.DeviceID)] = "short"
				case "unsupported-algorithm":
					v.Algorithms = []id.Algorithm{id.AlgorithmOlmV1}
				case "dehydrated":
					v.Dehydrated = true
				}
				origin.Keys.DeviceKeys[origin.UserID][origin.DeviceID] = v
			}
			if _, err := matrixproof.Verify(t.Context(), origin, root); err == nil {
				t.Fatal("unverified chain accepted", fault)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := matrixproof.Verify(ctx, f.Origin, f.Master.PublicKey()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDeviceActionSignatureBindsRetainedChallengeAndEntireRequest(t *testing.T) {
	f := newChain(t)
	device, err := matrixproof.Verify(t.Context(), f.Origin, f.Master.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	digest, err := matrixproof.RequestDigest(map[string]any{"task": "delivery", "token": "synthetic", "generation": 1})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	expected := matrixproof.Action{Version: 1, Generation: 1, Scope: f.Origin.Scope, Epoch: "11111111-1111-4111-8111-111111111111", Kind: "task.claim", Target: storage.PublicationTarget{RoomID: "!private:matrix.test", DeviceID: f.Origin.DeviceID.String()}, RootKey: f.Master.PublicKey().String(), RequestSHA256: digest, Nonce: "server_challenge", IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(time.Minute).UnixMilli()}
	proof := matrixproof.SignedAction{Action: expected}
	proof.Signatures = sdkSign(t, f.Device, proof, f.Origin.UserID, f.Origin.DeviceID.String())
	if err := device.VerifyAction(t.Context(), proof, expected, now); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"scope", "epoch", "generation", "kind", "room", "device", "root", "request", "nonce", "issued", "expiry", "expired", "unsigned"} {
		t.Run(fault, func(t *testing.T) {
			changed := expected
			signed := proof
			clock := now
			switch fault {
			case "scope":
				changed.Scope.Subject = "other"
			case "epoch":
				changed.Epoch = "22222222-2222-4222-8222-222222222222"
			case "generation":
				changed.Generation++
			case "kind":
				changed.Kind = "task.ack"
			case "room":
				changed.Target.RoomID = "!other:matrix.test"
			case "device":
				changed.Target.DeviceID = "OTHER"
			case "root":
				changed.RootKey = f.Self.PublicKey().String()
			case "request":
				changed.RequestSHA256, _ = matrixproof.RequestDigest(map[string]any{"task": "other"})
			case "nonce":
				changed.Nonce = "different"
			case "issued":
				changed.IssuedAt++
			case "expiry":
				changed.ExpiresAt++
			case "expired":
				clock = now.Add(2 * time.Minute)
			case "unsigned":
				signed.Signatures = nil
			}
			if err := device.VerifyAction(t.Context(), signed, changed, clock); err == nil {
				t.Fatal("changed transcript accepted", fault)
			}
		})
	}
}
