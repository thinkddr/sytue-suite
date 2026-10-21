// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// ConfigureCallIdentity makes the SQL commit guard persistent once calls are enabled.
func (s *Store) ConfigureCallIdentity(ctx context.Context, enabled bool, revoker callaccess.SubjectRevoker) error {
	guarded := false
	err := s.db.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT enabled FROM identity_call_guard_state WHERE singleton FOR UPDATE`).Scan(&guarded); err != nil {
			return err
		}
		if !enabled && !guarded {
			return nil
		}
		if revoker == nil {
			return callaccess.ErrUnavailable
		}
		guarded = true
		_, err := tx.Exec(ctx, `UPDATE identity_call_guard_state SET enabled=true WHERE singleton`)
		return err
	})
	if err == nil {
		if guarded {
			s.callIdentity = revoker
		}
		s.callIdentityProofEnabled = enabled
	}
	return err
}

func (s *Store) needsCallIdentityTransaction(q pg.Querier) bool {
	if q == nil {
		return true
	}
	if s.callIdentity == nil {
		return false
	}
	_, ok := q.(pgx.Tx)
	return !ok
}

func (s *Store) flushCallIdentity(ctx context.Context, tx pgx.Tx) error {
	if s.callIdentity == nil {
		return nil
	}
	return callaccess.FlushIdentity(ctx, tx, s.callIdentity)
}

// CallIdentity reads ID's current, consistently versioned authorization metadata.
func (s *Store) CallIdentity(ctx context.Context, subject, tenant string) (callaccess.IdentityProof, error) {
	out := callaccess.IdentityProof{Subject: subject, Tenant: tenant}
	if !s.callIdentityProofEnabled || s.callIdentity == nil {
		return out, callaccess.ErrUnavailable
	}
	err := s.InTx(ctx, func(q pg.Querier) error {
		var err error
		out, err = readCallIdentity(ctx, q, subject, tenant)
		return err
	})
	if err != nil {
		return out, err
	}
	reader, ok := s.callIdentity.(callaccess.SubjectFenceReader)
	if !ok {
		return out, callaccess.ErrUnavailable
	}
	minimum, err := reader.MinimumSubjectVersion(ctx, subject, tenant)
	if err != nil {
		return out, callaccess.ErrUnavailable
	}
	if out.Version >= minimum {
		return out, nil
	}
	// No ID transaction is held during the minimum-version RPC. After a failed
	// revocation, a new non-reused version repairs the safe overblock before proof.
	err = s.InTx(ctx, func(q pg.Querier) error {
		if _, err := q.Exec(ctx, `SELECT id_record_call_identity($1,$2)`, subject, tenant); err != nil {
			return err
		}
		var err error
		out, err = readCallIdentity(ctx, q, subject, tenant)
		if err != nil {
			return err
		}
		if out.Version <= minimum {
			return callaccess.ErrUnavailable
		}
		return nil
	})
	return out, err
}

func readCallIdentity(ctx context.Context, q pg.Querier, subject, tenant string) (callaccess.IdentityProof, error) {
	out := callaccess.IdentityProof{Subject: subject, Tenant: tenant}
	err := q.QueryRow(ctx, `SELECT COALESCE(o.status='active' AND m.role IN ('owner','admin'),false),u.status='active',COALESCE(o.status='active' AND m.user_id IS NOT NULL,false),COALESCE(f.version,0),
 CASE WHEN o.status='active' AND m.user_id IS NOT NULL THEN ARRAY(SELECT g.name FROM group_members gm JOIN groups g ON g.id=gm.group_id WHERE gm.user_id=u.id AND g.org_id=o.id ORDER BY g.name) ELSE ARRAY[]::text[] END,
 CASE WHEN o.status='active' AND m.user_id IS NOT NULL THEN ARRAY[m.role::text]||ARRAY(SELECT cr.name FROM user_custom_roles ur JOIN custom_roles cr ON cr.id=ur.role_id AND cr.org_id=ur.org_id WHERE ur.user_id=u.id AND ur.org_id=o.id ORDER BY cr.name) ELSE ARRAY[]::text[] END
 FROM users u LEFT JOIN organizations o ON o.slug=$2 LEFT JOIN memberships m ON m.org_id=o.id AND m.user_id=u.id
 LEFT JOIN identity_call_access_fences f ON f.subject=u.id AND f.org_slug=$2 WHERE u.id=$1`, subject, tenant).Scan(&out.OrganizationAdmin, &out.Active, &out.Member, &out.Version, &out.Groups, &out.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, callaccess.ErrUnavailable
	}
	return out, err
}
