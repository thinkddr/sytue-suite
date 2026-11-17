// SPDX-License-Identifier: Apache-2.0
package api

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// ResourceBootstrapPeerEvidence uses the authenticated FIXED Docs API backend;
// neither the caller nor roster chooses a service URL, registry pin or root.
func (d *Docs) ResourceBootstrapPeerEvidence(ctx context.Context, query resource.BootstrapPeerQuery) (resource.SessionEvidence, error) {
	if d == nil || d.c == nil || !query.Valid() || resource.OriginServiceID(query.Plan.Source.Resource.Product) != "docs" {
		return resource.SessionEvidence{}, resource.ErrInvalid
	}
	return PostJSON[resource.SessionEvidence](ctx, d.c, "/resources/bootstrap/peer", query)
}

type bootstrapPeerSessions struct {
	docs     *Docs
	plan     resource.SignedBootstrapPlan
	manifest resource.SignedBootstrapRosterManifest
}

func (s *bootstrapPeerSessions) CurrentResourceSessionEvidence(ctx context.Context, scope storage.Scope, device string) (resource.SessionEvidence, error) {
	return s.docs.ResourceBootstrapPeerEvidence(ctx, resource.BootstrapPeerQuery{Plan: s.plan, Manifest: s.manifest, Scope: scope, DeviceID: device})
}

// BootstrapPeerSessionProvider verifies public evidence through the SAME shared
// SessionProvider. Server must come from retained app/bootstrap configuration,
// never the roster or peer response. Every revalidation performs fresh HTTP.
func (d *Docs) BootstrapPeerSessionProvider(plan resource.SignedBootstrapPlan, manifest resource.SignedBootstrapRosterManifest, server string) (*resource.SessionProvider, error) {
	if d == nil || d.c == nil || !plan.Valid() || !manifest.ValidFor(plan.BootstrapPlan) || len(plan.Signatures) == 0 || len(manifest.Signatures) == 0 {
		return nil, resource.ErrInvalid
	}
	return resource.NewSessionEvidenceProvider(&bootstrapPeerSessions{docs: d, plan: plan, manifest: manifest}, server)
}
