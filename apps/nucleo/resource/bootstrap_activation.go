// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
)

// VerifiedBootstrapActivation contains both independently fetched SDK ACKs and
// the owner's final signed epoch. Activating SQL routing never proves that the
// client's original content copy is complete and never retires the source.
type VerifiedBootstrapActivation struct {
	plan                 VerifiedBootstrapPlan
	epoch                VerifiedEpoch
	anchor               Anchor
	baseline, checkpoint VerifiedBootstrapAcceptance
}

func (a VerifiedBootstrapActivation) Epoch() VerifiedEpoch { return a.epoch }
func (a VerifiedBootstrapActivation) Anchor() Anchor       { return a.anchor }
func (a VerifiedBootstrapActivation) PlanSHA256() string   { return a.plan.SHA256() }
func (a VerifiedBootstrapActivation) Revalidate(ctx context.Context) error {
	if !a.plan.valid() || !a.epoch.valid() || !a.baseline.valid() || !a.checkpoint.valid() {
		return ErrRevoked
	}
	return a.plan.Revalidate(ctx)
}

func bootstrapEpochMatchesPlan(signed SignedEpoch, p BootstrapPlan, b Baseline, cp Checkpoint) bool {
	return cp.SourceCutover == b.Position && signed.Resource == p.Source.Resource && signed.RoomID == p.RoomID && signed.ID == p.Epoch && signed.Generation == "1" && signed.Recipient == p.Recipient && signed.MembershipSHA256 == p.MembershipSHA256 && signed.History == p.History && signed.Validation == p.Validation && signed.Issuer == p.Creator && signed.Baseline == b && signed.Checkpoint == cp && signed.Cutover == b.Position && b.Position.Seq == p.Source.SourceSeq
}

func VerifyBootstrapActivation(ctx context.Context, signed SignedEpoch, plan VerifiedBootstrapPlan, baseline, checkpoint VerifiedBootstrapAcceptance) (VerifiedBootstrapActivation, error) {
	if !plan.valid() || !baseline.valid() || !checkpoint.valid() || baseline.planHash != plan.hash || checkpoint.planHash != plan.hash || baseline.kind != BootstrapBaseline || checkpoint.kind != BootstrapCheckpoint || baseline.reference == checkpoint.reference {
		return VerifiedBootstrapActivation{}, ErrInvalid
	}
	if err := plan.Revalidate(ctx); err != nil {
		return VerifiedBootstrapActivation{}, err
	}
	b, err := baseline.Baseline()
	if err != nil {
		return VerifiedBootstrapActivation{}, err
	}
	cp, err := checkpoint.Checkpoint()
	if err != nil {
		return VerifiedBootstrapActivation{}, err
	}
	p := plan.value
	if !bootstrapEpochMatchesPlan(signed, p, b, cp) {
		return VerifiedBootstrapActivation{}, ErrInvalid
	}
	hash, err := EpochDigest(signed.Epoch)
	if err != nil {
		return VerifiedBootstrapActivation{}, err
	}
	anchor := Anchor{Resource: p.Source.Resource, PolicySHA256: hash, OwnerScope: p.CreatorScope, Owner: p.Creator, Baseline: b.Position}
	epoch, err := VerifyInitialEpoch(ctx, signed, plan.roster, plan.session.Device(), anchor)
	if err != nil {
		return VerifiedBootstrapActivation{}, err
	}
	if err := plan.Revalidate(ctx); err != nil {
		return VerifiedBootstrapActivation{}, err
	}
	return VerifiedBootstrapActivation{plan: plan, epoch: epoch, anchor: anchor, baseline: baseline, checkpoint: checkpoint}, nil
}

func (a VerifiedBootstrapActivation) Plan() BootstrapPlan      { return a.plan.Value() }
func (a VerifiedBootstrapActivation) PublicEpochBytes() []byte { return bytes.Clone(a.epoch.proof) }

// RequireCurrentRosterCapture separates live backend activation from historical
// proof verification, which must remain possible after producer retirement.
func (a VerifiedBootstrapActivation) RequireCurrentRosterCapture(ctx context.Context) error {
	if err := a.Revalidate(ctx); err != nil {
		return err
	}
	return a.plan.RequireCurrentRosterCapture(ctx)
}
