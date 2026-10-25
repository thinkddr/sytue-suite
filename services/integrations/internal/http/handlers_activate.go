// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// Activate — POST /api/v1/instances/{id}/activate — MATERIALIZA una instancia: según el
// manifiesto de su plantilla, da de alta en Sytue ID la identidad SALIENTE (SP SAML o
// client OIDC), resuelve la URL de lanzamiento del tile y —si la plantilla aprovisiona—
// agenda el primer sync SCIM. Idempotente (reactivar no recrea el SP/client). Si la
// persistencia falla tras crear el recurso en id, se REVIERTE (no deja huérfanos).
// Escritura ⇒ owner/admin (router). Devuelve las credenciales a mostrar UNA vez.
func (h *Handlers) Activate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	inst, m, cfg, err := h.loadInstanceManifest(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	out, rollback, mErr := manifest.Materialize(ctx, h.idApps, inst.OrgID, m, cfg)
	if mErr != nil {
		httpx.Fail(w, r, materializeError(mErr))
		return
	}

	// Fusiona las claves internas resueltas (ids + launch) en la config y activa. Si la
	// persistencia falla, revierte lo creado en Sytue ID (rollback best-effort).
	for k, v := range out.Config {
		cfg[k] = v
	}
	merged, _ := json.Marshal(cfg)
	if _, err := h.store.UpdateInstance(ctx, id, inst.Name, merged, domain.StatusActive, inst.LevelEnabled); err != nil {
		if rbErr := rollback(ctx); rbErr != nil {
			log.From(ctx).Error("integrations: rollback de materialización falló (posible SP huérfano)", "instance", logsafe.Text(id), "error", rbErr)
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	// Primer sync SCIM (asíncrono, durable). Best-effort: un fallo al encolar no revierte
	// la activación (el SP/client ya está persistido); el admin puede reintentar el sync.
	if out.EnqueueSCIM {
		if err := h.store.EnqueueSync(ctx, id); err != nil {
			log.From(ctx).Warn("integrations: no se pudo agendar el sync SCIM inicial", "instance", logsafe.Text(id), "error", err)
		}
	}

	resp := map[string]any{"status": domain.StatusActive, "launch_url": out.LaunchURL}
	if out.OIDCClientSecret != "" {
		resp["oidc_client_id"] = out.Config[manifest.KeyOIDCClientID]
		resp["oidc_client_secret"] = out.OIDCClientSecret // se muestra UNA vez.
	}
	if out.SAMLMetadataURL != "" {
		resp["saml_metadata_url"] = out.SAMLMetadataURL
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// Deactivate — POST /api/v1/instances/{id}/deactivate — revierte la materialización:
// borra en Sytue ID el SP SAML, olvida las claves internas y pasa la instancia a
// 'disabled'. El client OIDC no se puede borrar (W2A no lo expone): se olvida su id.
// Escritura ⇒ owner/admin.
func (h *Handlers) Deactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	inst, m, cfg, err := h.loadInstanceManifest(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if err := manifest.Deactivate(ctx, h.idApps, inst.OrgID, cfg); err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "id_unreachable",
			"No se pudo revertir la identidad en Sytue ID.").Wrap(err))
		return
	}

	// DESCONECTAR TIENE QUE REVOCAR, no solo cambiar un estado. Antes esto revertía la
	// identidad en Sytue ID y dejaba intactos los secretos: el token seguía vivo en el
	// vault y, sobre todo, seguía siendo válido EN EL TERCERO. Una organización que
	// «desconectaba» Jira seguía teniendo una credencial capaz de leer su Jira.
	//
	// Primero se avisa al proveedor (si publica endpoint de revocación) y después se
	// borran los secretos, en ese orden: para revocar hay que tener aún el token.
	h.revokeAtProvider(ctx, id, inst.TemplateKey, m, cfg)
	if err := h.store.PutSecrets(ctx, id, nil); err != nil && !errors.Is(err, store.ErrNoCipher) {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	cleared, _ := json.Marshal(manifest.ClearedConfig(cfg))
	if _, err := h.store.UpdateInstance(ctx, id, inst.Name, cleared, domain.StatusDisabled, inst.LevelEnabled); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"status": domain.StatusDisabled})
}

// loadInstanceManifest carga la instancia, su manifiesto de plantilla (parseado) y su
// config pública como mapa. Errores ya mapeados a *httpx.Error.
func (h *Handlers) loadInstanceManifest(ctx context.Context, id string) (*domain.AppInstance, *manifest.Manifest, map[string]string, error) {
	inst, err := h.store.GetInstance(ctx, id)
	if err != nil {
		return nil, nil, nil, toAPIError(err)
	}
	if err := h.mailInstanceAccess(ctx, inst); err != nil {
		return nil, nil, nil, err
	}

	tpl, err := h.store.GetTemplate(ctx, inst.TemplateKey)
	if err != nil {
		return nil, nil, nil, toAPIError(err)
	}
	m, err := manifest.Parse(tpl.Manifest)
	if err != nil {
		// Un manifiesto de catálogo inválido es un fallo del servidor, no del cliente.
		return nil, nil, nil, httpx.ErrInternal.Wrap(err)
	}
	cfg := map[string]string{}
	if len(inst.Config) > 0 {
		_ = json.Unmarshal(inst.Config, &cfg)
	}
	// Drafts created by older native clients stored JSON null. OAuth completion
	// must be able to add token metadata to those drafts when consent is retried.
	if cfg == nil {
		cfg = map[string]string{}
	}
	return inst, m, cfg, nil
}

// materializeError mapea un fallo de materialización: entrada inválida (campo faltante,
// plantilla mal resuelta) ⇒ 400; un fallo llamando a Sytue ID ⇒ 502.
func materializeError(err error) error {
	if errors.Is(err, manifest.ErrInvalid) {
		return httpx.ErrBadRequest.WithMessage("Check the integration details: " + err.Error())
	}
	return httpx.NewError(http.StatusBadGateway, "id_unreachable",
		"No se pudo dar de alta la identidad en Sytue ID.").Wrap(err)
}
