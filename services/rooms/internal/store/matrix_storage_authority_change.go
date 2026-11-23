// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/matrixproof"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

func retainMatrixRootChallengeTx(ctx context.Context, tx pgx.Tx, request matrixproof.RootEnrollment, kind, epoch string, authorityGeneration int64) (matrixproof.RootChallenge, error) {
	var challenge matrixproof.RootChallenge
	scope := request.Scope
	var pending int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM matrix_storage_root_consents WHERE tenant=$1 AND subject=$2 AND product=$3 AND consumed_at IS NULL AND expires_at>now()`, scope.Tenant, scope.Subject, scope.Product).Scan(&pending); err != nil {
		return challenge, err
	}
	if pending >= 32 {
		return challenge, domain.ErrConflict
	}
	digest, err := matrixproof.RequestDigest(request)
	if err != nil {
		return challenge, err
	}
	now := time.Now().UTC()
	challenge.Request = request
	challenge.Action = matrixproof.Action{Version: 1, Generation: authorityGeneration, Scope: scope, Epoch: epoch, Kind: kind, Target: request.Target, RootKey: request.RootKey, RequestSHA256: digest, Nonce: uuid.NewString(), IssuedAt: now.UnixMilli(), ExpiresAt: now.Add(5 * time.Minute).UnixMilli()}
	action, err := json.Marshal(challenge.Action)
	if err != nil {
		return challenge, err
	}
	rawRequest, err := json.Marshal(request)
	if err != nil {
		return challenge, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_root_consents(tenant,subject,product,nonce,action,request,expires_at) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb,$7)`, scope.Tenant, scope.Subject, scope.Product, challenge.Action.Nonce, action, rawRequest, time.UnixMilli(challenge.Action.ExpiresAt))
	return challenge, err
}

func (s *Store) MatrixRootChangeChallenge(ctx context.Context, subject, product, kind, epoch string, target storage.PublicationTarget) (matrixproof.RootChallenge, error) {
	var challenge matrixproof.RootChallenge
	if !storage.MigrationEpoch(epoch) || (kind != "root.resume" && kind != "root.rotate") {
		return challenge, domain.ErrInvalidInput
	}
	directory, ok := s.matrixGate.(matrixIngressDirectory)
	if !ok {
		return challenge, domain.ErrMatrixUnavailable
	}
	err := s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		if err := matrixAuthorityOwnsTarget(ctx, tx, scope, target); err != nil {
			return err
		}
		current, err := matrixAuthorityTx(ctx, tx, scope)
		if err != nil {
			return err
		}
		if (kind == "root.resume" && current.Status != "revoked") || (kind == "root.rotate" && current.Status != "active") {
			return domain.ErrConflict
		}
		if err := sinTraspasoTx(ctx, tx, scope, domain.ErrConflict); err != nil {
			return err
		}
		origin, err := directory.IngressDeviceOrigin(ctx, scope, target)
		if err != nil {
			return err
		}
		root, err := matrixproof.EnrollmentRoot(origin)
		if err != nil {
			return err
		}
		request := matrixproof.RootEnrollment{Version: 1, Scope: scope, UserID: origin.UserID.String(), Target: target, RootKey: root.String(), Generation: current.Generation + 1, PreviousGeneration: current.Generation, PreviousRootKey: current.RootKey}
		challenge, err = retainMatrixRootChallengeTx(ctx, tx, request, kind, epoch, current.Generation)
		return err
	})
	return challenge, err
}

func previousMatrixAuthorityTx(ctx context.Context, tx pgx.Tx, current matrixproof.Authority, generation int64) (matrixproof.Authority, error) {
	previous := matrixproof.Authority{Version: 1, Scope: current.Scope, UserID: current.UserID, Generation: generation}
	err := tx.QueryRow(ctx, `SELECT master_key,status FROM matrix_storage_authority_history WHERE tenant=$1 AND subject=$2 AND product=$3 AND generation=$4`, current.Scope.Tenant, current.Scope.Subject, current.Scope.Product, generation).Scan(&previous.RootKey, &previous.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return previous, domain.ErrConflict
	}
	if err != nil {
		return previous, err
	}
	return previous, previous.Validate(current.Scope)
}

func confirmMatrixRootChangeTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, retained matrixproof.RootChallenge, consumed *time.Time, proof matrixproof.SignedAction, origin matrixproof.Origin) (matrixproof.Authority, error) {
	current, err := matrixAuthorityTx(ctx, tx, scope)
	if err != nil {
		return current, err
	}
	if err := sinTraspasoTx(ctx, tx, scope, domain.ErrConflict); err != nil {
		return current, err
	}
	previous := current
	if consumed != nil {
		previous, err = previousMatrixAuthorityTx(ctx, tx, current, retained.Request.PreviousGeneration)
		if err != nil {
			return current, err
		}
	}
	if err := matrixproof.VerifyRootChangeConsent(ctx, origin, previous, proof, retained, time.Now()); err != nil {
		return current, err
	}
	request := retained.Request
	if consumed != nil {
		if current.Generation != request.Generation || current.RootKey != request.RootKey || current.Status != "active" {
			return current, domain.ErrConflict
		}
		return current, nil
	}
	reason := "rotate"
	if retained.Action.Kind == "root.resume" {
		reason = "resume"
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_change',$1,true)`, reason); err != nil {
		return current, err
	}
	changed, err := tx.Exec(ctx, `UPDATE matrix_storage_authorities SET master_key=$4,generation=$5,status='active',updated_at=now() WHERE tenant=$1 AND subject=$2 AND product=$3 AND generation=$6 AND master_key=$7 AND status=$8`, scope.Tenant, scope.Subject, scope.Product, request.RootKey, request.Generation, previous.Generation, previous.RootKey, previous.Status)
	if err != nil {
		return current, err
	}
	if changed.RowsAffected() != 1 {
		return current, domain.ErrConflict
	}
	evidence, err := json.Marshal(proof)
	if err != nil {
		return current, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO matrix_storage_authority_history(tenant,subject,product,generation,master_key,status,reason,device_id,evidence) VALUES($1,$2,$3,$4,$5,'active',$6,$7,$8::jsonb)`, scope.Tenant, scope.Subject, scope.Product, request.Generation, request.RootKey, reason, proof.Target.DeviceID, evidence); err != nil {
		return current, err
	}
	current.RootKey, current.Generation, current.Status = request.RootKey, request.Generation, "active"
	return current, nil
}
