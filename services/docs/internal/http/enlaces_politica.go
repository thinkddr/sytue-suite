// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"log/slog"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

// Techo de caducidad de los enlaces de la organización (docs/security.md#matriz-acl).

// politicaEnlacesStore es lo que esto necesita del store, fuera del contrato documental.
type politicaEnlacesStore interface {
	PoliticaEnlaces(ctx context.Context) (domain.PoliticaEnlaces, error)
	GuardarPoliticaEnlaces(ctx context.Context, maxDias int, sub string) (int64, error)
}

// auditar deja en el registro estructurado un hecho de seguridad de los enlaces: quién,
// en qué organización y sobre qué. Nunca lleva tokens ni contraseñas.
//
// ponytail: Docs no tiene tabla de auditoría (docs/security.md §1.7); esto es el log del
// proceso, no una traza consultable. Cuando Docs tenga `audit_log`, escribir ahí.
func auditar(r *http.Request, accion string, attrs ...any) {
	c, _ := authn.FromContext(r.Context())
	t, _ := tenancy.From(r.Context())
	campos := make([]any, 0, 6+len(attrs))
	campos = append(campos, "auditoria", accion, "actor", c.Subject, "tenant", t.Slug)
	slog.InfoContext(r.Context(), "docs: auditoría de enlaces", logsafe.Args(append(campos, attrs...))...)
}

// verPoliticaEnlaces — GET /api/v1/link-policy. La lee cualquier persona de la
// organización: el diálogo de compartir enseña el máximo antes de crear el enlace.
func (h *Handler) verPoliticaEnlaces(w http.ResponseWriter, r *http.Request) {
	if h.politicaEnlaces == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	p, err := h.politicaEnlaces.PoliticaEnlaces(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"max_days": p.MaxDias, "is_default": p.PorDefecto, "can_manage": esAdminOrg(r),
	})
}

// guardarPoliticaEnlaces — PUT /api/v1/link-policy {"max_days"} (owner o admin de la
// organización; el router ya lo exige y aquí se repite: es la puerta de «sin caducidad»).
// 0 = sin techo. Contesta con cuántos enlaces vivos pasaron a caducar antes.
func (h *Handler) guardarPoliticaEnlaces(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok || h.politicaEnlaces == nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !esAdminOrg(r) || esExterno(r.Context()) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	var in struct {
		MaxDias *int `json:"max_days"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.MaxDias == nil {
		fail(w, r, domain.ErrTechoInvalido)
		return
	}
	ajustados, err := h.politicaEnlaces.GuardarPoliticaEnlaces(r.Context(), *in.MaxDias, subj.Sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	auditar(r, "enlaces.politica.cambiada", "max_dias", *in.MaxDias, "enlaces_ajustados", ajustados)
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"max_days": *in.MaxDias, "is_default": false, "can_manage": true, "adjusted": ajustados,
	})
}
