// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Handlers HTTP de los ADJUNTOS DE CANAL de Sytue Rooms: la ÚNICA puerta a un
// fichero con ámbito de canal (el resto de la API de Drive no lo ve, la RLS de la
// mig 0014 lo excluye del ámbito personal). Cada ruta lleva el canal en la RUTA, y
// el servicio exige MEMBRESÍA de ese canal contra services/rooms en cada petición
// (service/channelfiles.go): no-miembro, fichero de otro canal o inexistente ⇒ 404,
// nunca un 403 que confirme que el fichero existe.
package drivehttp

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// UploadChannelFile sube un adjunto al canal (POST /channels/{channelID}/files).
// Mismo contrato de cuerpo/nombre que UploadFile; el destino NO lo elige el cliente
// (es la carpeta del canal, que el store crea a demanda).
func (h *Handlers) UploadChannelFile(w http.ResponseWriter, r *http.Request) {
	channelID, err := parseNamedID(r, "channelID")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := fileNameFromRequest(r)
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The file name is missing (X-File-Name header or ?name=)."))
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" || strings.HasPrefix(contentType, "multipart/") {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	body := http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes+1)
	defer body.Close()

	f, err := h.svc.ChannelUploadFile(r.Context(), channelID, subFromCtx(r), name, contentType, body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "The file is larger than the maximum allowed size."))
			return
		}
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}

// GetChannelFile devuelve los metadatos del adjunto (nombre/tipo/tamaño): es lo que
// Rooms denormaliza en el mensaje al publicarlo.
func (h *Handlers) GetChannelFile(w http.ResponseWriter, r *http.Request) {
	channelID, fileID, err := channelAndFile(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.ChannelFile(r.Context(), channelID, fileID, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, f)
}

// DownloadChannelFile transmite el contenido del adjunto. Disposición SEGURA
// (attachment + nosniff), como DownloadFile: la decisión de mostrar algo inline la
// toma el proxy BFF de Rooms, que añade su CSP sandbox.
func (h *Handlers) DownloadChannelFile(w http.ResponseWriter, r *http.Request) {
	channelID, fileID, err := channelAndFile(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, rc, err := h.svc.ChannelFileContent(r.Context(), channelID, fileID, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+escapeFilename(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Checksum != "" {
		w.Header().Set("ETag", `"`+f.Checksum+`"`)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// ThumbnailChannelFile sirve la miniatura del adjunto (imágenes ráster), con la MISMA
// caché/ETag que la de «Mi unidad» y la verja del canal. Es lo que pinta la burbuja
// del mensaje en vez de descargar el original completo.
func (h *Handlers) ThumbnailChannelFile(w http.ResponseWriter, r *http.Request) {
	channelID, fileID, err := channelAndFile(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rc, contentType, etag, err := h.svc.ChannelThumbnail(r.Context(), channelID, fileID, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	defer rc.Close()

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, max-age=300")
	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

// channelAndFile extrae los dos ids de la ruta ({channelID} y {fileID}).
func channelAndFile(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	channelID, err := parseNamedID(r, "channelID")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	fileID, err := parseNamedID(r, "fileID")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return channelID, fileID, nil
}

// CopyChannelFileToMyDrive guarda una copia del adjunto en el espacio del usuario
// («Guardar en mi Drive»). POST y no GET: crea un recurso. Devuelve 201 con el fichero
// nuevo, cuyo id es el que la interfaz usa para enlazar «ver en Drive».
func (h *Handlers) CopyChannelFileToMyDrive(w http.ResponseWriter, r *http.Request) {
	channelID, fileID, err := channelAndFile(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.svc.CopyChannelFileToMyDrive(r.Context(), channelID, fileID, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, f)
}
