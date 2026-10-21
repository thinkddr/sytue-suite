// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// ---------------------------------------------------------------------------
// Endpoint INTERNO service-to-service: POST /internal/notifications
//
// Los servicios de la suite (Mail, Drive, Calendar, Rooms) lo llaman para emitir
// avisos unificados sin pasar por un JWT de usuario (los procesos en segundo plano
// —entrega SMTP, vencimiento de recordatorios— no tienen sesión).
//
// Autenticación: secreto compartido (Authorization: Bearer <HUB_INTERNAL_SECRET>),
// comparado en tiempo constante (subtle). El tenant y el subject viajan en el payload
// JSON; el tenant se inyecta en el contexto para aislamiento RLS.
// ---------------------------------------------------------------------------

type internalNotificationRequest struct {
	Tenant  string            `json:"tenant"`
	Subject string            `json:"subject"`
	Source  string            `json:"source"`
	Type    string            `json:"type"`
	Level   string            `json:"level"`
	Title   string            `json:"title"`
	Body    string            `json:"body"`
	URL     string            `json:"url"`
	Params  map[string]string `json:"params"`
}

// internalAuth valida el secreto compartido para las rutas /internal/*.
//
// Y antes, que la llamada venga de DENTRO. El Ingress de hub.sytue.eu publica «/»
// entero, así que esto se podía pedir desde internet (medido el 2026-09-24: un POST
// sin secreto contestaba 401 «Invalid internal secret», no 404), y ningún chart da
// HUB_INTERNAL_SECRET: los dos extremos caen al valor de desarrollo que está en el
// código. Cualquiera podía dejar un aviso con su enlace en la campana de cualquiera.
// Traefik es quien pone X-Forwarded-For y los servicios llaman al Service sin pasar
// por él: es la misma frontera que ya usa httpx.HealthHandler para la versión.
func (h *Handlers) internalAuth(next http.Handler) http.Handler {
	return httpx.SoloDesdeDentro(h.cfg.InternalSecret)(next)
}

// CreateInternalNotification atiende POST /internal/notifications.
func (h *Handlers) CreateInternalNotification(w http.ResponseWriter, r *http.Request) {
	var req internalNotificationRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	req.Tenant = strings.TrimSpace(req.Tenant)
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Tenant == "" || req.Subject == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("tenant and subject are required"))
		return
	}

	t, err := tenancy.New(req.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("invalid tenant slug"))
		return
	}
	tctx := tenancy.Into(r.Context(), t)

	n, err := h.notify.Create(tctx, domain.NewNotification{
		Subject: req.Subject,
		Source:  domain.Source(req.Source),
		Type:    req.Type,
		Level:   domain.NotificationLevel(req.Level),
		Title:   req.Title,
		Body:    req.Body,
		URL:     req.URL,
		Params:  req.Params,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, n)
}

type internalHistoricRequest struct {
	internalNotificationRequest
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateInternalHistoricNotification atiende POST /internal/notifications/historic:
// un aviso RECONSTRUIDO de algo que pasó antes de que existiera el registro de avisos.
// Solo lo llama el comando de mantenimiento `avisos-historicos` de cada servicio, con
// la misma verja que las demás rutas internas. Lo que puede dejar es más estrecho que
// lo de /internal/notifications —ya leído, sin tiempo real, fechado antes del registro
// en vivo (notify.Service.CreateHistorico)—, así que la ruta no abre nada nuevo.
// Contesta 200 con {"result": …} en todos los casos que no son un error: para quien
// reconstruye, «ya estaba» o «lo tiene silenciado» son el recuento, no un fallo.
func (h *Handlers) CreateInternalHistoricNotification(w http.ResponseWriter, r *http.Request) {
	var req internalHistoricRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := tenancy.New(strings.TrimSpace(req.Tenant))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("invalid tenant slug"))
		return
	}
	resultado, err := h.notify.CreateHistorico(tenancy.Into(r.Context(), t), domain.NewHistorico{
		NewNotification: domain.NewNotification{
			Subject: req.Subject,
			Source:  domain.Source(req.Source),
			Type:    req.Type,
			Level:   domain.NotificationLevel(req.Level),
			Title:   req.Title,
			Body:    req.Body,
			URL:     req.URL,
			Params:  req.Params,
		},
		Clave: req.Key,
		Fecha: req.CreatedAt,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]string{"result": resultado})
}

type internalEventRequest struct {
	Tenant  string          `json:"tenant"`
	Subject string          `json:"subject"`
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data"`
}

// eventosDeEstado son los nombres que acepta /internal/events. Lista cerrada: el
// nombre sale tal cual como `event:` del flujo, y `notification` o `ready` en
// manos de un emisor se harían pasar por lo que no son.
// ponytail: uno solo; se añade aquí el siguiente cuando haya otro estado que mover.
var eventosDeEstado = map[string]bool{hubnotify.EventoAvatar: true}

// CreateInternalEvent atiende POST /internal/events: un evento de ESTADO para las
// pantallas abiertas de una persona (hubnotify.Evento). No se guarda ni sale en la
// campana; 202 porque no queda nada que devolver.
func (h *Handlers) CreateInternalEvent(w http.ResponseWriter, r *http.Request) {
	var req internalEventRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	t, err := tenancy.New(strings.TrimSpace(req.Tenant))
	if err != nil || req.Subject == "" || !eventosDeEstado[req.Event] {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("tenant, subject and a known event are required"))
		return
	}
	if len(req.Data) == 0 {
		req.Data = json.RawMessage("{}")
	}
	if err := h.notify.PublishEvent(r.Context(), t.Slug, req.Subject, req.Event, req.Data); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// OlvidarBuzonInterno atiende POST /internal/notifications/forget {address}: Sytue ID
// ha retirado esa dirección, y sus avisos se borran en todas las organizaciones para
// que quien la reciba después no los herede (ADR 0053). 204 aunque no hubiera ninguno.
func (h *Handlers) OlvidarBuzonInterno(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !strings.Contains(req.Address, "@") {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("address is required"))
		return
	}
	if _, err := h.store.OlvidarBuzon(r.Context(), hubnotify.PorCorreo(req.Address)); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
