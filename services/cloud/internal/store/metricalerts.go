// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
)

// Eventos del bus que emite el evaluador de consumo. Los consume el notificador
// de alertas, que los traduce a los eventos cortos de alert_policies.
const (
	EventMetricCPUHigh    = "cloud.metric.cpu_high"
	EventMetricMemoryHigh = "cloud.metric.memory_high"
)

// PublishMetricAlert reserva y emite, EN LA MISMA TRANSACCIÓN, el aviso de que el
// consumo de un recurso lleva rato pegado al techo. Devuelve true solo si esta
// llamada fue la que lo emitió.
//
// Las tres garantías son las de su hermana PublishSizingAlert, con una diferencia
// que importa: aquí el UPDATE también se aplica si el porcentaje **subió**. Un
// proyecto que pasa del 90 % al 99 % ha empeorado, y callarlo por «ya avisé» es
// justo perder el aviso que servía. Bajar, en cambio, no vuelve a avisar hasta
// que pase el enfriamiento.
func (s *Store) PublishMetricAlert(
	ctx context.Context, tenant string, projectID uuid.UUID, kind string,
	percent int, cooldown time.Duration, busType string, payload any,
) (bool, error) {
	published := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO metric_alerts (project_id, kind, tenant, percent)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id, kind) DO UPDATE SET
				percent     = EXCLUDED.percent,
				notified_at = now()
			WHERE metric_alerts.notified_at < now() - $5::interval
			   OR EXCLUDED.percent > metric_alerts.percent`,
			projectID, kind, tenant, percent, cooldown)
		if err != nil {
			return fmtErr("reservar aviso de consumo", err)
		}
		if tag.RowsAffected() == 0 {
			return nil // ya avisado y sin empeorar, o aún en enfriamiento
		}
		ev, err := events.NewEvent(EventChannel, busType, tenant, payload)
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

// ClearMetricAlert olvida el aviso de un recurso que ha vuelto a la normalidad.
//
// Sin esto, un proyecto que se arregla —se sube de tamaño, se corrige la fuga—
// no volvería a avisar nunca si recae, porque la fila seguiría diciendo que ya se
// avisó al 95 %. El olvido es lo que hace que la segunda vez también se entere
// alguien.
func (s *Store) ClearMetricAlert(ctx context.Context, projectID uuid.UUID, kind string) error {
	return s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`DELETE FROM metric_alerts WHERE project_id=$1 AND kind=$2`, projectID, kind)
		if err != nil {
			return fmtErr("limpiar aviso de consumo", err)
		}
		return nil
	})
}

// MetricAlertPayload es lo que viaja al canal del cliente. Lleva las tres cifras
// que hacen falta para decidir sin abrir el panel: cuánto se usa, de cuánto, y
// sobre qué ventana se midió.
type MetricAlertPayload struct {
	ProjectID   string `json:"project_id"`
	Resource    string `json:"resource"` // cpu | memory
	Percent     int    `json:"percent"`
	Threshold   int    `json:"threshold"`
	InstanceSze string `json:"instance_size"`
	WindowHours int    `json:"window_hours"`
}

// MetricKindCPU / MetricKindMemory son los valores de metric_alerts.kind. Viven
// aquí y no en un CHECK de la base: el conjunto lo fija el código y una lista más
// en SQL sería otra copia que desincronizar.
const (
	MetricKindCPU    = "cpu"
	MetricKindMemory = "memory"
)
