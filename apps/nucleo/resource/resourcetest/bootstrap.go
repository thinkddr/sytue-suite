// SPDX-License-Identifier: Apache-2.0
package resourcetest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/crypto/signatures"
	"maunium.net/go/mautrix/event"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
)

// BootstrapFixture verifies real SDK signatures and ORIGINAL source callbacks.
// Its OpaqueCandidate is synthetic wire for metadata/SQL tests, never a claim of
// Matrix encryption/decryption or successful origin admission.
type BootstrapFixture struct {
	Roster   resource.VerifiedRoster
	Base     Fixture
	Current  resource.VerifiedSession
	Signed   resource.SignedBootstrapPlan
	Verified resource.VerifiedBootstrapPlan
	Sign     func(any) signatures.Signatures
}

//nolint:revive // testing.TB precedes context.Context in test fixture constructor
func NewBootstrap(t testing.TB, ctx context.Context, f Fixture, current resource.VerifiedSession, source resource.OriginalBootstrapSource, key resource.Resource) BootstrapFixture {
	t.Helper()
	if f.Chain == nil {
		t.Fatal("SDK test signing fixture required")
	}
	sign := func(value any) signatures.Signatures {
		return matrixprooftest.Sign(t, f.Chain.Device, value, f.Chain.Origin.UserID, current.Actor().DeviceID)
	}
	snapshot, err := source.BootstrapSource(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	members := f.Roster.Members()
	// Full-history genesis needs no unknown ciphertext head in its roster. The
	// exact retained source seq becomes Baseline.Position after actual ACK.
	for i := range members {
		members[i].From = resource.Position{Seq: "0"}
	}
	roster, err := resource.VerifyRoster(key, members, []resource.VerifiedRecipient{f.Enrolled})
	if err != nil {
		t.Fatal(err)
	}
	hash, err := matrixproof.RequestDigest(current.Binding())
	if err != nil {
		t.Fatal(err)
	}
	value := resource.BootstrapPlan{Version: 1, Domain: "org.sytue.resource.bootstrap.plan.v1", ID: uuid.NewString(), Source: snapshot, RoomID: f.Epoch.Value().RoomID, Epoch: "bootstrap-" + uuid.NewString(), Recipient: f.EpochIdentity.Recipient().String(), MembershipSHA256: roster.SHA256(), History: snapshot.History, Validation: f.Epoch.Value().Validation, CreatorScope: current.Scope(), Creator: current.Actor(), SessionBindingSHA256: hash}
	signed := resource.SignedBootstrapPlan{BootstrapPlan: value, Signatures: sign(value)}
	verified, err := resource.VerifyBootstrapPlan(ctx, signed, roster, current, source)
	if err != nil {
		t.Fatal(err)
	}
	return BootstrapFixture{Roster: roster, Base: f, Current: current, Signed: signed, Verified: verified, Sign: sign}
}

func (f BootstrapFixture) OpaqueCandidate(t testing.TB, kind resource.BootstrapKind, baseline *resource.VerifiedBootstrapAcceptance) resource.BootstrapCandidate {
	t.Helper()
	bootstrapCtx, err := f.Verified.BaselineContext()
	if kind == resource.BootstrapCheckpoint {
		if baseline == nil {
			t.Fatal("baseline ACK fixture required")
		}
		bootstrapCtx, err = f.Verified.CheckpointContext(*baseline)
	}
	if err != nil {
		t.Fatal(err)
	}
	hash, err := resource.BootstrapContextSHA256(bootstrapCtx)
	if err != nil {
		t.Fatal(err)
	}
	var encrypted event.EncryptedEventContent
	if json.Unmarshal(f.Base.Candidate.EncryptedEvent, &encrypted) != nil {
		t.Fatal("SDK event fixture invalid")
	}
	encrypted.MegolmCiphertext = []byte("opaque metadata bootstrap " + string(kind))
	wire, err := json.Marshal(&encrypted)
	if err != nil {
		t.Fatal(err)
	}
	outer, err := resource.OuterCipherDigest(wire)
	if err != nil {
		t.Fatal(err)
	}
	p := f.Signed.BootstrapPlan
	identity := f.Current.Device().Identity()
	expected := sessionproof.CertificateExpectation{Scope: p.CreatorScope, Resource: p.Source.Resource, Epoch: p.Epoch, Generation: "1", PolicySHA256: hash, Session: sessionproof.Session{RoomID: p.RoomID, SessionID: encrypted.SessionID.String(), SenderKey: encrypted.SenderKey.String(), SenderUser: p.Creator.UserID, SigningKey: identity.SigningKey.String()}, DeviceID: p.Creator.DeviceID, RootKey: p.Creator.RootKey}
	certificate := sessionproof.Certificate{Version: 1, Kind: sessionproof.CertificateKind, CertificateExpectation: expected}
	certificate.Signatures = f.Sign(certificate)
	raw, err := json.Marshal(certificate)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := sessionproof.EncodeCertificate(expected)
	if err != nil {
		t.Fatal(err)
	}
	publication := resource.BootstrapPublication{Version: 1, Domain: "org.sytue.resource.bootstrap.publication.v1", PlanSHA256: f.Verified.SHA256(), Kind: kind, TransactionID: uuid.NewString(), ContextSHA256: hash, InnerCipherSHA256: f.Base.Candidate.Proof.InnerCipherSHA256, OuterCipherSHA256: outer, SessionCertificateID: encoded.ID, Actor: p.Creator}
	return resource.BootstrapCandidate{Proof: resource.SignedBootstrapPublication{BootstrapPublication: publication, Signatures: f.Sign(publication)}, EncryptedEvent: wire, SessionCertificate: raw}
}

func (f BootstrapFixture) FinalEpoch(t testing.TB, baseline, checkpoint resource.VerifiedBootstrapAcceptance) resource.SignedEpoch {
	t.Helper()
	b, err := baseline.Baseline()
	if err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	p := f.Signed.BootstrapPlan
	value := resource.Epoch{Version: 1, Domain: "org.sytue.resource.epoch.v1", Resource: p.Source.Resource, RoomID: p.RoomID, ID: p.Epoch, Generation: "1", Recipient: p.Recipient, MembershipSHA256: p.MembershipSHA256, History: p.History, Cutover: b.Position, Baseline: b, Checkpoint: cp, Validation: p.Validation, Issuer: p.Creator}
	return resource.SignedEpoch{Epoch: value, Signatures: f.Sign(value)}
}
