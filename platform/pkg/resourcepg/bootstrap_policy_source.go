// SPDX-License-Identifier: Apache-2.0
package resourcepg

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/resource"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type bootstrapPolicySource struct {
	store   *BootstrapCaptureStore
	current resource.VerifiedSession
	key     resource.Resource
	plan    string
}

var _ resource.TrustedBootstrapPolicySource = bootstrapPolicySource{}

// AcceptedPolicySource is an ORIGINAL Source read-only metadata view for the
// current independently enrolled owner B. It needs no retained epoch/capability
// and never resolves retired A as live. Its original scalar pin/root generation
// comes from the pre-network roster capture, not from returned public JSON.
// A complete capture, preparing bootstrap or missing slot cannot become an ACK.
func (s *BootstrapCaptureStore) AcceptedPolicySource(ctx context.Context, current resource.VerifiedSession, key resource.Resource, plan string) (resource.TrustedBootstrapPolicySource, error) {
	v := bootstrapPolicySource{store: s, current: current, key: key, plan: plan}
	if _, err := v.RetainedBootstrapPolicy(ctx, key, plan); err != nil {
		return nil, err
	}
	return v, nil
}

func (v bootstrapPolicySource) check(ctx context.Context, key resource.Resource, plan string) error {
	if v.store == nil || v.store.relay == nil || v.store.relay.dependencies.CheckBootstrapSource == nil || key != v.key || plan != v.plan || v.current.Scope() != (storage.Scope{Tenant: key.Tenant, Subject: v.store.relay.subject, Product: key.Product}) {
		return resource.ErrRevoked
	}
	if err := v.store.relay.scoped(ctx, key); err != nil {
		return err
	}
	return v.current.Revalidate(ctx)
}

func (v bootstrapPolicySource) read(ctx context.Context, tx pgx.Tx) (resource.RetainedBootstrapPolicy, error) {
	var out resource.RetainedBootstrapPolicy
	capture, row, err := v.readCapturedBootstrapRow(ctx, tx)
	if err != nil {
		return out, err
	}
	head, err := v.store.relay.readHead(ctx, tx, v.key, true)
	if err != nil {
		return out, err
	}
	if head.state != "active" || head.generation != 1 || head.epochID != row.plan.Epoch || head.membership != row.plan.MembershipSHA256 {
		return out, resource.ErrRevoked
	}
	out.Capture = capture
	out.State, out.Epoch, out.Generation = head.state, head.epochID, "1"
	out.PolicySHA256, out.MembershipSHA256 = head.policy, head.membership
	out.SignedEpoch = head.epoch
	args := append(resourceKey(v.key), v.plan, v.store.relay.subject)
	if err := tx.QueryRow(ctx, `SELECT evidence_sha256,evidence FROM resource_e2e_bootstraps WHERE(tenant,product,kind,resource_id,plan_id)=($1,$2,$3,$4,$5)AND owner_sub=$6`, args...).Scan(&out.PlanEvidenceSHA256, &out.PlanEvidence); err != nil {
		return resource.RetainedBootstrapPolicy{}, err
	}
	if err := v.loadAcceptedSlots(ctx, tx, row, &out); err != nil {
		return resource.RetainedBootstrapPolicy{}, err
	}
	return out, nil
}

func (v bootstrapPolicySource) readCapturedBootstrapRow(ctx context.Context, tx pgx.Tx) (resource.RetainedBootstrapCapture, bootstrapRow, error) {
	capture, err := v.store.readRetainedCapture(ctx, tx, v.current, v.key, v.plan, false)
	if err != nil {
		return capture, bootstrapRow{}, err
	}
	if capture.State != "complete" {
		return capture, bootstrapRow{}, resource.ErrPending
	}
	// Preserve normal source lock ordering and the ORIGINAL freeze/owner/ACL.
	if err := v.store.relay.dependencies.CheckBootstrapSource(ctx, tx, capture.Source); err != nil {
		return capture, bootstrapRow{}, err
	}
	row, err := v.store.relay.readBootstrap(ctx, tx, v.key, true)
	if err != nil {
		return capture, bootstrapRow{}, err
	}
	originalGeneration, generationErr := capture.SessionGeneration.Int64()
	if generationErr != nil || row.generation != originalGeneration || row.state != "active" || row.plan.ID != v.plan || row.hash != capture.PlanSHA256 || row.plan.Source != capture.Source || row.plan.Creator != capture.Creator || row.plan.RoomID != capture.RoomID || row.plan.SessionBindingSHA256 != capture.SessionBindingSHA256 || row.generation < 1 {
		return capture, bootstrapRow{}, resource.ErrPending
	}
	return capture, row, nil
}

func (v bootstrapPolicySource) loadAcceptedSlots(ctx context.Context, tx pgx.Tx, row bootstrapRow, out *resource.RetainedBootstrapPolicy) error {
	for _, kind := range []resource.BootstrapKind{resource.BootstrapBaseline, resource.BootstrapCheckpoint} {
		slot, err := v.store.relay.bootstrapSlot(ctx, tx, row, kind, true)
		if err != nil {
			return err
		}
		if slot.State != "accepted" || slot.Reference == nil {
			return resource.ErrPending
		}
		retained := resource.RetainedBootstrapSlot{State: slot.State, Kind: kind, CandidateSHA256: slot.SHA256, Candidate: slot.Candidate, Reference: *slot.Reference}
		if kind == resource.BootstrapBaseline {
			out.Baseline = retained
		} else {
			out.Checkpoint = retained
		}
	}
	return nil
}

func (v bootstrapPolicySource) RetainedBootstrapPolicy(ctx context.Context, key resource.Resource, plan string) (resource.RetainedBootstrapPolicy, error) {
	var out resource.RetainedBootstrapPolicy
	if err := v.check(ctx, key, plan); err != nil {
		return out, err
	}
	err := v.store.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = v.read(ctx, tx)
		return err
	})
	if err != nil {
		return resource.RetainedBootstrapPolicy{}, err
	}
	if err := v.current.Revalidate(ctx); err != nil {
		return resource.RetainedBootstrapPolicy{}, err
	}
	return out, nil
}

func (v bootstrapPolicySource) PublicEvidence(ctx context.Context, key resource.Resource, sha string) (json.RawMessage, error) {
	if err := v.check(ctx, key, v.plan); err != nil {
		return nil, err
	}
	var raw []byte
	err := v.store.relay.dependencies.TenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := v.read(ctx, tx); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT value FROM resource_e2e_public_evidence WHERE tenant=$1 AND product=$2 AND sha256=$3`, key.Tenant, key.Product, sha).Scan(&raw)
	})
	if err != nil {
		return nil, err
	}
	if err := v.current.Revalidate(ctx); err != nil {
		return nil, err
	}
	return raw, nil
}
