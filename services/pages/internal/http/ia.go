// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ia"
)

// montarIA registra la clave propia, el cupo y los trabajos de IA (§6.3, §9.2-9.3).
func montarIA(g grupos, h *Handlers) {
	g.api.Route("/ia", func(r chi.Router) {
		r.Get("/key", h.obtenerClaveAI)
		r.Put("/key", h.guardarClaveAI)
		r.Delete("/key", h.borrarClaveAI)
		r.Post("/key:check", h.comprobarClaveAI)
		r.Get("/quota", h.obtenerCupoAI)
		r.Get("/jobs/{jid}", h.obtenerTrabajoAI)
		r.Get("/jobs/{jid}/events", h.streamTrabajoAI)
		r.With(conPlazo(plazoLargo)).Get("/jobs/{jid}/zip", h.descargarZipTrabajoAI)
	})

	g.api.Route("/sites/{id}/ia", func(r chi.Router) {
		r.Post("/design", h.crearTrabajoDesign)
		r.Post("/translate", h.crearTrabajoTranslate)
		r.Post("/export", h.crearTrabajoExport)
	})
}

func (h *Handlers) tratarErrorIA(w http.ResponseWriter, r *http.Request, err error) {
	if err == nil {
		return
	}
	var eCupo ia.ErrCupoAgotadoDetalle
	switch {
	case errors.As(err, &eCupo):
		httpx.Fail(w, r, httpx.ErrTooManyRequests.WithCode("ai_quota_exceeded").WithDetails(map[string]any{
			"reset_at": eCupo.ResetAt.Format(time.RFC3339),
		}))
	case errors.Is(err, ia.ErrAIOcupada):
		httpx.Fail(w, r, httpx.ErrTooManyRequests.WithCode("ai_busy"))
	case errors.Is(err, ia.ErrAvisoRequerido):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("notice_required"))
	case errors.Is(err, ia.ErrClaveInvalida):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("key_invalid"))
	case errors.Is(err, ia.ErrCifradoNoDisponible):
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("encryption_unavailable"))
	case errors.Is(err, ia.ErrModeloInvalido):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_model"))
	case errors.Is(err, ia.ErrAlcanceTraduccion):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_translation_scope"))
	case errors.Is(err, ia.ErrIdiomasTraduccion):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_languages"))
	case errors.Is(err, ia.ErrTraduccionTiendaNoDisponible):
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("translation_scope_unavailable"))
	case errors.Is(err, ia.ErrTrabajoNoEncontrado), errors.Is(err, domain.ErrNoExiste):
		httpx.Fail(w, r, httpx.ErrNotFound.WithCode("not_found"))
	case errors.Is(err, ia.ErrTrabajoNoExport):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("not_ready"))
	default:
		httpx.Fail(w, r, errorAPI(err))
	}
}

func (h *Handlers) obtenerClaveAI(w http.ResponseWriter, r *http.Request) {
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	t, _ := tenancy.From(r.Context())
	vista, err := h.IA.ObtenerClave(r.Context(), t.Slug)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, vista)
}

func (h *Handlers) guardarClaveAI(w http.ResponseWriter, r *http.Request) {
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	t, _ := tenancy.From(r.Context())
	var in ia.GuardarClavePeticion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	vista, err := h.IA.GuardarClave(r.Context(), t.Slug, sub(r), in)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, vista)
}

func (h *Handlers) borrarClaveAI(w http.ResponseWriter, r *http.Request) {
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	t, _ := tenancy.From(r.Context())
	if err := h.IA.BorrarClave(r.Context(), t.Slug, sub(r)); err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true})
}

type comprobarClavePeticion struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
}

func (h *Handlers) comprobarClaveAI(w http.ResponseWriter, r *http.Request) {
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	t, _ := tenancy.From(r.Context())
	var in comprobarClavePeticion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Provider == "" || in.Key == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("missing_fields"))
		return
	}
	modelos, err := h.IA.ComprobarClave(r.Context(), t.Slug, sub(r), in.Provider, in.Key)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"models": modelos})
}

func (h *Handlers) obtenerCupoAI(w http.ResponseWriter, r *http.Request) {
	t, _ := tenancy.From(r.Context())
	claims, _ := authn.FromContext(r.Context())
	cupo, err := h.IA.ObtenerCupo(r.Context(), t.Slug, claims.Plan)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, cupo)
}

func (h *Handlers) crearTrabajoDesign(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	claims, _ := authn.FromContext(r.Context())

	var in domain.DesignInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Prompt == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("prompt_required"))
		return
	}
	if in.Language == "" {
		in.Language = "es"
	}
	if in.Mode == "" {
		in.Mode = "sections"
	}

	job, err := h.IA.LanzarDesign(r.Context(), t.Slug, sub(r), id, in, claims.Plan)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"job": job})
}

func (h *Handlers) crearTrabajoTranslate(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	claims, _ := authn.FromContext(r.Context())

	var in domain.TranslateInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.From == "" || len(in.To) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_languages"))
		return
	}
	if in.Scope == "store" && !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	if in.Scope == "" {
		in.Scope = "site"
	}
	if err := ia.ValidarAlcanceTraduccion(in.Scope); err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	if err := ia.ValidarIdiomasTraduccion(in); err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}

	job, err := h.IA.LanzarTranslate(r.Context(), t.Slug, sub(r), id, in, claims.Plan)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"job": job})
}

func (h *Handlers) crearTrabajoExport(w http.ResponseWriter, r *http.Request) {
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, _ := tenancy.From(r.Context())
	claims, _ := authn.FromContext(r.Context())

	var in domain.ExportInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Destination == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("destination_required"))
		return
	}

	job, err := h.IA.LanzarExport(r.Context(), t.Slug, sub(r), id, in, claims.Plan)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"job": job})
}

func (h *Handlers) obtenerTrabajoAI(w http.ResponseWriter, r *http.Request) {
	jid, err := uuid.Parse(chi.URLParam(r, "jid"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_job_id"))
		return
	}
	t, _ := tenancy.From(r.Context())
	job, err := h.IA.ObtenerTrabajo(r.Context(), t.Slug, jid)
	if err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, job)
}

func (h *Handlers) streamTrabajoAI(w http.ResponseWriter, r *http.Request) {
	jid, err := uuid.Parse(chi.URLParam(r, "jid"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_job_id"))
		return
	}
	t, _ := tenancy.From(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			job, err := h.IA.ObtenerTrabajo(r.Context(), t.Slug, jid)
			if err != nil {
				return
			}
			data, _ := json.Marshal(job)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			if job.Status == domain.AIJobStatusDone || job.Status == domain.AIJobStatusFailed {
				return
			}
		}
	}
}

func (h *Handlers) descargarZipTrabajoAI(w http.ResponseWriter, r *http.Request) {
	if !esAdmin(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("admin_required"))
		return
	}
	jid, err := uuid.Parse(chi.URLParam(r, "jid"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithCode("invalid_job_id"))
		return
	}
	t, _ := tenancy.From(r.Context())

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="export-%s.zip"`, jid.String()[:8]))
	if err := h.IA.ObtenerZip(r.Context(), t.Slug, jid, w); err != nil {
		h.tratarErrorIA(w, r, err)
		return
	}
}
