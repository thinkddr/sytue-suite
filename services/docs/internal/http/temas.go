// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/logoorg"
)

// marcaOrgStore es lo que la marca de la organización necesita del store.
type marcaOrgStore interface {
	GuardarMarcaOrg(ctx context.Context, m domain.MarcaOrg, sub string) (*domain.MarcaOrg, error)
	MarcaOrg(ctx context.Context, version int) (*domain.MarcaOrg, error)
	VersionesMarcaOrg(ctx context.Context, limite int) ([]domain.MarcaOrg, error)
}

// listarTemas — GET /themes: el catálogo común, la marca vigente de la organización (si
// tiene) y sus versiones, y si quien mira puede cambiar la marca.
func (h *Handler) listarTemas(w http.ResponseWriter, r *http.Request) {
	if _, ok := subjectFrom(r); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	resp := map[string]any{
		"themes":     domain.CatalogoTemas(),
		"brand":      nil,
		"versions":   []domain.MarcaOrg{},
		"can_manage": esAdminOrg(r),
	}
	if h.marcaOrg != nil {
		if m, err := h.marcaOrg.MarcaOrg(r.Context(), 0); err == nil {
			resp["brand"] = m.Tema()
		} else if !errors.Is(err, domain.ErrNotFound) {
			fail(w, r, err)
			return
		}
		if v, err := h.marcaOrg.VersionesMarcaOrg(r.Context(), 50); err == nil {
			resp["versions"] = v
		}
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

// guardarMarcaOrg — PUT /brand: una versión NUEVA de la marca de la organización. Solo
// quien la administra; las versiones anteriores se conservan.
func (h *Handler) guardarMarcaOrg(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if !esAdminOrg(r) {
		httpx.Fail(w, r, httpx.ErrForbidden)
		return
	}
	var m domain.MarcaOrg
	if err := httpx.DecodeJSON(w, r, &m); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.Normalizar(); err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid brand: name, #RRGGBB colors and font names are required."))
		return
	}
	guardada, err := h.marcaOrg.GuardarMarcaOrg(r.Context(), m, subj.Sub)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"brand": guardada.Tema(), "version": guardada.Version})
}

// resolverTema es el tema que se pide por id: del catálogo, o la marca (`version` 0 = la
// vigente). Un id que no existe es un 400 y una organización sin marca, un 404.
func (h *Handler) resolverTema(ctx context.Context, id string, version int) (domain.TemaComun, error) {
	if id == domain.IDTemaMarca {
		if h.marcaOrg == nil {
			return domain.TemaComun{}, domain.ErrNotFound
		}
		m, err := h.marcaOrg.MarcaOrg(ctx, version)
		if err != nil {
			return domain.TemaComun{}, err
		}
		return m.Tema().ConLogo(h.logoDeLaOrganizacion(ctx)), nil
	}
	t, ok := domain.TemaPorID(id, nil)
	if !ok {
		return domain.TemaComun{}, domain.ErrInvalidOp
	}
	return t, nil
}

type aplicarTemaReq struct {
	Theme string `json:"theme"`
	// BrandVersion elige una versión de la marca; 0 = la vigente.
	BrandVersion int `json:"brand_version,omitempty"`
	// Preview: solo dice qué cambiaría (`changes`), sin escribir ni dejar punto de restauración.
	Preview bool `json:"preview,omitempty"`
}

// aplicarTema — POST /documents/{id}/theme: aplica un tema a un documento que ya existe,
// SIN tocar su contenido (domain.AplicarTema). Antes guarda una versión del estado de
// partida —«Antes del tema …»—, así que restaurarla lo deshace. Requiere editor.
func (h *Handler) aplicarTema(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req aplicarTemaReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	tema, err := h.resolverTema(r.Context(), req.Theme, req.BrandVersion)
	if err != nil {
		fail(w, r, err)
		return
	}
	if req.Preview {
		cambios, err := h.svc.PrevisualizarTema(r.Context(), subj, docID(r), tema, h.hydrateFromS3)
		if err != nil {
			fail(w, r, err)
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"changed": len(cambios) > 0, "changes": cambios, "version": tema.Version})
		return
	}
	// «Antes de la marca v3» / «Antes del tema cielo»: el punto de restauración dice qué deshace.
	antesDe := "Antes del tema " + tema.ID
	if tema.Marca {
		antesDe = "Antes de la marca v" + strconv.Itoa(tema.Version)
	}
	res, err := h.svc.AplicarTema(r.Context(), subj, docID(r), tema, h.hydrateFromS3,
		func(ctx context.Context) error {
			etiqueta := antesDe
			_, err := h.svc.Snapshot(ctx, subj, docID(r), &etiqueta, false, h.inline, h.hydrateFromS3, h.uploadSnapshot, snapshotKey)
			return err
		})
	if err != nil {
		fail(w, r, err)
		return
	}
	if res.Changed {
		h.hub.PublishOps(r.Context(), docID(r), res.Ops, nil)
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"changed": res.Changed, "seq": res.Seq, "ops": res.Ops})
}

// logoDeLaOrganizacion es el logotipo de la organización DE LA SESIÓN (el slug sale de
// `tenancy`, nunca de la petición), o nil si no tiene o Sytue ID no lo da: la marca se
// aplica igualmente, sin logo. Ver internal/logoorg.
func (h *Handler) logoDeLaOrganizacion(ctx context.Context) *domain.LogoMarca {
	t, err := tenancy.From(ctx)
	if err != nil || h.logos == nil {
		return nil
	}
	l, err := h.logos.Logo(ctx, t.Slug)
	if err != nil {
		if !errors.Is(err, logoorg.ErrSinLogo) {
			slog.WarnContext(ctx, "docs: el logotipo de la organización no se pudo traer", "err", err)
		}
		return nil
	}
	return &domain.LogoMarca{PNG: l.PNG, Hash: l.Hash, Ancho: l.Ancho, Alto: l.Alto}
}
