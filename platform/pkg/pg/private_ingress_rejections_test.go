// SPDX-License-Identifier: Apache-2.0
//go:build integration

package pg_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

func TestPrivateIngressRejectedBeginIsDurableConsumesNonceAndNeverStartsOnReplay(t *testing.T) {
	fixture := ingressHandoffFixture(t)
	h, cfg, fencer, queue, request, ctx := fixture.h, fixture.cfg, fixture.fencer, fixture.queue, fixture.request, fixture.ctx
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE private_ingress_tasks SET lease_until=now()-interval '30 seconds' WHERE id=$1`, h.task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	proof := ingressControlChallenge(t, h, cfg, request, fencer.keys)
	for range 2 {
		result, err := queue.Apply(ctx, cfg.Scope, request, proof)
		if err != nil || result.Handoff == nil || result.Handoff.Status != "not_started" || result.Handoff.Rejection == nil || result.Handoff.Fence != nil || result.Handoff.Claim != nil || result.Handoff.Receipt != nil {
			t.Fatal("changed assignment did not produce a durable refusal", err)
		}
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		var count int
		var consumed, revoked *time.Time
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_ingress_handoff_rejections WHERE id=$1`, request.Handoff.ID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("lost permanent refusal")
		}
		if err := tx.QueryRow(ctx, `SELECT consumed_at FROM private_ingress_actions WHERE nonce=$1`, proof.Nonce).Scan(&consumed); err != nil {
			return err
		}
		if consumed == nil {
			return errors.New("refusal did not consume its original nonce")
		}
		if err := tx.QueryRow(ctx, `SELECT revoked_at FROM private_ingress_devices WHERE device_id=$1`, h.target.DeviceID).Scan(&revoked); err != nil {
			return err
		}
		if revoked != nil {
			return errors.New("refusal revoked the original device")
		}
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM private_ingress_claim_history)+(SELECT count(*) FROM private_ingress_handoffs)`).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("refusal started a handoff")
		}
		// Even if an old assignment later matches again, this ID is closed.
		_, err := tx.Exec(ctx, `UPDATE private_ingress_tasks SET lease_until=$2 WHERE id=$1`, h.task.ID, request.Handoff.Previous.ExpiresAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	freshProof := ingressControlChallenge(t, h, cfg, request, fencer.keys)
	result, err := queue.Apply(ctx, cfg.Scope, request, freshProof)
	if err != nil || result.Handoff == nil || result.Handoff.Status != "not_started" || fencer.fences != 0 || fencer.reads != 0 {
		t.Fatal("fresh nonce restarted a rejected ID", err)
	}
	changed := request
	payload := *request.Handoff
	payload.Token = "replacement_secret"
	changed.Handoff = &payload
	changedProof := ingressControlChallenge(t, h, cfg, changed, fencer.keys)
	if _, err := queue.Apply(ctx, cfg.Scope, changed, changedProof); err == nil {
		t.Fatal("rejected ID accepted a different signed binding")
	}
	// A genuinely new exact inspection can begin; the refused ID stays intact.
	current, err := queue.Inspect(ctx, cfg.Scope, cfg.Epoch.String(), h.task.ID)
	if err != nil || current.Assignment == nil {
		t.Fatal(err)
	}
	payload = *request.Handoff
	payload.ID = uuid.NewString()
	payload.Previous = *current.Assignment
	changed = request
	changed.Handoff = &payload
	changedProof = ingressControlChallenge(t, h, cfg, changed, fencer.keys)
	result, err = queue.Apply(ctx, cfg.Scope, changed, changedProof)
	if err != nil || result.Handoff == nil || result.Handoff.Status != "readable" || fencer.fences != 1 {
		t.Fatal("new retained intent failed to start independently", err)
	}
	err = h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO private_ingress_handoffs(tenant,owner_sub,product,epoch,id,task_id,previous_generation,target_device_id,target_room_id,token_sha256,authority_generation,authority_user_id,authority_root_key,request_sha256,status)
SELECT tenant,owner_sub,product,epoch,$5::uuid,task_id,previous_generation,target_device_id,target_room_id,token_sha256,authority_generation,authority_user_id,authority_root_key,request_sha256,'fencing' FROM private_ingress_handoffs WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$6`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, request.Handoff.ID, changed.Handoff.ID)
		return err
	})
	var refused *pgconn.PgError
	if !errors.As(err, &refused) || refused.Code != "55000" {
		t.Fatal("rejected ID was not blocked before handoff constraints", err)
	}
	if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
		inventory, err := pg.ConsultaInventarioDeFusion(ctx, tx)
		if err != nil {
			return err
		}
		found := false
		for _, table := range inventory.ScopeBound {
			if table == "private_ingress_handoff_rejections" {
				found = true
			}
		}
		if !found {
			return errors.New("rejection lacks Scope-bound merge coverage")
		}
		return pg.RequireScopeAdmissionCoverage(ctx, tx, inventory)
	}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`DELETE FROM private_ingress_handoff_rejections`, `UPDATE private_ingress_handoff_rejections SET rejection=rejection`} {
		if err := h.run(t.Context(), func(ctx context.Context, tx pgx.Tx) error {
			if err := pg.BindPrivateIngressScope(ctx, tx, h.cfg); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, query)
			return err
		}); err == nil {
			t.Fatal("rejected intent evidence was mutable")
		}
	}
}
