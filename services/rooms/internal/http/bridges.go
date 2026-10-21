// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package chathttp

import (
	"errors"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
)

type createBridgeRequest struct {
	Platform   string `json:"platform"`
	WebhookURL string `json:"webhook_url"`
	InstanceID string `json:"instance_id"`
	RemoteID   string `json:"remote_id"`
}

// ListBridges — GET /channels/{channelID}/bridges — los puentes del canal (admin).
func (ih *intHandlers) ListBridges(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	bridges, err := ih.svc.ListBridges(r.Context(), channelID, actor)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if bridges == nil {
		bridges = []domain.Bridge{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"bridges": bridges})
}

// CreateBridge — POST /channels/{channelID}/bridges — conecta el canal con Slack,
// Teams o Google Chat. Si el otro lado rechaza el mensaje de prueba, 422 con su
// respuesta: es lo que el admin necesita para corregir la URL.
func (ih *intHandlers) CreateBridge(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createBridgeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	var b domain.Bridge
	if req.InstanceID != "" || req.RemoteID != "" {
		if req.WebhookURL != "" {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		b, err = ih.svc.CreateOAuthBridge(r.Context(), channelID, actor, strings.TrimSpace(req.Platform), req.InstanceID, req.RemoteID)
	} else {
		b, err = ih.svc.CreateBridge(r.Context(), channelID, actor, strings.TrimSpace(req.Platform), req.WebhookURL)
	}
	switch {
	case errors.Is(err, service.ErrBridgeRejected):
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithMessage(err.Error()))
		return
	case errors.Is(err, domain.ErrConflict):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("This channel already has the maximum number of bridges."))
		return
	case err != nil:
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"bridge": b})
}

// DeleteBridge — DELETE /bridges/{id} — desconecta un puente (admin de su canal).
func (ih *intHandlers) DeleteBridge(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor, _ := caller(r)
	if err := ih.svc.DeleteBridge(r.Context(), id, actor); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
