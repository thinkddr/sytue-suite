// SPDX-License-Identifier: Apache-2.0
package webport

import (
	"context"
	"encoding/json"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type (
	publicationContext struct{}
	publicationProof   struct {
		owner  *Storage
		scope  storage.Scope
		target storage.PublicationTarget
		policy storage.PublicationPolicy
	}
)

func (s *Storage) RequireRecoverableStorage(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	if scope != s.identity.Scope || !scope.Valid() || !s.StorageIdentityVersion().Valid() {
		return nil, ErrRejected
	}
	if _, err := s.PrepareStorage(ctx, scope); err != nil {
		return nil, err
	}
	if err := s.CheckStoragePublicationTarget(target); err != nil {
		return nil, err
	}
	proof, found := ctx.Value(publicationContext{}).(publicationProof)
	if found && (proof.owner != s || proof.scope != scope || proof.target != target) {
		return nil, ErrRejected
	}
	if err := s.call(ctx, StorageRecoverable, struct {
		Target storage.PublicationTarget `json:"target"`
	}{target}, nil); err != nil {
		return nil, err
	}
	if found {
		return ctx, nil
	}
	return context.WithValue(ctx, publicationContext{}, publicationProof{owner: s, scope: scope, target: target}), nil
}

func (s *Storage) RequireRecoverableIngress(ctx context.Context, scope storage.Scope, target storage.PublicationTarget) (context.Context, error) {
	if s.StorageIdentityVersion() != storage.IsolatedIdentity {
		return nil, ErrRejected
	}
	return s.RequireRecoverableStorage(ctx, scope, target)
}

func (s *Storage) RequireAuthorizedStoragePublication(ctx context.Context, scope storage.Scope, target storage.PublicationTarget, policy storage.PublicationPolicy) (context.Context, error) {
	if policy == nil || s.policies == nil {
		return nil, ErrRejected
	}
	bound, err := s.RequireRecoverableStorage(ctx, scope, target)
	if err != nil {
		return nil, err
	}
	proof, found := bound.Value(publicationContext{}).(publicationProof)
	if !found || proof.policy != nil {
		return nil, ErrRejected
	}
	proof.policy = policy
	return context.WithValue(bound, publicationContext{}, proof), nil
}

func (s *Storage) publish(ctx context.Context, target storage.PublicationTarget, method Method, input, out any) error {
	proof, found := ctx.Value(publicationContext{}).(publicationProof)
	if !found {
		return s.call(ctx, method, input, out)
	}
	if proof.owner != s || proof.scope != s.identity.Scope || proof.target != target {
		return ErrRejected
	}
	if proof.policy == nil {
		return s.call(ctx, method, input, out)
	}
	if s.policies == nil {
		return ErrRejected
	}
	if err := proof.policy(ctx); err != nil {
		return err
	}
	id, release, err := s.policies.Add(ctx, s.port, s.identity, target, proof.policy)
	if err != nil {
		return err
	}
	defer release()
	// Domain numeric bytes stay RawMessage/base64; framing never parses a
	// document number into float64 or replaces the original application bytes.
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	defer clear(encoded)
	var framed map[string]json.RawMessage
	if json.Unmarshal(encoded, &framed) != nil || framed == nil {
		return ErrRejected
	}
	authority, _ := json.Marshal(id)
	defer clear(authority)
	framed["authorization"] = authority
	return s.call(ctx, method, framed, out)
}

// HistoricalStoragePublicationContext keeps the original Go source policy on
// its exact legacy writer. Web ports contain no native SDK2 admission values to
// remove; this boundary validates both the live source and SDK recovery instead.
func (s *Storage) HistoricalStoragePublicationContext(ctx context.Context) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.StorageIdentityVersion() != storage.LegacyIdentity || s.policies == nil {
		return nil, ErrRejected
	}
	proof, ok := ctx.Value(publicationContext{}).(publicationProof)
	if !ok || proof.owner != s || proof.scope != s.identity.Scope || proof.policy == nil {
		return nil, ErrRejected
	}
	if err := proof.policy(ctx); err != nil {
		return nil, err
	}
	bound, err := s.RequireRecoverableStorage(ctx, proof.scope, proof.target)
	if err != nil {
		return nil, err
	}
	if _, err := s.StorageBindings(bound); err != nil {
		return nil, err
	}
	if err := s.CheckStoragePublicationTarget(proof.target); err != nil {
		return nil, err
	}
	if err := proof.policy(bound); err != nil {
		return nil, err
	}
	return bound, nil
}

var _ storage.HistoricalPublicationProjector = (*Storage)(nil)
