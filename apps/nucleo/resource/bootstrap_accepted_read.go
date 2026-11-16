// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/matrix/sessionproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// VerifiedBootstrapRead permits only the exact ORIGINAL accepted descriptor and
// the CURRENT owner's reader. It contains no live producer or publication hold.
// Source's accepted state alone is not an SDK readback or a work ACK.
type VerifiedBootstrapRead struct {
	value         BootstrapPublication
	candidateHash string
	reference     storage.Reference
	context       BootstrapContext
	session       sessionproof.Session
	reader        VerifiedAccess
	binding       SessionBinding
}

func (p VerifiedBootstrapRead) Publication() BootstrapPublication { return p.value }
func (p VerifiedBootstrapRead) CandidateSHA256() string           { return p.candidateHash }
func (p VerifiedBootstrapRead) Reference() storage.Reference      { return p.reference }
func (p VerifiedBootstrapRead) Context() BootstrapContext         { return p.context }
func (p VerifiedBootstrapRead) Session() sessionproof.Session     { return p.session }

func (p VerifiedBootstrapRead) valid() bool {
	return p.value.Valid() && digest(p.candidateHash) && p.reference.Valid() && p.context.Valid() && p.value.Kind == p.context.Kind && p.reference.RoomID == p.context.RoomID && p.reader.valid() && p.reader.epoch.restored != nil && p.reader.action == Read && p.binding.Valid() && p.value.Actor == p.context.Creator && p.reader.epoch.value.Resource == p.context.Resource && p.reader.epoch.value.ID == p.context.Epoch
}

func (p VerifiedBootstrapRead) RevalidateReader(ctx context.Context, reader VerifiedAccess, fence Fence) error {
	binding, ok := reader.CurrentSessionBinding()
	if !p.valid() || !ok || binding != p.binding || reader.action != Read || reader.epoch.hash != p.reader.epoch.hash || reader.capability.hash != p.reader.capability.hash || reader.epoch.restored != p.reader.epoch.restored {
		return ErrRevoked
	}
	return checkFence(ctx, fence, reader)
}

func (p RestoredBootstrapPolicy) selectAcceptedSlot(retained RetainedBootstrapPolicy, kind BootstrapKind) (RetainedBootstrapSlot, Position, error) {
	slot := retained.Baseline
	position := Position{Seq: p.plan.Source.SourceSeq}
	switch kind {
	case BootstrapBaseline:
	case BootstrapCheckpoint:
		slot, position = retained.Checkpoint, p.epoch.value.Cutover
	default:
		return RetainedBootstrapSlot{}, Position{}, ErrInvalid
	}
	if slot.Kind != kind || slot.State != "accepted" {
		return RetainedBootstrapSlot{}, Position{}, ErrPending
	}
	return slot, position, nil
}

// Accepted reads the SAME independently retained active genesis and both exact
// slots again. A complete capture, pending/sending slot, changed candidate or
// current session can never manufacture this proof from public JSON.
func (p RestoredBootstrapPolicy) Accepted(ctx context.Context, kind BootstrapKind, reader VerifiedAccess, fence Fence) (VerifiedBootstrapRead, error) {
	binding, ok := reader.CurrentSessionBinding()
	if p.epoch.restored == nil || !ok || binding != p.current.Binding() || reader.action != Read || reader.epoch.restored != p.epoch.restored || reader.epoch.hash != p.epoch.hash || reader.capability.hash != p.capability.hash {
		return VerifiedBootstrapRead{}, ErrRevoked
	}
	if err := checkFence(ctx, fence, reader); err != nil {
		return VerifiedBootstrapRead{}, err
	}
	authority := p.epoch.restored
	retained, err := authority.source.RetainedBootstrapPolicy(ctx, authority.key, authority.planID)
	if err != nil {
		return VerifiedBootstrapRead{}, err
	}
	sha, err := bootstrapPolicySnapshotSHA(retained)
	if err != nil || sha != authority.retainedSHA {
		return VerifiedBootstrapRead{}, ErrRevoked
	}
	slot, position, err := p.selectAcceptedSlot(retained, kind)
	if err != nil {
		return VerifiedBootstrapRead{}, err
	}
	floor, _ := reader.capability.value.From.Seq.Int64()
	cutover, _ := p.plan.Source.SourceSeq.Int64()
	if floor > cutover || floor != 0 && floor == cutover && reader.capability.value.From != p.epoch.value.Baseline.Position {
		return VerifiedBootstrapRead{}, ErrRevoked
	}
	contextValue, err := NewBootstrapContext(kind, authority.key, p.plan.RoomID, p.plan.Epoch, p.plan.Recipient, position, p.plan.CreatorScope, p.plan.Creator)
	if err != nil {
		return VerifiedBootstrapRead{}, err
	}
	certificate, err := sessionproof.DecodeCertificate(slot.Candidate.SessionCertificate)
	if err != nil {
		return VerifiedBootstrapRead{}, err
	}
	proof := VerifiedBootstrapRead{value: slot.Candidate.Proof.BootstrapPublication, candidateHash: slot.CandidateSHA256, reference: slot.Reference, context: contextValue, session: certificate.Session, reader: reader, binding: binding}
	if err := proof.RevalidateReader(ctx, reader, fence); err != nil {
		return VerifiedBootstrapRead{}, err
	}
	return proof, nil
}

// OpenBootstrapFrame uses the existing hybrid bootstrap codec only after exact
// historical metadata and CURRENT B/Source checks. It is neither an SDK event
// receipt nor permission to retire original pending bootstrap work.
func (h *EpochHandle) OpenBootstrapFrame(ctx context.Context, proof VerifiedBootstrapRead, envelope pqc.Envelope, reader VerifiedAccess, fence Fence) ([]byte, error) {
	if err := proof.RevalidateReader(ctx, reader, fence); err != nil {
		return nil, err
	}
	identity := h.identity()
	if !h.usable(identity) || h.window.through != nil || h.window.epoch.hash != proof.reader.epoch.hash {
		return nil, ErrRevoked
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	hash, err := CipherJSONDigest(raw)
	clear(raw)
	if err != nil || hash != proof.value.InnerCipherSHA256 {
		return nil, ErrInvalid
	}
	payload, err := OpenBootstrapDescriptor(identity, proof.context, envelope)
	if err != nil {
		return nil, err
	}
	if err := proof.RevalidateReader(ctx, reader, fence); err != nil {
		clear(payload)
		return nil, err
	}
	if !h.usable(identity) {
		clear(payload)
		return nil, ErrRevoked
	}
	return payload, nil
}
