// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package noteshttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
)

// # La señal de cambios a quienes ven lo escrito
//
// Lo que escribe una persona en un cuaderno compartido (o en una nota compartida suelta)
// lo ven su dueño y las demás personas con quien se compartió: les llega la señal de
// cambios del widget (ADR 0078), como en Calendar. Hasta el 2026-11-25 solo la recibía
// quien escribía, y el widget del dueño no se enteraba hasta su latido.
//
// Se mira ANTES de la escritura —después de borrar una nota ya no se sabe de qué
// cuaderno era— y solo cuenta si la escritura sale bien (hubnotify.TrasEscribir manda
// la señal solo con un 2xx). Una consulta por escritura con señal, ninguna en lecturas.

// quienesVen envuelve una escritura con `{id}` de cuaderno (porNota=false) o de nota.
func (h *Handlers) quienesVen(next http.HandlerFunc, porNota bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id, err := uuid.Parse(chi.URLParam(r, "id")); err == nil {
			ctx := r.Context()
			consulta, clave := h.store.QuienesVenCuaderno, "notes/notebook/"
			if porNota {
				consulta, clave = h.store.QuienesVenNota, "notes/note/"
			}
			hubnotify.AvisarQuienesVen(ctx, clave+id.String(), func() ([]string, error) {
				return consulta(ctx, id)
			})
		}
		next(w, r)
	}
}
