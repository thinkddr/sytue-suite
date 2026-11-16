// SPDX-License-Identifier: Apache-2.0
// Package matrixprooftest constructs disposable SDK signing identities for
// conformance fixtures. Applications must never import this test-only boundary.
package matrixprooftest

import (
	"testing"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/crypto/goolm/pk"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type Fixture struct {
	Origin               matrixproof.Origin
	Master, Self, Device *pk.Signing
}

func NewSigning(t testing.TB) *pk.Signing {
	t.Helper()
	key, err := pk.NewSigning()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func Sign(t testing.TB, key *pk.Signing, object any, user id.UserID, name string) signatures.Signatures {
	t.Helper()
	signature, err := key.SignJSON(object)
	if err != nil {
		t.Fatal(err)
	}
	return signatures.NewSingleSignature(user, id.KeyAlgorithmEd25519, name, signature)
}

func New(t testing.TB, scope storage.Scope, server string, deviceID id.DeviceID) *Fixture {
	t.Helper()
	return NewUser(t, scope, server, id.UserID(storage.IsolatedMatrixUserPrefix(scope)+server), deviceID)
}

// NewUser exercises the same maintained SDK chain for a canonical human user.
func NewUser(t testing.TB, scope storage.Scope, server string, user id.UserID, deviceID id.DeviceID) *Fixture {
	t.Helper()
	master, self, device := NewSigning(t), NewSigning(t), NewSigning(t)
	root := mautrix.CrossSigningKeys{UserID: user, Usage: []id.CrossSigningUsage{id.XSUsageMaster}, Keys: map[id.KeyID]id.Ed25519{id.NewKeyID(id.KeyAlgorithmEd25519, master.PublicKey().String()): master.PublicKey()}}
	sub := mautrix.CrossSigningKeys{UserID: user, Usage: []id.CrossSigningUsage{id.XSUsageSelfSigning}, Keys: map[id.KeyID]id.Ed25519{id.NewKeyID(id.KeyAlgorithmEd25519, self.PublicKey().String()): self.PublicKey()}}
	sub.Signatures = Sign(t, master, sub, user, master.PublicKey().String())
	dev := mautrix.DeviceKeys{UserID: user, DeviceID: deviceID, Algorithms: []id.Algorithm{id.AlgorithmMegolmV1, id.AlgorithmOlmV1}, Keys: mautrix.KeyMap{id.NewDeviceKeyID(id.KeyAlgorithmEd25519, deviceID): device.PublicKey().String(), id.NewDeviceKeyID(id.KeyAlgorithmCurve25519, deviceID): self.PublicKey().String()}}
	dev.Signatures = Sign(t, self, &dev, user, self.PublicKey().String())
	own := Sign(t, device, &dev, user, deviceID.String())
	dev.Signatures[user][id.NewKeyID(id.KeyAlgorithmEd25519, deviceID.String())] = own[user][id.NewKeyID(id.KeyAlgorithmEd25519, deviceID.String())]
	return &Fixture{Origin: matrixproof.Origin{Scope: scope, ServerName: server, UserID: user, DeviceID: deviceID, AccountActive: true, DeviceActive: true, Keys: mautrix.RespQueryKeys{MasterKeys: map[id.UserID]mautrix.CrossSigningKeys{user: root}, SelfSigningKeys: map[id.UserID]mautrix.CrossSigningKeys{user: sub}, DeviceKeys: map[id.UserID]map[id.DeviceID]mautrix.DeviceKeys{user: {deviceID: dev}}}}, Master: master, Self: self, Device: device}
}

// NewDevice creates another disposable SDK device signed by the same account root.
// It preserves historical device evidence and never relabels the original key.
func NewDevice(t testing.TB, account *Fixture, device id.DeviceID) *Fixture {
	t.Helper()
	b := NewUser(t, account.Origin.Scope, account.Origin.ServerName, account.Origin.UserID, device)
	b.Master = account.Master
	b.Self = account.Self
	b.Origin.Keys.MasterKeys = account.Origin.Keys.MasterKeys
	b.Origin.Keys.SelfSigningKeys = account.Origin.Keys.SelfSigningKeys
	keys := b.Origin.Keys.DeviceKeys[b.Origin.UserID][device]
	keys.Signatures = Sign(t, account.Self, &keys, b.Origin.UserID, account.Self.PublicKey().String())
	own := Sign(t, b.Device, &keys, b.Origin.UserID, device.String())
	keys.Signatures[b.Origin.UserID][id.NewKeyID(id.KeyAlgorithmEd25519, device.String())] = own[b.Origin.UserID][id.NewKeyID(id.KeyAlgorithmEd25519, device.String())]
	b.Origin.Keys.DeviceKeys[b.Origin.UserID][device] = keys
	return b
}
