// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
)

type AuthorizedPrivateIngressConfig struct {
	PrivateIngressConfig
	Authority callaccess.StorageAuthorityReader
	// Supplied only by the queue after an authenticated Rooms fence read.
	handoffFence *ingress.DeviceFence
}

// ChallengePrivateIngress retains the public transcript in the owning domain,
// with the fresh Rooms pin/generation. The request cannot select a trust root.
func ChallengePrivateIngress(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest) (matrixproof.Action, error) {
	var action matrixproof.Action
	if cfg.Authority == nil || request.Validate(cfg.Scope, cfg.Epoch.String()) != nil {
		return action, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg.PrivateIngressConfig); err != nil {
		return action, err
	}
	authority, err := cfg.Authority.ReadStorageAuthority(ctx, cfg.Scope)
	if err != nil {
		return action, err
	}
	if authority.RequireActive(cfg.Scope, authority.Generation) != nil {
		return action, matrixproof.ErrUnverified
	}
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM private_ingress_actions WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND consumed_at IS NULL AND expires_at>now()`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch).Scan(&pending); err != nil {
		return action, err
	}
	if pending >= 32 {
		return action, ingress.ErrContract
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return action, err
	}
	now := time.Now().UTC()
	action = matrixproof.Action{Version: 1, Generation: authority.Generation, Scope: cfg.Scope, Epoch: cfg.Epoch.String(), Kind: request.Kind, Target: request.Target, RootKey: authority.RootKey, RequestSHA256: digest, Nonce: uuid.NewString(), IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(5 * time.Minute).UnixMilli()}
	raw, err := json.Marshal(action)
	if err != nil {
		return matrixproof.Action{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO private_ingress_actions(tenant,owner_sub,product,epoch,nonce,action,expires_at) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7)`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, action.Nonce, raw, time.UnixMilli(action.ExpiresAt))
	return action, err
}

// ApplyPrivateIngressAction verifies against a retained nonce, then consumes
// that nonce in the SAME domain transaction as key/claim/ACK mutation. Retries
// revalidate current authority and run the operation's immutable idempotency
// checks; a previously consumed signature never authorizes a changed request.
func ApplyPrivateIngressAction(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, proof matrixproof.SignedAction) (ingress.ControlResult, error) {
	var result ingress.ControlResult
	if cfg.Authority == nil || request.Validate(cfg.Scope, cfg.Epoch.String()) != nil {
		return result, ingress.ErrContract
	}
	if err := BindPrivateIngressScope(ctx, tx, cfg.PrivateIngressConfig); err != nil {
		return result, err
	}
	retained, err := retainedPrivateIngressAction(ctx, tx, cfg, request, proof)
	if err != nil {
		return result, err
	}
	expected, nonce, consumed := retained.action, retained.nonce, retained.consumed
	authorization, err := cfg.Authority.VerifyStorageAction(ctx, expected, proof)
	if err != nil {
		return result, err
	}
	if authorization.Validate(expected) != nil {
		return result, matrixproof.ErrUnverified
	}
	result.Version = 1
	result.Authorization = authorization
	switch request.Kind {
	case "key.enroll":
		err = EnrollAuthorizedPrivateIngress(ctx, tx, cfg.PrivateIngressConfig, *request.Enrollment, request.Target, authorization)
	case "task.claim":
		var claim ingress.Claim
		claim, err = ClaimPrivateIngress(ctx, tx, cfg.PrivateIngressConfig, request.Claim.TaskID, request.Target, request.Claim.Token, time.Duration(request.Claim.LeaseSeconds)*time.Second)
		result.Claim = &claim
	case "task.ack":
		err = AcknowledgePrivateIngress(ctx, tx, cfg.PrivateIngressConfig, request.Acknowledgment.Claim, request.Acknowledgment.Receipt)
		result.Receipt = &request.Acknowledgment.Receipt
	case "task.handoff":
		result.Handoff, err = applyPrivateIngressHandoff(ctx, tx, cfg, request, authorization)
	default:
		err = ingress.ErrContract
	}
	if err != nil {
		return ingress.ControlResult{}, err
	}
	// A second independent check observes revocation that happened during receipt
	// readback or the operation. Retained originals are never deleted by an ACK.
	latest, err := cfg.Authority.VerifyStorageAction(ctx, expected, proof)
	if err != nil {
		return ingress.ControlResult{}, err
	}
	if latest.Validate(expected) != nil || latest.Authority != authorization.Authority || latest.SigningKey != authorization.SigningKey {
		return ingress.ControlResult{}, matrixproof.ErrUnverified
	}
	if consumed == nil {
		_, err = tx.Exec(ctx, `UPDATE private_ingress_actions SET consumed_at=now() WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND nonce=$5`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, nonce)
	}
	if err != nil {
		return ingress.ControlResult{}, err
	}
	return result, result.Validate(request, expected)
}

type retainedIngressAction struct {
	action   matrixproof.Action
	nonce    uuid.UUID
	consumed *time.Time
}

func retainedPrivateIngressAction(ctx context.Context, tx pgx.Tx, cfg AuthorizedPrivateIngressConfig, request ingress.ControlRequest, proof matrixproof.SignedAction) (retainedIngressAction, error) {
	nonce, err := uuid.Parse(proof.Nonce)
	if err != nil || nonce == uuid.Nil || nonce.String() != proof.Nonce {
		return retainedIngressAction{}, ingress.ErrContract
	}
	var raw []byte
	var consumed *time.Time
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT action,consumed_at,expires_at FROM private_ingress_actions WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 AND nonce=$5 FOR UPDATE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch, nonce).Scan(&raw, &consumed, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return retainedIngressAction{}, ErrPrivateIngressMissing
	}
	if err != nil {
		return retainedIngressAction{}, err
	}
	var expected matrixproof.Action
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return retainedIngressAction{}, err
	}
	if json.Unmarshal(raw, &expected) != nil || !expires.After(time.Now()) || proof.Action != expected || expected.Scope != cfg.Scope || expected.Epoch != cfg.Epoch.String() || expected.Kind != request.Kind || expected.Target != request.Target || expected.RequestSHA256 != digest {
		return retainedIngressAction{}, matrixproof.ErrUnverified
	}
	return retainedIngressAction{action: expected, nonce: nonce, consumed: consumed}, nil
}
