// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"cmp"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/plantillas"
)

// BLOQUES REUTILIZABLES: los de Sytue (catálogo común, plantillas.ListarBloques) y los de
// la organización (documentos de su biblioteca marcados como bloque). Los dos se piden
// igual y salen vestidos con la marca de la organización, en la versión que se pida.

// listarBloques — GET /blocks?kind=&locale=: lo que la cinta ofrece para insertar.
func (h *Handler) listarBloques(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	kind := domain.Kind(r.URL.Query().Get("kind"))
	if !kind.Valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown document kind."))
		return
	}
	lista, err := plantillas.ListarBloques(kind, r.URL.Query().Get("locale"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	deOrg := []domain.PlantillaOrg{}
	if h.plantillasOrg != nil {
		// El catálogo de Sytue se sigue ofreciendo aunque la biblioteca falle.
		if todas, err := h.plantillasOrg.ListarPlantillasOrg(r.Context(), kind); err == nil {
			for _, p := range todas {
				if p.Block {
					deOrg = append(deOrg, p)
				}
			}
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"blocks": lista, "org_blocks": deOrg})
}

// obtenerBloque — GET /blocks/{clave}?locale=&theme=&brand_version=&version=: el contenido
// de un bloque, listo para insertar. Sin `theme` se viste con la marca vigente de la
// organización si la tiene (y con ninguno si no); `theme=papel` lo pide neutro. `version`
// elige una versión publicada de un bloque de la organización (la vigente por defecto).
func (h *Handler) obtenerBloque(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	q := r.URL.Query()
	var tema *domain.TemaComun
	version, _ := strconv.Atoi(q.Get("brand_version"))
	switch t, err := h.resolverTema(r.Context(), cmp.Or(q.Get("theme"), domain.IDTemaMarca), version); {
	case err == nil:
		tema = &t
	case errors.Is(err, domain.ErrNotFound) && q.Get("theme") == "":
		// La organización no tiene marca: el bloque sale neutro.
	default:
		fail(w, r, err)
		return
	}
	clave := chi.URLParam(r, "clave")
	resp := map[string]any{"key": clave, "brand_version": 0}
	if tema != nil && tema.Marca {
		resp["brand_version"] = tema.Version
	}
	if idOrg, deOrg := strings.CutPrefix(clave, prefijoPlantillaOrg); deOrg {
		kind, contenido, no, err := h.bloqueDeOrg(r, idOrg, q.Get("version"))
		if err != nil {
			fail(w, r, err)
			return
		}
		if tema != nil {
			if contenido, err = domain.AplicarTemaBloque(kind, contenido, *tema); err != nil {
				fail(w, r, err)
				return
			}
		}
		// El uso queda en el registro de la biblioteca, como al crear desde una plantilla.
		_ = h.plantillasOrg.RegistrarUsoPlantillaOrg(r.Context(), idOrg, subj.Sub, no)
		resp["kind"], resp["content"], resp["version"] = kind, contenido, no
		httpx.JSON(w, r, http.StatusOK, resp)
		return
	}
	info, contenido, err := plantillas.GenerarBloque(clave, q.Get("locale"), tema)
	if errors.Is(err, plantillas.ErrNoExiste) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	resp["kind"], resp["content"] = info.Kind, contenido
	httpx.JSON(w, r, http.StatusOK, resp)
}

// bloqueDeOrg es el contenido de un bloque de la organización en la versión `no` (la
// vigente si viene vacía). Solo lo PUBLICADO y solo de la propia organización (RLS): no
// hace falta permiso sobre el documento, igual que para usar una plantilla.
func (h *Handler) bloqueDeOrg(r *http.Request, idDoc, no string) (domain.Kind, string, int, error) {
	if h.plantillasOrg == nil {
		return "", "", 0, domain.ErrNotFound
	}
	p, err := h.plantillasOrg.GetPlantillaOrg(r.Context(), idDoc)
	if err != nil {
		return "", "", 0, err
	}
	if !p.Block || len(p.Versions) == 0 {
		return "", "", 0, domain.ErrNotFound
	}
	elegida := p.Versions[0]
	if no != "" {
		n, _ := strconv.Atoi(no)
		elegida = domain.VersionPlantilla{}
		for _, v := range p.Versions {
			if v.No == n {
				elegida = v
			}
		}
		if elegida.VersionID == "" {
			return "", "", 0, domain.ErrNotFound
		}
	}
	estado, _, err := h.store.SnapshotState(r.Context(), elegida.SnapshotID, h.hydrateFromS3)
	if err != nil {
		return "", "", 0, err
	}
	contenido, err := domain.ExtraerBloque(p.Kind, estado.Contenido(p.Kind))
	return p.Kind, contenido, elegida.No, err
}
