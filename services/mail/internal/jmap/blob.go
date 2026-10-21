// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// uploadResponse es la respuesta de subida de blob (RFC 8620 §6.1).
type uploadResponse struct {
	AccountID string `json:"accountId"`
	BlobID    string `json:"blobId"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
}

// blobKey compone la clave S3 de un blob JMAP a partir del blobId opaco. El
// blobId ES la parte estable de la clave (blobs/<account>/<uuid>); nunca aceptamos
// una clave con rutas relativas para evitar path traversal fuera del prefijo.
func blobKey(blobID string) (string, bool) {
	clean := path.Clean("/" + blobID)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || strings.Contains(clean, "..") || !strings.HasPrefix(clean, "blobs/") {
		return "", false
	}
	return clean, true
}

// handleUpload recibe un blob (adjunto/cuerpo), lo guarda en S3 bajo el prefijo
// del tenant y devuelve un blobId opaco que el cliente referencia en Email/set.
// El límite de tamaño es MaxMessageBytes (anti-abuso).
func (a *API) handleUpload(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if a.blobs == nil {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("almacenamiento no disponible."))
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	// Limitar el tamaño de subida (anti-abuso). +1 detecta el exceso.
	limited := io.LimitReader(r.Body, a.cfg.MaxMessageBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The body could not be read.").Wrap(err))
		return
	}
	if int64(len(data)) > a.cfg.MaxMessageBytes {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("blob demasiado grande."))
		return
	}

	key := fmt.Sprintf("blobs/%s/%s", acc.ID, uuid.NewString())
	if err := a.blobs.Put(r.Context(), t, key, bytes.NewReader(data), int64(len(data)),
		objectstore.PutOptions{ContentType: contentType}); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("The blob could not be saved.").Wrap(err))
		return
	}

	httpx.JSON(w, r, http.StatusCreated, uploadResponse{
		AccountID: acc.ID.String(),
		BlobID:    key,
		Type:      contentType,
		Size:      int64(len(data)),
	})
}

// handleDownload streamea un blob por id desde S3. El blobId lleva el accountId
// embebido en su prefijo; verificamos además que el account de la URL coincide
// con el del token (RLS de S3 por prefijo de tenant lo refuerza).
func (a *API) handleDownload(w http.ResponseWriter, r *http.Request) {
	acc, err := a.account(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, ok := authn.FromContext(r.Context()); !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if a.blobs == nil {
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("almacenamiento no disponible."))
		return
	}

	key, ok := blobKey(chi.URLParam(r, "blobId"))
	if !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid blobId."))
		return
	}
	// El blob debe pertenecer a la cuenta del token (prefijo blobs/<account>/).
	if !strings.HasPrefix(key, "blobs/"+acc.ID.String()+"/") &&
		!strings.HasPrefix(key, "messages/"+acc.ID.String()+"/") {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}

	rc, err := a.blobs.Get(r.Context(), t, key)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	defer rc.Close()

	name := chi.URLParam(r, "name")
	if name == "" {
		name = "blob"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, rc)
}
