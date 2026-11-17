// SPDX-License-Identifier: Apache-2.0
package resource_test

import (
	"errors"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/resource/resourcetest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof/matrixprooftest"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

func rotateSessionEpoch(t testing.TB, prior resource.VerifiedEpoch, capability resource.VerifiedCapability, original resource.SignedEpoch, session resource.VerifiedSession, source *resourcetest.SessionSource, cutover ...resource.Position) (resource.VerifiedEpoch, resource.VerifiedCapability, *pqc.Identity) {
	t.Helper()
	key, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	member := capability.Member()
	member.Recipient = session.Recipient().Value().Recipient
	member.ProductKeyID = session.Recipient().Value().KeyID
	member.EnrollmentSHA256 = session.Recipient().SHA256()
	roster, err := resource.VerifyRoster(prior.Value().Resource, []resource.Member{member}, []resource.VerifiedRecipient{session.Recipient()})
	if err != nil {
		t.Fatal(err)
	}
	next := original
	next.Epoch = prior.Value()
	if len(cutover) != 0 {
		next.Cutover = cutover[0]
	}
	next.ID = "epoch-" + session.Actor().DeviceID
	next.Generation, err = prior.Value().Generation.Next(1)
	if err != nil {
		t.Fatal(err)
	}
	next.PreviousEpoch = prior.Value().ID
	next.PreviousPolicySHA256 = prior.SHA256()
	next.MembershipSHA256 = roster.SHA256()
	next.Recipient = key.Recipient().String()
	next.Issuer = session.Actor()
	checkpoint, err := resource.NewBootstrapContext(resource.BootstrapCheckpoint, next.Resource, next.RoomID, next.ID, next.Recipient, next.Cutover, session.Scope(), session.Actor())
	if err != nil {
		t.Fatal(err)
	}
	contextHash, err := resource.BootstrapContextSHA256(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	next.Checkpoint = resource.Checkpoint{SourceCutover: next.Cutover, Reference: storage.Reference{RoomID: next.RoomID, EventID: "$checkpoint-" + session.Actor().DeviceID}, CipherSHA256: strings.Repeat("9", 64), ContextSHA256: contextHash, SessionCertificateID: "certificate-" + session.Actor().DeviceID}
	// The fixture retains the real SDK signing key exclusively in its directory.
	// It does not manufacture any VerifiedAccess/Device from JSON.
	next.Signatures = matrixprooftest.Sign(t, source.Chain.Device, next, source.Chain.Origin.UserID, session.Actor().DeviceID)
	if _, err := resource.VerifyNextEpoch(t.Context(), next, roster, session.Device(), prior, capability, next.Cutover); err == nil {
		t.Fatal("strict legacy verifier impersonated previous issuer")
	}
	epoch, err := resource.VerifyNextEpochWithSession(t.Context(), next, roster, session.Device(), prior, capability, next.Cutover, session)
	if err != nil {
		t.Fatal(err)
	}
	signed := resource.SignedCapability{Capability: capability.Value()}
	signed.Epoch = next.ID
	signed.Generation = next.Generation
	signed.MembershipSHA256 = roster.SHA256()
	signed.PolicySHA256 = epoch.SHA256()
	signed.ProductKeyID = member.ProductKeyID
	signed.Issuer = session.Actor()
	signed.Signatures = matrixprooftest.Sign(t, source.Chain.Device, signed, source.Chain.Origin.UserID, session.Actor().DeviceID)
	verified, err := resource.VerifyCapability(t.Context(), signed, epoch, session.Device())
	if err != nil {
		t.Fatal(err)
	}
	return epoch, verified, key
}

func TestResourceFreshSessionPreservesHistoricalRoleAndRotatesDeviceRecipient(t *testing.T) {
	f := resourcetest.New(t, false)
	b := matrixprooftest.NewDevice(t, f.Chain, "DOCS_B")
	bKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	current, bSource := resourcetest.EnrolledSession(t, b, bKey)
	manager, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, current, resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	if manager.Actor() != current.Actor() || manager.Scope() != f.Scope || manager.Capability().Enrollment().Issuer != f.Actor {
		t.Fatal("current B overwrote historical A role issuer")
	}
	if _, ok := manager.CurrentSessionBinding(); !ok {
		t.Fatal("current enrollment was discarded")
	}
	readerB, err := resource.Reauthorize(t.Context(), manager, f.Scope.Subject, resource.Manage)
	if err != nil || readerB.Actor() != current.Actor() {
		t.Fatal("reauthorization discarded current B", err)
	}
	if _, ok := readerB.CurrentSessionBinding(); !ok {
		t.Fatal("reauthorization dropped Provider token")
	}
	if _, err := resource.Reauthorize(t.Context(), manager, f.Scope.Subject, resource.Read); err == nil {
		t.Fatal("reauthorization replaced the independently verified action")
	}
	if _, err := resource.Reauthorize(t.Context(), manager, "another-subject", resource.Manage); err == nil {
		t.Fatal("reauthorization changed authenticated subject")
	}
	wrong := matrixprooftest.NewDevice(t, f.Chain, "MAIL_B")
	wrong.Origin.Scope.Product = "mail"
	wrongKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	mail, _ := resourcetest.EnrolledSession(t, wrong, wrongKey)
	if _, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, mail, resource.Manage); err == nil {
		t.Fatal("Mail enrollment inherited Docs role")
	}
	if _, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, resource.VerifiedSession{}, resource.Manage); err == nil {
		t.Fatal("raw empty session inherited Docs role")
	}
	unrelated := matrixprooftest.NewUser(t, f.Scope, f.Chain.Origin.ServerName, f.Chain.Origin.UserID, "UNRELATED_ROOT")
	unrelatedKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	unrelatedSession, _ := resourcetest.EnrolledSession(t, unrelated, unrelatedKey)
	if _, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, unrelatedSession, resource.Manage); err == nil {
		t.Fatal("unrelated root inherited historical role")
	}
	writer, err := resource.AuthorizeSession(t.Context(), f.Epoch, f.Cap, current, resource.Publish)
	if err != nil {
		t.Fatal(err)
	}
	changed := f.Candidate
	changed.Proof.Actor = current.Actor()
	changed.Proof.Signatures = matrixprooftest.Sign(t, b.Device, changed.Proof, b.Origin.UserID, current.Actor().DeviceID)
	if _, err := resource.VerifyProducerCandidate(t.Context(), changed, writer, current.Device()); err == nil {
		t.Fatal("B published under A historical capability before new roster")
	}
	next, nextCap, _ := rotateSessionEpoch(t, f.Epoch, f.Cap, f.SignedEpoch, current, bSource)
	if nextCap.Enrollment().Issuer != current.Actor() || f.Cap.Enrollment().Issuer != f.Actor {
		t.Fatal("new roster failed to preserve A/B identities")
	}
	c := matrixprooftest.NewDevice(t, b, "DOCS_C")
	cKey, err := pqc.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	cSession, cSource := resourcetest.EnrolledSession(t, c, cKey)
	future, futureCap, futureKey := rotateSessionEpoch(t, next, nextCap, f.SignedEpoch, cSession, cSource)
	bSource.Retire()
	if _, err := resource.Reauthorize(t.Context(), manager, f.Scope.Subject, resource.Manage); err == nil {
		t.Fatal("reauthorization cached retired B")
	}
	if _, err := resource.AuthorizeSession(t.Context(), next, nextCap, current, resource.Manage); err == nil {
		t.Fatal("retired B remained manager")
	}
	futureWindow, err := resource.CurrentEpoch(future)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := resource.BindOwnedEpoch(futureWindow, futureKey)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	futureManager, err := resource.AuthorizeSession(t.Context(), future, futureCap, cSession, resource.Manage)
	if err != nil {
		t.Fatal(err)
	}
	futureReader, err := resource.AuthorizeSession(t.Context(), future, futureCap, cSession, resource.Read)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := handle.PrepareGrant(t.Context(), futureManager, futureCap, resourceFence(activeResourceFence))
	if err != nil {
		t.Fatal(err)
	}
	rawContext, err := matrixproof.CanonicalBytes(grant.Context)
	if err != nil {
		t.Fatal(err)
	}
	for name, retired := range map[string]*pqc.Identity{"A": f.RecipientIdentity, "B": bKey} {
		if plaintext, err := pqc.Open(retired, rawContext, grant.Envelope); err == nil || plaintext != nil {
			clear(plaintext)
			t.Fatal("retired leaf opened future grant", name)
		}
	}
	// B still has the SAME account master/self signing private keys as C. Those
	// global Matrix secrets do not open a grant addressed to C's independent leaf.
	if b.Master != c.Master || b.Self != c.Self {
		t.Fatal("test lost global Matrix recovery secret knowledge")
	}
	signedGrant := resource.SignedEpochGrant{EpochGrant: grant}
	signedGrant.Signatures = matrixprooftest.Sign(t, c.Device, signedGrant, c.Origin.UserID, cSession.Actor().DeviceID)
	opened, err := resource.OpenGrant(t.Context(), signedGrant, futureWindow, futureManager, futureReader, cKey, cSession.Device(), resourceFence(activeResourceFence))
	if err != nil {
		t.Fatal(err)
	}
	opened.Close()
	cSource.Retire()
	if err := futureReader.RevalidateSession(t.Context()); !errors.Is(err, resource.ErrRevoked) {
		t.Fatal("reader retained withdrawn session", err)
	}
}
