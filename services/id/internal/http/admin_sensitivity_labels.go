// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Etiquetas de sensibilidad (paridad Purview / Google Drive labels): la TAXONOMÍA de
// la organización activa. Dos superficies con permisos distintos:
//
//   - ADMINISTRACIÓN (/api/v1/admin/sensitivity-labels, CRUD): reservada a owner/admin,
//     como el resto del gobierno de la org (roles, PIM, gobernanza, cumplimiento).
//     Definir la taxonomía de clasificación es una decisión de cumplimiento, no una
//     tarea delegable con un permiso del catálogo actual — y el catálogo es FIJO
//     (domain.AllPermissions), así que no se inventa uno nuevo aquí.
//   - LECTURA (/api/v1/sensitivity-labels): cualquier usuario autenticado de la org.
//     La consumen el frontend (para pintar el selector de etiqueta) y OTROS SERVICIOS
//     —Drive resuelve el id de etiqueta a su snapshot al etiquetar un fichero— con el
//     Bearer del propio usuario. Solo lectura y acotada a SU org.
//
// Aislamiento por tenant en TODO (claim tenant → org activa): una etiqueta de otra org
// ⇒ 404, nunca se lee ni se toca.
package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// labelWire es la forma JSON de una etiqueta. La MISMA para la administración y para
// la lectura de otros servicios: lo que Drive necesita para el snapshot (id, name,
// rank, block_external_share) es exactamente lo que ve el admin.
func labelWire(l domain.SensitivityLabel) map[string]any {
	return map[string]any{
		"id":                   l.ID,
		"name":                 l.Name,
		"rank":                 l.Rank,
		"color":                l.Color,
		"description":          l.Description,
		"block_external_share": l.BlockExternalShare,
		"created_at":           l.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		"updated_at":           l.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// labelRequest es el cuerpo de create/update. Punteros/omitidos = "no tocar" (PATCH).
type labelRequest struct {
	Name               *string `json:"name"`
	Rank               *int    `json:"rank"`
	Color              *string `json:"color"`
	Description        *string `json:"description"`
	BlockExternalShare *bool   `json:"block_external_share"`
}

// apply vuelca los campos presentes sobre l y normaliza/valida el resultado.
func (req labelRequest) apply(l *domain.SensitivityLabel) error {
	if req.Name != nil {
		l.Name = *req.Name
	}
	if req.Rank != nil {
		l.Rank = *req.Rank
	}
	if req.Color != nil {
		l.Color = *req.Color
	}
	if req.Description != nil {
		l.Description = strings.TrimSpace(*req.Description)
	}
	if req.BlockExternalShare != nil {
		l.BlockExternalShare = *req.BlockExternalShare
	}
	if err := l.Normalize(); err != nil {
		// El motivo ya está en llano dentro del error de dominio; se le quita el
		// prefijo del centinela para no enseñarle «domain: …» al admin.
		return httpx.ErrBadRequest.WithMessage(
			strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": "),
		)
	}
	return nil
}

// handleListSensitivityLabels — GET /api/v1/sensitivity-labels — taxonomía de la org
// activa, de menos a más sensible. LECTURA para cualquier miembro autenticado: es la
// que consumen el selector de etiqueta del frontend y Drive al etiquetar un fichero.
func (s *Server) handleListSensitivityLabels(w http.ResponseWriter, r *http.Request) {
	s.writeLabelList(w, r)
}

// handleAdminListSensitivityLabels — GET /api/v1/admin/sensitivity-labels — la misma
// taxonomía desde la Console (mismo contenido; el bloque admin exige owner/admin).
func (s *Server) handleAdminListSensitivityLabels(w http.ResponseWriter, r *http.Request) {
	s.writeLabelList(w, r)
}

func (s *Server) writeLabelList(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	list, err := s.store.ListSensitivityLabels(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, l := range list {
		items = append(items, labelWire(l))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handleAdminCreateSensitivityLabel — POST /api/v1/admin/sensitivity-labels.
func (s *Server) handleAdminCreateSensitivityLabel(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	var req labelRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	l := domain.SensitivityLabel{OrgID: org.ID, CreatedBy: c.Subject}
	if err := req.apply(&l); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := s.store.CreateSensitivityLabel(r.Context(), nil, l)
	if err != nil {
		httpx.Fail(w, r, labelErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sensitivity_label.created", created.ID, r))
	httpx.JSON(w, r, http.StatusCreated, labelWire(created))
}

// handleAdminUpdateSensitivityLabel — PATCH /api/v1/admin/sensitivity-labels/{id}.
// Cambia la TAXONOMÍA; el contenido ya etiquetado conserva su snapshot.
func (s *Server) handleAdminUpdateSensitivityLabel(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	l, err := s.loadOrgLabel(r, org.ID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req labelRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := req.apply(&l); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := s.store.UpdateSensitivityLabel(r.Context(), nil, l)
	if err != nil {
		httpx.Fail(w, r, labelErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sensitivity_label.updated", updated.ID, r))
	httpx.JSON(w, r, http.StatusOK, labelWire(updated))
}

// handleAdminDeleteSensitivityLabel — DELETE /api/v1/admin/sensitivity-labels/{id}.
func (s *Server) handleAdminDeleteSensitivityLabel(w http.ResponseWriter, r *http.Request) {
	org, err := s.activeOrg(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, _ := claimsFrom(r.Context())
	id := chi.URLParam(r, "id")
	if _, perr := uuid.Parse(id); perr != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ok, err := s.store.DeleteSensitivityLabel(r.Context(), org.ID, id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "sensitivity_label.deleted", id, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// loadOrgLabel carga la etiqueta {id} acotada a la org (cross-org/inexistente ⇒ 404).
func (s *Server) loadOrgLabel(r *http.Request, orgID string) (domain.SensitivityLabel, error) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		return domain.SensitivityLabel{}, httpx.ErrNotFound
	}
	l, err := s.store.GetSensitivityLabel(r.Context(), orgID, id)
	if err != nil {
		return domain.SensitivityLabel{}, mapDomainErr(err)
	}
	return l, nil
}

// labelErr traduce el conflicto de nombre a un 409 con motivo legible.
func labelErr(err error) error {
	if errors.Is(err, domain.ErrConflict) {
		return httpx.ErrConflict.WithMessage("A label with that name already exists in the organisation.")
	}
	return mapDomainErr(err)
}
