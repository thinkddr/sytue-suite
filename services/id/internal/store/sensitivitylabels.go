// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Taxonomía de etiquetas de sensibilidad (mig 0024). Aislamiento por tenant idéntico
// al resto (docs/CONVENTIONS.md §0.7): CADA método recibe orgID (la org activa del
// token, resuelta en el handler) y lo cruza con sensitivity_labels.org_id en TODA
// consulta. Una etiqueta de otra org no liga (ErrNotFound / 0 filas): deny-by-default,
// sin fuga cross-tenant. El nombre duplicado en la MISMA org da 23505 ⇒ ErrConflict.

const labelCols = `id, org_id, name, rank::int, color, description, block_external_share,
	coalesce(created_by::text,''), created_at, updated_at`

func scanLabel(row pgx.Row) (domain.SensitivityLabel, error) {
	var l domain.SensitivityLabel
	err := row.Scan(&l.ID, &l.OrgID, &l.Name, &l.Rank, &l.Color, &l.Description,
		&l.BlockExternalShare, &l.CreatedBy, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return domain.SensitivityLabel{}, err
	}
	return l, nil
}

// ListSensitivityLabels devuelve la taxonomía de una org de MENOS a MÁS sensible
// (rank, y nombre para desempatar). Sirve tanto a la administración como a la lectura
// que consumen otros servicios. Una org tiene decenas de etiquetas, no miles: sin
// keyset (como ListAccessPolicies). Sirve sensitivity_labels_org_rank_idx.
func (s *Store) ListSensitivityLabels(ctx context.Context, orgID string) ([]domain.SensitivityLabel, error) {
	const sql = `SELECT ` + labelCols + ` FROM sensitivity_labels
		WHERE org_id = $1 ORDER BY rank, lower(name), id`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListSensitivityLabels", err)
	}
	defer rows.Close()
	out := make([]domain.SensitivityLabel, 0, 8)
	for rows.Next() {
		l, err := scanLabel(rows)
		if err != nil {
			return nil, errf("ListSensitivityLabels.scan", err)
		}
		out = append(out, l)
	}
	return out, errf("ListSensitivityLabels.iter", rows.Err())
}

// GetSensitivityLabel recupera una etiqueta POR (org, id). Es la verja de aislamiento:
// un id de otra org no coincide con org_id y da ErrNotFound (404 sin fuga).
func (s *Store) GetSensitivityLabel(ctx context.Context, orgID, id string) (domain.SensitivityLabel, error) {
	const sql = `SELECT ` + labelCols + ` FROM sensitivity_labels WHERE id = $1 AND org_id = $2`
	l, err := scanLabel(s.DB().QueryRow(ctx, sql, id, orgID))
	return l, errf("GetSensitivityLabel", err)
}

// CreateSensitivityLabel inserta una etiqueta en la org dada. Nombre repetido en la
// MISMA org ⇒ ErrConflict (índice único case-insensitive).
func (s *Store) CreateSensitivityLabel(ctx context.Context, q pg.Querier, l domain.SensitivityLabel) (domain.SensitivityLabel, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO sensitivity_labels
		(org_id, name, rank, color, description, block_external_share, created_by)
		VALUES ($1,$2,$3,$4,$5,$6, nullif($7,'')::uuid)
		RETURNING ` + labelCols
	got, err := scanLabel(q.QueryRow(ctx, sql, l.OrgID, l.Name, l.Rank, l.Color,
		l.Description, l.BlockExternalShare, l.CreatedBy))
	return got, errf("CreateSensitivityLabel", err)
}

// UpdateSensitivityLabel actualiza una etiqueta acotada por (org, id): una de otra org
// NO se toca. ErrNotFound si no pertenece a la org.
//
// OJO (intencionado): esto cambia la TAXONOMÍA, no el contenido ya etiquetado. Los
// snapshots que los productos guardaron siguen como estaban — la etiqueta de un
// fichero de hace un año no se reescribe a espaldas de nadie.
func (s *Store) UpdateSensitivityLabel(ctx context.Context, q pg.Querier, l domain.SensitivityLabel) (domain.SensitivityLabel, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `UPDATE sensitivity_labels SET
		name=$3, rank=$4, color=$5, description=$6, block_external_share=$7, updated_at=now()
		WHERE id=$1 AND org_id=$2
		RETURNING ` + labelCols
	got, err := scanLabel(q.QueryRow(ctx, sql, l.ID, l.OrgID, l.Name, l.Rank, l.Color,
		l.Description, l.BlockExternalShare))
	return got, errf("UpdateSensitivityLabel", err)
}

// DeleteSensitivityLabel borra una etiqueta acotada por (org, id). Devuelve true si
// borró una fila. El contenido ya etiquetado NO se toca: conserva su snapshot (y su
// bloqueo), que es justo lo que se espera de un registro de cumplimiento.
func (s *Store) DeleteSensitivityLabel(ctx context.Context, orgID, id string) (bool, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM sensitivity_labels WHERE id=$1 AND org_id=$2`, id, orgID)
	if err != nil {
		return false, errf("DeleteSensitivityLabel", err)
	}
	return tag.RowsAffected() > 0, nil
}
