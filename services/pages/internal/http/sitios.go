// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/plantillas"
)

func montarSitios(g grupos, h *Handlers) {
	g.api.Route("/sites", func(r chi.Router) {
		r.Get("/", h.listarSitios)
		r.Post("/", h.crearSitio)
		r.Get("/check-slug", h.comprobarSlug)

		// Quitar de internet la web de la organización, o devolverla, es de quien la
		// administra, como publicar y retirar (WEB-02). Un miembro edita; no decide eso.
		soloAdmin := h.Verifier.RequireRole("owner", "admin")
		r.Route("/{id}", func(sr chi.Router) {
			sr.Get("/", h.obtenerSitio)
			sr.Put("/", h.actualizarSitio)
			sr.With(soloAdmin).Delete("/", h.borrarSitio)
			sr.With(soloAdmin).Post("/restore", h.restaurarSitio)
			sr.Get("/versions", h.listarVersiones)
			sr.Post("/versions/{vid}/restore", h.restaurarVersion)
		})
	})

	// El conector de búsqueda del Hub: GET /api/v1/search?q=&limit=.
	g.api.Get("/search", h.buscarSitios)

	g.api.Route("/templates", func(r chi.Router) {
		r.Get("/", h.listarPlantillas)
		r.Get("/sections", h.listarSeccionesPlantilla)
		r.Get("/{id}", h.obtenerPlantilla)
	})
}

func (h *Handlers) listarSitios(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}

	incluirBorrados := r.URL.Query().Get("include_deleted") == "true" || r.URL.Query().Get("deleted") == "true"
	sitios, err := h.Store.ListarSitios(r.Context(), tenant.Slug, incluirBorrados)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if sitios == nil {
		sitios = []domain.SitioCompleto{}
	}
	httpx.JSON(w, r, http.StatusOK, sitios)
}

// buscarSitios responde a la búsqueda federada del Hub: {"items":[…]}. Sin `q` devuelve
// los sitios editados más recientemente.
func (h *Handlers) buscarSitios(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	limite, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := h.Store.BuscarSitios(r.Context(), tenant.Slug, r.URL.Query().Get("q"), limite)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if items == nil {
		items = []domain.ResultadoBusqueda{}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) crearSitio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}

	var in domain.CrearSitioInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	sitio, err := h.Store.CrearSitio(r.Context(), tenant.Slug, in, sub(r))
	if err != nil {
		if errors.Is(err, domain.ErrConflictoBorrador) {
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "slug_conflict", err.Error()))
			return
		}
		if errors.Is(err, domain.ErrBorradorInvalido) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_input").Wrap(err))
			return
		}
		if errors.Is(err, domain.ErrLimiteSitios) {
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "site_limit", err.Error()))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	httpx.JSON(w, r, http.StatusCreated, sitio)
}

func (h *Handlers) comprobarSlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if slug == "" {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"available": false, "reason": "empty"})
		return
	}
	disponible, err := h.Store.ComprobarSlugDisponible(r.Context(), slug)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"slug": slug, "available": disponible})
}

func (h *Handlers) obtenerSitio(w http.ResponseWriter, r *http.Request) {
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
	httpx.JSON(w, r, http.StatusOK, sitio)
}

func (h *Handlers) actualizarSitio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var in domain.ActualizarSitioInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	sitio, err := h.Store.ActualizarAjustesSitio(r.Context(), tenant.Slug, id, in, esAdmin(r))
	if err != nil {
		if errors.Is(err, domain.ErrConflictoBorrador) {
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "slug_conflict", err.Error()))
			return
		}
		if errors.Is(err, domain.ErrBorradorInvalido) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_input").Wrap(err))
			return
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	httpx.JSON(w, r, http.StatusOK, sitio)
}

func (h *Handlers) borrarSitio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if err := h.Store.BorrarSitio(r.Context(), tenant.Slug, id); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "deleted": true})
}

func (h *Handlers) restaurarSitio(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	if err := h.Store.RestaurarSitio(r.Context(), tenant.Slug, id); err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "restored": true})
}

func (h *Handlers) listarVersiones(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	versiones, err := h.Store.ListarVersiones(r.Context(), tenant.Slug, id)
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	if versiones == nil {
		versiones = []domain.VersionSitio{}
	}
	httpx.JSON(w, r, http.StatusOK, versiones)
}

func (h *Handlers) restaurarVersion(w http.ResponseWriter, r *http.Request) {
	tenant, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vid, err := uuid.Parse(chi.URLParam(r, "vid"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_version_id").Wrap(err))
		return
	}

	rev, err := h.Store.RestaurarVersion(r.Context(), tenant.Slug, id, vid, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "rev": rev})
}

func (h *Handlers) listarPlantillas(w http.ResponseWriter, r *http.Request) {
	lista, err := plantillas.ListarPlantillas()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, lista)
}

func (h *Handlers) obtenerPlantilla(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := plantillas.ObtenerPlantilla(id)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("template_not_found"))
		return
	}
	httpx.JSON(w, r, http.StatusOK, p)
}

func (h *Handlers) listarSeccionesPlantilla(w http.ResponseWriter, r *http.Request) {
	secciones, err := plantillas.ListarSecciones()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, secciones)
}
