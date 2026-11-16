// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/storage"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// MatrixPublicationCursor is an internal keyset cursor over retained public
// evidence. It never authorizes a publication or accepts client pagination.
type MatrixPublicationCursor struct {
	Scope storage.Scope
	ID    uuid.UUID
}
type MatrixPublicationPending struct {
	Cursor       MatrixPublicationCursor
	Reference    storage.Reference
	MatrixUserID string
}

// PendingMatrixPublicationOrigins reads public routing only. This explicit,
// read-only maintenance policy does not grant cross-owner write access.
func (s *Store) PendingMatrixPublicationOrigins(ctx context.Context, after *MatrixPublicationCursor, limit int) ([]MatrixPublicationPending, *MatrixPublicationCursor, error) {
	if limit < 1 || limit > 64 {
		return nil, nil, domain.ErrInvalidInput
	}
	cursor := MatrixPublicationCursor{}
	if after != nil {
		if !after.Scope.Valid() || after.ID == uuid.Nil {
			return nil, nil, domain.ErrInvalidInput
		}
		cursor = *after
	}
	var result []MatrixPublicationPending
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.matrix_publication_contract','1',true),set_config('app.matrix_publication_reconcile','1',true)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT tenant,subject,product,id,room_id,event_id,matrix_user_id FROM matrix_storage_publications WHERE status='sending' AND (tenant,subject,product,id)>($1,$2,$3,$4) ORDER BY tenant,subject,product,id LIMIT $5`, cursor.Scope.Tenant, cursor.Scope.Subject, cursor.Scope.Product, cursor.ID, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var value MatrixPublicationPending
			if err := rows.Scan(&value.Cursor.Scope.Tenant, &value.Cursor.Scope.Subject, &value.Cursor.Scope.Product, &value.Cursor.ID, &value.Reference.RoomID, &value.Reference.EventID, &value.MatrixUserID); err != nil {
				return err
			}
			if !value.Cursor.Scope.Valid() || value.Cursor.ID == uuid.Nil || !value.Reference.Valid() || value.MatrixUserID == "" {
				return domain.ErrConflict
			}
			result = append(result, value)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, nil, err
	}
	var next *MatrixPublicationCursor
	if len(result) == limit {
		value := result[len(result)-1].Cursor
		next = &value
	}
	return result, next, nil
}
