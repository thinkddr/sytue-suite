// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

// BootstrapPeerMetadata verifies IMMUTABLE signed statements only. It contains
// no live provider, permission, epoch/key, routing or send method. A peer read
// must separately resolve CURRENT owner/B and original SQL at entry AND exit.
type BootstrapPeerMetadata struct {
	plan                     SignedBootstrapPlan
	manifest                 SignedBootstrapRosterManifest
	planProof, manifestProof []byte
	hash                     string
	binding                  SessionBinding
	ownerPin                 matrixproof.Identity
	scope                    storage.Scope
	device                   string
}

func verifyBootstrapPeerMetadata(ctx context.Context, plan SignedBootstrapPlan, manifest SignedBootstrapRosterManifest, current VerifiedSession) (BootstrapPeerMetadata, error) {
	var zero BootstrapPeerMetadata
	if !manifest.ValidFor(plan.BootstrapPlan) {
		return zero, ErrInvalid
	}
	_, proof, err := verifyBootstrapOwnerMetadata(ctx, plan, current)
	if err != nil {
		return zero, err
	}
	if err := current.Device().VerifyJSON(ctx, plan.CreatorScope, storage.PublicationTarget{RoomID: plan.RoomID, DeviceID: plan.Creator.DeviceID}, manifest); err != nil {
		return zero, err
	}
	raw, err := matrixproof.CanonicalBytes(manifest)
	if err != nil {
		return zero, err
	}
	sha, err := CipherJSONDigest(raw)
	if err != nil {
		return zero, err
	}
	var retainedPlan SignedBootstrapPlan
	var retainedManifest SignedBootstrapRosterManifest
	if json.Unmarshal(proof, &retainedPlan) != nil || json.Unmarshal(raw, &retainedManifest) != nil {
		return zero, ErrInvalid
	}
	return BootstrapPeerMetadata{plan: retainedPlan, manifest: retainedManifest, planProof: proof, manifestProof: raw, hash: sha, binding: current.Binding(), ownerPin: current.Device().Identity()}, nil
}

func VerifyBootstrapPeerMetadata(ctx context.Context, query BootstrapPeerQuery, current VerifiedSession) (BootstrapPeerMetadata, error) {
	if !query.Valid() {
		return BootstrapPeerMetadata{}, ErrInvalid
	}
	out, err := verifyBootstrapPeerMetadata(ctx, query.Plan, query.Manifest, current)
	if err != nil {
		return BootstrapPeerMetadata{}, err
	}
	out.scope, out.device = query.Scope, query.DeviceID
	return out, nil
}

func (m BootstrapPeerMetadata) Value() BootstrapRosterManifest {
	return m.manifest.BootstrapRosterManifest
}
func (m BootstrapPeerMetadata) Plan() BootstrapPlan            { return m.plan.BootstrapPlan }
func (m BootstrapPeerMetadata) SHA256() string                 { return m.hash }
func (m BootstrapPeerMetadata) PublicPlanBytes() []byte        { return bytes.Clone(m.planProof) }
func (m BootstrapPeerMetadata) PublicManifestBytes() []byte    { return bytes.Clone(m.manifestProof) }
func (m BootstrapPeerMetadata) SessionBinding() SessionBinding { return m.binding }
func (m BootstrapPeerMetadata) Scope() storage.Scope           { return m.scope }
func (m BootstrapPeerMetadata) DeviceID() string               { return m.device }
func (m BootstrapPeerMetadata) valid() bool {
	return digest(m.hash) && m.manifest.ValidFor(m.plan.BootstrapPlan) && len(m.planProof) > 0 && len(m.manifestProof) > 0 && m.binding.Valid()
}

// SelectPeer performs a full globally ordered stream/membership check. Current
// checks deliberately belong to the encompassing operation, never each page.
func (m BootstrapPeerMetadata) SelectPeer(ctx context.Context, pages BootstrapRosterPageSource) (BootstrapRosterInput, error) {
	if !m.valid() {
		return BootstrapRosterInput{}, ErrInvalid
	}
	return selectBootstrapRosterPeer(ctx, m.Plan(), m.Value(), pages, m.scope, m.device)
}

// CheckOwnerSnapshot/CheckPeerSnapshot compare already verified snapshots;
// neither performs a live check or grants authority. The operation must obtain
// its PRE/POST snapshots freshly, and must not retain its witness after return.
func (m BootstrapPeerMetadata) CheckOwnerSnapshot(current VerifiedSession) error {
	if !m.valid() || !current.valid() || current.Binding() != m.binding || current.Device().Identity() != m.ownerPin {
		return ErrRevoked
	}
	return nil
}

func (m BootstrapPeerMetadata) CheckPeerSnapshot(input BootstrapRosterInput, current VerifiedSession) error {
	if !m.valid() || input.Member.Scope != m.scope || current.Scope() != m.scope || current.Actor().DeviceID != m.device {
		return ErrRevoked
	}
	return checkBootstrapPeerSnapshot(input, current)
}
