// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// SinMetadatos es una foto indexada sin tipo o sin tamaño.
type SinMetadatos struct {
	ID      uuid.UUID
	DriveID string
}

// SinTipoNiTamanyo lista las fotos de esa persona que se guardaron con el tipo vacío
// o el tamaño a 0, por id y a partir de `despues` (uuid.Nil para empezar).
//
// Son las que indexó el cliente de Drive mientras leía `contentType`/`sizeBytes`
// en vez de `content_type`/`size_bytes`. Incluye las de la papelera: también se
// restauran y se exportan.
func (s *Store) SinTipoNiTamanyo(ctx context.Context, duenyo string, despues uuid.UUID, limite int) ([]SinMetadatos, error) {
	var out []SinMetadatos
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, drive_file_id FROM assets
			WHERE tenant = $1 AND owner_sub = $2 AND (media_type = '' OR bytes = 0) AND id > $3
			ORDER BY id LIMIT $4`, tenant, duenyo, despues, limite)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p SinMetadatos
			if err := rows.Scan(&p.ID, &p.DriveID); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// CompletarTipoYTamanyo rellena el tipo y el tamaño de una foto que no los tiene.
//
// SOLO ESOS DOS CAMPOS, y no un `Ingerir`: el upsert de alta saca la foto de la
// papelera (`deleted_at = NULL`), y reparar los metadatos no puede devolverle a
// nadie una foto que había borrado. Tampoco pisa un valor que ya esté bien.
func (s *Store) CompletarTipoYTamanyo(ctx context.Context, id uuid.UUID, duenyo, tipo string, bytes int64) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		tenant, err := tenantDe(ctx)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE assets SET
			    media_type = CASE WHEN media_type = '' THEN $4 ELSE media_type END,
			    bytes      = CASE WHEN bytes = 0 THEN $5 ELSE bytes END,
			    updated_at = now()
			WHERE tenant = $1 AND id = $2 AND owner_sub = $3 AND (media_type = '' OR bytes = 0)`,
			tenant, id, duenyo, tipo, bytes)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}
