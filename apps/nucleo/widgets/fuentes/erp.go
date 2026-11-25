// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"errors"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// ERP lee las tareas propias pendientes que vencen hoy o ya vencieron, también las que
// otra persona acaba de asignar. Peticiones: 1.
//
// Las tareas son del servicio y el repositorio privado no las tiene (api.ERPData): con la
// cuenta en Matrix la app enseña su lista de tareas vacía, y el widget lo mismo, sin
// contador ni filas —el marco de «sin datos» con su acción—, en vez de lo congelado.
func ERP(c Comunes, e *api.ERP) widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		hoy := c.ahora().Format(time.DateOnly)
		tareas, err := e.Actividades(ctx, api.ERPFiltroActividades{Estado: "pendiente", Mias: true, Hasta: hoy})
		if errors.Is(err, api.ErrPrivateERPOperation) {
			return widgets.Detail{}, nil
		}
		if err != nil {
			return widgets.Detail{}, err
		}
		items := make([]widgets.Item, 0, min(len(tareas), widgets.MaxItems))
		for _, a := range tareas {
			titulo := primero(a.Resumen, a.RegistroEtiqueta)
			if titulo == "" {
				continue
			}
			vencida := a.Vence != "" && a.Vence < hoy
			color := ""
			if vencida {
				color = "#dc2626"
			}
			items = append(items, widgets.Item{
				Title: titulo, Subtitle: primero(a.RegistroEtiqueta, a.Modelo), Badge: a.Vence,
				Color: color, Unread: vencida || a.Vence == hoy, Route: RutaTareasERP(),
			})
			if llenos(items) {
				break
			}
		}
		return widgets.Detail{
			Counters: []widgets.Counter{{Kind: "pending", Value: int64(len(tareas))}},
			Items:    widgets.SanitizeItems(items),
		}, nil
	}
}
