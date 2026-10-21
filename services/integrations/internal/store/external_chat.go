// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

func (s *Store) BindExternalChatActor(ctx context.Context, id, actor string) error {
	if actor == "" {
		return domain.ErrInvalidInput
	}
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE app_instances SET external_chat_actor=$2 WHERE id=$1 AND org_id=current_setting('app.tenant',true) AND created_by=$2 AND (external_chat_actor='' OR external_chat_actor=$2)`, id, actor)
		if err == nil && tag.RowsAffected() != 1 {
			return domain.ErrForbidden
		}
		return err
	})
}

func (s *Store) RequireExternalChatActor(ctx context.Context, id, actor string) error {
	if actor == "" {
		return domain.ErrForbidden
	}
	return s.tenantTx(ctx, func(tx pgx.Tx) error {
		var allowed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_instances WHERE id=$1 AND org_id=current_setting('app.tenant',true) AND external_chat_actor=$2)`, id, actor).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return domain.ErrForbidden
		}
		return nil
	})
}

func (s *Store) ListExternalChatInstances(ctx context.Context, actor string) ([]domain.AppInstance, error) {
	out := []domain.AppInstance{}
	err := s.tenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+instanceCols+` FROM app_instances WHERE org_id=current_setting('app.tenant',true) AND external_chat_actor=$1 ORDER BY created_at DESC,id`, actor)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var inst domain.AppInstance
			if err := scanInstance(rows, &inst); err != nil {
				return err
			}
			out = append(out, inst)
		}
		return rows.Err()
	})
	return out, err
}
