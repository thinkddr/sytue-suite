// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"context"
	"iter"
	"net/http"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// paginaExport es cuántos mensajes se leen de una vez al exportar.
const paginaExport = 500

// Export entrega lo de Rooms para «Exportar tus datos» (GET /api/v1/export): los
// canales en los que está la persona y los mensajes que ESCRIBIÓ, hilos
// incluidos. Los mensajes de los demás son de ellos y no salen.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	sub, _ := caller(r)
	p, err := portabilidad.Nuevo(w, r)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	if err := h.exportar(r.Context(), sub, p); err != nil {
		_ = p.Fallo(err)
		return
	}
	_ = p.Cerrar()
}

func (h *Handlers) exportar(ctx context.Context, sub string, p *portabilidad.Paquete) error {
	teams, err := h.svc.ListTeams(ctx, sub)
	if err != nil {
		return err
	}
	var canales []domain.Channel
	for _, t := range teams {
		cs, err := h.svc.ListChannels(ctx, t.ID, sub, true)
		if err != nil {
			return err
		}
		canales = append(canales, cs...)
	}
	if err := portabilidad.Tabla(p, "channels", "channel", portabilidad.Todas(canales)); err != nil {
		return err
	}
	return portabilidad.Tabla(p, "messages", "message", h.mensajesPropios(ctx, sub))
}

// mensajesPropios recorre los mensajes de sub por páginas: se escriben según se
// leen, sin cargar el historial entero en memoria.
func (h *Handlers) mensajesPropios(ctx context.Context, sub string) iter.Seq2[domain.Message, error] {
	return func(yield func(domain.Message, error) bool) {
		var (
			desde   time.Time
			desdeID uuid.UUID
		)
		for {
			pagina, err := h.svc.ListOwnMessages(ctx, sub, desde, desdeID, paginaExport)
			if err != nil {
				yield(domain.Message{}, err)
				return
			}
			for _, m := range pagina {
				if !yield(m, nil) {
					return
				}
			}
			if len(pagina) < paginaExport {
				return
			}
			ultimo := pagina[len(pagina)-1]
			desde, desdeID = ultimo.CreatedAt, ultimo.ID
		}
	}
}
