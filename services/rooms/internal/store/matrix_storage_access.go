// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// The denial is already durable. Lock identity then tenant, as provisioning does,
// before remote withdrawal. A late retry cannot lock a newly reactivated account.
// This transaction never waits for a channel lock while holding identity locks.
func (s *Store) revokeMatrixTenantStorageAccess(ctx context.Context, subject string) error {
	tenant := tenancy.MustFrom(ctx)
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_identity_access(subject) VALUES($1) ON CONFLICT DO NOTHING`, subject); err != nil {
			return err
		}
		var locked string
		if err := tx.QueryRow(ctx, `SELECT subject FROM matrix_identity_access WHERE subject=$1 FOR UPDATE`, subject).Scan(&locked); err != nil {
			return err
		}
		if err := pg.SetTenant(ctx, tx, tenant.Slug); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM matrix_tenant_access WHERE tenant=$1 AND subject=$2 FOR UPDATE`, tenant.Slug, subject).Scan(&status); err != nil {
			return err
		}
		if status == "active" {
			return nil
		}
		if s.matrixGate == nil {
			return domain.ErrMatrixUnavailable
		}
		return s.matrixGate.RevokeStorageAccess(ctx, subject, tenant.Slug)
	})
}
