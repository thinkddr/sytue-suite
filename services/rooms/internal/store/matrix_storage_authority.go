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

type matrixIngressDirectory interface {
	IngressDeviceOrigin(context.Context, storage.Scope, storage.PublicationTarget) (matrixproof.Origin, error)
}

func (s *Store) withMatrixAuthority(ctx context.Context, subject, product string, fn func(context.Context, pgx.Tx, storage.Scope) error) error {
	scope, err := cutoverScope(ctx, subject, product)
	if err != nil {
		return err
	}
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		return s.WithMatrixIdentity(matrixContext(ctx, tx), subject, func(ctx context.Context) error {
			var merge string
			if err := tx.QueryRow(ctx, `SELECT coalesce(current_setting('app.merge',true),'')`).Scan(&merge); err != nil {
				return err
			}
			if merge != "" && merge != "off" && merge != "false" && merge != "0" {
				return domain.ErrForbidden
			}
			if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_owner',$1,true),set_config('app.matrix_authority_product',$2,true)`, scope.Subject, scope.Product); err != nil {
				return err
			}
			return fn(ctx, tx, scope)
		})
	})
}

func matrixAuthorityTx(ctx context.Context, tx pgx.Tx, scope storage.Scope) (matrixproof.Authority, error) {
	out := matrixproof.Authority{Version: 1, Scope: scope}
	err := tx.QueryRow(ctx, `SELECT matrix_user_id,master_key,generation,status FROM matrix_storage_authorities WHERE tenant=$1 AND subject=$2 AND product=$3 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product).Scan(&out.UserID, &out.RootKey, &out.Generation, &out.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, matrixproof.ErrRootEnrollment
	}
	if err != nil {
		return out, err
	}
	return out, out.Validate(scope)
}

func (s *Store) MatrixStorageAuthority(ctx context.Context, subject, product string) (matrixproof.Authority, error) {
	var result matrixproof.Authority
	err := s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		result, err = matrixAuthorityTx(ctx, tx, scope)
		return err
	})
	return result, err
}

func matrixAuthorityOwnsTarget(ctx context.Context, tx pgx.Tx, scope storage.Scope, target storage.PublicationTarget) error {
	if !target.Valid() {
		return domain.ErrInvalidInput
	}
	var owns bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM matrix_storage_isolated_rooms WHERE tenant=$1 AND subject=$2 AND product=$3 AND room_id=$4)`, scope.Tenant, scope.Subject, scope.Product, target.RoomID).Scan(&owns)
	if err != nil {
		return err
	}
	if !owns {
		return domain.ErrForbidden
	}
	return nil
}

// MatrixRootChallenge queries the configured origin before offering a first
// trust decision. It never accepts a proposed master from the request and
// never replaces an existing pin, even for an authenticated account owner.
func (s *Store) MatrixRootChallenge(ctx context.Context, subject, product, epoch string, target storage.PublicationTarget) (matrixproof.RootChallenge, error) {
	var challenge matrixproof.RootChallenge
	if !storage.MigrationEpoch(epoch) {
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
		if _, err := matrixAuthorityTx(ctx, tx, scope); !errors.Is(err, matrixproof.ErrRootEnrollment) {
			if err != nil {
				return err
			}
			return domain.ErrConflict
		}
		origin, err := directory.IngressDeviceOrigin(ctx, scope, target)
		if err != nil {
			return err
		}
		root, err := matrixproof.EnrollmentRoot(origin)
		if err != nil {
			return err
		}
		request := matrixproof.RootEnrollment{Version: 1, Scope: scope, UserID: origin.UserID.String(), Target: target, RootKey: root.String(), Generation: 1}
		challenge, err = retainMatrixRootChallengeTx(ctx, tx, request, "root.enroll", epoch, 1)
		return err
	})
	return challenge, err
}

// ConfirmMatrixRoot consumes the retained first-enrollment challenge in the
// same transaction as the pin and immutable evidence. A JWT or request master
// is insufficient; the independently queried verified device must sign the
// exact public transcript. Retries can only return the identical committed pin.
func (s *Store) ConfirmMatrixRoot(ctx context.Context, subject, product string, proof matrixproof.SignedAction) (matrixproof.Authority, error) {
	var result matrixproof.Authority
	nonce, err := uuid.Parse(proof.Nonce)
	if err != nil || nonce == uuid.Nil || nonce.String() != proof.Nonce {
		return result, domain.ErrInvalidInput
	}
	directory, ok := s.matrixGate.(matrixIngressDirectory)
	if !ok {
		return result, domain.ErrMatrixUnavailable
	}
	err = s.withMatrixAuthority(ctx, subject, product, func(ctx context.Context, tx pgx.Tx, scope storage.Scope) error {
		var err error
		result, err = confirmMatrixRootTx(ctx, tx, scope, nonce, proof, directory)
		return err
	})
	return result, err
}

func matrixRootConsentTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, nonce uuid.UUID) (matrixproof.RootChallenge, *time.Time, error) {
	var rawAction, rawRequest []byte
	var expires time.Time
	var consumed *time.Time
	var retained matrixproof.RootChallenge
	err := tx.QueryRow(ctx, `SELECT action,request,expires_at,consumed_at FROM matrix_storage_root_consents WHERE tenant=$1 AND subject=$2 AND product=$3 AND nonce=$4 FOR UPDATE`, scope.Tenant, scope.Subject, scope.Product, nonce).Scan(&rawAction, &rawRequest, &expires, &consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return retained, nil, domain.ErrNotFound
	}
	if err != nil {
		return retained, nil, err
	}
	if json.Unmarshal(rawAction, &retained.Action) != nil || json.Unmarshal(rawRequest, &retained.Request) != nil || !expires.After(time.Now()) {
		return retained, nil, domain.ErrForbidden
	}
	return retained, consumed, nil
}

func confirmMatrixRootTx(ctx context.Context, tx pgx.Tx, scope storage.Scope, nonce uuid.UUID, proof matrixproof.SignedAction, directory matrixIngressDirectory) (matrixproof.Authority, error) {
	retained, consumed, err := matrixRootConsentTx(ctx, tx, scope, nonce)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if err := matrixAuthorityOwnsTarget(ctx, tx, scope, retained.Action.Target); err != nil {
		return matrixproof.Authority{}, err
	}
	origin, err := directory.IngressDeviceOrigin(ctx, scope, retained.Action.Target)
	if err != nil {
		return matrixproof.Authority{}, err
	}
	if retained.Action.Kind != "root.enroll" {
		result, err := confirmMatrixRootChangeTx(ctx, tx, scope, retained, consumed, proof, origin)
		if err != nil {
			return result, err
		}
		if consumed == nil {
			_, err = tx.Exec(ctx, `UPDATE matrix_storage_root_consents SET consumed_at=now() WHERE tenant=$1 AND subject=$2 AND product=$3 AND nonce=$4`, scope.Tenant, scope.Subject, scope.Product, nonce)
		}
		return result, err
	}
	if err := matrixproof.VerifyRootConsent(ctx, origin, proof, retained, time.Now()); err != nil {
		return matrixproof.Authority{}, err
	}
	current, err := matrixAuthorityTx(ctx, tx, scope)
	if consumed != nil {
		if err != nil {
			return current, err
		}
		if current.Generation != 1 || current.Status != "active" || current.RootKey != retained.Request.RootKey || current.UserID != retained.Request.UserID {
			return current, domain.ErrConflict
		}
		return current, nil
	}
	if !errors.Is(err, matrixproof.ErrRootEnrollment) {
		if err != nil {
			return current, err
		}
		return current, domain.ErrConflict
	}
	result := matrixproof.Authority{Version: 1, Scope: scope, UserID: origin.UserID.String(), RootKey: retained.Request.RootKey, Generation: 1, Status: "active"}
	if err := insertMatrixRootTx(ctx, tx, result, proof); err != nil {
		return matrixproof.Authority{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE matrix_storage_root_consents SET consumed_at=now() WHERE tenant=$1 AND subject=$2 AND product=$3 AND nonce=$4`, scope.Tenant, scope.Subject, scope.Product, nonce)
	return result, err
}

func insertMatrixRootTx(ctx context.Context, tx pgx.Tx, result matrixproof.Authority, proof matrixproof.SignedAction) error {
	scope := result.Scope
	if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_authority_change','enroll',true)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO matrix_storage_authorities(tenant,subject,product,matrix_user_id,master_key,generation,status) VALUES($1,$2,$3,$4,$5,1,'active')`, scope.Tenant, scope.Subject, scope.Product, result.UserID, result.RootKey); err != nil {
		return err
	}
	evidence, err := json.Marshal(proof)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO matrix_storage_authority_history(tenant,subject,product,generation,master_key,status,reason,device_id,evidence) VALUES($1,$2,$3,1,$4,'active','enroll',$5,$6::jsonb)`, scope.Tenant, scope.Subject, scope.Product, result.RootKey, proof.Target.DeviceID, evidence)
	return err
}
