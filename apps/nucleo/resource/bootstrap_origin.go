// SPDX-License-Identifier: Apache-2.0
package resource

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// BootstrapOriginHold contains independent columns from the ORIGINAL durable
// bootstrap slot. A current plan or client WAL alone cannot authorize the first
// network event: the source must already hold this exact Sending/Ambiguous slot.
// Prepared, accepted and finalized slots are never new publication authority.
type BootstrapOriginHold struct {
	Resource             Resource
	PlanID               string
	PlanSHA256           string
	Phase                string
	Source               BootstrapSourceSnapshot
	SessionBindingSHA256 string
	SessionGeneration    Decimal
	RoomID               string
	Actor                Actor
	Kind                 BootstrapKind
	State                string
	TransactionID        string
	CandidateSHA256      string
	ContextSHA256        string
	OuterCipherSHA256    string
}

type TrustedBootstrapOriginSource interface {
	BootstrapOriginHold(context.Context, Resource, string, BootstrapKind) (BootstrapOriginHold, error)
}

// VerifiedBootstrapOriginRequest is pre-policy origin authority. It cannot be
// used as an epoch, capability or receipt. In particular it does not expose an
// invented final policy before baseline and checkpoint have actually arrived.
type VerifiedBootstrapOriginRequest struct {
	candidate VerifiedBootstrapCandidate
	source    TrustedBootstrapOriginSource
	request   publication.VerifiedRequest
	binding   SessionBinding
}

func bootstrapOriginHoldMatches(c VerifiedBootstrapCandidate, h BootstrapOriginHold) bool {
	p := c.plan.value
	v := c.Publication()
	b := c.plan.SessionBinding()
	return h.Resource == p.Source.Resource && h.PlanID == p.ID && h.PlanSHA256 == c.plan.hash && h.Phase == "preparing" && h.Source == p.Source && h.SessionBindingSHA256 == p.SessionBindingSHA256 && h.SessionGeneration == b.Generation && h.RoomID == p.RoomID && h.Actor == p.Creator && h.Kind == v.Kind && (h.State == "sending" || h.State == "ambiguous") && h.TransactionID == v.TransactionID && h.CandidateSHA256 == c.hash && h.ContextSHA256 == v.ContextSHA256 && h.OuterCipherSHA256 == v.OuterCipherSHA256
}

func bootstrapOriginSignedMatches(candidate VerifiedBootstrapCandidate, current VerifiedSession, signed publication.SignedRequest) bool {
	plan := candidate.plan
	binding := current.Binding()
	p := candidate.Publication()
	generation, err := binding.Generation.Int64()
	return err == nil && generation >= 1 && binding == plan.SessionBinding() && current.Actor() == plan.value.Creator && signed.Scope == binding.Scope && signed.Namespace == binding.Namespace && signed.Namespace == "resources:"+plan.value.Source.Resource.Product && signed.Target.RoomID == plan.value.RoomID && signed.Target.DeviceID == p.Actor.DeviceID && signed.TransactionID == p.TransactionID && signed.CipherSHA256 == p.OuterCipherSHA256 && signed.AuthorityGeneration == generation
}

func VerifyBootstrapOriginRequest(ctx context.Context, candidate VerifiedBootstrapCandidate, current VerifiedSession, signed publication.SignedRequest, source TrustedBootstrapOriginSource) (VerifiedBootstrapOriginRequest, error) {
	if !candidate.valid() || !current.valid() || source == nil || !bootstrapOriginSignedMatches(candidate, current, signed) {
		return VerifiedBootstrapOriginRequest{}, ErrInvalid
	}
	plan := candidate.plan
	binding := current.Binding()
	p := candidate.Publication()
	if err := plan.Revalidate(ctx); err != nil {
		return VerifiedBootstrapOriginRequest{}, err
	}
	h, err := source.BootstrapOriginHold(ctx, plan.value.Source.Resource, plan.value.ID, p.Kind)
	if err != nil {
		return VerifiedBootstrapOriginRequest{}, err
	}
	if !bootstrapOriginHoldMatches(candidate, h) {
		return VerifiedBootstrapOriginRequest{}, ErrRevoked
	}
	if err := current.Revalidate(ctx); err != nil {
		return VerifiedBootstrapOriginRequest{}, err
	}
	verified, err := publication.VerifyRequest(ctx, signed, signed.Request, current.Device())
	if err != nil {
		return VerifiedBootstrapOriginRequest{}, err
	}
	out := VerifiedBootstrapOriginRequest{candidate: candidate, source: source, request: verified, binding: binding}
	if err := out.Revalidate(ctx); err != nil {
		return VerifiedBootstrapOriginRequest{}, err
	}
	return out, nil
}

func (o VerifiedBootstrapOriginRequest) Request() publication.VerifiedRequest { return o.request }
func (o VerifiedBootstrapOriginRequest) SessionBinding() SessionBinding       { return o.binding }
func (o VerifiedBootstrapOriginRequest) Plan() BootstrapPlan                  { return o.candidate.plan.Value() }

func (o VerifiedBootstrapOriginRequest) PlanSHA256() string { return o.candidate.plan.SHA256() }

func (o VerifiedBootstrapOriginRequest) CandidateSHA256() string { return o.candidate.SHA256() }

func (o VerifiedBootstrapOriginRequest) Kind() BootstrapKind { return o.candidate.Publication().Kind }

func (o VerifiedBootstrapOriginRequest) Revalidate(ctx context.Context) error {
	if !o.candidate.valid() || o.source == nil || !o.request.Value().Valid() || o.request.SHA256() == "" || o.binding != o.candidate.plan.SessionBinding() {
		return ErrRevoked
	}
	if err := o.candidate.plan.Revalidate(ctx); err != nil {
		return err
	}
	p := o.candidate.plan.value
	held, err := o.source.BootstrapOriginHold(ctx, p.Source.Resource, p.ID, o.Kind())
	if err != nil {
		return err
	}
	if !bootstrapOriginHoldMatches(o.candidate, held) {
		return ErrRevoked
	}
	return o.candidate.plan.Revalidate(ctx)
}
