// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Aprobaciones de dos personas para lo irreversible (mig 0046): suspender una
// organización y fusionar. La caducidad se evalúa contra `now` EN LA CONSULTA,
// nunca reescribiendo filas — mismo criterio que el PIM (0043).

// PlatformApproval es una solicitud de operación irreversible.
type PlatformApproval struct {
	ID             string
	Action         string
	Payload        []byte // jsonb crudo; lo interpreta quien conoce la acción
	Reason         string
	RequestedBy    string
	RequesterEmail string
	Status         string
	DecidedBy      string
	DeciderEmail   string
	DecidedAt      *time.Time
	Result         string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// EffectiveStatus aplica la caducidad perezosa: una pendiente pasada de fecha se
// reporta 'expired', que es como se comporta.
func (a PlatformApproval) EffectiveStatus(now time.Time) string {
	if a.Status == "pending" && !a.ExpiresAt.After(now) {
		return "expired"
	}
	return a.Status
}

const approvalCols = `a.id, a.action, a.payload, a.reason, a.requested_by,
	coalesce(r.email, ''), a.status, coalesce(a.decided_by::text, ''),
	coalesce(d.email, ''), a.decided_at, a.result, a.created_at, a.expires_at`

const approvalFrom = ` FROM platform_approvals a
	LEFT JOIN users r ON r.id = a.requested_by
	LEFT JOIN users d ON d.id = a.decided_by`

func scanApproval(row pgx.Row) (PlatformApproval, error) {
	var a PlatformApproval
	err := row.Scan(&a.ID, &a.Action, &a.Payload, &a.Reason, &a.RequestedBy,
		&a.RequesterEmail, &a.Status, &a.DecidedBy, &a.DeciderEmail, &a.DecidedAt,
		&a.Result, &a.CreatedAt, &a.ExpiresAt)
	return a, err
}

// CreatePlatformApproval registra una solicitud pendiente.
func (s *Store) CreatePlatformApproval(ctx context.Context, q pg.Querier, action string, payload []byte, reason, requestedBy string, now time.Time, ttl time.Duration) (PlatformApproval, error) {
	if q == nil {
		q = s.DB()
	}
	const sql = `INSERT INTO platform_approvals
		(action, payload, reason, requested_by, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	var id string
	if err := q.QueryRow(ctx, sql, action, payload, reason, requestedBy, now, now.Add(ttl)).
		Scan(&id); err != nil {
		return PlatformApproval{}, errf("CreatePlatformApproval", err)
	}
	return s.GetPlatformApproval(ctx, id)
}

// GetPlatformApproval devuelve una solicitud por id.
func (s *Store) GetPlatformApproval(ctx context.Context, id string) (PlatformApproval, error) {
	a, err := scanApproval(s.DB().QueryRow(ctx,
		`SELECT `+approvalCols+approvalFrom+` WHERE a.id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlatformApproval{}, domain.ErrNotFound
		}
		return PlatformApproval{}, errf("GetPlatformApproval", err)
	}
	return a, nil
}

// DecidePlatformApproval cierra una solicitud PENDIENTE Y VIVA, de forma atómica.
//
// La transición va en el UPDATE y no en una lectura previa: dos aprobaciones
// simultáneas de la misma solicitud ejecutarían la fusión dos veces si primero
// se comprobara y luego se escribiera. El segundo se encuentra 0 filas y sabe
// que llegó tarde. La caducidad entra en el WHERE por lo mismo.
//
// Devuelve false si ya estaba decidida, caducada o no existe.
func (s *Store) DecidePlatformApproval(ctx context.Context, q pg.Querier, id, status, decidedBy, result string, now time.Time) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE platform_approvals
		SET status = $2, decided_by = $3, decided_at = $4, result = $5
		WHERE id = $1 AND status = 'pending' AND expires_at > $4`,
		id, status, decidedBy, now, result)
	if err != nil {
		return false, errf("DecidePlatformApproval", err)
	}
	return tag.RowsAffected() > 0, nil
}

// SetPlatformApprovalResult anota cómo acabó una solicitud ya ejecutada. Se usa
// cuando la operación termina después de decidirla (una fusión corre en segundo
// plano) y no cambia el estado.
func (s *Store) SetPlatformApprovalResult(ctx context.Context, q pg.Querier, id, result string) error {
	if q == nil {
		q = s.DB()
	}
	_, err := q.Exec(ctx, `UPDATE platform_approvals SET result = $2 WHERE id = $1`, id, result)
	return errf("SetPlatformApprovalResult", err)
}

// ListPlatformApprovals devuelve el histórico reciente, lo pendiente primero.
// `soloVivas` filtra a las que aún se pueden aprobar, que es lo que el panel
// necesita para decir «hay dos esperándote».
func (s *Store) ListPlatformApprovals(ctx context.Context, soloVivas bool, now time.Time, limit int) ([]PlatformApproval, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	sql := `SELECT ` + approvalCols + approvalFrom
	args := []any{limit}
	if soloVivas {
		args = append(args, now)
		sql += ` WHERE a.status = 'pending' AND a.expires_at > $2`
	}
	sql += ` ORDER BY a.created_at DESC LIMIT $1`
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, errf("ListPlatformApprovals", err)
	}
	defer rows.Close()
	var out []PlatformApproval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, errf("ListPlatformApprovals.scan", err)
		}
		out = append(out, a)
	}
	return out, errf("ListPlatformApprovals.iter", rows.Err())
}
