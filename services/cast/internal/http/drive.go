// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package casthttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// ficheroDeDriveDTO es lo que `GET /videos:from-drive` enseña antes de subir.
type ficheroDeDriveDTO struct {
	FileID      string `json:"file_id"`
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	// Title es el título que llevará el vídeo si no se cambia.
	Title string `json:"title"`
}

// PrevisualizarDeDrive — GET /api/v1/videos:from-drive?file_id=: comprueba, sin
// crear nada, que el fichero de Drive se puede subir a Cast y dice cuál es. Los
// mismos errores que el POST, para que la pantalla los enseñe antes del botón.
func (h *Handlers) PrevisualizarDeDrive(w http.ResponseWriter, r *http.Request) {
	f, err := h.svc.PrevisualizarDeDrive(r.Context(), quienDe(r), r.URL.Query().Get("file_id"))
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, ficheroDeDriveDTO{
		FileID: f.ID, Name: f.Nombre, SizeBytes: f.Bytes, ContentType: f.ContentType, Title: f.Titulo,
	})
}

type deDriveDTO struct {
	FileID      string `json:"file_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	Qualities   []int  `json:"qualities"`
}

// SubirDeDrive — POST /api/v1/videos:from-drive {file_id, title?, description?,
// visibility?} → 202 con el vídeo en `uploading`. La copia sigue en segundo plano:
// el vídeo pasa a `processing` al llegar entera, o a `failed` con el motivo.
func (h *Handlers) SubirDeDrive(w http.ResponseWriter, r *http.Request) {
	var in deDriveDTO
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v, err := h.svc.SubirDeDrive(r.Context(), quienDe(r), service.DeDrive{
		FileID: in.FileID, Title: in.Title, Description: in.Description, Visibility: domain.Visibility(in.Visibility),
		Qualities: in.Qualities,
	})
	if err != nil {
		httpx.Fail(w, r, traducir(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, h.aDTO(v, soloDueno()))
}
