// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// BootstrapPeerSource owns the ORIGINAL frozen owner, ACL and lifecycle rows.
// Its checks must use the same source/document lock as writes and revocations.
type BootstrapPeerSource interface {
	resource.OriginalBootstrapSource
	CheckBootstrapPeer(context.Context, resource.BootstrapSourceSnapshot, resource.BootstrapRosterInput) error
}

// CurrentPeerEvidence reads no peer until the COMPLETE retained stream matches
// the owner's signed manifest and the original SQL authorizes that exact leaf.
// Remote CURRENT resolution occurs outside SQL transactions; original source,
// manifest and peer binding are rechecked after the network boundary.
func (s *BootstrapCaptureStore) CurrentPeerEvidence(ctx context.Context, p resource.VerifiedBootstrapRosterManifest, scope storage.Scope, device string, source BootstrapPeerSource, authority *SessionAuthorityClient) (resource.SessionEvidence, error) {
	var zero resource.SessionEvidence
	if source == nil || authority == nil {
		return zero, resource.ErrInvalid
	}
	if err := s.check(ctx, p); err != nil {
		return zero, err
	}
	input, err := resource.BootstrapRosterPeer(ctx, p, s.Pages(p), scope, device)
	if err != nil {
		return zero, err
	}
	if err := source.CheckBootstrapPeer(ctx, p.Plan().Source, input); err != nil {
		return zero, err
	}
	evidence, err := authority.CurrentResourceSessionEvidence(ctx, scope, device)
	if err != nil {
		return zero, err
	}
	provider, err := authority.Provider()
	if err != nil {
		return zero, err
	}
	current, err := provider.Resolve(ctx, scope, device)
	if err != nil {
		return zero, err
	}
	if current.Binding() != evidence.Record.Binding {
		return zero, resource.ErrRevoked
	}
	if err := resource.CheckBootstrapPeerSession(ctx, input, current); err != nil {
		return zero, err
	}
	if err := source.CheckBootstrapPeer(ctx, p.Plan().Source, input); err != nil {
		return zero, err
	}
	if err := s.check(ctx, p); err != nil {
		return zero, err
	}
	if err := current.Revalidate(ctx); err != nil {
		return zero, err
	}
	return evidence, nil
}
