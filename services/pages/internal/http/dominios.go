// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// montarDominios registra los dominios propios de un sitio (§6.3).
func montarDominios(g grupos, h *Handlers) {
	g.api.Route("/sites/{id}/domains", func(r chi.Router) {
		r.Use(h.Verifier.RequireRole("owner", "admin"))
		r.Get("/", h.listarDominios)
		r.Post("/", h.crearDominio)
		r.Post("/{did}/check", h.comprobarDominio)
		r.Post("/{did}/primary", h.hacerDominioPrincipal)
		r.Delete("/{did}", h.borrarDominio)
	})
}

func (h *Handlers) listarDominios(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	lista, err := h.Dominios.Listar(r.Context(), tenant.Slug, siteID)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, lista)
}

type crearDominioPeticion struct {
	Host string `json:"host"`
}

func (h *Handlers) crearDominio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var req crearDominioPeticion
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_json").Wrap(err))
		return
	}

	creados, err := h.Dominios.Alta(r.Context(), tenant.Slug, siteID, req.Host, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, creados)
}

func (h *Handlers) comprobarDominio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	did, err := uuid.Parse(chi.URLParam(r, "did"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_domain_id").Wrap(err))
		return
	}

	res, err := h.Dominios.Comprobar(r.Context(), tenant.Slug, siteID, did)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

func (h *Handlers) hacerDominioPrincipal(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	did, err := uuid.Parse(chi.URLParam(r, "did"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_domain_id").Wrap(err))
		return
	}

	res, err := h.Dominios.HacerPrincipal(r.Context(), tenant.Slug, siteID, did)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

func (h *Handlers) borrarDominio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	siteID, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	did, err := uuid.Parse(chi.URLParam(r, "did"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_domain_id").Wrap(err))
		return
	}

	if err := h.Dominios.Borrar(r.Context(), tenant.Slug, siteID, did); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
