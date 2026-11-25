// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/seo"
)

// montarSEO registra la revisión SEO del borrador de un sitio.
//
// La lee cualquier miembro: la revisión es del BORRADOR, que es lo que edita quien
// escribe los títulos, y no enseña nada que el editor no tenga ya delante. Publicar
// sigue siendo de quien administra; esto solo avisa antes.
func montarSEO(g grupos, h *Handlers) {
	g.api.Get("/sites/{id}/seo", h.revisarSEO)
}

// revisarSEO — GET /api/v1/sites/{id}/seo -> {issues: [seo.Aviso], pages, draft_rev}.
func (h *Handlers) revisarSEO(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	borrador, err := h.Store.Borrador(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	sitio, err := ir.Decodificar(borrador.Draft)
	if err != nil {
		// Un borrador que no se puede leer no tiene revisión SEO: lo dice la verja
		// de publicar con el detalle. Aquí basta con no inventarse un «todo bien».
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("ir_invalid"))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"issues":    seo.Revisar(sitio),
		"pages":     len(sitio.Pages),
		"draft_rev": borrador.DraftRev,
	})
}
