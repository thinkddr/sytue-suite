// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// La cola de denuncias del OPERADOR de la plataforma (ADR 0073, «Cola del
// operador»). Quién es operador lo decide Sytue ID antes de llegar aquí
// (platform/pkg/operador); este fichero solo cruza organizaciones.
//
// Lo que cruza es `message_reports` y nada más, por la política de mantenimiento
// (migración 0039). El canal y el mensaje denunciado se leen después en la
// transacción de SU organización, con la misma consulta que la cola de quien
// modera allí: el operador no navega la conversación, ve lo que la denuncia
// señala.

// maxCuerpoOperador acota el mensaje que ve el operador: lo mismo que el extracto
// que guarda una denuncia de Cast.
const maxCuerpoOperador = 300

// ListReportsOperador es la cola de todas las organizaciones, también las
// denuncias contra quien administra cada una; no las que son contra `actor`.
func (s *Store) ListReportsOperador(ctx context.Context, actor string, abiertas bool) ([]domain.Report, error) {
	out := []domain.Report{}
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT tenant, id FROM message_reports
			  WHERE ($1 = false OR status = 'open')
			  ORDER BY created_at DESC LIMIT $2`, abiertas, reportsPageSize)
		if err != nil {
			return err
		}
		porOrg := map[string][]uuid.UUID{}
		for rows.Next() {
			var tenant string
			var id uuid.UUID
			if err := rows.Scan(&tenant, &id); err != nil {
				rows.Close()
				return err
			}
			porOrg[tenant] = append(porOrg[tenant], id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		// ponytail: una consulta por organización con denuncias en la página (≤ 200);
		// con cientos de organizaciones a la vez, una sola consulta con política
		// propia en channels y messages.
		for tenant, ids := range porOrg {
			if err := pg.SetTenant(ctx, tx, tenant); err != nil {
				return err
			}
			if err := leerDeOrg(ctx, tx, tenant, actor, ids, &out); err != nil {
				return err
			}
		}
		return nil
	})
	slices.SortFunc(out, func(a, b domain.Report) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, err
}

func leerDeOrg(ctx context.Context, tx pgx.Tx, tenant, actor string, ids []uuid.UUID, out *[]domain.Report) error {
	rows, err := tx.Query(ctx, reportSelect+` AND r.id = ANY($4)`, tenant, actor, true, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		r, err := scanReport(rows)
		if err != nil {
			return err
		}
		r.Tenant = tenant
		r.MessageBody = validation.TruncateRunes(r.MessageBody, maxCuerpoOperador)
		*out = append(*out, r)
	}
	return rows.Err()
}

// OrgDeDenuncia dice de qué organización es una denuncia, cruzando todas.
func (s *Store) OrgDeDenuncia(ctx context.Context, id uuid.UUID) (string, error) {
	var tenant string
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT tenant FROM message_reports WHERE id = $1`, id).Scan(&tenant)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return tenant, err
}

// ResolveReportOperador es ResolveReport desde la cola del operador. El contexto
// lleva la organización de la denuncia (OrgDeDenuncia); la verja de moderar es
// la de quien administra esa organización, y queda marcada como de la plataforma.
func (s *Store) ResolveReportOperador(ctx context.Context, id uuid.UUID, actor, action string) (domain.Report, error) {
	return s.resolveReport(ctx, id, actor, true, action, "platform")
}

// RetencionDenuncias es cuánto se conserva una denuncia ya resuelta o descartada.
// Las abiertas no caducan: se conservan hasta que alguien las resuelve. Es el
// mismo plazo que en Cast (services/cast/internal/social/limpieza.go). Rooms no
// copia el mensaje en la denuncia, así que lo que se borra es la fila: quién, a
// quién, el motivo, la nota y la resolución.
const RetencionDenuncias = 365 * 24 * time.Hour

// PurgarDenuncias borra, de todas las organizaciones y por lotes, las denuncias
// resueltas hace más de RetencionDenuncias.
func (s *Store) PurgarDenuncias(ctx context.Context) (int64, error) {
	var total int64
	for {
		var n int64
		err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx,
				`DELETE FROM message_reports WHERE id IN (
				    SELECT id FROM message_reports
				     WHERE status <> 'open' AND resolved_at < now() - $1::interval
				     ORDER BY resolved_at LIMIT 5000)`, RetencionDenuncias)
			n = tag.RowsAffected()
			return err
		})
		total += n
		if err != nil || n < 5000 {
			return total, err
		}
	}
}
