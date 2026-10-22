// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// ---------------------------------------------------------------------------
// Uptime checks
// ---------------------------------------------------------------------------

type createUptimeCheckRequest struct {
	Name            string `json:"name"`
	TargetURL       string `json:"target_url"`
	IntervalSeconds int    `json:"interval_seconds"`
	Enabled         *bool  `json:"enabled,omitempty"`
}

// CreateUptimeCheck crea una sonda de uptime para un proyecto. Si target_url va
// vacío, el servicio usa la URL del deployment de producción activo.
func (h *Handlers) CreateUptimeCheck(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req createUptimeCheckRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.CreateUptimeCheck(r.Context(), service.CreateUptimeCheckInput{
		ProjectID:       id,
		Name:            req.Name,
		TargetURL:       req.TargetURL,
		IntervalSeconds: req.IntervalSeconds,
		Enabled:         req.Enabled,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, c)
}

// ListUptimeChecks lista las sondas de un proyecto.
func (h *Handlers) ListUptimeChecks(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cs, err := h.svc.ListUptimeChecks(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if cs == nil {
		cs = []domain.UptimeCheck{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"uptime_checks": cs})
}

// GetUptimeCheck devuelve una sonda por ID.
func (h *Handlers) GetUptimeCheck(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.GetUptimeCheck(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, c)
}

type updateUptimeCheckRequest struct {
	Name            *string `json:"name,omitempty"`
	TargetURL       *string `json:"target_url,omitempty"`
	IntervalSeconds *int    `json:"interval_seconds,omitempty"`
	Enabled         *bool   `json:"enabled,omitempty"`
}

// UpdateUptimeCheck aplica una actualización parcial de una sonda.
func (h *Handlers) UpdateUptimeCheck(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateUptimeCheckRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := h.svc.UpdateUptimeCheck(r.Context(), id, store.UpdateUptimeCheckRecord{
		Name: req.Name, TargetURL: req.TargetURL,
		IntervalSeconds: req.IntervalSeconds, Enabled: req.Enabled,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, c)
}

// DeleteUptimeCheck elimina una sonda.
func (h *Handlers) DeleteUptimeCheck(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteUptimeCheck(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// ---------------------------------------------------------------------------
// Alert policies
// ---------------------------------------------------------------------------

type createAlertPolicyRequest struct {
	ProjectID  string `json:"project_id,omitempty"` // "" = alcance organización
	Event      string `json:"event"`
	ChannelURL string `json:"channel_url"`
	Enabled    *bool  `json:"enabled,omitempty"`
}

// CreateAlertPolicy crea una política de alerta por evento.
func (h *Handlers) CreateAlertPolicy(w http.ResponseWriter, r *http.Request) {
	var req createAlertPolicyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var projectID *uuid.UUID
	if req.ProjectID != "" {
		pid, perr := uuid.Parse(req.ProjectID)
		if perr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid project_id."))
			return
		}
		projectID = &pid
	}
	p, err := h.svc.CreateAlertPolicy(r.Context(), service.CreateAlertPolicyInput{
		ProjectID:  projectID,
		Event:      req.Event,
		ChannelURL: req.ChannelURL,
		Enabled:    req.Enabled,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, p)
}

// ListAlertPolicies lista las políticas de alerta del tenant.
func (h *Handlers) ListAlertPolicies(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.ListAlertPolicies(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if ps == nil {
		ps = []domain.AlertPolicy{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"alert_policies": ps})
}

type updateAlertPolicyRequest struct {
	Event      *string `json:"event,omitempty"`
	ChannelURL *string `json:"channel_url,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`
}

// UpdateAlertPolicy aplica una actualización parcial de una política.
func (h *Handlers) UpdateAlertPolicy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req updateAlertPolicyRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p, err := h.svc.UpdateAlertPolicy(r.Context(), id, store.UpdateAlertPolicyRecord{
		Event: req.Event, ChannelURL: req.ChannelURL, Enabled: req.Enabled,
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

// DeleteAlertPolicy elimina una política.
func (h *Handlers) DeleteAlertPolicy(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.svc.DeleteAlertPolicy(r.Context(), id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}
