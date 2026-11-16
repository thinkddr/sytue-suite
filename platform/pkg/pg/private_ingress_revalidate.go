// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
)

func privateIngressRevalidationRequest(cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, claim ingress.Claim) bool {
	return cfg.Authority != nil && request.Kind == "task.claim" && request.Validate(cfg.Scope, cfg.Epoch.String()) == nil && claim.Validate(cfg.Scope, cfg.Epoch.String()) == nil && claim.Target == request.Target && claim.Task.ID == request.Claim.TaskID && claim.Token == request.Claim.Token
}

func privateIngressClaimMatches(stored PrivateIngressDelivery, claim ingress.Claim) bool {
	digest := sha256.Sum256([]byte(claim.Token))
	return stored.Task == claim.Task && stored.Target == claim.Target && stored.ClaimGeneration == claim.Generation && subtle.ConstantTimeCompare(stored.claimHash, digest[:]) == 1
}

// RevalidatePrivateIngressPublication verifies the exact original claim without
// renewing its lease. A fresh retained signature may be consumed for this
// strictly lesser read-only use of a claim permission. Every call independently
// checks Rooms' current root generation and active device. An acknowledged task
// permits receipt reconciliation after expiry, never a new claim or takeover.
func RevalidatePrivateIngressPublication(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, proof matrixproof.SignedAction, claim ingress.Claim) (matrixproof.Authorization, error) {
	var result matrixproof.Authorization
	if !privateIngressRevalidationRequest(cfg, request, claim) {
		return result, ingress.ErrClaim
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg.PrivateIngressConfig); err != nil {
		return result, err
	}
	retained, err := retainedPrivateIngressAction(ctx, tx, cfg, request, proof)
	if err != nil {
		return result, err
	}
	if err := requirePrivateIngressDevice(ctx, tx, cfg.PrivateIngressConfig, claim.Target); err != nil {
		return result, err
	}
	stored, err := readPrivateIngressDelivery(ctx, tx, cfg.PrivateIngressConfig, claim.Task.ID, false)
	if err != nil {
		return result, err
	}
	if !privateIngressClaimMatches(stored, claim) {
		return result, ingress.ErrClaim
	}
	if stored.Receipt == nil && (stored.LeaseUntil == nil || !stored.LeaseUntil.After(time.Now())) {
		return result, ingress.ErrClaim
	}
	result, err = cfg.Authority.VerifyStorageAction(ctx, retained.action, proof)
	if err != nil {
		return matrixproof.Authorization{}, err
	}
	if err := result.Validate(retained.action); err != nil {
		return matrixproof.Authorization{}, err
	}
	if retained.consumed == nil {
		_, err = tx.Exec(ctx, `UPDATE private_ingress_actions SET consumed_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND nonce=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, retained.nonce)
	}
	return result, err
}
