// SPDX-License-Identifier: Apache-2.0
// Package resourcetest builds public SDK evidence through the production verifiers.
// It never constructs a private Verified type or grants authority from raw rows.
package resourcetest

import (
	"encoding/json"
	"strings"
	"testing"

	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

type Fixture struct {
	Chain             *matrixprooftest.Fixture
	RecipientIdentity *pqc.Identity
	EpochIdentity     *pqc.Identity
	Enrolled          resource.VerifiedRecipient
	Device            matrixproof.Device
	Scope             storage.Scope
	Actor             resource.Actor
	Roster            resource.VerifiedRoster
	Epoch             resource.VerifiedEpoch
	SignedEpoch       resource.SignedEpoch
	Cap               resource.VerifiedCapability
	SignedCap         resource.SignedCapability
	Candidate         resource.Candidate
	Anchor            resource.Anchor
}

func New(t testing.TB, privateRules bool) Fixture {
	t.Helper()
	scope := storage.Scope{Tenant: "fixture", Subject: "owner", Product: "docs"}
	f := matrixprooftest.NewUser(t, scope, "matrix.test", "@owner:matrix.test", "DOCS_DEVICE")
	encrypted, err := json.Marshal(&event.EncryptedEventContent{Algorithm: id.AlgorithmMegolmV1, SessionID: id.SessionID(f.Master.PublicKey()), SenderKey: id.Curve25519(f.Self.PublicKey()), DeviceID: f.Origin.DeviceID, MegolmCiphertext: []byte("opaque synthetic ciphertext")}) //nolint:staticcheck // SA1019: legacy Matrix device_id fixture field
	if err != nil {
		t.Fatal(err)
	}
	out := NewWithSDK(t, privateRules, f.Origin, f.Master.PublicKey(), func(value any) signatures.Signatures {
		return matrixprooftest.Sign(t, f.Device, value, f.Origin.UserID, f.Origin.DeviceID.String())
	}, "!document:matrix.test", encrypted)
	out.Chain = f
	return out
}

// NewWithSDK reuses the same authority builder with a maintained SDK producer
// and actual encrypted wire. It imports no private Verified constructors and
// never treats an unverified origin or server roster as authority.
func NewWithSDK(t testing.TB, privateRules bool, origin matrixproof.Origin, root id.Ed25519, sign func(any) signatures.Signatures, room string, encrypted json.RawMessage) Fixture {
	return newWithSDKRecipient(t, privateRules, origin, root, sign, room, encrypted, nil)
}

// NewWithSDKRecipient uses the same builder and the device leaf already proven
// by a real product Registry. It never creates an alternate signing authority.
func NewWithSDKRecipient(t testing.TB, privateRules bool, origin matrixproof.Origin, root id.Ed25519, sign func(any) signatures.Signatures, room string, encrypted json.RawMessage, leaf *pqc.Identity) Fixture {
	t.Helper()
	if leaf == nil || leaf.Profile() != pqc.HybridProfile {
		t.Fatal("verified fixture device leaf required")
	}
	return newWithSDKRecipient(t, privateRules, origin, root, sign, room, encrypted, leaf)
}

func resolveFixtureLeaf(t testing.TB, leaf *pqc.Identity) *pqc.Identity {
	t.Helper()
	if leaf != nil {
		return leaf
	}
	key, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustCheckCandidate(t testing.TB, candidate resource.Candidate) {
	t.Helper()
	if !candidate.Valid() {
		t.Fatal("candidate structural invalid", candidate.Proof.Valid(), candidate.Proof.Publication.Valid())
	}
	if err := resource.CheckCandidate(candidate); err != nil {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(candidate.EncryptedEvent, &fields)
		names := make([]string, 0, len(fields))
		for name := range fields {
			names = append(names, name)
		}
		t.Fatal("candidate ciphertext check", names, err)
	}
}

func newWithSDKRecipient(t testing.TB, privateRules bool, origin matrixproof.Origin, root id.Ed25519, sign func(any) signatures.Signatures, room string, encrypted json.RawMessage, leaf *pqc.Identity) Fixture {
	t.Helper()
	if sign == nil {
		t.Fatal("SDK signer required")
	}
	scope := origin.Scope
	var sdkCipher event.EncryptedEventContent
	if json.Unmarshal(encrypted, &sdkCipher) != nil || sdkCipher.Algorithm != id.AlgorithmMegolmV1 {
		t.Fatal("SDK encrypted event required")
	}
	device, err := matrixproof.VerifyUser(t.Context(), origin, origin.UserID, root)
	if err != nil {
		t.Fatal(err)
	}
	actor := resource.Actor{UserID: origin.UserID.String(), DeviceID: origin.DeviceID.String(), RootKey: root.String()}
	target := storage.PublicationTarget{RoomID: room, DeviceID: actor.DeviceID}
	key := resolveFixtureLeaf(t, leaf)
	keyID, _ := pqc.RecipientID(key.Recipient())
	recipient := resource.SignedRecipient{RecipientEnrollment: resource.RecipientEnrollment{Version: 1, Domain: "org.sytue.resource.recipient.v1", Scope: scope, UserID: actor.UserID, Recipient: key.Recipient().String(), KeyID: keyID, Issuer: actor}}
	recipient.Signatures = sign(recipient)
	enrolled, err := resource.VerifyRecipient(t.Context(), recipient, device, target)
	if err != nil {
		t.Fatal(err)
	}
	r := resource.Resource{Tenant: scope.Tenant, Product: scope.Product, Kind: "document", ID: "document1"}
	baseline := resource.Position{Seq: "9007199254740993", HeadCipherSHA256: strings.Repeat("1", 64)}
	actions := []resource.Action{resource.Comment, resource.Manage, resource.Publish, resource.Read, resource.Validate}
	roster, err := resource.VerifyRoster(r, []resource.Member{{Scope: scope, UserID: actor.UserID, RootKey: actor.RootKey, Recipient: key.Recipient().String(), ProductKeyID: keyID, EnrollmentSHA256: enrolled.SHA256(), Role: "owner", Actions: actions, From: baseline}}, []resource.VerifiedRecipient{enrolled})
	if err != nil {
		t.Fatal(err)
	}
	epochKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	signedEpoch := resource.SignedEpoch{Epoch: resource.Epoch{Version: 1, Domain: "org.sytue.resource.epoch.v1", Resource: r, RoomID: target.RoomID, ID: "epoch1", Generation: "1", Recipient: epochKey.Recipient().String(), MembershipSHA256: roster.SHA256(), History: resource.HistoryFull, Cutover: baseline, Validation: resource.ValidationPolicy{PrivateRules: privateRules, RulesVersion: strings.Repeat("2", 64)}, Issuer: actor}}
	baselineContext, err := resource.NewBootstrapContext(resource.BootstrapBaseline, r, target.RoomID, "epoch1", epochKey.Recipient().String(), resource.Position{Seq: baseline.Seq}, scope, actor)
	if err != nil {
		t.Fatal(err)
	}
	baselineHash, err := resource.BootstrapContextSHA256(baselineContext)
	if err != nil {
		t.Fatal(err)
	}
	checkpointContext, err := resource.NewBootstrapContext(resource.BootstrapCheckpoint, r, target.RoomID, "epoch1", epochKey.Recipient().String(), baseline, scope, actor)
	if err != nil {
		t.Fatal(err)
	}
	checkpointHash, err := resource.BootstrapContextSHA256(checkpointContext)
	if err != nil {
		t.Fatal(err)
	}
	signedEpoch.Baseline = resource.Baseline{Position: baseline, Reference: storage.Reference{RoomID: target.RoomID, EventID: "$baseline"}, ContextSHA256: baselineHash, SessionCertificateID: "baseline-certificate"}
	signedEpoch.Checkpoint = resource.Checkpoint{SourceCutover: baseline, Reference: storage.Reference{RoomID: target.RoomID, EventID: "$checkpoint"}, CipherSHA256: strings.Repeat("4", 64), ContextSHA256: checkpointHash, SessionCertificateID: "checkpoint-certificate"}
	hash, err := resource.EpochDigest(signedEpoch.Epoch)
	if err != nil {
		t.Fatal(err)
	}
	signedEpoch.Signatures = sign(signedEpoch)
	anchor := resource.Anchor{Resource: r, PolicySHA256: hash, OwnerScope: scope, Owner: actor, Baseline: baseline}
	epoch, err := resource.VerifyInitialEpoch(t.Context(), signedEpoch, roster, device, anchor)
	if err != nil {
		t.Fatal(err)
	}
	signedCap := resource.SignedCapability{Capability: resource.Capability{Version: 1, Domain: "org.sytue.resource.capability.v1", Resource: r, RoomID: target.RoomID, Epoch: "epoch1", Generation: "1", MembershipSHA256: roster.SHA256(), PolicySHA256: epoch.SHA256(), Subject: scope.Subject, UserID: actor.UserID, ProductKeyID: keyID, Role: "owner", Actions: actions, From: baseline, Issuer: actor}}
	signedCap.Signatures = sign(signedCap)
	verifiedCap, err := resource.VerifyCapability(t.Context(), signedCap, epoch, device)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := resource.CipherJSONDigest(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	expected := sessionproof.CertificateExpectation{Scope: scope, Resource: r, Epoch: "epoch1", Generation: "1", PolicySHA256: epoch.SHA256(), Session: sessionproof.Session{RoomID: target.RoomID, SessionID: sdkCipher.SessionID.String(), SenderKey: sdkCipher.SenderKey.String(), SenderUser: actor.UserID, SigningKey: origin.Keys.DeviceKeys[origin.UserID][origin.DeviceID].Keys[id.NewDeviceKeyID(id.KeyAlgorithmEd25519, origin.DeviceID)]}, DeviceID: actor.DeviceID, RootKey: actor.RootKey}
	encoded, err := sessionproof.EncodeCertificate(expected)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := sessionproof.DecodeCertificate(encoded.Payload)
	if err != nil {
		t.Fatal(err)
	}
	cert.Signatures = sign(cert)
	certificate, err := json.Marshal(cert)
	if err != nil {
		t.Fatal(err)
	}
	proof := resource.SignedPublication{CipherPublication: resource.CipherPublication{Publication: resource.Publication{TransactionID: "00000000-0000-4000-8000-000000000001", Version: 1, Domain: "org.sytue.resource.publication.v1", Resource: r, RoomID: target.RoomID, Epoch: "epoch1", Generation: "1", MembershipSHA256: roster.SHA256(), PolicySHA256: epoch.SHA256(), CapabilitySHA256: verifiedCap.SHA256(), Actor: actor, OpID: "op1", Action: resource.Publish, ExpectedBase: baseline, OpCount: "2"}, SessionCertificateID: encoded.ID, InnerCipherSHA256: strings.Repeat("3", 64), OuterCipherSHA256: outer}}
	proof.Signatures = sign(proof)
	candidate := resource.Candidate{Proof: proof, EncryptedEvent: encrypted, SessionCertificate: certificate}
	mustCheckCandidate(t, candidate)
	return Fixture{RecipientIdentity: key, EpochIdentity: epochKey, Enrolled: enrolled, Device: device, Scope: scope, Actor: actor, Roster: roster, Epoch: epoch, SignedEpoch: signedEpoch, Cap: verifiedCap, SignedCap: signedCap, Candidate: candidate, Anchor: anchor}
}
