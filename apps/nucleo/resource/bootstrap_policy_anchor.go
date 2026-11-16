// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

type bootstrapPolicyAnchorSecret struct {
	Version                   int                        `json:"v"`
	Domain                    string                     `json:"domain"`
	PlanID                    string                     `json:"plan_id"`
	PlanSHA256                string                     `json:"plan_sha256"`
	Anchor                    Anchor                     `json:"anchor"`
	OriginalBinding           SessionBinding             `json:"original_binding"`
	OwnerPin                  matrixproof.Identity       `json:"owner_pin"`
	OwnerEvidence             matrixproof.DeviceEvidence `json:"owner_evidence"`
	SignedPlan                json.RawMessage            `json:"signed_plan"`
	SignedEpoch               json.RawMessage            `json:"signed_epoch"`
	BaselineCandidateSHA256   string                     `json:"baseline_candidate_sha256"`
	CheckpointCandidateSHA256 string                     `json:"checkpoint_candidate_sha256"`
	OwnerCapabilitySHA256     string                     `json:"owner_capability_sha256"`
	OwnerCapability           CapabilityEvidence         `json:"owner_capability"`
	CapabilityRecords         []EvidenceRecord           `json:"capability_records"`
}

// BootstrapPolicyAnchorSecretID addresses the FINAL policy separately from the
// pre-publication plan/key secret. A missing final secret requires recovery;
// it must never fall back to a server-selected anchor or overwrite the key.
func BootstrapPolicyAnchorSecretID(key Resource, planID string) (string, error) {
	if !key.Valid() || !canonicalTransaction(planID) {
		return "", ErrInvalid
	}
	return "resource_bootstrap_policy_" + planID, nil
}

// EncodeBootstrapPolicyAnchor can only capture an already locally verified
// owner-signed genesis with BOTH independently confirmed SDK acceptances. The
// returned bytes belong only in that owner's authenticated private product
// vault; they are an independent pin, never public Source evidence.
func EncodeBootstrapPolicyAnchor(ctx context.Context, activation VerifiedBootstrapActivation, signedOwner SignedCapability) ([]byte, error) {
	if err := activation.Revalidate(ctx); err != nil {
		return nil, err
	}
	if !activation.baseline.valid() || !activation.checkpoint.valid() || activation.baseline.kind != BootstrapBaseline || activation.checkpoint.kind != BootstrapCheckpoint || activation.baseline.planHash != activation.PlanSHA256() || activation.checkpoint.planHash != activation.PlanSHA256() {
		return nil, ErrInvalid
	}
	owner, err := VerifyBootstrapOwnerCapability(ctx, signedOwner, activation)
	if err != nil {
		return nil, err
	}
	records := recordCollector{}
	capability, err := records.capability(owner, activation.epoch)
	if err != nil {
		return nil, err
	}
	evidence, err := activation.epoch.device.PublicEvidence()
	if err != nil {
		return nil, err
	}
	value := bootstrapPolicyAnchorSecret{Version: 1, Domain: "org.sytue.resource.bootstrap.policy-anchor.v1", PlanID: activation.Plan().ID, PlanSHA256: activation.PlanSHA256(), Anchor: activation.Anchor(), OriginalBinding: activation.plan.SessionBinding(), OwnerPin: activation.epoch.device.Identity(), OwnerEvidence: evidence, SignedPlan: activation.plan.PublicPlanBytes(), SignedEpoch: activation.PublicEpochBytes(), BaselineCandidateSHA256: activation.baseline.candidateHash, CheckpointCandidateSHA256: activation.checkpoint.candidateHash}
	value.OwnerCapabilitySHA256, value.OwnerCapability, value.CapabilityRecords = owner.SHA256(), capability, records.records()
	raw, err := matrixproof.CanonicalBytes(value)
	if err != nil {
		return nil, err
	}
	if len(raw) > storage.MaxRecordBytes {
		clear(raw)
		return nil, ErrInvalid
	}
	if err := activation.Revalidate(ctx); err != nil {
		clear(raw)
		return nil, err
	}
	return raw, nil
}

// DecodeBootstrapPolicyAnchor is ONLY the strict private-vault codec. The
// caller must obtain raw from authenticated SDK SSSS in the same owner/product;
// JSON from a public Source/directory is never an independent anchor. The live
// current SessionProvider must validate current separately before/after vault IO.
func DecodeBootstrapPolicyAnchor(ctx context.Context, raw []byte, key Resource, planID string, current SessionBinding) (Anchor, error) {
	evidence, err := DecodeBootstrapPolicyAuthority(ctx, raw, key, planID, current)
	if err != nil {
		return Anchor{}, err
	}
	return evidence.anchor, nil
}

func validPolicyAnchorEnvelope(value bootstrapPolicyAnchorSecret, key Resource, planID string, current SessionBinding) bool {
	b := value.OriginalBinding
	a := value.Anchor
	return value.Version == 1 && value.Domain == "org.sytue.resource.bootstrap.policy-anchor.v1" && value.PlanID == planID && digest(value.PlanSHA256) && digest(value.BaselineCandidateSHA256) && digest(value.CheckpointCandidateSHA256) && b.Valid() && a.Resource == key && a.OwnerScope == current.Scope && b.Scope == current.Scope && b.UserID == current.UserID && b.RootKey == current.RootKey && b.RootGeneration == current.RootGeneration && b.Actor() == a.Owner && identityMatches(value.OwnerPin, a.Owner, a.OwnerScope)
}

func validPolicyAnchorPlan(plan SignedBootstrapPlan, signed SignedEpoch, value bootstrapPolicyAnchorSecret, key Resource, planID string) bool {
	b := value.OriginalBinding
	planSHA, err := BootstrapPlanSHA256(plan.BootstrapPlan)
	bindingSHA, bindingErr := matrixproof.RequestDigest(b)
	return err == nil && bindingErr == nil && planSHA == value.PlanSHA256 && plan.ID == planID && plan.Source.Resource == key && plan.CreatorScope == b.Scope && plan.Creator == b.Actor() && plan.SessionBindingSHA256 == bindingSHA && plan.RoomID == signed.RoomID && plan.Epoch == signed.ID && plan.Recipient == signed.Recipient && plan.MembershipSHA256 == signed.MembershipSHA256 && plan.History == signed.History && plan.Validation == signed.Validation && plan.Source.SourceSeq == signed.Baseline.Position.Seq
}

func decodeBootstrapPolicyAnchorSecret(ctx context.Context, raw []byte, key Resource, planID string, current SessionBinding) (bootstrapPolicyAnchorSecret, error) {
	if len(raw) == 0 || len(raw) > storage.MaxRecordBytes || !key.Valid() || !canonicalTransaction(planID) || !current.Valid() {
		return bootstrapPolicyAnchorSecret{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	var value bootstrapPolicyAnchorSecret
	if err := decodeReservationValue(raw, &value); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	canonical, err := matrixproof.CanonicalBytes(value)
	if err != nil || !bytes.Equal(canonical, raw) {
		return bootstrapPolicyAnchorSecret{}, ErrInvalid
	}
	defer clear(canonical)
	if !validPolicyAnchorEnvelope(value, key, planID, current) {
		return bootstrapPolicyAnchorSecret{}, ErrRevoked
	}
	a := value.Anchor
	var signed SignedEpoch
	if err := decodeReservationValue(value.SignedEpoch, &signed); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	if err := ValidateInitialBaseline(signed.Epoch, a); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	var plan SignedBootstrapPlan
	if err := decodeReservationValue(value.SignedPlan, &plan); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	if !validPolicyAnchorPlan(plan, signed, value, key, planID) {
		return bootstrapPolicyAnchorSecret{}, ErrInvalid
	}
	signer, err := matrixproof.VerifyHistoricalDevice(ctx, value.OwnerEvidence, value.OwnerPin)
	if err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	if err := signer.VerifyJSON(ctx, a.OwnerScope, storage.PublicationTarget{RoomID: signed.RoomID, DeviceID: a.Owner.DeviceID}, signed); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	if err := signer.VerifyJSON(ctx, a.OwnerScope, storage.PublicationTarget{RoomID: signed.RoomID, DeviceID: a.Owner.DeviceID}, plan); err != nil {
		return bootstrapPolicyAnchorSecret{}, err
	}
	return value, ctx.Err()
}
