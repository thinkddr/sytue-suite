// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Retenciones del cron de purga/rollup (ver PurgeExpired). Valores fijos y
// documentados; el parámetro es `now`, lo que mantiene la purga determinista y
// testeable.
const (
	// BuildRetention conserva builds terminadas 30 días (además de las
	// KeepBuildsPerProject más recientes de cada proyecto, que no caducan).
	BuildRetention = 30 * 24 * time.Hour
	// WebhookRetention conserva la auditoría de webhooks git 30 días.
	WebhookRetention = 30 * 24 * time.Hour
	// OutboxRetention conserva eventos del outbox ya procesados 7 días.
	OutboxRetention = 7 * 24 * time.Hour
	// AlertDeliveryRetention conserva el dedupe de alertas enviadas 7 días (el
	// event_id referenciado ya no existe pasado OutboxRetention; sin reentrega
	// posible, la fila solo ocupa espacio). Tabla de crecimiento continuo.
	AlertDeliveryRetention = 7 * 24 * time.Hour
	// PreviewRetention conserva deployments de preview retirados 7 días.
	PreviewRetention = 7 * 24 * time.Hour
	// UsageDetailRetention conserva el detalle de usage_records 90 días; el
	// detalle más antiguo se agrega en filas mensuales (rollup) y se purga.
	UsageDetailRetention = 90 * 24 * time.Hour
	// RuntimeLogRetention conserva los logs de runtime 14 días. Es la tabla que
	// más crece de todo el control plane (una fila por línea que escriba
	// CUALQUIER app), así que la ventana es corta a propósito: es lo único que
	// acota el coste de que los logs sean buscables. Los de BUILD no están aquí
	// —siguen en Object Storage, que es más barato para leer un objeto entero.
	RuntimeLogRetention = 14 * 24 * time.Hour
	// RuntimeLogPurgeBatch acota las filas de log que borra UNA pasada. Sin tope,
	// el primer barrido tras un pico borraría millones de filas en una sola
	// transacción (bloat de WAL y un lock largo sobre la tabla que están usando
	// las escrituras del recolector). El cron corre cada hora: lo que no entre en
	// esta pasada se lleva la siguiente.
	RuntimeLogPurgeBatch = 50_000
	// KeepBuildsPerProject es el nº de builds más recientes que se conservan
	// por proyecto aunque hayan caducado: son los objetivos de rollback (el
	// controller resuelve la imagen vía build_id).
	KeepBuildsPerProject = 10
)

// PurgeStats es el recuento de filas afectadas por una pasada de purga/rollup
// (se loguea como métrica del cron).
type PurgeStats struct {
	Builds          int64 // builds terminadas purgadas
	Webhooks        int64 // git_webhooks purgados
	OutboxEvents    int64 // events_outbox procesados purgados
	Previews        int64 // deployments preview retirados purgados
	UsageDetail     int64 // filas de detalle de uso agregadas y purgadas
	UsageRollups    int64 // filas mensuales de uso creadas por el rollup
	AlertDeliveries int64 // dedupe de alertas enviadas purgado
	RuntimeLogs     int64 // líneas de log de runtime caducadas
}

// Total suma las filas purgadas (sin contar los rollups creados).
func (p PurgeStats) Total() int64 {
	return p.Builds + p.Webhooks + p.OutboxEvents + p.Previews + p.UsageDetail +
		p.AlertDeliveries + p.RuntimeLogs
}

// alignedMonthSQL identifica filas de usage_records que ya son un rollup
// mensual: su periodo cubre exactamente un mes natural UTC. La aritmética se
// hace en timestamp naive UTC (AT TIME ZONE 'UTC') para que el resultado no
// dependa del timezone de la sesión.
const alignedMonthSQL = `(
	period_from = date_trunc('month', period_from AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
	AND period_to = (date_trunc('month', period_from AT TIME ZONE 'UTC') + interval '1 month') AT TIME ZONE 'UTC'
)`

// PurgeExpired ejecuta una pasada de purga y rollup de retención (cron de
// cloud-api). Cross-tenant acotado: corre dentro de una transacción de
// mantenimiento (pg.InMaintenanceTx) que habilita las políticas PERMISSIVE
// `*_maintenance` de la migración 0002 — nunca toca app.tenant. Idempotente:
// re-ejecutar sobre el mismo estado no borra ni duplica nada.
//
//   - builds terminadas (succeeded/failed/canceled) con más de BuildRetention,
//     conservando las keepBuilds más recientes de cada proyecto (rollback);
//   - git_webhooks con más de WebhookRetention;
//   - events_outbox procesados con más de OutboxRetention;
//   - deployments preview retirados con más de PreviewRetention (sus logs de
//     runtime se van en cascada con ellos);
//   - logs de runtime con más de RuntimeLogRetention, por lotes;
//   - rollup de usage_records: el detalle con más de UsageDetailRetention se
//     agrega en filas mensuales (mes natural UTC) y se borra, en una sola
//     sentencia (mismo snapshot: no se pierde detalle concurrente).
func (s *Store) PurgeExpired(ctx context.Context, now time.Time, keepBuilds int) (PurgeStats, error) {
	if keepBuilds < 0 {
		keepBuilds = 0
	}
	var st PurgeStats
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		// Builds terminadas y caducadas, salvo las keepBuilds últimas del
		// proyecto. El filtro exterior usa builds_terminal_created_idx; la
		// subconsulta correlada, builds_project_created_id_idx (LIMIT K).
		// deployments.build_id es ON DELETE SET NULL: los deployments
		// históricos conservan su fila, solo pierden la referencia.
		//
		// ponytail: NOTA de retención — el objeto de log de la build en S3
		// (build-logs/<id>.log, ver builder.S3LogSink) queda huérfano al purgar su
		// fila; NO se borra aquí (el cron no tiene el cliente S3 ni el tenant). Se
		// limpia con una regla de LIFECYCLE del bucket build-cache (expiración por
		// prefijo build-logs/ a BuildRetention), fijada en la IaC del Object
		// Storage, no en el plano de control. Añadir el barrido S3 aquí solo si la
		// IaC no puede fijar la lifecycle rule.
		tag, err := tx.Exec(ctx, `
			DELETE FROM builds b
			WHERE b.status IN ('succeeded', 'failed', 'canceled')
			  AND b.created_at < $1
			  AND b.id NOT IN (
			      SELECT k.id FROM builds k
			      WHERE k.tenant = b.tenant AND k.project_id = b.project_id
			      ORDER BY k.created_at DESC, k.id DESC
			      LIMIT $2)`,
			now.Add(-BuildRetention), keepBuilds)
		if err != nil {
			return fmtErr("purgar builds", err)
		}
		st.Builds = tag.RowsAffected()

		tag, err = tx.Exec(ctx,
			`DELETE FROM git_webhooks WHERE received_at < $1`,
			now.Add(-WebhookRetention))
		if err != nil {
			return fmtErr("purgar webhooks", err)
		}
		st.Webhooks = tag.RowsAffected()

		// events_outbox no tiene RLS; se purga aquí por cohesión del cron.
		tag, err = tx.Exec(ctx,
			`DELETE FROM events_outbox WHERE processed_at IS NOT NULL AND processed_at < $1`,
			now.Add(-OutboxRetention))
		if err != nil {
			return fmtErr("purgar outbox", err)
		}
		st.OutboxEvents = tag.RowsAffected()

		tag, err = tx.Exec(ctx,
			`DELETE FROM deployments
			 WHERE target = 'preview' AND status = 'retired' AND updated_at < $1`,
			now.Add(-PreviewRetention))
		if err != nil {
			return fmtErr("purgar previews", err)
		}
		st.Previews = tag.RowsAffected()

		// Dedupe de alertas: crecimiento continuo (evento × política). La política
		// alert_deliveries_maintenance (0010) habilita este DELETE cross-tenant.
		tag, err = tx.Exec(ctx,
			`DELETE FROM alert_deliveries WHERE sent_at < $1`,
			now.Add(-AlertDeliveryRetention))
		if err != nil {
			return fmtErr("purgar alert deliveries", err)
		}
		st.AlertDeliveries = tag.RowsAffected()

		// Logs de runtime caducados. Va por LOTES (a diferencia del resto, que
		// borran conjuntos pequeños): aquí una pasada puede tener millones de
		// filas candidatas. El subselect ordenado por ts recorre
		// runtime_logs_ts_idx, así que el lote sale del extremo más antiguo.
		tag, err = tx.Exec(ctx, `
			DELETE FROM runtime_logs
			WHERE id IN (
			    SELECT id FROM runtime_logs WHERE ts < $1 ORDER BY ts LIMIT $2
			)`,
			now.Add(-RuntimeLogRetention), RuntimeLogPurgeBatch)
		if err != nil {
			return fmtErr("purgar logs de runtime", err)
		}
		st.RuntimeLogs = tag.RowsAffected()

		// Rollup mensual del detalle de uso: DELETE...RETURNING + INSERT en la
		// misma sentencia (un solo snapshot). Las filas ya mensuales (alineadas
		// al mes natural UTC) no se re-agregan jamás: idempotente. SummarizeUsage
		// suma cantidades, así que un mes con varios rollups (detalle tardío)
		// sigue dando totales correctos.
		err = tx.QueryRow(ctx, `
			WITH del AS (
			    DELETE FROM usage_records
			    WHERE period_from < $1 AND NOT `+alignedMonthSQL+`
			    RETURNING tenant, project_id, metric, quantity, unit, period_from
			), ins AS (
			    INSERT INTO usage_records (tenant, project_id, metric, quantity, unit, period_from, period_to)
			    SELECT tenant, project_id, metric, sum(quantity),
			           coalesce(max(nullif(unit, '')), ''),
			           date_trunc('month', period_from AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
			           (date_trunc('month', period_from AT TIME ZONE 'UTC') + interval '1 month') AT TIME ZONE 'UTC'
			    FROM del
			    GROUP BY tenant, project_id, metric, date_trunc('month', period_from AT TIME ZONE 'UTC')
			    RETURNING 1
			)
			SELECT (SELECT count(*) FROM del), (SELECT count(*) FROM ins)`,
			now.Add(-UsageDetailRetention)).Scan(&st.UsageDetail, &st.UsageRollups)
		if err != nil {
			return fmtErr("rollup de uso", err)
		}
		return nil
	})
	if err != nil {
		return PurgeStats{}, err
	}
	return st, nil
}
