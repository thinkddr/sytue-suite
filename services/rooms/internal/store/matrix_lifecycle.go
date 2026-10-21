// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

type matrixTxKey struct{}

func matrixContext(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, matrixTxKey{}, tx)
}

// A personal block affects every two-person DM across organizations. The same
// maintenance transaction updates Matrix's write policy before committing it.
func (s *Store) reconcileMatrixBlockPairTx(ctx context.Context, tx pgx.Tx, actor, other string) error {
	type binding struct {
		tenant string
		id     uuid.UUID
	}
	var bindings []binding
	rows, err := tx.Query(ctx, `SELECT c.tenant,c.id FROM channels c WHERE c.kind='direct'
 AND EXISTS(SELECT 1 FROM channel_members cm WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$1)
 AND EXISTS(SELECT 1 FROM channel_members cm WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$2)
 AND (EXISTS(SELECT 1 FROM matrix_channel_rooms mr WHERE mr.tenant=c.tenant AND mr.channel_id=c.id)
 OR EXISTS(SELECT 1 FROM rooms r WHERE r.tenant=c.tenant AND r.status<>'ended' AND r.metadata->'source'->>'kind'='chat' AND r.metadata->'source'->>'ref_id'=c.id::text))
 ORDER BY c.tenant,c.id`, actor, other)
	if err != nil {
		return err
	}
	for rows.Next() {
		var b binding
		if err := rows.Scan(&b.tenant, &b.id); err != nil {
			rows.Close()
			return err
		}
		bindings = append(bindings, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, b := range bindings {
		if err := pg.SetTenant(ctx, tx, b.tenant); err != nil {
			return err
		}
		var pair bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM channels c WHERE c.tenant=$1 AND c.id=$2 AND c.kind='direct'
 AND (SELECT count(*) FROM channel_members cm WHERE cm.tenant=c.tenant AND cm.channel_id=c.id)=2
 AND EXISTS(SELECT 1 FROM channel_members cm WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$3)
 AND EXISTS(SELECT 1 FROM channel_members cm WHERE cm.tenant=c.tenant AND cm.channel_id=c.id AND cm.subject=$4))`, b.tenant, b.id, actor, other).Scan(&pair); err != nil {
			return err
		}
		if !pair {
			continue
		}
		if err := lockMatrixChannelTx(ctx, tx, b.tenant, b.id); err != nil {
			return err
		}
		if err := s.revokeMatrixTx(ctx, tx, b.tenant, b.id); err != nil {
			return err
		}
	}
	if t, err := tenancy.From(ctx); err == nil {
		return pg.SetTenant(ctx, tx, t.Slug)
	}
	return nil
}

// WithMatrixIdentity shares the channel transaction when provisioning invites.
// This avoids acquiring a second pool connection while a channel is locked.
func (s *Store) WithMatrixIdentity(ctx context.Context, subject string, fn func(context.Context) error) error {
	apply := func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_identity_access(subject) VALUES($1) ON CONFLICT DO NOTHING`, subject); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM matrix_identity_access WHERE subject=$1 FOR UPDATE`, subject).Scan(&status); err != nil {
			return err
		}
		if status != "active" {
			return domain.ErrForbidden
		}
		// Serialize provisioning with organization revocation too. Otherwise a
		// room created after the revoker's mapping snapshot could retain access.
		if tenant, err := tenancy.From(ctx); err == nil {
			if err := pg.SetTenant(ctx, tx, tenant.Slug); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO matrix_tenant_access(tenant,subject,status,version,applied) VALUES($1,$2,'active',0,true) ON CONFLICT DO NOTHING`, tenant.Slug, subject); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT status FROM matrix_tenant_access WHERE tenant=$1 AND subject=$2 FOR UPDATE`, tenant.Slug, subject).Scan(&status); err != nil {
				return err
			}
			if status != "active" {
				return domain.ErrForbidden
			}
		}
		return fn(ctx)
	}
	if tx, ok := ctx.Value(matrixTxKey{}).(pgx.Tx); ok {
		return apply(tx)
	}
	return s.pool.InMaintenanceTx(ctx, apply)
}

// ApplyMatrixAccountAccess orders lifecycle commands by ID's monotonic version.
// The remote suspend happens under the identity row lock. Room revocation runs
// after that commit, so it never holds an identity lock while waiting for channels.
func (s *Store) ApplyMatrixAccountAccess(ctx context.Context, subject, status string, version int64, apply func(context.Context) error) error {
	needsRevoke := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_identity_access(subject) VALUES($1) ON CONFLICT DO NOTHING`, subject); err != nil {
			return err
		}
		var current int64
		var oldStatus string
		var applied bool
		if err := tx.QueryRow(ctx, `SELECT version,status,applied FROM matrix_identity_access WHERE subject=$1 FOR UPDATE`, subject).Scan(&current, &oldStatus, &applied); err != nil {
			return err
		}
		if version < current || version == current && oldStatus == status && applied {
			return nil
		}
		if version == current && oldStatus != status {
			return domain.ErrConflict
		}
		if err := apply(ctx); err != nil {
			return err
		}
		needsRevoke = status != "active"
		_, err := tx.Exec(ctx, `UPDATE matrix_identity_access SET status=$2,version=$3,applied=$4 WHERE subject=$1`, subject, status, version, !needsRevoke)
		return err
	})
	if err != nil || !needsRevoke {
		return err
	}
	if err := s.revokeMatrixIdentityRooms(ctx, subject); err != nil {
		return err
	}
	if err := s.revokeSubjectCalls(ctx, subject, ""); err != nil {
		return err
	}
	if err := s.revokeMatrixDraftRooms(ctx, subject, ""); err != nil {
		return err
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_identity_access SET applied=true WHERE subject=$1 AND version=$2`, subject, version)
		return err
	})
}

func (s *Store) revokeMatrixIdentityRooms(ctx context.Context, subject string) error {
	type binding struct {
		tenant string
		id     uuid.UUID
	}
	var bindings []binding
	if err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT mr.tenant,mr.channel_id FROM matrix_channel_rooms mr JOIN channels c ON c.tenant=mr.tenant AND c.id=mr.channel_id
 WHERE EXISTS(SELECT 1 FROM channel_members cm WHERE cm.tenant=mr.tenant AND cm.channel_id=mr.channel_id AND cm.subject=$1)
 OR (c.kind='public' AND EXISTS(SELECT 1 FROM team_members tm WHERE tm.tenant=c.tenant AND tm.team_id=c.team_id AND tm.subject=$1))
 ORDER BY mr.tenant,mr.channel_id`, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b binding
			if err := rows.Scan(&b.tenant, &b.id); err != nil {
				return err
			}
			bindings = append(bindings, b)
		}
		return rows.Err()
	}); err != nil {
		return err
	}
	for _, b := range bindings {
		err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(ctx, tx, b.tenant); err != nil {
				return err
			}
			if err := lockMatrixChannelTx(ctx, tx, b.tenant, b.id); err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					return nil
				}
				return err
			}
			var status string
			if err := tx.QueryRow(ctx, `SELECT status FROM matrix_identity_access WHERE subject=$1`, subject).Scan(&status); err != nil {
				return err
			}
			if status == "active" {
				return nil
			}
			return s.revokeMatrixTx(ctx, tx, b.tenant, b.id)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Tenant denials are durable even when an ID transaction later rolls back. That
// safe overblocking is cleared only by a newer, proven active lifecycle command.
//
//nolint:gocognit,gocyclo // durable tenant access lifecycle application enforces sequential versions and durable overblocking
func (s *Store) ApplyMatrixTenantAccess(ctx context.Context, subject, status string, version int64, proof func(context.Context) error) error {
	t := tenancy.MustFrom(ctx)
	needsRevoke := false
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO matrix_tenant_access(tenant,subject,status,version,applied) VALUES($1,$2,'active',0,true) ON CONFLICT DO NOTHING`, t.Slug, subject); err != nil {
			return err
		}
		var current int64
		var oldStatus string
		var applied bool
		if err := tx.QueryRow(ctx, `SELECT version,status,applied FROM matrix_tenant_access WHERE tenant=$1 AND subject=$2 FOR UPDATE`, t.Slug, subject).Scan(&current, &oldStatus, &applied); err != nil {
			return err
		}
		if version < current || version == current && oldStatus == status && applied {
			return nil
		}
		if version == current && oldStatus != status {
			return domain.ErrConflict
		}
		if status == "active" {
			if err := proof(ctx); err != nil {
				return err
			}
		}
		needsRevoke = status != "active"
		_, err := tx.Exec(ctx, `UPDATE matrix_tenant_access SET status=$3,version=$4,applied=$5 WHERE tenant=$1 AND subject=$2`, t.Slug, subject, status, version, !needsRevoke)
		return err
	})
	if err != nil || !needsRevoke {
		return err
	}
	err = s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		rows, err := tx.Query(ctx, `SELECT c.id FROM channels c JOIN matrix_channel_rooms m ON m.tenant=c.tenant AND m.channel_id=c.id WHERE c.tenant=$1 ORDER BY c.id FOR UPDATE OF c`, t.Slug)
		if err != nil {
			return err
		}
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM matrix_tenant_access WHERE tenant=$1 AND subject=$2`, t.Slug, subject).Scan(&status); err != nil {
			return err
		}
		if status == "active" {
			return nil
		}
		for _, id := range ids {
			if err := s.revokeMatrixTx(ctx, tx, t.Slug, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.revokeSubjectCalls(ctx, subject, t.Slug); err != nil {
		return err
	}
	if err := s.revokeMatrixDraftRooms(ctx, subject, t.Slug); err != nil {
		return err
	}
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE matrix_tenant_access SET applied=true WHERE tenant=$1 AND subject=$2 AND version=$3`, t.Slug, subject, version)
		return err
	})
}
