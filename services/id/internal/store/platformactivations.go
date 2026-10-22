// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
)

// Activaciones del alcance de PLATAFORMA (mig 0043). `platform_admins` dice quién
// PUEDE elevarse; esto dice quién está elevado AHORA, hasta cuándo y por qué.
//
// La caducidad se evalúa siempre contra `now` EN LA CONSULTA, nunca reescribiendo
// filas: un estado 'expired' que hay que ir a poner con un barrido es un estado
// que algún día no se pone, y entonces una activación caducada sigue
// autorizando. Mismo criterio que el PIM de organización (mig 0020).

// PlatformActivation es una elevación del alcance global.
type PlatformActivation struct {
	ID              string
	UserID          string
	UserEmail       string
	UserName        string
	Status          string
	Justification   string
	DurationMinutes int
	ActivatedAt     time.Time
	ExpiresAt       time.Time
	RevokedAt       *time.Time
}

// EffectiveStatus aplica la caducidad perezosa: una fila 'active' pasada de
// fecha se reporta 'expired', que es como se comporta de verdad.
func (a PlatformActivation) EffectiveStatus(now time.Time) string {
	if a.Status == "active" && !a.ExpiresAt.After(now) {
		return "expired"
	}
	return a.Status
}

// HasLivePlatformActivation responde la pregunta del gate: ¿está este usuario
// elevado en este instante? Es la consulta que corre en CADA petición de la API
// de plataforma, y por eso va contra el índice parcial de las activas.
//
// Fail-closed por construcción: el error se propaga y el llamador debe denegar.
func (s *Store) HasLivePlatformActivation(ctx context.Context, userID string, now time.Time) (bool, error) {
	const sql = `SELECT EXISTS (
		SELECT 1 FROM platform_activations
		WHERE user_id = $1 AND status = 'active' AND expires_at > $2)`
	var ok bool
	err := s.DB().QueryRow(ctx, sql, userID, now).Scan(&ok)
	return ok, errf("HasLivePlatformActivation", err)
}

// CurrentPlatformActivation devuelve la activación viva del usuario, si la hay.
// El segundo valor dice si existía: sin ella, la primera es el cero.
func (s *Store) CurrentPlatformActivation(ctx context.Context, userID string, now time.Time) (PlatformActivation, bool, error) {
	const sql = `SELECT id, user_id, status, justification, duration_minutes,
		activated_at, expires_at, revoked_at
		FROM platform_activations
		WHERE user_id = $1 AND status = 'active' AND expires_at > $2
		ORDER BY activated_at DESC LIMIT 1`
	var a PlatformActivation
	err := s.DB().QueryRow(ctx, sql, userID, now).Scan(&a.ID, &a.UserID, &a.Status,
		&a.Justification, &a.DurationMinutes, &a.ActivatedAt, &a.ExpiresAt, &a.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlatformActivation{}, false, nil
		}
		return PlatformActivation{}, false, errf("CurrentPlatformActivation", err)
	}
	return a, true, nil
}

// CreatePlatformActivation eleva a un usuario por una ventana acotada.
//
// Cierra ANTES cualquier activación viva suya, en la misma transacción: dos
// elevaciones simultáneas del mismo usuario no significan nada y dejarían el
// histórico diciendo que estuvo elevado dos veces a la vez. Reactivar mientras
// una sigue viva es, en la práctica, extender la ventana con un motivo nuevo, y
// así queda escrito.
func (s *Store) CreatePlatformActivation(ctx context.Context, q pg.Querier, userID, justification string, duration int, now time.Time) (PlatformActivation, error) {
	if q == nil {
		q = s.DB()
	}
	if _, err := q.Exec(ctx, `UPDATE platform_activations
		SET status = 'revoked', revoked_at = $2
		WHERE user_id = $1 AND status = 'active'`, userID, now); err != nil {
		return PlatformActivation{}, errf("CreatePlatformActivation.cerrar", err)
	}
	const sql = `INSERT INTO platform_activations
		(user_id, status, justification, duration_minutes, activated_at, expires_at)
		VALUES ($1, 'active', $2, $3, $4, $5)
		RETURNING id, user_id, status, justification, duration_minutes, activated_at, expires_at, revoked_at`
	var a PlatformActivation
	err := q.QueryRow(ctx, sql, userID, justification, duration, now,
		now.Add(time.Duration(duration)*time.Minute)).
		Scan(&a.ID, &a.UserID, &a.Status, &a.Justification, &a.DurationMinutes,
			&a.ActivatedAt, &a.ExpiresAt, &a.RevokedAt)
	return a, errf("CreatePlatformActivation", err)
}

// RevokePlatformActivation baja la elevación viva del usuario. Devuelve si había
// alguna: terminar algo que ya había terminado no es un error.
//
// El userID va en el WHERE y no se confía al llamador: nadie baja la elevación
// de otro por esta vía.
func (s *Store) RevokePlatformActivation(ctx context.Context, q pg.Querier, userID string, now time.Time) (bool, error) {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE platform_activations
		SET status = 'revoked', revoked_at = $2
		WHERE user_id = $1 AND status = 'active'`, userID, now)
	if err != nil {
		return false, errf("RevokePlatformActivation", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListPlatformActivations devuelve el histórico reciente con el correo de quien
// se elevó — la pregunta que se hace después de un incidente es «quién estaba
// elevado cuando pasó», y un uuid no la responde.
func (s *Store) ListPlatformActivations(ctx context.Context, limit int) ([]PlatformActivation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const sql = `SELECT a.id, a.user_id, u.email, u.name, a.status, a.justification,
		a.duration_minutes, a.activated_at, a.expires_at, a.revoked_at
		FROM platform_activations a JOIN users u ON u.id = a.user_id
		ORDER BY a.activated_at DESC LIMIT $1`
	rows, err := s.DB().Query(ctx, sql, limit)
	if err != nil {
		return nil, errf("ListPlatformActivations", err)
	}
	defer rows.Close()
	var out []PlatformActivation
	for rows.Next() {
		var a PlatformActivation
		if err := rows.Scan(&a.ID, &a.UserID, &a.UserEmail, &a.UserName, &a.Status,
			&a.Justification, &a.DurationMinutes, &a.ActivatedAt, &a.ExpiresAt, &a.RevokedAt); err != nil {
			return nil, errf("ListPlatformActivations.scan", err)
		}
		out = append(out, a)
	}
	return out, errf("ListPlatformActivations.iter", rows.Err())
}
