// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package pageshttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// montarNucleo registra lo del agente `servicio` (PAGES-ENCARGO §6.3): sitios,
// borrador, versiones, instantáneas, vistas previas, publicaciones, recursos,
// plantillas, catálogo y «Exportar tus datos».
//
// ESQUELETO: vacío. Las rutas llegan con las PR2 y PR3 de `servicio`.
func montarNucleo(g grupos, h *Handlers) {
	pubGate := httpx.SecretoEnCabecera("X-Sytue-Publicador", h.Cfg.PublicadorSecret)

	// RUTAS PLANAS y no `Route("/sites/{id}", …)`: un montaje con `{id}` se queda con TODO
	// lo que cuelga de `/sites/{id}` y tapa lo que otra área registre en el mismo prefijo.
	// `sitios.go` monta `/sites/{id}` con su GET, PUT, DELETE, restore y versions, y
	// perdían contra este: respondían 404/405 (TestNingunaRutaQuedaTapada).
	sitio := func(sub string) string { return "/sites/{id}" + sub }
	g.api.Get(sitio("/draft"), h.obtenerBorrador)
	g.api.Put(sitio("/draft"), h.guardarBorrador)
	g.api.Post(sitio("/versions"), h.crearVersion)
	g.api.Get(sitio("/render-data"), h.obtenerDatosRender)
	// Una vista de Sytue Bases como tabla del sitio (bases.go).
	g.api.With(httpx.MaxBodyBytes(4<<10)).Post(sitio("/bases/table"), h.tablaDesdeBases)
	g.api.Post(sitio("/snapshots"), h.crearSnapshot)
	g.api.With(pubGate).Post(sitio("/previews"), h.crearVistaPrevia)
	g.api.Get(sitio("/publications"), h.listarPublicaciones)
	g.api.Post(sitio("/publications/{number}/rollback"), h.rollbackPublicacion)
	g.api.Post(sitio("/unpublish"), h.despublicarSitio)
	g.api.With(conPlazo(plazoLargo), pubGate).Post(sitio("/publications"), h.subirPublicacion)
	g.api.With(conPlazo(plazoLargo), pubGate).Post(sitio("/exports/astro"), h.exportarAstro)
	g.api.Get(sitio("/files/{nombre}"), h.sitioFichero)
	g.api.Get(sitio("/assets"), h.listarRecursos)
	g.api.With(conPlazo(plazoLargo)).Post(sitio("/assets"), h.subirRecurso)
	g.api.Delete(sitio("/assets/{aid}"), h.borrarRecurso)

	g.api.Route("/catalog", func(r chi.Router) {
		r.Get("/fonts", h.catalogoFuentes)
		r.Get("/library", h.catalogoBiblioteca)
		r.Get("/themes", h.catalogoTemas)
		r.Get("/files/{nombre}", h.catalogoFichero)
	})

	g.apiPersonal.Get("/export", h.exportarHub)
}

func idSitio(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrBadRequest.WithCode("invalid_site_id").Wrap(err)
	}
	return id, nil
}

func sub(r *http.Request) string {
	if claims, ok := authn.FromContext(r.Context()); ok {
		return claims.Subject
	}
	return ""
}

func (h *Handlers) obtenerBorrador(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err == nil {
		var sitio domain.Sitio
		sitio, err = h.Store.Borrador(r.Context(), id)
		if err == nil {
			httpx.JSON(w, r, http.StatusOK, map[string]any{"id": sitio.ID, "slug": sitio.Slug, "name": sitio.Name, "template": sitio.Template, "ir": sitio.Draft, "rev": sitio.DraftRev})
			return
		}
	}
	httpx.Fail(w, r, errorAPI(err))
}

type guardarBorradorPeticion struct {
	Rev int64           `json:"rev"`
	IR  json.RawMessage `json:"ir"`
}

func (h *Handlers) guardarBorrador(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in guardarBorradorPeticion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	problemas := validarBorrador(in.IR)
	if hayErrores(problemas) {
		httpx.Fail(w, r, httpx.ErrUnprocessable.WithCode("ir_invalid").WithParams(map[string]any{"problems": problemas}))
		return
	}
	rev, err := h.Store.GuardarBorrador(r.Context(), id, in.Rev, in.IR, sub(r))
	if err != nil {
		if errors.Is(err, domain.ErrConflictoBorrador) {
			sitio, leer := h.Store.Borrador(r.Context(), id)
			if leer == nil {
				httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "draft_conflict", "The draft changed.").WithParams(map[string]any{"current_rev": sitio.DraftRev}))
				return
			}
		}
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	avisos := soloAvisos(problemas)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"rev": rev, "warnings": avisos})
}

type crearVersionPeticion struct {
	Name string `json:"name"`
}

func (h *Handlers) crearVersion(w http.ResponseWriter, r *http.Request) {
	id, err := idSitio(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in crearVersionPeticion
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.Store.CrearVersion(r.Context(), id, in.Name, sub(r))
	if err != nil {
		httpx.Fail(w, r, errorAPI(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, v)
}

func validarBorrador(raw []byte) []ir.Problema {
	if problemas := ir.ValidarCrudo(raw); len(problemas) > 0 {
		return problemas
	}
	sitio, err := ir.Decodificar(raw)
	if err != nil {
		return []ir.Problema{{Code: "json", Severity: "error"}}
	}
	return ir.Validar(sitio, nil, ir.ModoBorrador)
}

func hayErrores(problemas []ir.Problema) bool {
	for _, p := range problemas {
		if p.Severity == "error" {
			return true
		}
	}
	return false
}

func soloAvisos(problemas []ir.Problema) []ir.Problema {
	out := make([]ir.Problema, 0, len(problemas))
	for _, p := range problemas {
		if p.Severity == "warning" {
			out = append(out, p)
		}
	}
	return out
}

func errorAPI(err error) error {
	if errors.Is(err, domain.ErrNoExiste) {
		return httpx.ErrNotFound.Wrap(err)
	}
	if errors.Is(err, domain.ErrBorradorInvalido) {
		return httpx.ErrBadRequest.WithCode("invalid_draft").Wrap(err)
	}
	if errors.Is(err, domain.ErrSoloAdministracion) {
		return httpx.ErrForbidden.WithCode("admin_required").Wrap(err)
	}
	if errors.Is(err, domain.ErrDominioInvalido) {
		return httpx.NewError(http.StatusUnprocessableEntity, "domain_invalid", err.Error())
	}
	if errors.Is(err, domain.ErrDominioReservado) {
		return httpx.NewError(http.StatusUnprocessableEntity, "domain_reserved", err.Error())
	}
	if errors.Is(err, domain.ErrDominioEnUso) {
		return httpx.NewError(http.StatusConflict, "domain_taken", err.Error())
	}
	if errors.Is(err, domain.ErrLimiteDominios) {
		return httpx.NewError(http.StatusConflict, "domain_limit", err.Error())
	}
	if errors.Is(err, domain.ErrSitioSuspendido) {
		return httpx.NewError(http.StatusConflict, "site_suspended", err.Error())
	}
	return err
}
