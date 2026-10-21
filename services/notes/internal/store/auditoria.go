// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/notes/internal/domain"
)

// WriteAudit registra una acción en la traza, bajo el tenant del contexto.
//
// Se llama desde el store y no desde el handler para que la entrada caiga en la
// MISMA transacción que el cambio que describe: si el UPDATE se revierte, la
// línea de auditoría se revierte con él. Una traza que afirma cosas que no
// llegaron a pasar es peor que no tenerla, porque se usa para reconstruir
// incidentes.
func writeAuditTx(ctx context.Context, tx pgx.Tx, tenant, actor, action, target string, detail any) error {
	raw := []byte(`{}`)
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			// El detalle es contexto, no la acción: si no serializa, se registra
			// igual con `{}` en vez de perder la línea entera.
			b = []byte(`{}`)
		}
		raw = b
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_log (tenant, actor, action, target, detail)
		VALUES ($1, $2, $3, $4, $5)`, tenant, actor, action, target, raw)
	return err
}

// ListAudit devuelve las últimas entradas del tenant, de la más reciente a la
// más antigua. La lee la Console de cumplimiento.
// FiltroAudit acota la consulta de la traza. Los ceros significan «sin acotar»,
// que es lo que pide la vista por defecto: lo último que haya pasado.
type FiltroAudit struct {
	Desde time.Time
	Hasta time.Time
	Limit int
}

func (s *Store) ListAudit(ctx context.Context, f FiltroAudit) ([]domain.AuditEntry, error) {
	t := tenancy.MustFrom(ctx)
	// Mismo tope que Drive: sin límite, una organización con años de traza
	// tumbaría la respuesta, y quien la lee pagina.
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []domain.AuditEntry
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Las fechas se pasan como NULL cuando no vienen, y el predicado las
		// ignora con un OR: así la consulta es UNA sola y el planificador usa el
		// mismo índice (tenant, occurred_at DESC) en los cuatro casos, en vez de
		// tener que armar el SQL a trozos según qué filtros llegaron.
		var desde, hasta any
		if !f.Desde.IsZero() {
			desde = f.Desde
		}
		if !f.Hasta.IsZero() {
			hasta = f.Hasta
		}
		rows, err := tx.Query(ctx, `
			SELECT id, occurred_at, tenant, actor, action, target, detail
			  FROM audit_log
			 WHERE tenant = $1
			   AND ($3::timestamptz IS NULL OR occurred_at >= $3)
			   AND ($4::timestamptz IS NULL OR occurred_at <= $4)
			 ORDER BY occurred_at DESC, id DESC
			 LIMIT $2`, t.Slug, limit, desde, hasta)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = nil
		for rows.Next() {
			var e domain.AuditEntry
			var detail []byte
			if err := rows.Scan(&e.ID, &e.OccurredAt, &e.Tenant, &e.Actor,
				&e.Action, &e.Target, &detail); err != nil {
				return err
			}
			e.Detail = detail
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
