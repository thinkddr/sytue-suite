// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// EventSpendThreshold es el evento del bus que emite el evaluador de coste al
// cruzarse un umbral del límite de gasto. Lo consume el notificador de alertas.
const EventSpendThreshold = "cloud.billing.spend_threshold"

// SpendLimitedTenant es un tenant con límite de gasto configurado: los únicos
// candidatos a recibir un aviso (sin límite no hay techo que cruzar).
type SpendLimitedTenant struct {
	Tenant   string
	Plan     string
	LimitEur float64
}

// ListSpendLimitedTenants devuelve los tenants con límite de gasto, cross-tenant.
//
// El filtro va en SQL y no en Go a propósito: el evaluador hace UNA consulta de
// uso por tenant devuelto, así que cada fila de más es una consulta de más. Con
// decenas de miles de organizaciones —la inmensa mayoría sin límite— eso no es
// una micro-optimización, es la diferencia entre un barrido barato y uno inútil.
func (s *Store) ListSpendLimitedTenants(ctx context.Context, limit int) ([]SpendLimitedTenant, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	var out []SpendLimitedTenant
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT tenant, plan, spend_limit_eur
			FROM billing_accounts
			WHERE spend_limit_eur IS NOT NULL AND spend_limit_eur > 0
			ORDER BY tenant
			LIMIT $1`, limit)
		if err != nil {
			return fmtErr("listar tenants con límite de gasto", err)
		}
		defer rows.Close()
		for rows.Next() {
			var t SpendLimitedTenant
			if err := rows.Scan(&t.Tenant, &t.Plan, &t.LimitEur); err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// PublishSpendAlert reserva y emite, EN LA MISMA TRANSACCIÓN, el aviso de haber
// cruzado `thresholdPct` en (tenant, periodo). Devuelve true solo si esta llamada
// fue la que lo emitió.
//
// Dos garantías, ambas por construcción y sin locks:
//
//  1. Una sola réplica avisa. La exclusión mutua es la CLAVE PRIMARIA: un
//     `ON CONFLICT DO NOTHING` es atómico, así que de N réplicas evaluando a la vez
//     exactamente una inserta fila y las demás ven `RowsAffected() == 0`.
//  2. Ni se pierde ni se duplica el aviso. La reserva y la publicación al outbox
//     comparten transacción: si la publicación falla, la reserva se deshace con
//     ella y el siguiente ciclo lo reintenta. Reservar y publicar por separado
//     dejaría el caso de «reservado pero nunca enviado», que se come el único
//     aviso del mes sin que nadie se entere.
//
// Al cambiar de mes cambia `period_start`, así que el aviso vuelve a estar
// disponible sin ninguna tarea de reinicio.
func (s *Store) PublishSpendAlert(
	ctx context.Context, tenant string, periodStart time.Time, thresholdPct int,
	spendEur, limitEur float64,
) (bool, error) {
	published := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO billing_spend_alerts (tenant, period_start, threshold_pct, spend_eur, limit_eur)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant, period_start, threshold_pct) DO NOTHING`,
			tenant, periodStart, thresholdPct, spendEur, limitEur)
		if err != nil {
			return fmtErr("reservar aviso de coste", err)
		}
		if tag.RowsAffected() == 0 {
			return nil // ya avisado este periodo (otra réplica o un ciclo previo)
		}
		ev, err := events.NewEvent(EventChannel, EventSpendThreshold, tenant, domain.SpendAlertPayload{
			ThresholdPct: thresholdPct,
			SpendEur:     spendEur,
			LimitEur:     limitEur,
			PeriodStart:  periodStart,
		})
		if err != nil {
			return err
		}
		if err := events.Publish(ctx, tx, ev); err != nil {
			return err
		}
		published = true
		return nil
	})
	return published, err
}
