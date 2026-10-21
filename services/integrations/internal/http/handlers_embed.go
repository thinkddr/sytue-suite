// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"errors"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
)

// EmbedToken — POST /api/v1/instances/{id}/embed/token — plano de PRESENTACIÓN (L4):
// mintea SERVER-SIDE un artefacto de embed acotado (token de vida corta) para
// incrustar una vista del SaaS (informe Power BI) en la suite. El cuerpo es la
// petición del driver (report_id + group_id/access_level opcionales, opaca para el
// núcleo); la respuesta es el Embed del contrato: { embed_url, token, token_type,
// expires_at }.
//
// Autorización: la lista de la conexión (NEG-06). Mintar un embed es una lectura de la
// instancia con la credencial de la organización —gobernamos QUIÉN mintea, no el
// contenido del informe (L4 es integración de presentación, no DLP)—. Despacha a la capacidad EmbedBroker del
// driver; el egress sale por el Runtime (SSRF + rate-limit + auditoría + Bearer del
// service principal inyectado desde el vault). En egress_audit el Authorization va
// REDACTADO y el cuerpo —que lleva el token minted— jamás se persiste.
func (h *Handlers) EmbedToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	spec, err := readSpec(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// Misma verja que la consulta tabular: incrustar un informe es leer del tercero.
	if err := h.gateCapability(ctx, id, manifest.CapData); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.puedeConsultar(ctx, id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	inst, err := h.loadInstance(ctx, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	drv, cfg, err := h.resolveCapability(ctx, inst.TemplateKey, inst.Config, manifest.CapData)
	if err != nil {
		httpx.Fail(w, r, capabilityError(err, "embedding reports"))
		return
	}
	inst.Config = cfg
	broker, ok := drv.(connector.EmbedBroker)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This connector does not support embedding."))
		return
	}

	// Runtime: único cliente de egress. El Bearer del service principal sale del vault
	// y lo inyecta la pasarela como cabecera; el driver no lo ve.
	rt := h.egressRuntime(inst, log.From(ctx).With("instance", inst.ID, "org", inst.OrgID))

	// El driver recibe la instancia SIN el "bearer" (inyectado en el Runtime); los
	// demás secretos sí (mismo criterio que QueryData).
	driverInst := inst
	driverInst.Secrets = driverSecrets(inst.Secrets)

	embed, err := broker.EmbedToken(ctx, driverInst, rt, spec)
	if err != nil {
		httpx.Fail(w, r, embedError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, embed)
}

// embedError mapea el fallo de un mint de embed: entrada inválida del usuario
// (connector.ErrBadQuery) ⇒ 400 con el detalle; cualquier otro (destino bloqueado por
// SSRF, upstream, red) ⇒ 502.
func embedError(err error) error {
	if blocked := blockedErr(err); blocked != nil {
		return blocked
	}
	if errors.Is(err, connector.ErrBadQuery) {
		msg := "The embed request is not valid."
		if _, detail, found := strings.Cut(err.Error(), connector.ErrBadQuery.Error()+": "); found {
			msg = "The embed request is not valid: " + detail
		}
		return httpx.ErrBadRequest.WithMessage(msg)
	}
	return httpx.NewError(http.StatusBadGateway, "embed_failed",
		"No se pudo emitir el artefacto de incrustación.").Wrap(err)
}
