// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// SCIMToken es un token de aprovisionamiento de UNA organización (mig 0055).
// El secreto no está aquí: solo su hash, y solo se enseña al crearlo.
type SCIMToken struct {
	ID         string     `json:"id"`
	OrgID      string     `json:"org_id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// CreateSCIMToken guarda el hash de un token nuevo y devuelve la fila.
func (s *Store) CreateSCIMToken(ctx context.Context, q pg.Querier, orgID, tokenHash, label, createdBy string) (SCIMToken, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO scim_tokens (org_id, token_hash, label, created_by)
		VALUES ($1::uuid, $2, $3, nullif($4,'')::uuid)
		RETURNING id, org_id, label, created_at, last_used_at`
	var t SCIMToken
	err := q.QueryRow(ctx, sql, orgID, tokenHash, label, createdBy).
		Scan(&t.ID, &t.OrgID, &t.Label, &t.CreatedAt, &t.LastUsedAt)
	return t, errf("CreateSCIMToken", err)
}

// ListSCIMTokens devuelve los tokens VIVOS de una organización.
func (s *Store) ListSCIMTokens(ctx context.Context, orgID string) ([]SCIMToken, error) {
	const sql = `SELECT id, org_id, label, created_at, last_used_at
		FROM scim_tokens WHERE org_id = $1::uuid AND revoked_at IS NULL
		ORDER BY created_at DESC`
	rows, err := s.DB().Query(ctx, sql, orgID)
	if err != nil {
		return nil, errf("ListSCIMTokens", err)
	}
	defer rows.Close()
	var out []SCIMToken
	for rows.Next() {
		var t SCIMToken
		if err := rows.Scan(&t.ID, &t.OrgID, &t.Label, &t.CreatedAt, &t.LastUsedAt); err != nil {
			return nil, errf("ListSCIMTokens.scan", err)
		}
		out = append(out, t)
	}
	return out, errf("ListSCIMTokens.iter", rows.Err())
}

// RevokeSCIMToken corta un token. Acotado a la organización que lo pide: sin
// eso, el id de la URL bastaría para revocar el de otra empresa.
func (s *Store) RevokeSCIMToken(ctx context.Context, q pg.Querier, orgID, id string) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE scim_tokens SET revoked_at = now()
		WHERE id = $1::uuid AND org_id = $2::uuid AND revoked_at IS NULL`, id, orgID)
	if err != nil {
		return false, errf("RevokeSCIMToken", err)
	}
	return tag.RowsAffected() > 0, nil
}

// OrgBySCIMToken resuelve la organización a la que pertenece un token vivo y
// deja anotado el uso.
//
// El sello de uso va en el MISMO UPDATE que la consulta, y no en una escritura
// aparte: cada petición SCIM pasa por aquí, así que dos viajes serían el doble
// de carga en el camino más caliente del aprovisionamiento. `RETURNING` da la
// organización, y si no hay fila el token no vale.
func (s *Store) OrgBySCIMToken(ctx context.Context, tokenHash string) (domain.Organization, error) {
	const sql = `UPDATE scim_tokens SET last_used_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
		RETURNING org_id`
	var orgID string
	if err := s.DB().QueryRow(ctx, sql, tokenHash).Scan(&orgID); err != nil {
		return domain.Organization{}, errf("OrgBySCIMToken", err)
	}
	return s.GetOrgByID(ctx, orgID)
}
