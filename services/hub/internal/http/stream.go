// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// keepAliveInterval es el periodo del comentario de keep-alive del SSE (mantiene
// viva la conexión a través de proxies y detecta cierres del cliente).
const keepAliveInterval = 25 * time.Second

// Stream abre el flujo SSE (text/event-stream) de notificaciones en tiempo real
// del usuario autenticado: GET /api/v1/notifications/stream.
//
// Protocolo (WHATWG EventSource):
//   - event: ready   -> handshake con el contador inicial de no leídas.
//   - event: notification -> una notificación nueva (JSON en data).
//   - event: avatar -> un evento de ESTADO (no se guarda; ver PublishEvent).
//   - ': keep-alive' -> comentario periódico de mantenimiento.
func (h *Handlers) Stream(w http.ResponseWriter, r *http.Request) {
	claims, ok := authn.FromContext(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("The token is missing the tenant."))
		return
	}

	dest := domain.Destinatarios(claims.Subject, claims.Email, claims.Mailbox)
	propios := buzonesPersonalesDel(claims)

	// Cabeceras SSE y, sobre todo, sin los plazos del servidor: con ellos el flujo
	// moría a los 30 s (ver httpx.AbrirSSE).
	flusher, ok := httpx.AbrirSSE(w)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Streaming no soportado."))
		return
	}

	ch, cancel := h.hub.Subscribe(t.Slug, propios, dest...)
	defer cancel()

	// Handshake: contador inicial de no leídas para pintar el badge sin un GET extra.
	unread, _ := h.notify.UnreadCount(personales(r.Context()), dest)
	writeSSE(w, "ready", map[string]any{"unread": unread})
	flusher.Flush()

	keepAlive := time.NewTicker(keepAliveInterval)
	defer keepAlive.Stop()

	// El flujo no vive más que su token (COM-08): al reconectar, el nuevo dice qué ve.
	ctx, cancelar := authn.HastaQueCaduque(r.Context(), claims)
	defer cancelar()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if n, ok := e.Datos.(domain.Notification); ok {
				n = n.VistoDesde(t.Slug)
				uno := []domain.Notification{n}
				h.conNombres(ctx, uno)
				e.Datos = uno[0]
			}
			if !writeSSE(w, e.Nombre, e.Datos) {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				log.From(ctx).Debug("SSE: cliente desconectado", "error", err)
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSE serializa v como un evento SSE con nombre. Devuelve false si la
// escritura falla (cliente desconectado).
func writeSSE(w http.ResponseWriter, event string, v any) bool {
	data, err := json.Marshal(v)
	if err != nil {
		return false
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	return err == nil
}
