// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// EventSizingAdvice es el evento del bus que emite el evaluador de tamaño cuando
// la recomendación de un proyecto pasa a ser accionable. Lo consume el
// notificador de alertas.
const EventSizingAdvice = "cloud.sizing.advice"

// SizingCandidate es un proyecto sobre el que PUEDE haber recomendación: tiene
// muestras bastantes en la ventana. Que la haya lo decide el motor con el
// histórico completo; esto solo descarta a los que seguro que no.
type SizingCandidate struct {
	Tenant    string
	ProjectID uuid.UUID
}

// ListSizingCandidates devuelve los proyectos con muestras suficientes desde
// `since`, cross-tenant.
//
// El filtro va en SQL y no en Go a propósito: el evaluador carga el histórico
// COMPLETO de cada proyecto que le devolvamos (una consulta y ~672 filas por
// proyecto con la ventana de 7 días), así que cada fila de más aquí es un barrido
// entero de más. La inmensa mayoría de los proyectos de una plataforma no tienen
// una app de producción midiéndose, y esos ni se tocan.
//
// El umbral es domain.MinSamplesForAdvice, el MISMO que aplica el motor: si el
// prefiltro fuera más laxo, se cargarían histories para acabar en «sin datos
// suficientes»; si fuera más estricto, se perderían consejos en silencio.
//
// ponytail: muestreo ALEATORIO cuando hay más candidatos que `limit`. Un orden
// estable (por id) dejaría a los mismos proyectos de la cola sin evaluar JAMÁS,
// que es un techo invisible; así cada ciclo mira un subconjunto distinto y todos
// entran a la vuelta de unas horas. Si el barrido llegara a quedarse corto de
// verdad, la salida es un cursor por «último evaluado», no subir el número.
func (s *Store) ListSizingCandidates(ctx context.Context, since time.Time, limit int) ([]SizingCandidate, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	var out []SizingCandidate
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.tenant, p.id
			FROM projects p
			JOIN resource_samples rs ON rs.project_id = p.id
			WHERE p.deleted_at IS NULL AND rs.bucket >= $1
			GROUP BY p.id, p.tenant
			HAVING count(*) >= $2
			ORDER BY random()
			LIMIT $3`, since.UTC(), domain.MinSamplesForAdvice, limit)
		if err != nil {
			return fmtErr("listar candidatos a aviso de tamaño", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c SizingCandidate
			if err := rows.Scan(&c.Tenant, &c.ProjectID); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// PublishSizingAlert reserva y emite, EN LA MISMA TRANSACCIÓN, el aviso de que la
// recomendación de tamaño del proyecto pasó a `from → to`. Devuelve true solo si
// esta llamada fue la que lo emitió.
//
// Tres garantías, todas por construcción y sin locks:
//
//  1. Una sola réplica avisa. La exclusión mutua es la CLAVE PRIMARIA del
//     proyecto: el ON CONFLICT es atómico, así que de N evaluadores simultáneos
//     exactamente uno actualiza fila y los demás ven RowsAffected() == 0.
//  2. No se insiste. Mientras la recomendación no CAMBIE, el UPDATE no se aplica
//     (IS DISTINCT FROM) y no se emite nada: quien no la aplicó ya la tiene
//     delante en el panel del proyecto.
//  3. Ni se pierde ni se duplica. Reserva y publicación al outbox comparten
//     transacción: si la publicación falla, la reserva se deshace con ella y el
//     siguiente ciclo lo reintenta.
//
// `cooldown` es el enfriamiento mínimo entre dos avisos del mismo proyecto: una
// recomendación que oscila (down → up → down) avisaría en cada ciclo, y ese es
// justo el camino a que el cliente silencie el canal.
func (s *Store) PublishSizingAlert(
	ctx context.Context, tenant string, projectID uuid.UUID, cooldown time.Duration,
	pl domain.SizingAlertPayload,
) (bool, error) {
	published := false
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO sizing_alerts (project_id, tenant, from_size, to_size)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id) DO UPDATE SET
				from_size   = EXCLUDED.from_size,
				to_size     = EXCLUDED.to_size,
				notified_at = now()
			WHERE (sizing_alerts.from_size, sizing_alerts.to_size)
			      IS DISTINCT FROM (EXCLUDED.from_size, EXCLUDED.to_size)
			  AND sizing_alerts.notified_at < now() - $5::interval`,
			projectID, tenant, pl.FromSize, pl.ToSize, cooldown)
		if err != nil {
			return fmtErr("reservar aviso de tamaño", err)
		}
		if tag.RowsAffected() == 0 {
			return nil // misma recomendación, o aún en enfriamiento
		}
		ev, err := events.NewEvent(EventChannel, EventSizingAdvice, tenant, pl)
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
