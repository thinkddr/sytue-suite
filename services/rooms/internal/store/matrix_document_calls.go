// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	mediadomain "git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

func (s *Store) SetDocumentCallsEnabled(enabled bool) { s.documentCallsEnabled = enabled }
func lockDocumentCallFenceTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (int64, error) {
	if _, err := tx.Exec(ctx, `INSERT INTO matrix_document_access(document_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
		return 0, err
	}
	var version int64
	err := tx.QueryRow(ctx, `SELECT version FROM matrix_document_access WHERE document_id=$1 FOR UPDATE`, id).Scan(&version)
	return version, err
}

func (s *Store) requireDocumentCallProof(ctx context.Context, st callState, actor string) error {
	if st.DocumentID == uuid.Nil {
		return nil
	}
	if !s.documentCallsEnabled {
		return mediadomain.ErrEncryptedOperation
	}
	proof, ok := mediadomain.DocumentCallAccessFrom(ctx)
	if !ok || proof.DocumentID != st.DocumentID.String() || proof.Tenant != st.Tenant || proof.Actor != actor || proof.Version < st.DocumentVersion || proof.IdentityVersion < st.IdentityVersion || proof.ExpiresAtMS < 0 || proof.ExpiresAtMS > 0 && proof.ExpiresAtMS <= time.Now().Add(2*time.Second).UnixMilli() {
		return mediadomain.ErrPermission
	}
	return nil
}

// AdvanceDocumentCallAccess is ordered before the Docs transaction commits. A
// concurrent join carrying its old assertion cannot pass the same fence lock.
//
//nolint:gocognit // fence synchronization between docs and rooms transactions requires atomic multi-step validation and eviction
func (s *Store) AdvanceDocumentCallAccess(ctx context.Context, id uuid.UUID, tenant string, version int64) error {
	if id == uuid.Nil || !tenancy.ValidSlug(tenant) || version < 1 {
		return mediadomain.ErrInvalidName
	}
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		current, err := lockDocumentCallFenceTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if version <= current {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE matrix_document_access SET version=$2 WHERE document_id=$1`, id, version); err != nil {
			return err
		}
		// UUID-scoped closure includes calls created before an organization move.
		rows, err := tx.Query(ctx, `SELECT tenant,id FROM rooms WHERE status<>'ended' AND metadata->'source'->>'kind'='document' AND metadata->'source'->>'ref_id'=$1 ORDER BY tenant,id`, id.String())
		if err != nil {
			return err
		}
		type call struct {
			tenant string
			id     uuid.UUID
		}
		var calls []call
		for rows.Next() {
			var c call
			if err := rows.Scan(&c.tenant, &c.id); err != nil {
				rows.Close()
				return err
			}
			calls = append(calls, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, c := range calls {
			t, err := tenancy.New(c.tenant)
			if err != nil {
				return err
			}
			callCtx := tenancy.Into(ctx, t)
			if err := pg.SetTenant(callCtx, tx, c.tenant); err != nil {
				return err
			}
			st, err := lockCallTx(callCtx, tx, c.tenant, c.id)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE matrix_call_members SET removed=true,ready_generation=0 WHERE tenant=$1 AND call_id=$2`, c.tenant, c.id); err != nil {
				return err
			}
			if err := callMembersTx(callCtx, tx, &st); err != nil {
				return err
			}
			if _, err := s.rotateCallTx(callCtx, tx, st, true); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE rooms SET status='ended',ended_at=COALESCE(ended_at,now()),updated_at=now() WHERE tenant=$1 AND id=$2`, c.tenant, c.id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DocumentCallAccessVersion(ctx context.Context, id uuid.UUID) (int64, error) {
	var version int64
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM matrix_document_access WHERE document_id=$1),0)`, id).Scan(&version)
	})
	return version, err
}
