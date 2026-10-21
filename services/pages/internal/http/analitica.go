// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// montarAnalitica registra el panel de analítica de un sitio (§6.3, §12).
func montarAnalitica(g grupos, h *Handlers) {
	g.api.Get("/sites/{id}/analytics/summary", func(w http.ResponseWriter, r *http.Request) {
		id, err := idSitio(r)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		sitio, err := h.Store.SitioCompleto(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, errorAPI(err))
			return
		}
		if h.Analitica == nil {
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("analytics_unavailable"))
			return
		}
		tenant, _ := tenancy.From(r.Context())
		resumen, err := h.Analitica.Resumen(r.Context(), tenant.Slug, sitio.AnalyticsSiteID)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnavailable.WithCode("analytics_unavailable"))
			return
		}
		httpx.JSON(w, r, http.StatusOK, resumen)
	})
}
