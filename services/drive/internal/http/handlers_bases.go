// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// basesExporter es lo que estos manejadores piden al servicio. Aparte de `driveService`
// para no ensanchar esa interfaz ni sus dobles de prueba.
type basesExporter interface {
	ExportarVistaDeBases(ctx context.Context, folderID uuid.UUID, token, ownerSub string) (domain.File, store.Procedencia, error)
	ProcedenciaDeFichero(ctx context.Context, id uuid.UUID) ([]store.Procedencia, error)
}

var errConexionBases = httpx.NewError(http.StatusUnprocessableEntity, "bases_connection_invalid",
	"That Sytue Bases connection is not valid for Sytue Drive: it may be revoked, expired, granted to another product or to another organisation.")

// ExportarVistaDeBases — POST /api/v1/folders/{id}/imports/bases {token}: deja en la carpeta
// un CSV con la vista que abarca el token de conexión, y devuelve el fichero y su procedencia.
// El token viaja en el cuerpo (nunca en la URL) y no se guarda.
func (h *Handlers) ExportarVistaDeBases(w http.ResponseWriter, r *http.Request) {
	if h.bases == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	folderID, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	f, p, err := h.bases.ExportarVistaDeBases(r.Context(), folderID, in.Token, subFromCtx(r))
	if errors.Is(err, service.ErrConexionBases) {
		httpx.Fail(w, r, errConexionBases)
		return
	}
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"file": f, "provenance": p})
}

// ProcedenciaDeFichero — GET /api/v1/files/{id}/provenance: de dónde salió cada versión
// exportada del fichero (lista vacía si lo subió una persona).
func (h *Handlers) ProcedenciaDeFichero(w http.ResponseWriter, r *http.Request) {
	if h.bases == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ps, err := h.bases.ProcedenciaDeFichero(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"provenance": ps})
}
