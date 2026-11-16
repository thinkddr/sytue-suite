// SPDX-License-Identifier: Apache-2.0
package pg

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/apps/nucleo/storage/ingress"
)

func privateIngressEnrollmentError(err error) error {
	var failure *pgconn.PgError
	if errors.As(err, &failure) && failure.Code == "PIG01" {
		return ingress.ErrEnrollmentEpoch
	}
	return err
}

// The caller has already bound the authenticated scope. This table intentionally
// exposes only the canonical epoch across that owner's scope, not key material.
func requirePrivateIngressEpoch(ctx context.Context, tx pgx.Tx, cfg PrivateIngressConfig) error {
	var accepted uuid.UUID
	err := tx.QueryRow(ctx, `SELECT epoch FROM private_ingress_scopes WHERE tenant=$1 AND owner_sub=$2 AND product=$3 FOR SHARE`, cfg.Scope.Tenant, cfg.Scope.Subject, cfg.Scope.Product).Scan(&accepted)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if accepted != cfg.Epoch {
		return ingress.ErrEnrollmentEpoch
	}
	return nil
}
