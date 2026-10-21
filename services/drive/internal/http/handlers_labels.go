// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Etiquetado de ficheros con etiquetas de SENSIBILIDAD (paridad Purview / Google
// Drive labels).
//
// La taxonomía es de Sytue ID; Drive guarda un SNAPSHOT sobre el fichero (mig 0013).
// Por eso poner una etiqueta tiene DOS pasos:
//
//  1. RESOLVER el id de etiqueta contra la taxonomía VIVA de la organización
//     (GET /api/v1/sensitivity-labels en Sytue ID, con el MISMO Bearer del usuario:
//     ve exactamente las etiquetas de SU org y ninguna más).
//  2. GUARDAR el snapshot resuelto sobre el fichero.
//
// El cliente manda SOLO el id de etiqueta: nombre, nivel y política los pone el
// servidor desde la taxonomía. Si el cliente pudiera mandarlos, cualquiera podría
// marcar un fichero como «Confidencial» con block_external_share=false y saltarse el
// bloqueo. Aquí no hay nada que falsificar.
//
// Camino FRÍO (una llamada por etiquetado). El camino caliente —crear un enlace,
// descargar— decide con el snapshot local, sin salir a la red.
package drivehttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// labelLookupTimeout acota la llamada a Sytue ID al etiquetar. Si el plano de control
// no responde, el etiquetado falla (503) en vez de colgar la petición: es preferible
// «no se pudo etiquetar ahora» a guardar un snapshot a medias o inventado.
const labelLookupTimeout = 5 * time.Second

// setFileLabelRequest es el cuerpo de PUT /files/{id}/label. label_id vacío o ausente
// no vale aquí (para quitar la etiqueta está el DELETE, explícito).
type setFileLabelRequest struct {
	LabelID string `json:"label_id"`
}

// taxonomyLabel es una etiqueta TAL COMO la publica Sytue ID. Lleva más campos que el
// snapshot (color y descripción son de presentación: los pinta el selector, no
// participan en ninguna decisión), por eso no es domain.Label.
type taxonomyLabel struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Rank               int    `json:"rank"`
	Color              string `json:"color,omitempty"`
	Description        string `json:"description,omitempty"`
	BlockExternalShare bool   `json:"block_external_share"`
}

// snapshot es lo ÚNICO que se guarda sobre el fichero: identidad, nivel y política.
func (l taxonomyLabel) snapshot() domain.Label {
	return domain.Label{ID: l.ID, Name: l.Name, Rank: l.Rank, BlockExternalShare: l.BlockExternalShare}
}

// SetFileLabel — PUT /api/v1/files/{id}/label — pone la etiqueta de sensibilidad de un
// fichero, resolviéndola contra la taxonomía de la organización.
func (h *Handlers) SetFileLabel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req setFileLabelRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	labelID := strings.TrimSpace(req.LabelID)
	if labelID == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Give label_id. To remove the label, send DELETE to this same resource.",
		))
		return
	}
	label, err := h.resolveLabel(r, labelID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.SetFileLabel(r.Context(), id, label, domain.PermWrite)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// ClearFileLabel — DELETE /api/v1/files/{id}/label — quita la etiqueta del fichero. No
// consulta la taxonomía: desetiquetar es limpiar el snapshot.
func (h *Handlers) ClearFileLabel(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.SetFileLabel(r.Context(), id, domain.Label{}, domain.PermWrite)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// ListSensitivityLabels — GET /api/v1/sensitivity-labels — proxy de LECTURA de la
// taxonomía de la org del usuario, para que la UI de Drive pinte el selector sin
// necesitar una URL de Sytue ID ni otro token. Mismo Bearer, mismo aislamiento.
func (h *Handlers) ListSensitivityLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := h.fetchTaxonomy(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"labels": labels})
}

// resolveLabel busca labelID en la taxonomía de la org del solicitante. Un id que no
// esté en ELLA (inexistente o de otra organización) ⇒ 400: no se etiqueta con algo que
// esta organización no ha definido.
func (h *Handlers) resolveLabel(r *http.Request, labelID string) (domain.Label, error) {
	labels, err := h.fetchTaxonomy(r)
	if err != nil {
		return domain.Label{}, err
	}
	for _, l := range labels {
		if l.ID == labelID {
			return l.snapshot(), nil
		}
	}
	return domain.Label{}, httpx.ErrBadRequest.WithMessage(
		"That label does not exist in your organisation's taxonomy.",
	)
}

// fetchTaxonomy lee la taxonomía de la org del solicitante desde Sytue ID reenviando
// su propio Authorization (nunca una credencial de servicio): Sytue ID acota por el
// tenant del token, así que Drive no puede pedir —ni por error— las etiquetas de otra
// organización.
//
// ponytail: sin caché. Es un camino frío (etiquetar / abrir el selector) y una caché
// obligaría a invalidarla cuando el admin toca la taxonomía. Si el selector se
// convierte en un camino caliente, cachear por (org, ETag) unos segundos.
func (h *Handlers) fetchTaxonomy(r *http.Request) ([]taxonomyLabel, error) {
	authz := r.Header.Get("Authorization")
	if authz == "" {
		return nil, httpx.ErrUnauthorized
	}
	base := strings.TrimRight(h.cfg.IDBaseURL, "/")
	if base == "" {
		return nil, httpx.ErrInternal.WithMessage("Sytue ID is not configured in this service.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), labelLookupTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/sensitivity-labels", http.NoBody)
	if err != nil {
		return nil, httpx.ErrInternal.Wrap(err)
	}
	req.Header.Set("Authorization", authz)
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, httpx.NewError(http.StatusServiceUnavailable, "id_unavailable",
			"No se ha podido consultar las etiquetas de la organización. Inténtalo de nuevo en unos segundos.").Wrap(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return nil, httpx.ErrForbidden.WithMessage("Your session does not allow reading the organisation's labels.")
	}
	if res.StatusCode != http.StatusOK {
		return nil, httpx.NewError(http.StatusServiceUnavailable, "id_unavailable",
			"No se ha podido consultar las etiquetas de la organización.").
			Wrap(fmt.Errorf("sytue id: status %d", res.StatusCode))
	}
	var body struct {
		Items []taxonomyLabel `json:"items"`
	}
	// Cota de lectura: la taxonomía de una org son decenas de etiquetas; un cuerpo
	// mayor es un error de configuración, no un caso legítimo.
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return nil, httpx.ErrInternal.Wrap(err)
	}
	if body.Items == nil {
		body.Items = []taxonomyLabel{}
	}
	return body.Items, nil
}
