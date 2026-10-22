// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// maxQuerySpecBytes acota el cuerpo de una consulta (dataset/modelo + filtros +
// paginación): es una spec pequeña, jamás datos.
const maxQuerySpecBytes = 64 << 10

// QueryData — POST /api/v1/instances/{id}/data/query — el plano de datos L3: trae
// datos tabulares del SaaS de la instancia a Sytue Sheets. El cuerpo es la spec
// del driver (dataset/modelo + $select/filtros/paginación, opaca para el núcleo);
// la respuesta es el Table del contrato: { columns, rows }.
//
// La consultan los administradores, quien creó la conexión y quien esté en su lista
// (NEG-06): sale con la credencial de la organización. Despacha a la capacidad
// TabularSource del driver de la instancia; el egress sale por el Runtime de la
// pasarela (SSRF + rate-limit + auditoría + inyección de auth).
func (h *Handlers) QueryData(w http.ResponseWriter, r *http.Request) {
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

	// Verja: una conexión desactivada, o con la capacidad de datos apagada, NO sale a
	// la red. Sin esto, «desactivar» era cosmético para justo el camino que toca al
	// tercero.
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
		httpx.Fail(w, r, capabilityError(err, "fetching data"))
		return
	}
	inst.Config = cfg
	ts, ok := drv.(connector.TabularSource)
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("This connector does not expose tabular data."))
		return
	}

	// Runtime: único cliente de egress. El Bearer convencional sale del vault y lo
	// inyecta la pasarela como cabecera (conectores header-auth: OData). El driver no lo ve.
	rt := h.egressRuntime(inst, log.From(ctx).With("instance", inst.ID, "org", inst.OrgID))

	// El driver recibe la instancia SIN el secreto "bearer" (va inyectado en el
	// Runtime). Los DEMÁS secretos SÍ se entregan: protocolos que llevan la
	// credencial en el CUERPO (Odoo JSON-RPC: db/uid/api_key) no pueden usar la
	// inyección en cabecera. Los conectores header-auth (OData) ignoran los
	// secretos y usan el Bearer inyectado, así que su garantía se mantiene.
	driverInst := inst
	driverInst.Secrets = driverSecrets(inst.Secrets)

	table, err := ts.Query(ctx, driverInst, rt, spec)
	if err != nil {
		httpx.Fail(w, r, queryError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, table)
}

// readSpec lee y valida el cuerpo de la consulta (JSON, acotado). Un cuerpo vacío
// es válido (el driver aplica sus defaults); uno demasiado grande o no-JSON, 400.
func readSpec(r *http.Request) (json.RawMessage, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxQuerySpecBytes+1))
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("The query body could not be read.")
	}
	if len(body) > maxQuerySpecBytes {
		return nil, httpx.ErrBadRequest.WithMessage("The query is too large.")
	}
	if len(body) == 0 {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(body) {
		return nil, httpx.ErrBadRequest.WithMessage("The query must be a JSON object.")
	}
	return json.RawMessage(body), nil
}

// driverSecrets devuelve los secretos que se entregan al driver: TODOS menos la
// clave "bearer" convencional (esa la inyecta el Runtime como cabecera). Devuelve
// nil si no queda ninguno (caso header-auth puro: OData).
func driverSecrets(all map[string]string) map[string]string {
	out := make(map[string]string, len(all))
	for k, v := range all {
		if k != store.SecretKeyBearer {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// queryError mapea el fallo de una consulta tabular: entrada inválida del usuario
// (connector.ErrBadQuery) ⇒ 400 con el detalle concreto; cualquier otro (destino
// bloqueado por SSRF, upstream, red) ⇒ 502.
func queryError(err error) error {
	if blocked := blockedErr(err); blocked != nil {
		return blocked
	}
	if errors.Is(err, connector.ErrBadQuery) {
		msg := "The query is not valid."
		if _, detail, found := strings.Cut(err.Error(), connector.ErrBadQuery.Error()+": "); found {
			msg = "The query is not valid: " + detail
		}
		return httpx.ErrBadRequest.WithMessage(msg)
	}
	return httpx.NewError(http.StatusBadGateway, "query_failed",
		"La consulta al origen de datos falló.").Wrap(err)
}
