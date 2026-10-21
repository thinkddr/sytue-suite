// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// integracionesDeSerie son las que cada persona conecta desde la app con solo el
// consentimiento. Admin no las configura: solo las apaga, una a una.
//
// Solo las que algún producto comprueba (`proveedorDePlantilla`): Box y Zoom
// estaban en la lista sin que nada las mirase, así que su interruptor no apagaba
// nada. Una fila ya guardada para ellas se queda sin efecto, como antes.
var integracionesDeSerie = []string{
	"google-drive", "onedrive", "dropbox",
	"slack", "microsoft-teams", "google-chat",
	"google-meet",
	"gmail", "microsoft", "yahoo", "imap", "pop3",
}

var errIntegracionApagada = httpx.ErrForbidden.WithCode("integration_disabled").WithMessage("Your organization has turned off this integration.")

// integracionEncendida dice si la organización activa deja usar ese proveedor.
func (h *Handlers) integracionEncendida(ctx context.Context, provider string) error {
	off, err := h.store.DisabledIntegrations(ctx)
	if err != nil {
		return toAPIError(err)
	}
	if off[provider] {
		return errIntegracionApagada
	}
	return nil
}

// ListIntegrations — GET /api/v1/admin/integrations.
func (h *Handlers) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	off, err := h.store.DisabledIntegrations(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]map[string]any, 0, len(integracionesDeSerie))
	for _, p := range integracionesDeSerie {
		items = append(items, map[string]any{"provider": p, "enabled": !off[p]})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// SetIntegration — PUT /api/v1/admin/integrations/{provider} {"enabled": bool}.
func (h *Handlers) SetIntegration(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !slices.Contains(integracionesDeSerie, provider) || req.Enabled == nil {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if err := h.store.SetIntegrationEnabled(r.Context(), provider, *req.Enabled); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"provider": provider, "enabled": *req.Enabled})
}

// proveedorDePlantilla connects the existing personal and organization entry
// points to the same opt-out policy. A missing row means enabled by default.
func proveedorDePlantilla(key string) string {
	if provider := personalStorageProvider(key); provider != "" {
		return provider
	}
	if provider := externalChatProvider(key); provider != "" {
		return provider
	}
	if provider := externalMailProvider(key); provider != "" {
		return provider
	}
	switch key {
	case tmplSlackRooms:
		return "slack"
	case tmplTeamsRooms:
		return "microsoft-teams"
	case tmplGoogleChat:
		return "google-chat"
	case tmplGoogleMeet:
		return "google-meet"
	default:
		return ""
	}
}

func (h *Handlers) plantillaEncendida(ctx context.Context, key string) error {
	if provider := proveedorDePlantilla(key); provider != "" {
		return h.integracionEncendida(ctx, provider)
	}
	return nil
}
