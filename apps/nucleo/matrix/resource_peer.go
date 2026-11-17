// SPDX-License-Identifier: Apache-2.0
package matrix

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type resourceBootstrapPeerSessions struct {
	client   *Client
	docs     *api.Docs
	plan     resource.SignedBootstrapPlan
	manifest resource.SignedBootstrapRosterManifest
	lease    offline.WorkLease
}

func (s *resourceBootstrapPeerSessions) check(ctx context.Context) error {
	c := s.client
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	actor, err := c.resourceSigningActor(ctx)
	if err != nil {
		return err
	}
	if s.plan.CreatorScope != c.resourceScope() || s.plan.Creator != actor {
		return ErrIdentity
	}
	return nil
}

func (s *resourceBootstrapPeerSessions) CurrentResourceSessionEvidence(ctx context.Context, scope storage.Scope, device string) (resource.SessionEvidence, error) {
	var zero resource.SessionEvidence
	release, err := resourceLease(ctx, s.lease)
	if err != nil {
		return zero, err
	}
	defer release()
	if err := s.check(ctx); err != nil {
		return zero, err
	}
	out, err := s.docs.ResourceBootstrapPeerEvidence(ctx, resource.BootstrapPeerQuery{Plan: s.plan, Manifest: s.manifest, Scope: scope, DeviceID: device})
	if err != nil {
		return zero, err
	}
	if err := s.check(ctx); err != nil {
		return zero, err
	}
	return out, nil
}

// ResourceBootstrapPeerSessionProvider retains the CURRENT owner's SDK and work
// lease while resolving peers through the fixed authenticated Docs backend. The
// canonical server comes from this independently verified own SDK identity;
// callers supply neither a homeserver/pin nor historical producer authority.
func (c *Client) ResourceBootstrapPeerSessionProvider(ctx context.Context, docs *api.Docs, plan resource.SignedBootstrapPlan, manifest resource.SignedBootstrapRosterManifest, lease offline.WorkLease) (*resource.SessionProvider, error) {
	if c == nil || docs == nil || lease == nil || !plan.Valid() || !manifest.ValidFor(plan.BootstrapPlan) || len(plan.Signatures) == 0 || len(manifest.Signatures) == 0 {
		return nil, ErrIdentity
	}
	source := &resourceBootstrapPeerSessions{client: c, docs: docs, plan: plan, manifest: manifest, lease: lease}
	release, err := resourceLease(ctx, lease)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := source.check(ctx); err != nil {
		return nil, err
	}
	c.stateMu.RLock()
	_, server, err := c.session.UserID.Parse()
	c.stateMu.RUnlock()
	if err != nil {
		return nil, ErrIdentity
	}
	return resource.NewSessionEvidenceProvider(source, server)
}
