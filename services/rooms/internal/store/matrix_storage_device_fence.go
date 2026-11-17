// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// matrixDeviceFenceEvidence retains only public, verified consent. The request
// contains a claim token and must never be persisted as audit evidence.
type matrixDeviceFenceEvidence struct {
	Proof         matrixproof.SignedAction  `json:"proof"`
	Authorization matrixproof.Authorization `json:"authorization"`
}

// withMatrixDeviceFence verifies the signed begin transcript on every operation.
// The configured origin, pinned root and current identity remain authoritative.
func (s *Store) withMatrixDeviceFence(ctx context.Context, proof ingress.ControlProof, beginOnly bool, fn func(context.Context, pgx.Tx, matrixproof.Authorization) error) error {
	request := proof.Request
	if proof.Validate(request.Scope, false) != nil || request.Kind != "task.handoff" || request.Handoff == nil || beginOnly && request.Handoff.Phase != "begin" {
		return domain.ErrInvalidInput
	}
	directory, ok := s.matrixGate.(matrixIngressDirectory)
	if !ok {
		return domain.ErrMatrixUnavailable
	}
	return s.withMatrixAuthority(ctx, request.Scope.Subject, request.Scope.Product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		authorization, err := verifyMatrixStorageActionTx(ctx, tx, scope, proof.Proof.Action, proof.Proof, directory)
		if err != nil {
			return err
		}
		if err := matrixAuthorityOwnsTarget(ctx, tx, scope, request.Handoff.Previous.Target); err != nil {
			return err
		}
		return fn(ctx, tx, authorization)
	})
}

// PlanMatrixStorageDeviceFence commits the original public evidence BEFORE a
// service may revoke the original device. A failed COMMIT never authorizes IO.
func (s *Store) PlanMatrixStorageDeviceFence(ctx context.Context, proof ingress.ControlProof) (ingress.DeviceFence, error) {
	var result ingress.DeviceFence
	err := s.withMatrixDeviceFence(ctx, proof, true, func(ctx context.Context, tx pgx.Tx, authorization matrixproof.Authorization) error {
		existing, err := matrixDeviceFenceTx(ctx, tx, proof.Request, authorization.Authority)
		if err == nil {
			result = existing
			return nil
		}
		if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		request := proof.Request
		h := request.Handoff
		result = ingress.DeviceFence{Version: 1, Scope: request.Scope, Epoch: request.Epoch, ID: h.ID, TaskID: h.TaskID, CipherSHA256: h.CipherSHA256, Previous: h.Previous, Target: request.Target, Authority: authorization.Authority, RequestSHA256: authorization.RequestSHA256, Status: "planned"}
		if err := result.Validate(request, authorization.Authority); err != nil {
			return err
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return err
		}
		evidence, err := json.Marshal(matrixDeviceFenceEvidence{Proof: proof.Proof, Authorization: authorization})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_device_fences(tenant,subject,product,epoch,id,request_sha256,fence,evidence) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, request.Scope.Tenant, request.Scope.Subject, request.Scope.Product, request.Epoch, h.ID, result.RequestSHA256, raw, evidence)
		return err
	})
	return result, err
}

func matrixDeviceFenceTx(ctx context.Context, tx pgx.Tx, request ingress.ControlRequest, authority matrixproof.Authority) (ingress.DeviceFence, error) {
	var result ingress.DeviceFence
	var raw []byte
	var digest, status string
	var confirmed *time.Time
	scope := request.Scope
	err := tx.QueryRow(ctx, `SELECT request_sha256,fence,status,confirmed_at FROM matrix_storage_device_fences WHERE tenant=$1 AND subject=$2 AND product=$3 AND epoch=$4 AND id=$5 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product, request.Epoch, request.Handoff.ID).Scan(&digest, &raw, &status, &confirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, domain.ErrNotFound
	}
	if err != nil {
		return result, err
	}
	if json.Unmarshal(raw, &result) != nil || result.Status != "planned" || result.ConfirmedAt != nil || result.RequestSHA256 != digest {
		return ingress.DeviceFence{}, domain.ErrConflict
	}
	result.Status, result.ConfirmedAt = status, confirmed
	if err := result.Validate(request, authority); err != nil {
		return ingress.DeviceFence{}, err
	}
	return result, nil
}

// ConfirmMatrixStorageDeviceFence holds live authority/identity locks through
// the configured origin's revoke/readback. The plan already survived its own
// COMMIT. A retry of a confirmed plan may check absence but never revoke again.
func (s *Store) ConfirmMatrixStorageDeviceFence(ctx context.Context, proof ingress.ControlProof, verify func(context.Context, storage.Scope, ingress.Assignment, bool) error) (ingress.DeviceFence, error) {
	if verify == nil {
		return ingress.DeviceFence{}, domain.ErrMatrixUnavailable
	}
	var result ingress.DeviceFence
	err := s.withMatrixDeviceFence(ctx, proof, true, func(ctx context.Context, tx pgx.Tx, authorization matrixproof.Authorization) error {
		var err error
		result, err = matrixDeviceFenceTx(ctx, tx, proof.Request, authorization.Authority)
		if err != nil {
			return err
		}
		pending, err := matrixPublicationPendingTx(ctx, tx, result.Scope, result.Previous.Target.DeviceID)
		if err != nil {
			return err
		}
		if pending {
			return domain.ErrConflict
		}
		if err := verify(ctx, result.Scope, result.Previous, result.Status == "planned"); err != nil {
			return err
		}
		if result.Status == "confirmed" {
			return nil
		}
		scope := result.Scope
		err = tx.QueryRow(ctx, `UPDATE matrix_storage_device_fences SET status='confirmed',confirmed_at=clock_timestamp() WHERE tenant=$1 AND subject=$2 AND product=$3 AND epoch=$4 AND id=$5 AND status='planned' RETURNING confirmed_at`, scope.Tenant, scope.Subject, scope.Product, result.Epoch, result.ID).Scan(&result.ConfirmedAt)
		if err != nil {
			return err
		}
		result.Status = "confirmed"
		return result.Validate(proof.Request, authorization.Authority)
	})
	return result, err
}

// ReadMatrixStorageDeviceFence is read-only: planned evidence alone cannot
// trigger revocation, and a confirmed timestamp cannot replace origin readback.
func (s *Store) ReadMatrixStorageDeviceFence(ctx context.Context, proof ingress.ControlProof, absent func(context.Context, storage.Scope, storage.PublicationTarget) error) (ingress.DeviceFence, error) {
	if absent == nil {
		return ingress.DeviceFence{}, domain.ErrMatrixUnavailable
	}
	var result ingress.DeviceFence
	err := s.withMatrixDeviceFence(ctx, proof, false, func(ctx context.Context, tx pgx.Tx, authorization matrixproof.Authorization) error {
		var err error
		result, err = matrixDeviceFenceTx(ctx, tx, proof.Request, authorization.Authority)
		if err != nil {
			return err
		}
		if result.Status == "confirmed" {
			return absent(ctx, result.Scope, result.Previous.Target)
		}
		return nil
	})
	return result, err
}
