// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"bytes"
	"context"
	"encoding/json"

	"maunium.net/go/mautrix/crypto/signatures"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/pqc"
)

// BootstrapSourceSnapshot is read from the ORIGINAL frozen application source.
// SourceEpoch and ACLVersion are public lifecycle versions, never content hashes.
// This first archive format requires the ORIGINAL source to authorize full
// history. Joined/explicit legacy histories require separate historical archive
// grants; they must not be exposed under the current-epoch recipient.
// Metadata selected by a client is not an OriginalBootstrapSource implementation.
type BootstrapSourceSnapshot struct {
	Resource    Resource      `json:"resource"`
	OwnerScope  storage.Scope `json:"owner_scope"`
	SourceEpoch string        `json:"source_epoch"`
	History     HistoryPolicy `json:"history"`
	SourceSeq   Decimal       `json:"source_seq"`
	ACLVersion  Decimal       `json:"acl_version"`
}

func (s BootstrapSourceSnapshot) Valid() bool {
	_, seqErr := s.SourceSeq.Int64()
	_, aclErr := s.ACLVersion.Int64()
	return s.Resource.Valid() && s.OwnerScope.Valid() && s.OwnerScope.Tenant == s.Resource.Tenant && s.OwnerScope.Product == s.Resource.Product && canonicalTransaction(s.SourceEpoch) && s.History == HistoryFull && seqErr == nil && aclErr == nil
}

type OriginalBootstrapSource interface {
	BootstrapSource(context.Context, Resource) (BootstrapSourceSnapshot, error)
	CheckBootstrapSource(context.Context, BootstrapSourceSnapshot) error
}

// BootstrapPlan precedes the first ciphertext and has no invented Epoch policy
// or accepted references. Its source, roster, recipient and actual producer are
// immutable while baseline/checkpoint publications are prepared and reconciled.
type BootstrapPlan struct {
	Version              int                     `json:"v"`
	Domain               string                  `json:"domain"`
	ID                   string                  `json:"id"`
	Source               BootstrapSourceSnapshot `json:"source"`
	RoomID               string                  `json:"room_id"`
	Epoch                string                  `json:"epoch"`
	Recipient            string                  `json:"recipient"`
	MembershipSHA256     string                  `json:"membership_sha256"`
	History              HistoryPolicy           `json:"history"`
	Validation           ValidationPolicy        `json:"validation"`
	CreatorScope         storage.Scope           `json:"creator_scope"`
	Creator              Actor                   `json:"creator"`
	SessionBindingSHA256 string                  `json:"session_binding_sha256"`
}
type SignedBootstrapPlan struct {
	BootstrapPlan
	Signatures signatures.Signatures `json:"signatures"`
}

func (p BootstrapPlan) Valid() bool {
	_, err := pqc.ParseRecipient(pqc.HybridProfile, p.Recipient)
	return p.Version == 1 && p.Domain == "org.sytue.resource.bootstrap.plan.v1" && canonicalTransaction(p.ID) && p.Source.Valid() && matrixID(p.RoomID, '!') && storage.Identifier(p.Epoch) && err == nil && digest(p.MembershipSHA256) && p.History.Valid() && p.Validation.Valid() && p.History == p.Source.History && p.CreatorScope == p.Source.OwnerScope && p.Creator.Valid() && digest(p.SessionBindingSHA256)
}

func BootstrapPlanSHA256(plan BootstrapPlan) (string, error) {
	if !plan.Valid() {
		return "", ErrInvalid
	}
	return matrixproof.RequestDigest(plan)
}

// VerifiedBootstrapPlan is granted only by current product enrollment AND the
// original application's frozen owner/version. It grants neither a capability
// to a new member nor access to any private owner vault or historical epoch key.
type VerifiedBootstrapPlan struct {
	value             BootstrapPlan
	proof             json.RawMessage
	hash              string
	session           VerifiedSession
	roster            VerifiedRoster
	source            OriginalBootstrapSource
	currentRecipients []VerifiedSession
}

func VerifyBootstrapPlan(ctx context.Context, signed SignedBootstrapPlan, roster VerifiedRoster, current VerifiedSession, source OriginalBootstrapSource) (VerifiedBootstrapPlan, error) {
	value := signed.BootstrapPlan
	if !roster.valid() || roster.resource != value.Source.Resource || roster.hash != value.MembershipSHA256 || !roster.issuer(value.CreatorScope, value.Creator) {
		return VerifiedBootstrapPlan{}, ErrInvalid
	}
	hash, proof, err := verifyBootstrapOwnerIntent(ctx, signed, current, source)
	if err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	out := VerifiedBootstrapPlan{value: value, proof: proof, hash: hash, session: current, roster: roster, source: source}
	if err := out.Revalidate(ctx); err != nil {
		return VerifiedBootstrapPlan{}, err
	}
	return out, nil
}
func (p VerifiedBootstrapPlan) Value() BootstrapPlan           { return p.value }
func (p VerifiedBootstrapPlan) PublicPlanBytes() []byte        { return bytes.Clone(p.proof) }
func (p VerifiedBootstrapPlan) SHA256() string                 { return p.hash }
func (p VerifiedBootstrapPlan) SessionBinding() SessionBinding { return p.session.Binding() }
func (p VerifiedBootstrapPlan) valid() bool {
	return p.source != nil && p.value.Valid() && digest(p.hash) && p.session.valid() && p.roster.valid() && p.roster.resource == p.value.Source.Resource && p.roster.hash == p.value.MembershipSHA256
}

func (p VerifiedBootstrapPlan) Revalidate(ctx context.Context) error {
	if !p.valid() {
		return ErrRevoked
	}
	if err := revalidateBootstrapOwnerIntent(ctx, p.value, p.session, p.source); err != nil {
		return err
	}
	for _, current := range p.currentRecipients {
		if err := current.Revalidate(ctx); err != nil {
			return err
		}
	}
	return p.session.Revalidate(ctx)
}

func (p VerifiedBootstrapPlan) BaselineContext() (BootstrapContext, error) {
	if !p.valid() {
		return BootstrapContext{}, ErrInvalid
	}
	return NewBootstrapContext(BootstrapBaseline, p.value.Source.Resource, p.value.RoomID, p.value.Epoch, p.value.Recipient, Position{Seq: p.value.Source.SourceSeq}, p.value.CreatorScope, p.value.Creator)
}
