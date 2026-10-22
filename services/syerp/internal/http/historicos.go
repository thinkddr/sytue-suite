// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package syerphttp

import (
	"context"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// AvisosHistoricos son los avisos de «te han asignado una tarea» de la organización
// del contexto que la campana no llegó a guardar, con la hora en que se creó cada
// tarea. Solo las que puso OTRA persona (actividades.go avisa de esas) y no se
// cancelaron, y a quien la tiene. Las que Sytue AI crea para quien se las pide también
// avisaban, pero se guardan igual que las que alguien se pone a sí mismo, y esas no:
// sin poder distinguirlas, no se reconstruye ninguna de las dos. Sin sesión no hay nombre de quien la puso: el título es la tarea.
func AvisosHistoricos(ctx context.Context, pool *pg.Pool, publicURL string) ([]hubnotify.Historico, error) {
	t := tenancy.MustFrom(ctx)
	var out []hubnotify.Historico
	err := pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id::text, modelo, resumen, vence::text, responsable_sub, creada_en
			FROM actividades
			WHERE tenant = $1 AND responsable_sub <> '' AND responsable_sub <> creada_por
			  AND estado <> 'cancelada'
			ORDER BY creada_en, id`, t.Slug)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a store.Actividad
			if err := rows.Scan(&a.ID, &a.Modelo, &a.Resumen, &a.Vence, &a.ResponsableSub, &a.CreadaEn); err != nil {
				return err
			}
			out = append(out, hubnotify.Historico{
				Notification: avisoDeTarea(publicURL, t.Slug, a, ""),
				Clave:        "erp:task:" + a.ID,
				Fecha:        a.CreadaEn,
			})
		}
		return rows.Err()
	})
	return out, err
}
