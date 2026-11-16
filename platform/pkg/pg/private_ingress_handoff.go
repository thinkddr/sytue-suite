// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
)

type privateIngressHandoff struct {
	view          ingress.HandoffView
	authority     matrixproof.Authority
	digest        string
	tokenHash     []byte
	resolveDigest *string
}

func readPrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, request ingress.ControlRequest) (privateIngressHandoff, error) {
	rejected, err := readPrivateIngressRejectedHandoff(ctx, tx, cfg, request)
	if err == nil {
		return rejected, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return privateIngressHandoff{}, err
	}
	h := request.Handoff
	out := privateIngressHandoff{view: ingress.HandoffView{Version: 1, ID: h.ID}, authority: matrixproof.Authority{Version: 1, Scope: cfg.Scope, Status: "active"}}
	var fence, receipt []byte
	err = tx.QueryRow(ctx, `SELECT h.status,h.target_device_id,h.target_room_id,h.token_sha256,h.authority_generation,h.authority_user_id,h.authority_root_key,h.request_sha256,h.resolve_request_sha256,h.fence,h.resolved_receipt,c.device_id,c.room_id,c.claim_generation,c.signing_key,c.lease_until
FROM private_ingress_handoffs h JOIN private_ingress_claim_history c ON (c.tenant,c.owner_sub,c.product,c.epoch,c.task_id,c.claim_generation)=(h.tenant,h.owner_sub,h.product,h.epoch,h.task_id,h.previous_generation)
WHERE h.tenant=$1 AND h.owner_sub=$2 AND h.product=$3 AND h.epoch=$4 AND h.id=$5 FOR UPDATE OF h`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.ID).Scan(&out.view.Status, &out.view.Target.DeviceID, &out.view.Target.RoomID, &out.tokenHash, &out.authority.Generation, &out.authority.UserID, &out.authority.RootKey, &out.digest, &out.resolveDigest, &fence, &receipt, &out.view.Previous.Target.DeviceID, &out.view.Previous.Target.RoomID, &out.view.Previous.Generation, &out.view.Previous.SigningKey, &out.view.Previous.ExpiresAt)
	if err != nil {
		return out, err
	}
	if fence != nil {
		out.view.Fence = &ingress.DeviceFence{}
		if json.Unmarshal(fence, out.view.Fence) != nil {
			return out, ingress.ErrContract
		}
	}
	if receipt != nil {
		out.view.Receipt = &ingress.Receipt{}
		if json.Unmarshal(receipt, out.view.Receipt) != nil {
			return out, ingress.ErrReceipt
		}
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg, h.TaskID, true)
	if err != nil {
		return out, err
	}
	out.view.Task = stored.Task
	if out.view.Status == "claimed" {
		if stored.LeaseUntil == nil {
			return out, ingress.ErrClaim
		}
		out.view.Claim = &ingress.Claim{Version: 1, Task: stored.Task, Target: stored.Target, Generation: stored.ClaimGeneration, Token: h.Token, ExpiresAt: *stored.LeaseUntil}
		if !privateIngressClaimMatches(stored, *out.view.Claim) {
			return out, ingress.ErrClaim
		}
	}
	return out, nil
}

func (h privateIngressHandoff) requireBinding(request ingress.ControlRequest, authorization matrixproof.Authorization) error {
	begin, err := request.BeginHandoffRequest()
	if err != nil {
		return err
	}
	digest, err := matrixproof.RequestDigest(begin)
	if err != nil {
		return err
	}
	tokenHash := sha256.Sum256([]byte(request.Handoff.Token))
	if h.authority != authorization.Authority || h.digest != digest || subtle.ConstantTimeCompare(h.tokenHash, tokenHash[:]) != 1 || h.view.ID != request.Handoff.ID || h.view.Task.ID != request.Handoff.TaskID || h.view.Task.Cipher.SHA256 != request.Handoff.CipherSHA256 || !h.view.Previous.Same(request.Handoff.Previous) || h.view.Target != request.Target {
		return ingress.ErrRevoked
	}
	if request.Handoff.Phase == "resolve" && h.resolveDigest != nil {
		resolved, err := matrixproof.RequestDigest(request)
		if err != nil || resolved != *h.resolveDigest {
			return ingress.ErrClaim
		}
	}
	return nil
}

func applyPrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, authorization matrixproof.Authorization) (*ingress.HandoffView, error) {
	if err := requirePrivateIngressDevice(ctx, tx, cfg.PrivateIngressConfig, request.Target); err != nil {
		return nil, err
	}
	retained, err := readPrivateIngressHandoff(ctx, tx, cfg.PrivateIngressConfig, request)
	if errors.Is(err, pgx.ErrNoRows) && request.Handoff.Phase == "begin" {
		retained, err = beginPrivateIngressHandoff(ctx, tx, cfg.PrivateIngressConfig, request, authorization)
	}
	if err != nil {
		return nil, err
	}
	if err := retained.requireBinding(request, authorization); err != nil {
		return nil, err
	}
	if retained.view.Status == "not_started" {
		return &retained.view, nil
	}
	if request.Handoff.Phase == "resolve" {
		return resolvePrivateIngressHandoff(ctx, tx, cfg, request, authorization, retained)
	}
	if retained.view.Status != "fencing" && retained.view.Status != "readable" {
		return nil, ingress.ErrClaim
	}
	if cfg.handoffFence != nil {
		if cfg.handoffFence.Status != "confirmed" || cfg.handoffFence.Validate(request, authorization.Authority) != nil {
			return nil, ingress.ErrRevoked
		}
		if retained.view.Status == "fencing" {
			raw, err := json.Marshal(cfg.handoffFence)
			if err != nil {
				return nil, err
			}
			_, err = tx.Exec(ctx, `UPDATE private_ingress_handoffs SET status='readable',fence=$6::jsonb,updated_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5 AND status='fencing'`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, request.Handoff.ID, raw)
			if err != nil {
				return nil, err
			}
			_, err = tx.Exec(ctx, `UPDATE private_ingress_devices SET fenced_at=coalesce(fenced_at,now()) WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND device_id=$5 AND revoked_at IS NOT NULL`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, request.Handoff.Previous.Target.DeviceID)
			if err != nil {
				return nil, err
			}
			retained.view.Status = "readable"
			retained.view.Fence = cfg.handoffFence
		}
	}
	return &retained.view, nil
}

func beginPrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig, request ingress.ControlRequest, authorization matrixproof.Authorization) (privateIngressHandoff, error) {
	h := request.Handoff
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg, h.TaskID, true)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	rejected, rejectionErr := readPrivateIngressRejectedHandoff(ctx, tx, cfg, request)
	if rejectionErr == nil {
		return rejected, nil
	}
	if !errors.Is(rejectionErr, pgx.ErrNoRows) {
		return privateIngressHandoff{}, rejectionErr
	}
	state, err := privateIngressTaskState(ctx, tx, cfg, stored)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	if stored.Task.Cipher.SHA256 != h.CipherSHA256 {
		return privateIngressHandoff{}, ingress.ErrClaim
	}
	if state.Assignment == nil || !state.Assignment.Same(h.Previous) || (stored.Receipt == nil && state.Assignment.ExpiresAt.After(time.Now())) {
		return rejectPrivateIngressHandoff(ctx, tx, cfg, stored.Task, request, authorization.Authority)
	}
	_, err = tx.Exec(ctx, `INSERT INTO private_ingress_claim_history(tenant,owner_sub,product,epoch,task_id,claim_generation,device_id,room_id,signing_key,claim_token_sha256,lease_until,receipt_room,receipt_event,acknowledged_at)
SELECT tenant,owner_sub,product,epoch,id,claim_generation,device_id,room_id,$6,claim_token_sha256,lease_until,receipt_room,receipt_event,acknowledged_at FROM private_ingress_tasks WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.TaskID, h.Previous.SigningKey)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	begin, err := request.BeginHandoffRequest()
	if err != nil {
		return privateIngressHandoff{}, err
	}
	digest, err := matrixproof.RequestDigest(begin)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	tokenHash := sha256.Sum256([]byte(h.Token))
	status := "fencing"
	var receipt []byte
	if stored.Receipt != nil {
		status = "readable"
		receipt, err = json.Marshal(stored.Receipt)
		if err != nil {
			return privateIngressHandoff{}, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO private_ingress_handoffs(tenant,owner_sub,product,epoch,id,task_id,previous_generation,target_device_id,target_room_id,token_sha256,authority_generation,authority_user_id,authority_root_key,request_sha256,status,resolved_receipt) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.ID, h.TaskID, h.Previous.Generation, request.Target.DeviceID, request.Target.RoomID, tokenHash[:], authorization.Authority.Generation, authorization.Authority.UserID, authorization.Authority.RootKey, digest, status, receipt)
	if err != nil {
		return privateIngressHandoff{}, err
	}
	if stored.Receipt == nil {
		if err := RevokePrivateIngressDevice(ctx, tx, cfg, h.Previous.Target.DeviceID); err != nil {
			return privateIngressHandoff{}, err
		}
	}
	return readPrivateIngressHandoff(ctx, tx, cfg, request)
}

func resolvePrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, authorization matrixproof.Authorization, retained privateIngressHandoff) (*ingress.HandoffView, error) {
	h := request.Handoff
	if retained.view.Status == "acknowledged" {
		return &retained.view, retained.view.Validate(request, authorization.Authority)
	}
	if cfg.handoffFence == nil && retained.view.Receipt == nil {
		return nil, ingress.ErrRevoked
	}
	if cfg.handoffFence != nil && (cfg.handoffFence.Status != "confirmed" || cfg.handoffFence.Validate(request, authorization.Authority) != nil || retained.view.Fence == nil || retained.view.Fence.Validate(request, authorization.Authority) != nil || !cfg.handoffFence.ConfirmedAt.Equal(*retained.view.Fence.ConfirmedAt)) {
		return nil, ingress.ErrRevoked
	}
	if h.Resolution == "committed" {
		return acknowledgePrivateIngressHandoff(ctx, tx, cfg, request, retained)
	}
	return claimPrivateIngressHandoff(ctx, tx, cfg, request, retained)
}

func claimPrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, retained privateIngressHandoff) (*ingress.HandoffView, error) {
	h := request.Handoff
	if retained.view.Status != "readable" && retained.view.Status != "claimed" {
		return nil, ingress.ErrClaim
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg.PrivateIngressConfig, h.TaskID, true)
	if err != nil {
		return nil, err
	}
	if stored.Receipt != nil {
		return nil, ingress.ErrReceipt
	}
	if retained.view.Status == "readable" && (stored.Target != h.Previous.Target || stored.ClaimGeneration != h.Previous.Generation || stored.LeaseUntil == nil || !stored.LeaseUntil.Equal(h.Previous.ExpiresAt)) {
		return nil, ingress.ErrClaim
	}
	tokenHash := sha256.Sum256([]byte(h.Token))
	if retained.view.Status == "claimed" && (stored.Target != request.Target || stored.ClaimGeneration != h.Previous.Generation+1 || subtle.ConstantTimeCompare(stored.claimHash, tokenHash[:]) != 1) {
		return nil, ingress.ErrClaim
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return nil, err
	}
	if retained.view.Status == "readable" {
		_, err = tx.Exec(ctx, `UPDATE private_ingress_handoffs SET status='claimed',new_claim_generation=$6,resolve_request_sha256=$7,updated_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.ID, h.Previous.Generation+1, digest)
		if err != nil {
			return nil, err
		}
	}
	until := time.Now().UTC().Add(time.Duration(h.LeaseSeconds) * time.Second)
	_, err = tx.Exec(ctx, `UPDATE private_ingress_tasks SET device_id=$6,room_id=$7,claim_generation=$8,claim_token_sha256=$9,lease_until=$10 WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.TaskID, request.Target.DeviceID, request.Target.RoomID, h.Previous.Generation+1, tokenHash[:], until)
	if err != nil {
		return nil, err
	}

	retained.view.Status = "claimed"
	retained.view.Claim = &ingress.Claim{Version: 1, Task: stored.Task, Target: request.Target, Token: h.Token, Generation: h.Previous.Generation + 1, ExpiresAt: until}
	return &retained.view, nil
}

func acknowledgePrivateIngressHandoff(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, retained privateIngressHandoff) (*ingress.HandoffView, error) {
	h := request.Handoff
	verifier, ok := cfg.Authority.(callaccess.StorageReceiptVerifier)
	if !ok || retained.view.Status != "readable" {
		return nil, ingress.ErrReceipt
	}
	if err := verifier.VerifyStorageReceipt(ctx, cfg.Scope, h.Previous.Target, h.Receipt.Commit); err != nil {
		return nil, err
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg.PrivateIngressConfig, h.TaskID, true)
	if err != nil {
		return nil, err
	}
	if stored.Target != h.Previous.Target || stored.ClaimGeneration != h.Previous.Generation {
		return nil, ingress.ErrClaim
	}
	if stored.Receipt != nil && *stored.Receipt != *h.Receipt {
		return nil, ingress.ErrReceipt
	}
	if stored.Receipt == nil {
		_, err = tx.Exec(ctx, `UPDATE private_ingress_tasks SET receipt_room=$6,receipt_event=$7,acknowledged_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.TaskID, h.Receipt.Commit.RoomID, h.Receipt.Commit.EventID)
		if err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(h.Receipt)
	if err != nil {
		return nil, err
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE private_ingress_handoffs SET status='acknowledged',resolved_receipt=$6::jsonb,resolve_request_sha256=$7,updated_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND id=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, h.ID, raw, digest)
	if err != nil {
		return nil, err
	}
	retained.view.Status = "acknowledged"
	retained.view.Receipt = h.Receipt
	return &retained.view, nil
}
