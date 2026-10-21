// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// POST /internal/orgs/status {slugs:[…≤500]}
// → {orgs:[{slug, status: "active"|"suspended"|"merged"|"deleted"}]}
//
// Informa del estado de ciclo de vida de hasta 500 organizaciones por slug.
// Un slug desconocido se devuelve como "deleted". Lo usan el mapa host→sitio/versión
// y la comprobación de slugs de Pages para saber si una organización sigue activa
// (suspendida ⇒ la web deja de servirse; eliminada o fusionada ⇒ ídem).
//
// Solo desde DENTRO con su secreto PROPIO (Config.EstadoOrgsSecret, compartido
// solo con Pages). Sin secreto configurado la ruta NO se monta. Con techo global.
func (s *Server) handleInternalOrgsStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Slugs []string `json:"slugs"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.Slugs) == 0 {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"orgs": []any{}})
		return
	}
	if len(req.Slugs) > 500 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many slugs (max 500)."))
		return
	}

	estados, err := s.store.EstadoDeOrgs(r.Context(), req.Slugs)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}

	type orgEstado struct {
		Slug   string `json:"slug"`
		Status string `json:"status"`
	}
	resultado := make([]orgEstado, 0, len(req.Slugs))
	for _, slug := range req.Slugs {
		estado := "deleted"
		if s, ok := estados[slug]; ok {
			estado = s
		}
		resultado = append(resultado, orgEstado{Slug: slug, Status: estado})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"orgs": resultado})
}
