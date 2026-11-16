// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// OriginHoldSource is fixed by the original product Relay, not a request URL,
// owner-vault selector or JSON trust anchor. It exposes only a CURRENT original
// producer's durable Sending/Ambiguous hold. Committed history uses a different
// read-only protocol and can never be relabelled as an active send hold.
type OriginHoldSource struct {
	source resourceHoldSource
	key    resource.Resource
	op     string
}

var _ resource.TrustedHoldSource = (*OriginHoldSource)(nil)

func (r *Relay) OriginHoldSource(ctx context.Context, key resource.Resource, op string) (*OriginHoldSource, error) {
	entry, err := r.Pending(ctx, key, op)
	if err != nil {
		return nil, err
	}
	_, access, err := r.access(ctx, key, entry.Candidate.Proof.Action)
	if err != nil {
		return nil, err
	}
	binding, ok := access.CurrentSessionBinding()
	if !ok || access.Actor() != entry.Candidate.Proof.Actor || binding.Scope.Subject != r.subject || binding.Scope.Product != key.Product {
		return nil, resource.ErrRevoked
	}
	source := &OriginHoldSource{source: resourceHoldSource{relay: r, access: access}, key: key, op: op}
	if err := source.Revalidate(ctx); err != nil {
		return nil, err
	}
	return source, nil
}

// Revalidate repeats product ACL/generation and the CURRENT Provider before
// and after external verification. Product ACL mutation must drain this same
// original hold; this is not a replacement for that durable fence.
func (s *OriginHoldSource) Revalidate(ctx context.Context) error {
	if s == nil || s.source.relay == nil {
		return resource.ErrRevoked
	}
	_, access, err := s.source.relay.access(ctx, s.key, s.source.access.Action())
	if err != nil {
		return err
	}
	old, ok := s.source.access.CurrentSessionBinding()
	current, active := access.CurrentSessionBinding()
	if !ok || !active || old != current || access.Actor() != s.source.access.Actor() || access.Epoch().SHA256() != s.source.access.Epoch().SHA256() || access.Capability().SHA256() != s.source.access.Capability().SHA256() {
		return resource.ErrRevoked
	}
	if err := access.RevalidateSession(ctx); err != nil {
		return err
	}
	held, err := s.source.HeldReservation(ctx, s.key, s.op)
	if err != nil {
		return err
	}
	if held.Candidate.Proof.Actor != access.Actor() {
		return resource.ErrRevoked
	}
	return ctx.Err()
}

func (s *OriginHoldSource) HeldReservation(ctx context.Context, key resource.Resource, op string) (resource.HeldReservation, error) {
	if s == nil || key != s.key || op != s.op {
		return resource.HeldReservation{}, resource.ErrRevoked
	}
	if err := s.Revalidate(ctx); err != nil {
		return resource.HeldReservation{}, err
	}
	return s.source.HeldReservation(ctx, key, op)
}

func (s *OriginHoldSource) PublicEvidence(ctx context.Context, key resource.Resource, hash string) (json.RawMessage, error) {
	if s == nil || key != s.key {
		return nil, resource.ErrRevoked
	}
	// The pinned source's normal RLS/head check applies to every evidence read.
	return s.source.PublicEvidence(ctx, key, hash)
}

func (s *OriginHoldSource) Restore(ctx context.Context) (resource.VerifiedReservation, error) {
	if err := s.Revalidate(ctx); err != nil {
		return resource.VerifiedReservation{}, err
	}
	reserved, err := resource.RestoreReservation(ctx, s.key, s.op, s)
	if err != nil {
		return resource.VerifiedReservation{}, err
	}
	if err := s.Revalidate(ctx); err != nil {
		return resource.VerifiedReservation{}, err
	}
	return reserved, nil
}

func (s *OriginHoldSource) VerifyRequest(ctx context.Context, current resource.VerifiedSession, signed publication.SignedRequest) (resource.VerifiedOriginRequest, error) {
	if s == nil {
		return resource.VerifiedOriginRequest{}, resource.ErrRevoked
	}
	binding, ok := s.source.access.CurrentSessionBinding()
	if !ok || current.Binding() != binding {
		return resource.VerifiedOriginRequest{}, resource.ErrRevoked
	}
	reserved, err := s.Restore(ctx)
	if err != nil {
		return resource.VerifiedOriginRequest{}, err
	}
	verified, err := resource.VerifyOriginRequest(ctx, reserved, current, signed)
	if err != nil {
		return resource.VerifiedOriginRequest{}, err
	}
	if err := s.Revalidate(ctx); err != nil {
		return resource.VerifiedOriginRequest{}, err
	}
	return verified, nil
}
