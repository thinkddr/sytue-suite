// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/plantillas"
)

// listarPlantillas es la galería de «Crear nuevo»: GET /api/v1/templates?locale=es.
// Nombre y descripción ya vienen en el idioma pedido (el inglés si no lo hay).
func (h *Handler) listarPlantillas(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	lista, err := plantillas.Listar(r.URL.Query().Get("locale"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Las de la organización van aparte: son documentos suyos (con versiones), no
	// modelos embebidos, y `can_manage` dice si quien mira puede marcarlas.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"templates":     lista,
		"org_templates": h.plantillasOrgEnGaleria(r),
		"can_manage":    esAdminOrg(r),
	})
}

// desdePlantilla resuelve la plantilla de un documento nuevo: su ficha y la
// operación que siembra su contenido.
func (h *Handler) desdePlantilla(clave, idioma string) (plantillas.Info, domain.Op, error) {
	info, contenido, err := plantillas.Generar(clave, idioma)
	if errors.Is(err, plantillas.ErrNoExiste) {
		return info, domain.Op{}, httpx.ErrBadRequest.WithMessage("Unknown template.")
	}
	if err != nil {
		return info, domain.Op{}, httpx.ErrInternal.Wrap(err)
	}
	op, err := domain.ContentOp(info.Kind, contenido)
	return info, op, err
}
