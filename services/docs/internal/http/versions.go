// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

func (h *Handler) listVersions(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, _, err := h.svc.Authorize(r.Context(), docID(r), subj, domain.RoleViewer); err != nil {
		fail(w, r, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	versions, err := h.store.ListVersions(r.Context(), docID(r), limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"versions": versions})
}

type createVersionReq struct {
	Label string `json:"label,omitempty"`
}

// createVersion consolida un snapshot del estado actual y registra una versión
// nombrada (manual). Requiere rol editor.
func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req createVersionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var label *string
	if req.Label != "" {
		label = &req.Label
	}
	version, err := h.svc.Snapshot(r.Context(), subj, docID(r), label, false,
		h.inline, h.hydrateFromS3, h.uploadSnapshot, snapshotKey)
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"version": version})
}

type restoreResp struct {
	Restored bool                 `json:"restored"`
	Changed  bool                 `json:"changed"`
	Seq      int64                `json:"seq"`
	Version  *domain.Version      `json:"version,omitempty"`
	Ops      []domain.SequencedOp `json:"ops,omitempty"`
}

// restoreVersion restaura el documento al contenido de una versión histórica. La
// restauración es NO DESTRUCTIVA y coherente con el modelo append-only del CRDT:
// el dominio calcula el diff (estado actual -> estado de la versión) y lo añade
// como operaciones nuevas (ver domain.Service.RestoreVersion). El historial se
// conserva íntegro y la propia restauración es reversible. Requiere rol editor.
// Las operaciones de restauración se difunden a los colaboradores conectados.
func (h *Handler) restoreVersion(w http.ResponseWriter, r *http.Request) {
	subj, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	// Cuerpo opcional: la contraseña de protección, si la versión toca algo protegido.
	var req struct {
		Proteccion *domain.Proteccion `json:"proteccion,omitempty"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		// Tolerante a propósito: sin cuerpo (o ilegible) simplemente no hay contraseña.
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req)
	}
	res, err := h.svc.RestoreVersion(domain.ConProteccion(r.Context(), req.Proteccion), subj, docID(r), chi.URLParam(r, "versionId"), h.hydrateFromS3)
	if err != nil {
		fail(w, r, err)
		return
	}
	if res.Changed {
		h.hub.PublishOps(r.Context(), docID(r), res.Ops, nil)
	}
	httpx.JSON(w, r, http.StatusOK, restoreResp{
		Restored: true, Changed: res.Changed, Seq: res.Seq, Version: res.Version, Ops: res.Ops,
	})
}
