// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ---------------------------------------------------------------------------
// Puerta INTERNA servicio-a-servicio: PUT /internal/quota/external/{app}
//
// La cuota del plan es UNA para toda la suite y Drive es su dueño (CAST-ENCARGO §8):
// Sytue Cast guarda los vídeos en su bucket, pero lo que ocupan cuenta aquí. Cada
// app declara el TOTAL que ocupa en una organización —valor absoluto, idempotente:
// una declaración perdida la corrige la siguiente— y `used_bytes` de la cuota lo
// suma (mig 0026).
//
// No lleva JWT de persona: el secreto compartido (Authorization: Bearer
// <CAST_INTERNAL_SECRET>) ES la autorización, comparado en tiempo constante por
// httpx.SecretoInterno. El tenant viaja en el cuerpo, se valida como slug y se fija
// en la transacción, así que RLS aísla la fila en la organización de la que habla.
// Fuera de /api/v1: no hereda su RequireAuth ni su tope por identidad. El Ingress
// de Drive no publica /internal (ingress_test lo comprueba).
// ---------------------------------------------------------------------------

// externalUsageStore es lo que la puerta necesita del store (lo satisface *store.Store).
type externalUsageStore interface {
	SetExternalUsage(ctx context.Context, app string, bytes int64) error
}

// externalApps son las apps de la suite que pueden declarar uso en esta cuota.
// Conjunto CERRADO a propósito: una app que no esté aquí no existe para la cuota
// (400), y así ni un secreto filtrado ni una errata en el cliente inventan
// sumandos con nombres nuevos que nadie sabría desglosar.
var externalApps = map[string]bool{"cast": true}

type externalUsageRequest struct {
	Tenant string `json:"tenant"`
	Bytes  int64  `json:"bytes"`
}

// SetExternalUsage atiende PUT /internal/quota/external/{app}: 204 sin cuerpo.
func (h *Handlers) SetExternalUsage(w http.ResponseWriter, r *http.Request) {
	app := chi.URLParam(r, "app")
	if !externalApps[app] {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown app."))
		return
	}
	var req externalUsageRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := tenancy.New(strings.TrimSpace(req.Tenant))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid tenant."))
		return
	}
	if req.Bytes < 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("bytes cannot be negative."))
		return
	}
	if err := h.externalUsage.SetExternalUsage(tenancy.Into(r.Context(), t), app, req.Bytes); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
