// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress/agecipher"
)

// ReadPrivateIngressEnrollment reads the highest retained generation, without
// filtering revoked keys or allocating a generation. RLS and the exact epoch are
// bound before the query; a database failure can never become an absent result.
func ReadPrivateIngressEnrollment(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig) (ingress.EnrollmentState, error) {
	var result ingress.EnrollmentState
	if err := BindPrivateIngressScope(ctx, tx, cfg); err != nil {
		return result, err
	}
	if err := requirePrivateIngressEpoch(ctx, tx, cfg); err != nil {
		return result, err
	}
	result = ingress.EnrollmentState{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String()}
	plan := ingress.KeyPlan{Version: 1, Scope: cfg.Scope, Epoch: cfg.Epoch.String()}
	var revoked *time.Time
	err := tx.QueryRow(ctx, `SELECT key_generation,key_id,profile,public_recipient,revoked_at FROM private_ingress_key_plans WHERE tenant=$1 AND owner_sub=$2 AND product=$3 AND epoch=$4 ORDER BY key_generation DESC LIMIT 1 FOR SHARE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product, cfg.Epoch).Scan(&plan.Generation, &plan.KeyID, &plan.Profile, &plan.Recipient, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		result.Status = "absent"
		return result, result.Validate(cfg.Scope, cfg.Epoch.String())
	}
	if err != nil {
		return ingress.EnrollmentState{}, err
	}
	if err := agecipher.ValidateKeyPlan(plan); err != nil {
		return ingress.EnrollmentState{}, err
	}
	result.Generation = plan.Generation
	if revoked != nil {
		result.Status = "revoked"
	} else {
		result.Status = "active"
		result.Plan = &plan
	}
	return result, result.Validate(cfg.Scope, cfg.Epoch.String())
}

func (q PrivateIngressQueue) Enrollment(ctx context.Context, scope storage.Scope, epoch string) (ingress.EnrollmentState, error) {
	var result ingress.EnrollmentState
	if q.Transaction == nil || !scope.Valid() || !storage.MigrationEpoch(epoch) {
		return result, ingress.ErrContract
	}
	err := q.Transaction(ctx, scope, epoch, "enrollment", func(tx pgx.Tx, cfg AuthorizedPrivateIngressConfig) error {
		if cfg.Scope != scope || cfg.Epoch.String() != epoch {
			return ingress.ErrContract
		}
		if cfg.Authority == nil {
			return ingress.ErrRevoked
		}
		authority, err := cfg.Authority.ReadStorageAuthority(ctx, scope)
		if err != nil {
			return err
		}
		if authority.RequireActive(scope, authority.Generation) != nil {
			return ingress.ErrRevoked
		}
		result, err = ReadPrivateIngressEnrollment(ctx, tx, cfg.PrivateIngressConfig)
		return err
	})
	if err != nil {
		return ingress.EnrollmentState{}, err
	}
	return result, result.Validate(scope, epoch)
}
