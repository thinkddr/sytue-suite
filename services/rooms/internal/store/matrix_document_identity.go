// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	mediadomain "git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

func lockDocumentIdentityFenceTx(ctx context.Context, tx pgx.Tx, tenant, actor string) (int64, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO matrix_document_identity_access(org_slug,subject) VALUES($1,$2) ON CONFLICT DO NOTHING`, tenant, actor); err != nil {
		return 0, err
	}
	var version int64
	err := tx.QueryRow(ctx, `SELECT version FROM matrix_document_identity_access WHERE org_slug=$1 AND subject=$2 FOR UPDATE`, tenant, actor).Scan(&version)
	return version, err
}

// AdvanceDocumentIdentityAccess removes the actor from document calls before an
// ID group/role change commits; other native channels and drafts are unaffected.
func (s *Store) AdvanceDocumentIdentityAccess(ctx context.Context, actor, tenant string, version int64) error {
	if actor == "" || !tenancy.ValidSlug(tenant) || version < 1 {
		return mediadomain.ErrInvalidName
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		current, err := lockDocumentIdentityFenceTx(ctx, tx, tenant, actor)
		if err != nil {
			return err
		}
		if version <= current {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE matrix_document_identity_access SET version=$3 WHERE org_slug=$1 AND subject=$2`, tenant, actor, version); err != nil {
			return err
		}
		ids, err := documentSubjectCallsTx(ctx, tx, tenant, actor)
		if err != nil {
			return err
		}
		t, err := tenancy.New(tenant)
		if err != nil {
			return err
		}
		ctx = tenancy.Into(ctx, t)
		for _, id := range ids {
			st, err := lockCallTx(ctx, tx, tenant, id)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE matrix_call_members SET removed=true,ready_generation=0 WHERE tenant=$1 AND call_id=$2 AND subject=$3`, tenant, id, actor); err != nil {
				return err
			}
			if err := callMembersTx(ctx, tx, &st); err != nil {
				return err
			}
			if _, err := s.rotateCallTx(ctx, tx, st, true); err != nil {
				return err
			}
		}
		return nil
	})
}

func documentSubjectCallsTx(ctx context.Context, tx pgx.Tx, tenant, actor string) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT r.id FROM rooms r JOIN matrix_call_members m ON m.tenant=r.tenant AND m.call_id=r.id WHERE r.tenant=$1 AND m.subject=$2 AND NOT m.removed AND r.status<>'ended' AND r.metadata->'source'->>'kind'='document' ORDER BY r.metadata->'source'->>'ref_id',r.id`, tenant, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DocumentIdentityAccessVersion reads metadata only, without opening Matrix sessions.
func (s *Store) DocumentIdentityAccessVersion(ctx context.Context, actor, tenant string) (int64, error) {
	var version int64
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM matrix_document_identity_access WHERE org_slug=$1 AND subject=$2),0)`, tenant, actor).Scan(&version)
	})
	return version, err
}
