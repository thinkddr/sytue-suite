// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/publication"
)

// BootstrapOriginSource is fixed to the ORIGINAL product relay, source snapshot
// and verified plan. No URL, tenant, subject or database selector comes from the
// signed publication body. Every read rechecks the source/ACL under its original
// lock before reading independent slot columns under normal tenant RLS.
type BootstrapOriginSource struct {
	relay *Relay
	plan  resource.VerifiedBootstrapPlan
}

var _ resource.TrustedBootstrapOriginSource = (*BootstrapOriginSource)(nil)

func (r *Relay) BootstrapOriginSource(ctx context.Context, plan resource.VerifiedBootstrapPlan) (*BootstrapOriginSource, error) {
	if r == nil {
		return nil, resource.ErrInvalid
	}
	if err := r.checkBootstrap(ctx, plan); err != nil {
		return nil, err
	}
	source := &BootstrapOriginSource{relay: r, plan: plan}
	// Registration of a plan is allowed before its first slot. It is not a send
	// permit: BootstrapOriginHold below requires the original durable send hold.
	err := r.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := r.lockedBootstrap(ctx, tx, plan)
		if err != nil {
			return err
		}
		if row.state != "preparing" {
			return resource.ErrRevoked
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := plan.Revalidate(ctx); err != nil {
		return nil, err
	}
	return source, nil
}

func (s *BootstrapOriginSource) BootstrapOriginHold(ctx context.Context, key resource.Resource, planID string, kind resource.BootstrapKind) (resource.BootstrapOriginHold, error) {
	var out resource.BootstrapOriginHold
	if s == nil || s.relay == nil || key != s.plan.Value().Source.Resource || planID != s.plan.Value().ID || (kind != resource.BootstrapBaseline && kind != resource.BootstrapCheckpoint) {
		return out, resource.ErrRevoked
	}
	if err := s.relay.checkBootstrap(ctx, s.plan); err != nil {
		return out, err
	}
	err := s.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		row, err := s.relay.lockedBootstrap(ctx, tx, s.plan)
		if err != nil {
			return err
		}
		if row.state != "preparing" {
			return resource.ErrRevoked
		}
		// These are separately persisted scalar columns, not values decoded from
		// slot JSON or copied from the request. The table constraints bind them to
		// the immutable original plan and SDK ciphertext.
		var generation int64
		out.Resource = key
		out.Source = row.plan.Source
		out.PlanID = row.plan.ID
		out.PlanSHA256 = row.hash
		out.Phase = row.state
		out.RoomID = row.plan.RoomID
		out.Actor = row.plan.Creator
		out.Kind = kind
		args := append(resourceKey(key), planID, kind)
		err = tx.QueryRow(ctx, `SELECT b.session_generation,b.session_binding_sha256,s.state,s.attempt_id::text,s.candidate_sha256,s.context_sha256,s.outer_cipher_sha256 FROM resource_e2e_bootstraps b JOIN resource_e2e_bootstrap_slots s USING(tenant,product,kind,resource_id,plan_id)WHERE(b.tenant,b.product,b.kind,b.resource_id,b.plan_id)=($1,$2,$3,$4,$5)AND s.tenant=$1 AND s.product=$2 AND s.kind=$3 AND s.resource_id=$4 AND s.plan_id=$5 AND s.bootstrap_kind=$6 FOR SHARE OF s`, args...).Scan(&generation, &out.SessionBindingSHA256, &out.State, &out.TransactionID, &out.CandidateSHA256, &out.ContextSHA256, &out.OuterCipherSHA256)
		if err != nil {
			return err
		}
		out.SessionGeneration, err = resource.Sequence(generation)
		if err != nil {
			return err
		}
		if out.State != "sending" && out.State != "ambiguous" {
			return resource.ErrPending
		}
		return nil
	})
	if err != nil {
		return resource.BootstrapOriginHold{}, err
	}
	if err := s.plan.Revalidate(ctx); err != nil {
		return resource.BootstrapOriginHold{}, err
	}
	return out, nil
}

func (s *BootstrapOriginSource) VerifyRequest(ctx context.Context, candidate resource.VerifiedBootstrapCandidate, current resource.VerifiedSession, signed publication.SignedRequest) (resource.VerifiedBootstrapOriginRequest, error) {
	if s == nil {
		return resource.VerifiedBootstrapOriginRequest{}, resource.ErrRevoked
	}
	return resource.VerifyBootstrapOriginRequest(ctx, candidate, current, signed, s)
}
