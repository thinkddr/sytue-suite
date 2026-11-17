// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapRecoveryRequirement binds ONE bootstrap's private recovery backup.
// It is public metadata, never an ACK, a SourceComplete command or authority.
type BootstrapRecoveryRequirement struct {
	Resource              Resource
	PlanID                string
	PlanSHA256            string
	PolicySHA256          string
	OwnerCapabilitySHA256 string
	VaultEvidenceSHA256   string
}

func (r BootstrapRecoveryRequirement) Valid() bool {
	return r.Resource.Valid() && canonicalTransaction(r.PlanID) && digest(r.PlanSHA256) && digest(r.PolicySHA256) && digest(r.OwnerCapabilitySHA256) && digest(r.VaultEvidenceSHA256)
}

// BootstrapRecoveryPrecondition checks only recoverability for one resource.
// The whole product's SourceComplete must additionally establish ALL required
// resource/content receipts; this port cannot invoke it or retire any source.
type BootstrapRecoveryPrecondition interface {
	CheckBootstrapRecoveryReady(context.Context, BootstrapRecoveryRequirement) error
}

// BootstrapPolicyAuthorityEvidence is retained PUBLIC signed evidence obtained
// from the independently authenticated private vault. It is NOT a capability,
// epoch or permission. Restore still verifies its ORIGINAL current Source and
// roster before constructing any VerifiedCapability through the shared reader.
type BootstrapPolicyAuthorityEvidence struct {
	anchor                                    Anchor
	binding                                   SessionBinding
	planID, planHash, capabilityHash, rawHash string
	capability                                CapabilityEvidence
	records                                   map[string]json.RawMessage
}

func (e BootstrapPolicyAuthorityEvidence) Requirement() BootstrapRecoveryRequirement {
	return BootstrapRecoveryRequirement{Resource: e.anchor.Resource, PlanID: e.planID, PlanSHA256: e.planHash, PolicySHA256: e.anchor.PolicySHA256, OwnerCapabilitySHA256: e.capabilityHash, VaultEvidenceSHA256: e.rawHash}
}

type BootstrapPolicyAuthoritySource interface {
	BootstrapPolicyAuthority(context.Context, Resource, string) (BootstrapPolicyAuthorityEvidence, error)
}

// CheckBootstrapOwnerCapabilityIntent validates an explicit original owner
// intent before SDK signing. It never infers a grant from a role/roster or
// creates a VerifiedCapability without that original device's actual signature.
func CheckBootstrapOwnerCapabilityIntent(ctx context.Context, value Capability, activation VerifiedBootstrapActivation) error {
	if err := activation.Revalidate(ctx); err != nil {
		return err
	}
	selected, err := selectCapabilityMember(value, activation.epoch.value, activation.epoch.hash, activation.epoch.roster.members)
	b := activation.plan.SessionBinding()
	if err != nil || value.Issuer != activation.Plan().Creator || selected.Scope != b.Scope || selected.UserID != b.UserID || selected.RootKey != b.RootKey || selected.ProductKeyID != b.ProductKeyID || selected.EnrollmentSHA256 != b.EnrollmentSHA256 || !value.Allows(Read) || !value.Allows(Manage) {
		return ErrInvalid
	}
	return activation.Revalidate(ctx)
}

func VerifyBootstrapOwnerCapability(ctx context.Context, signed SignedCapability, activation VerifiedBootstrapActivation) (VerifiedCapability, error) {
	if err := CheckBootstrapOwnerCapabilityIntent(ctx, signed.Capability, activation); err != nil {
		return VerifiedCapability{}, err
	}
	verified, err := VerifyCapability(ctx, signed, activation.epoch, activation.plan.session.Device())
	if err != nil {
		return VerifiedCapability{}, err
	}
	if err := activation.Revalidate(ctx); err != nil {
		return VerifiedCapability{}, err
	}
	return verified, nil
}

type bootstrapPublicRecords struct {
	key    Resource
	values map[string]json.RawMessage
}

func (s bootstrapPublicRecords) PublicEvidence(ctx context.Context, key Resource, hash string) (json.RawMessage, error) {
	if key != s.key || s.values[hash] == nil {
		return nil, ErrInvalid
	}
	return bytes.Clone(s.values[hash]), ctx.Err()
}

func collectCapabilityRecords(items []EvidenceRecord) (map[string]json.RawMessage, error) {
	if len(items) == 0 {
		return nil, ErrInvalid
	}
	records := map[string]json.RawMessage{}
	previous := ""
	for _, record := range items {
		sha, err := CipherJSONDigest(record.Value)
		canonical, canonicalErr := matrixproof.CanonicalBytes(record.Value)
		if err != nil || canonicalErr != nil || sha != record.SHA256 || record.SHA256 <= previous || !bytes.Equal(canonical, record.Value) {
			return nil, ErrInvalid
		}
		records[sha] = bytes.Clone(record.Value)
		previous = sha
	}
	return records, nil
}

func validPolicyOwnerCapability(capability Capability, epoch SignedEpoch, value bootstrapPolicyAnchorSecret, key Resource, sha string) bool {
	b := value.OriginalBinding
	return sha == value.OwnerCapabilitySHA256 && capability.Resource == key && capability.RoomID == epoch.RoomID && capability.Epoch == epoch.ID && capability.Generation == epoch.Generation && capability.PolicySHA256 == value.Anchor.PolicySHA256 && capability.MembershipSHA256 == epoch.MembershipSHA256 && capability.Subject == b.Scope.Subject && capability.UserID == b.UserID && capability.ProductKeyID == b.ProductKeyID && capability.Issuer == b.Actor() && capability.Allows(Read) && capability.Allows(Manage)
}

// DecodeBootstrapPolicyAuthority is ONLY a private-vault codec. Its caller must
// authenticate the SDK2 vault first; request/Source JSON is not that trust port.
// It verifies the original signatures and exact bindings but returns evidence,
// never VerifiedCapability. Source membership/current authority remain required.
func DecodeBootstrapPolicyAuthority(ctx context.Context, raw []byte, key Resource, planID string, current SessionBinding) (BootstrapPolicyAuthorityEvidence, error) {
	value, err := decodeBootstrapPolicyAnchorSecret(ctx, raw, key, planID, current)
	if err != nil {
		return BootstrapPolicyAuthorityEvidence{}, err
	}
	if !digest(value.OwnerCapabilitySHA256) || value.OwnerCapability.Capability.Signer.Pin != value.OwnerPin {
		return BootstrapPolicyAuthorityEvidence{}, ErrInvalid
	}
	records, err := collectCapabilityRecords(value.CapabilityRecords)
	if err != nil {
		return BootstrapPolicyAuthorityEvidence{}, err
	}
	r := reservationReader{source: bootstrapPublicRecords{key: key, values: records}, resource: key, cache: map[string]json.RawMessage{}}
	var epoch SignedEpoch
	if err := decodeReservationValue(value.SignedEpoch, &epoch); err != nil {
		return BootstrapPolicyAuthorityEvidence{}, err
	}
	var signed SignedCapability
	if _, err := r.statement(ctx, value.OwnerCapability.Capability, &signed, value.Anchor.Owner, value.Anchor.OwnerScope, epoch.RoomID); err != nil {
		return BootstrapPolicyAuthorityEvidence{}, err
	}
	capability := signed.Capability
	b := value.OriginalBinding
	sha, err := CapabilityDigest(capability)
	if err != nil || !validPolicyOwnerCapability(capability, epoch, value, key, sha) {
		return BootstrapPolicyAuthorityEvidence{}, ErrInvalid
	}
	member := Member{Scope: b.Scope, UserID: b.UserID, RootKey: b.RootKey, Recipient: b.Recipient, ProductKeyID: b.ProductKeyID, EnrollmentSHA256: b.EnrollmentSHA256, Role: capability.Role, Actions: capability.Actions, From: capability.From}
	if _, err := r.enrollment(ctx, value.OwnerCapability.Recipient, member, epoch.RoomID); err != nil {
		return BootstrapPolicyAuthorityEvidence{}, err
	}
	issuer, err := r.enrollment(ctx, value.OwnerCapability.IssuerEnrollment, member, epoch.RoomID)
	if err != nil || issuer.Issuer != b.Actor() {
		return BootstrapPolicyAuthorityEvidence{}, ErrInvalid
	}
	rawSHA, err := CipherJSONDigest(raw)
	if err != nil {
		return BootstrapPolicyAuthorityEvidence{}, err
	}
	return BootstrapPolicyAuthorityEvidence{anchor: value.Anchor, binding: b, planID: planID, planHash: value.PlanSHA256, capabilityHash: sha, rawHash: rawSHA, capability: value.OwnerCapability, records: records}, ctx.Err()
}

type bootstrapVaultEvidenceSource struct {
	original TrustedBootstrapPolicySource
	evidence BootstrapPolicyAuthorityEvidence
}

func (s bootstrapVaultEvidenceSource) RetainedBootstrapPolicy(ctx context.Context, key Resource, plan string) (RetainedBootstrapPolicy, error) {
	return s.original.RetainedBootstrapPolicy(ctx, key, plan)
}

func (s bootstrapVaultEvidenceSource) PublicEvidence(ctx context.Context, key Resource, hash string) (json.RawMessage, error) {
	if raw, ok := s.evidence.records[hash]; ok {
		if key != s.evidence.anchor.Resource {
			return nil, ErrRevoked
		}
		if _, err := s.original.RetainedBootstrapPolicy(ctx, key, s.evidence.planID); err != nil {
			return nil, err
		}
		return bytes.Clone(raw), ctx.Err()
	}
	return s.original.PublicEvidence(ctx, key, hash)
}

type bootstrapEvidenceAnchor struct {
	evidence BootstrapPolicyAuthorityEvidence
}

func (s bootstrapEvidenceAnchor) BootstrapPolicyAnchor(ctx context.Context, key Resource, plan string) (Anchor, error) {
	if key != s.evidence.anchor.Resource || plan != s.evidence.planID || !s.evidence.Requirement().Valid() {
		return Anchor{}, ErrInvalid
	}
	return s.evidence.anchor, ctx.Err()
}

// RestoreBootstrapPolicyFromVault needs no SQL capability/normal operation.
// The ORIGINAL signed owner grant is fetched from authenticated SDK2 alongside
// its independent pin, then the same historical Source/roster verifier is used.
func RestoreBootstrapPolicyFromVault(ctx context.Context, key Resource, planID string, current VerifiedSession, vault BootstrapPolicyAuthoritySource, source TrustedBootstrapPolicySource) (RestoredBootstrapPolicy, error) {
	if vault == nil || source == nil {
		return RestoredBootstrapPolicy{}, ErrInvalid
	}
	if err := current.Revalidate(ctx); err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	evidence, err := vault.BootstrapPolicyAuthority(ctx, key, planID)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	b := evidence.binding
	c := current.Binding()
	if !evidence.Requirement().Valid() || evidence.anchor.Resource != key || evidence.planID != planID || b.Scope != c.Scope || b.UserID != c.UserID || b.RootKey != c.RootKey || b.RootGeneration != c.RootGeneration {
		return RestoredBootstrapPolicy{}, ErrRevoked
	}
	wrapped := bootstrapVaultEvidenceSource{original: source, evidence: evidence}
	policy, err := RestoreBootstrapPolicy(ctx, key, planID, evidence.capability, current, bootstrapEvidenceAnchor{evidence}, wrapped)
	if err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	if _, err := policy.RecoveryRequirement(ctx, evidence); err != nil {
		return RestoredBootstrapPolicy{}, err
	}
	return policy, nil
}

func (p RestoredBootstrapPolicy) RecoveryRequirement(ctx context.Context, evidence BootstrapPolicyAuthorityEvidence) (BootstrapRecoveryRequirement, error) {
	if _, err := p.Access(ctx, Manage); err != nil {
		return BootstrapRecoveryRequirement{}, err
	}
	planSHA, err := BootstrapPlanSHA256(p.plan)
	requirement := evidence.Requirement()
	if err != nil || !requirement.Valid() || requirement.Resource != p.plan.Source.Resource || requirement.PlanID != p.plan.ID || requirement.PlanSHA256 != planSHA || requirement.PolicySHA256 != p.epoch.hash || requirement.OwnerCapabilitySHA256 != p.capability.hash {
		return BootstrapRecoveryRequirement{}, ErrInvalid
	}
	return requirement, nil
}
