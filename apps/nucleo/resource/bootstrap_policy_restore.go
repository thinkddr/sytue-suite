// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// RetainedBootstrapSlot is read from an ORIGINAL immutable accepted SQL slot.
// Prepared/sending/ambiguous slots and a complete roster capture are not ACKs.
type RetainedBootstrapSlot struct {
	State           string
	Kind            BootstrapKind
	CandidateSHA256 string
	Candidate       BootstrapCandidate
	Reference       storage.Reference
}

// RetainedBootstrapPolicy binds independently retained Source scalars to its
// original public plan transcript and the CURRENT active genesis policy. It
// contains no key, content, caller-selected root or new publication authority.
type RetainedBootstrapPolicy struct {
	Capture                   RetainedBootstrapCapture
	State                     string
	Epoch                     string
	Generation                Decimal
	PolicySHA256              string
	MembershipSHA256          string
	PlanEvidenceSHA256        string
	PlanEvidence, SignedEpoch json.RawMessage
	Baseline, Checkpoint      RetainedBootstrapSlot
}

type TrustedBootstrapPolicySource interface {
	PublicEvidenceSource
	RetainedBootstrapPolicy(context.Context, Resource, string) (RetainedBootstrapPolicy, error)
}

// BootstrapPolicyAnchorSource is an explicit independent trust port: its anchor
// must come from this owner's authenticated private product vault (or an
// authenticated invitation), never from the ORIGINAL Source response itself.
type BootstrapPolicyAnchorSource interface {
	BootstrapPolicyAnchor(context.Context, Resource, string) (Anchor, error)
}

type restoredBootstrapAuthority struct {
	source      TrustedBootstrapPolicySource
	current     VerifiedSession
	key         Resource
	planID      string
	retainedSHA string
}

func (a *restoredBootstrapAuthority) revalidate(ctx context.Context, current VerifiedSession) error {
	if a == nil || a.source == nil || !current.valid() || current.Binding() != a.current.Binding() || current.Device().Identity() != a.current.Device().Identity() {
		return ErrRevoked
	}
	// Revalidate the ORIGINAL consumed Provider witness retained at restore,
	// even if a caller presents another token with identical public bindings.
	if err := a.current.Revalidate(ctx); err != nil {
		return err
	}
	retained, err := a.source.RetainedBootstrapPolicy(ctx, a.key, a.planID)
	if err != nil {
		return err
	}
	sha, err := bootstrapPolicySnapshotSHA(retained)
	if err != nil || sha != a.retainedSHA || !bootstrapPolicyOwner(retained, current) {
		return ErrRevoked
	}
	return a.current.Revalidate(ctx)
}

func bootstrapPolicySnapshotSHA(value RetainedBootstrapPolicy) (string, error) {
	return matrixproof.RequestDigest(value)
}

func bootstrapPolicyOwner(value RetainedBootstrapPolicy, current VerifiedSession) bool {
	c := value.Capture
	b := current.Binding()
	return current.valid() && c.State == "complete" && value.State == "active" && value.Generation == "1" && c.Source.Valid() && c.Source.Resource == c.Resource && c.Source.OwnerScope == b.Scope && c.OwnerPin.Scope == b.Scope && identityMatches(c.OwnerPin, c.Creator, b.Scope) && c.Creator.UserID == b.UserID && c.Creator.RootKey == b.RootKey && c.RootGeneration == b.RootGeneration && c.RootGeneration != "0"
}

// RestoredBootstrapPolicy exposes only authority bound to actual CURRENT B and
// the ORIGINAL Source fence. Access()/Window() recheck that fence; all access
// subsequently retains it through AuthorizeSession, Reauthorize and checkFence.
// There is deliberately no raw VerifiedEpoch/VerifiedCapability getter.
type RestoredBootstrapPolicy struct {
	epoch      VerifiedEpoch
	capability VerifiedCapability
	current    VerifiedSession
	plan       BootstrapPlan
}

func (p RestoredBootstrapPolicy) Access(ctx context.Context, action Action) (VerifiedAccess, error) {
	if p.epoch.restored == nil {
		return VerifiedAccess{}, ErrRevoked
	}
	return AuthorizeSession(ctx, p.epoch, p.capability, p.current, action)
}

func (p RestoredBootstrapPolicy) Window(ctx context.Context) (EpochWindow, error) {
	if p.epoch.restored == nil {
		return EpochWindow{}, ErrRevoked
	}
	if err := p.epoch.restored.revalidate(ctx, p.current); err != nil {
		return EpochWindow{}, err
	}
	return CurrentEpoch(p.epoch)
}

func restoredEpochMatchesPlan(signed SignedEpoch, retained RetainedBootstrapPolicy, key Resource, p BootstrapPlan, roster VerifiedRoster) bool {
	return signed.ID == retained.Epoch && signed.Generation == retained.Generation && signed.MembershipSHA256 == retained.MembershipSHA256 && signed.Resource == key && signed.RoomID == p.RoomID && signed.ID == p.Epoch && signed.Recipient == p.Recipient && signed.MembershipSHA256 == p.MembershipSHA256 && signed.History == p.History && signed.Validation == p.Validation && signed.Issuer == p.Creator && signed.Baseline.Position.Seq == p.Source.SourceSeq && roster.issuer(p.CreatorScope, p.Creator)
}

func verifyRestoredBootstrapEpoch(ctx context.Context, retained RetainedBootstrapPolicy, key Resource, anchor Anchor, p BootstrapPlan, roster VerifiedRoster, signer matrixproof.HistoricalDevice) (SignedEpoch, error) {
	var signed SignedEpoch
	if err := decodeReservationValue(retained.SignedEpoch, &signed); err != nil {
		return SignedEpoch{}, err
	}
	if err := ValidateInitialBaseline(signed.Epoch, anchor); err != nil {
		return SignedEpoch{}, err
	}
	if !restoredEpochMatchesPlan(signed, retained, key, p, roster) {
		return SignedEpoch{}, ErrInvalid
	}
	if err := signer.VerifyJSON(ctx, p.CreatorScope, storage.PublicationTarget{RoomID: p.RoomID, DeviceID: p.Creator.DeviceID}, signed); err != nil {
		return SignedEpoch{}, err
	}
	if err := verifyRetainedBootstrapSlot(ctx, retained.Baseline, p, signed.Epoch, signer); err != nil {
		return SignedEpoch{}, err
	}
	if err := verifyRetainedBootstrapSlot(ctx, retained.Checkpoint, p, signed.Epoch, signer); err != nil {
		return SignedEpoch{}, err
	}
	if retained.Baseline.Kind != BootstrapBaseline || retained.Checkpoint.Kind != BootstrapCheckpoint || retained.Baseline.Reference == retained.Checkpoint.Reference {
		return SignedEpoch{}, ErrInvalid
	}
	return signed, nil
}

// RestoreBootstrapPolicy restores GENESIS public authority without any held
// VerifiedEpoch/VerifiedCapability or live lookup of retired producer A. It does
// not restore a private key, ACK work, authorize a send or activate SQL routing.
// Capability evidence is independently reverified against the anchored roster;
// a hash/reference supplied by a caller conveys no authority by itself.
//
//nolint:gocyclo // sequential fail-closed verification steps of security-critical code; splitting them would scatter the check order
func RestoreBootstrapPolicy(ctx context.Context, key Resource, planID string, capability CapabilityEvidence, current VerifiedSession, anchors BootstrapPolicyAnchorSource, source TrustedBootstrapPolicySource) (RestoredBootstrapPolicy, error) {
	if !key.Valid() || !canonicalTransaction(planID) || !current.valid() || anchors == nil || source == nil {
		return RestoredBootstrapPolicy{}, ErrInvalid
	}
	if err := current.Revalidate(ctx); err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	anchor, err := anchors.BootstrapPolicyAnchor(ctx, key, planID)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	retained, err := source.RetainedBootstrapPolicy(ctx, key, planID)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	c := retained.Capture
	if !bootstrapPolicyOwner(retained, current) || c.Resource != key || c.PlanID != planID || anchor.Resource != key || anchor.OwnerScope != c.Source.OwnerScope || anchor.Owner != c.Creator || anchor.PolicySHA256 != retained.PolicySHA256 {
		return RestoredBootstrapPolicy{}, ErrRevoked
	}
	if _, err := restoreBootstrapCaptureEvidence(ctx, key, planID, c, source); err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	records := reservationReader{source: source, resource: key, cache: map[string]json.RawMessage{}}
	plan, roster, signer, err := records.bootstrapPolicyPlan(ctx, retained)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	p := plan.BootstrapPlan
	signed, err := verifyRestoredBootstrapEpoch(ctx, retained, key, anchor, p, roster, signer)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	restored, err := records.capability(ctx, capability, signed.Epoch, retained.PolicySHA256, roster.members)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	var signedCap SignedCapability
	capSigner, err := records.statement(ctx, capability.Capability, &signedCap, restored.value.Issuer, capability.Capability.Signer.Pin.Scope, p.RoomID)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	sha, err := bootstrapPolicySnapshotSHA(retained)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	authority := &restoredBootstrapAuthority{source: source, current: current, key: key, planID: planID, retainedSHA: sha}
	epoch := VerifiedEpoch{proof: bytes.Clone(retained.SignedEpoch), device: signer, value: signed.Epoch, roster: roster, hash: retained.PolicySHA256, restored: authority}
	verifiedCap := VerifiedCapability{proof: bytes.Clone(records.cache[capability.Capability.StatementSHA256]), device: capSigner, value: restored.value, member: restored.member, enrollment: restored.enrollment, epochHash: epoch.hash, hash: restored.hash}
	out := RestoredBootstrapPolicy{epoch: epoch, capability: verifiedCap, current: current, plan: p}
	if _, err := out.Access(ctx, Read); err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	return out, nil
}

func validBootstrapPlanEvidence(retained RetainedBootstrapPolicy) (BootstrapPlanEvidence, error) {
	c := retained.Capture
	sha, err := CipherJSONDigest(retained.PlanEvidence)
	if err != nil || sha != retained.PlanEvidenceSHA256 {
		return BootstrapPlanEvidence{}, ErrInvalid
	}
	var descriptor BootstrapPlanEvidence
	if err := decodeReservationValue(retained.PlanEvidence, &descriptor); err != nil {
		return BootstrapPlanEvidence{}, err
	}
	if descriptor.Version != 1 || descriptor.Domain != "org.sytue.resource.bootstrap.evidence.v1" || descriptor.PlanSHA256 != c.PlanSHA256 || descriptor.Plan.Signer.Pin != c.OwnerPin || descriptor.SessionBindingSHA256 != c.SessionBindingSHA256 {
		return BootstrapPlanEvidence{}, ErrInvalid
	}
	return descriptor, nil
}

func (r *reservationReader) bootstrapPolicyRecipients(ctx context.Context, refs []StatementEvidence) ([]VerifiedRecipient, error) {
	recipients := make([]VerifiedRecipient, 0, len(refs))
	for _, ref := range refs {
		var value SignedRecipient
		if err := r.record(ctx, ref.StatementSHA256, &value); err != nil {
			return nil, err
		}
		device, err := r.signer(ctx, ref.Signer)
		if err != nil || !value.Valid() || !identityMatches(device.Identity(), value.Issuer, value.Scope) {
			return nil, ErrInvalid
		}
		if err := device.VerifyAccountJSON(ctx, value.Scope, value); err != nil {
			return nil, err
		}
		hash, err := matrixproof.RequestDigest(value.RecipientEnrollment)
		if err != nil {
			return nil, err
		}
		recipients = append(recipients, VerifiedRecipient{proof: bytes.Clone(r.cache[ref.StatementSHA256]), device: device, enrollment: value.RecipientEnrollment, hash: hash})
	}
	return recipients, nil
}

func (r *reservationReader) bootstrapPolicyPlan(ctx context.Context, retained RetainedBootstrapPolicy) (SignedBootstrapPlan, VerifiedRoster, matrixproof.HistoricalDevice, error) {
	c := retained.Capture
	descriptor, err := validBootstrapPlanEvidence(retained)
	if err != nil {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, err
	}
	var plan SignedBootstrapPlan
	signer, err := r.statement(ctx, descriptor.Plan, &plan, c.Creator, c.Source.OwnerScope, c.RoomID)
	if err != nil {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, err
	}
	planSHA, err := BootstrapPlanSHA256(plan.BootstrapPlan)
	if err != nil || planSHA != c.PlanSHA256 || plan.ID != c.PlanID || plan.Source != c.Source || plan.RoomID != c.RoomID || plan.Creator != c.Creator || plan.SessionBindingSHA256 != c.SessionBindingSHA256 {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, ErrInvalid
	}
	var binding SessionBinding
	if err := r.record(ctx, descriptor.SessionBindingSHA256, &binding); err != nil {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, err
	}
	if !binding.Valid() || binding.Scope != c.Source.OwnerScope || binding.Actor() != c.Creator || binding.Generation != c.SessionGeneration || binding.RootGeneration != c.RootGeneration {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, ErrInvalid
	}
	var members []Member
	if err := r.record(ctx, descriptor.MembersSHA256, &members); err != nil {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, err
	}
	recipients, err := r.bootstrapPolicyRecipients(ctx, descriptor.Recipients)
	if err != nil {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, err
	}
	roster, err := VerifyRoster(c.Resource, members, recipients)
	if err != nil || roster.hash != plan.MembershipSHA256 {
		return SignedBootstrapPlan{}, VerifiedRoster{}, matrixproof.HistoricalDevice{}, ErrInvalid
	}
	return plan, roster, signer, nil
}

func retainedSlotMatchesEpoch(slot RetainedBootstrapSlot, plan BootstrapPlan, epoch Epoch) (string, string, error) {
	position := Position{Seq: plan.Source.SourceSeq}
	ref, cipher, contextHash, certificate := epoch.Baseline.Reference, epoch.Baseline.Position.HeadCipherSHA256, epoch.Baseline.ContextSHA256, epoch.Baseline.SessionCertificateID
	switch slot.Kind {
	case BootstrapBaseline:
	case BootstrapCheckpoint:
		position = epoch.Cutover
		ref, cipher, contextHash, certificate = epoch.Checkpoint.Reference, epoch.Checkpoint.CipherSHA256, epoch.Checkpoint.ContextSHA256, epoch.Checkpoint.SessionCertificateID
	default:
		return "", "", ErrInvalid
	}
	contextValue, err := NewBootstrapContext(slot.Kind, epoch.Resource, plan.RoomID, plan.Epoch, plan.Recipient, position, plan.CreatorScope, plan.Creator)
	if err != nil {
		return "", "", err
	}
	p := slot.Candidate.Proof.BootstrapPublication
	wantContext, err := BootstrapContextSHA256(contextValue)
	if err != nil || wantContext != contextHash || slot.Reference != ref || p.OuterCipherSHA256 != cipher || p.ContextSHA256 != contextHash || p.SessionCertificateID != certificate {
		return "", "", ErrInvalid
	}
	return contextHash, certificate, nil
}

func verifyRetainedBootstrapSlot(ctx context.Context, slot RetainedBootstrapSlot, plan BootstrapPlan, epoch Epoch, signer matrixproof.HistoricalDevice) error {
	if slot.State != "accepted" || !slot.Reference.Valid() || slot.Reference.RoomID != plan.RoomID {
		return ErrPending
	}
	p := slot.Candidate.Proof.BootstrapPublication
	planSHA, err := BootstrapPlanSHA256(plan)
	if err != nil || p.PlanSHA256 != planSHA || p.Actor != plan.Creator || p.Kind != slot.Kind {
		return ErrInvalid
	}
	hash, err := BootstrapCandidateSHA256(slot.Candidate)
	if err != nil || hash != slot.CandidateSHA256 {
		return ErrInvalid
	}
	contextHash, certificate, err := retainedSlotMatchesEpoch(slot, plan, epoch)
	if err != nil {
		return err
	}
	if err := signer.VerifyJSON(ctx, plan.CreatorScope, storage.PublicationTarget{RoomID: plan.RoomID, DeviceID: plan.Creator.DeviceID}, slot.Candidate.Proof); err != nil {
		return err
	}
	expected := sessionproof.CertificateExpectation{Scope: plan.CreatorScope, Resource: epoch.Resource, Epoch: epoch.ID, Generation: "1", PolicySHA256: contextHash, Session: sessionproof.Session{RoomID: plan.RoomID, SenderUser: plan.Creator.UserID, SigningKey: signer.Identity().SigningKey.String()}, DeviceID: plan.Creator.DeviceID, RootKey: plan.Creator.RootKey}
	verified, err := verifyEncryptedSessionCertificate(ctx, slot.Candidate.SessionCertificate, slot.Candidate.EncryptedEvent, expected, func(ctx context.Context, raw []byte, expected sessionproof.CertificateExpectation) (sessionproof.VerifiedCertificate, error) {
		return sessionproof.VerifyHistoricalCertificate(ctx, raw, expected, signer)
	})
	if err != nil || verified.ID() != certificate {
		return ErrInvalid
	}
	return ctx.Err()
}
