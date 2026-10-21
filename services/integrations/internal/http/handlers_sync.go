// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// PostSync — POST /api/v1/instances/{id}/sync — dispara una reconciliación SCIM
// saliente (aprovisionar/desaprovisionar usuarios+grupos en el SaaS). Encola el
// trabajo en el outbox (durable, con backoff/attempts en el Dispatcher) y responde
// 202: el trabajo corre de forma asíncrona. Escritura ⇒ owner/admin (router).
func (h *Handlers) PostSync(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	// Verja: aprovisionar da de alta y de baja personas EN EL SAAS. Una conexión
	// desactivada no debe seguir haciéndolo, y hasta ahora lo hacía.
	if err := h.gateCapability(ctx, id, manifest.CapProvision); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// La instancia debe existir en la org y su conector debe soportar aprovisionamiento.
	inst, err := h.store.GetInstance(ctx, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	drv, _, err := h.resolveCapability(ctx, inst.TemplateKey, inst.ConfigMap(), manifest.CapProvision)
	if err != nil {
		httpx.Fail(w, r, capabilityError(err, "provisioning and deprovisioning people"))
		return
	}
	if _, ok := drv.(connector.Provisioner); !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This connector does not support provisioning."))
		return
	}

	if err := h.store.EnqueueSync(ctx, id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"status": "queued", "instance_id": id})
}

// ListSyncRuns — GET /api/v1/instances/{id}/sync-runs — historial de
// reconciliaciones de la instancia (más reciente primero). Lectura ⇒ cualquier
// miembro de la org (RLS). limit opcional (1..200, 50 por defecto).
func (h *Handlers) ListSyncRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if _, err := h.store.GetInstance(ctx, id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid limit (1..200)."))
			return
		}
		limit = n
	}
	runs, err := h.store.ListSyncRuns(ctx, id, limit)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"sync_runs": runs})
}
