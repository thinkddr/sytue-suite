// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

type CallAccessRevoker = callaccess.Revoker

// Enabling the persistent guard is monotonic. Older unconfigured replicas fail
// their deferred commit constraint instead of bypassing the new security policy.
func (s *Store) ConfigureCallAccess(ctx context.Context, enabled bool, revoker CallAccessRevoker) error {
	guarded := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT enabled FROM doc_call_guard_state WHERE singleton FOR UPDATE`).Scan(&guarded); err != nil {
			return err
		}
		if enabled || guarded {
			if revoker == nil {
				return domain.ErrCallAccessUnavailable
			}
			guarded = true
			_, err := tx.Exec(ctx, `UPDATE doc_call_guard_state SET enabled=true WHERE singleton`)
			return err
		}
		return nil
	})
	if err == nil {
		if guarded {
			s.callAccess = revoker
		}
		s.documentCallProofEnabled = enabled
	}
	return err
}

// SetCallIdentitySource supplies the private, current ID membership authority.
func (s *Store) SetCallIdentitySource(source callaccess.IdentityReader) { s.callIdentity = source }

func (s *Store) flushCallRevocations(ctx context.Context, tx pgx.Tx) error {
	if s.callAccess == nil {
		return nil
	}
	return callaccess.Flush(ctx, tx, tenancy.MustFrom(ctx).Slug, s.callAccess)
}

// Calls use live ID membership and both identity/document fences. JWT groups
// never authorize this surface; stale sessions cannot restore a removed grant.
func (s *Store) DocumentCallAccess(ctx context.Context, id, actor string) (domain.DocumentCallAccess, error) {
	out := domain.DocumentCallAccess{DocumentID: id, Actor: actor}
	if s.callAccess == nil || !s.documentCallProofEnabled || s.callIdentity == nil {
		return out, domain.ErrCallAccessUnavailable
	}
	identity, err := s.callIdentity.ReadCallIdentity(ctx, actor, tenancy.MustFrom(ctx).Slug)
	if err != nil {
		return out, domain.ErrCallAccessUnavailable
	}
	if !identity.Active {
		return out, domain.ErrForbidden
	}
	err = s.tenantTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = documentCallAccessTx(ctx, tx, id, actor, identity)
		return err
	})
	if err != nil {
		return out, err
	}
	reader, ok := s.callAccess.(callaccess.FenceReader)
	if !ok {
		return out, nil
	}
	minimum, err := reader.MinimumDocumentVersion(ctx, id)
	if err != nil {
		return out, domain.ErrCallAccessUnavailable
	}
	if out.Version >= minimum {
		return out, nil
	}
	// The query above is finished before calling Rooms: never hold a Docs SQL
	// transaction while reading the remote fence. Recovery acquires the same
	// document lock as ACL changes, then issues a fresh non-reused sequence value.
	err = s.documentTx(ctx, id, func(tx pgx.Tx) error {
		current, err := documentCallAccessTx(ctx, tx, id, actor, identity)
		if err != nil {
			return err
		}
		if current.Version >= minimum {
			out = current
			return nil
		}
		if _, err := tx.Exec(ctx, `SELECT docs_record_call_fence($1,$2)`, id, current.Tenant); err != nil {
			return err
		}
		out, err = documentCallAccessTx(ctx, tx, id, actor, identity)
		if err != nil {
			return err
		}
		if out.Version <= minimum {
			return domain.ErrCallAccessUnavailable
		}
		return nil
	})
	return out, err
}

func documentCallAccessTx(ctx context.Context, tx pgx.Tx, id, actor string, identity callaccess.IdentityProof) (domain.DocumentCallAccess, error) {
	out := domain.DocumentCallAccess{DocumentID: id, Actor: actor, IdentityVersion: identity.Version, OrganizationAdmin: identity.OrganizationAdmin}
	var until *time.Time
	err := tx.QueryRow(ctx, `WITH eligible AS (
 SELECT d.id,d.tenant,d.owner_sub,COALESCE(f.version,0) AS version,p.role AS direct_role,l.expires_at,
 EXISTS(SELECT 1 FROM doc_permissions gp WHERE gp.document_id=d.id AND docs_permiso_vivo(gp.via_link,gp.bloqueado_desde)
 AND $4 AND ((gp.principal_kind='group' AND gp.principal_id=ANY($3::text[])) OR (gp.principal_kind='tenant' AND $4 AND gp.principal_id=d.tenant))) AS collective
 FROM documents d LEFT JOIN doc_call_access_fences f ON f.document_id=d.id
 LEFT JOIN doc_permissions p ON p.document_id=d.id AND p.principal_kind='user' AND p.principal_id=$2 AND docs_permiso_vivo(p.via_link,p.bloqueado_desde)
 LEFT JOIN doc_share_links l ON l.id=p.via_link WHERE d.id=$1 AND d.deleted_at IS NULL)
 SELECT tenant,version,CASE WHEN owner_sub=$2 OR collective THEN NULL ELSE expires_at END FROM eligible
 WHERE owner_sub=$2 OR direct_role IS NOT NULL OR collective`, id, actor, identity.Groups, identity.Member).Scan(&out.Tenant, &out.Version, &until)
	if isNoRows(err) {
		return out, domain.ErrForbidden
	}
	if until != nil {
		out.ExpiresAtMS = until.UnixMilli()
	}
	return out, err
}
