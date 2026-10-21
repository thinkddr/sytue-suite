// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"errors"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/contacts/internal/domain"
)

// ---------------------------------------------------------------------------
// Acciones EN LOTE (multiseleccion): papelera / favorito / grupo.
//
// Mutaciones por POST con cuerpo JSON (CSRF-safe: el navegador no puede enviarlas
// cross-origin sin preflight; el BFF Astro ademas rechaza DELETE/PUT). El store
// resuelve cada accion en UNA consulta `uid = ANY` y SOLO sobre contactos del
// propio dueno (subject). Se responde con el recuento afectado.
// ---------------------------------------------------------------------------

// maxBatchUIDs acota cuantos contactos admite una accion en lote (defensa).
const maxBatchUIDs = 1000

type batchRequest struct {
	UIDs []string `json:"uids"`
}

func (b batchRequest) valid() bool { return len(b.UIDs) > 0 && len(b.UIDs) <= maxBatchUIDs }

// BatchTrash envia a la papelera los contactos seleccionados (por UID).
func (h *Handlers) BatchTrash(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !req.valid() {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Select between 1 and 1000 contacts."))
		return
	}
	n, err := h.store.TrashContacts(r.Context(), subFromCtx(r), req.UIDs)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"trashed": n})
}

type batchFavoriteRequest struct {
	UIDs     []string `json:"uids"`
	Favorite bool     `json:"favorite"`
}

// BatchFavorite destaca o quita el destacado de los contactos seleccionados.
func (h *Handlers) BatchFavorite(w http.ResponseWriter, r *http.Request) {
	var req batchFavoriteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.UIDs) == 0 || len(req.UIDs) > maxBatchUIDs {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Select between 1 and 1000 contacts."))
		return
	}
	n, err := h.store.SetFavorites(r.Context(), subFromCtx(r), req.UIDs, req.Favorite)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"updated": n, "favorite": req.Favorite})
}

type groupMembersRequest struct {
	UIDs []string `json:"uids"`
	Add  bool     `json:"add"`
}

// BatchGroupMembers anade (add=true) o quita (add=false) EN LOTE los contactos
// seleccionados a un grupo del dueno (mantiene la vCard MEMBER sincronizada).
func (h *Handlers) BatchGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req groupMembersRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.UIDs) == 0 || len(req.UIDs) > maxBatchUIDs {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Select between 1 and 1000 contacts."))
		return
	}
	g, err := h.store.ModifyGroupMembers(r.Context(), subFromCtx(r), id, req.UIDs, req.Add)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	setETag(w, g.ETag)
	httpx.JSON(w, r, http.StatusOK, g)
}

// ---------------------------------------------------------------------------
// Foto de contacto (avatar): PHOTO embebida en la vCard como data URI.
//
// La imagen se valida en el servidor (tipo raster + tamano) como frontera de
// confianza, ademas del redimensionado del cliente. Direccionada por ?uid= (owner-
// scoped). El listado NO devuelve la foto; el avatar de la ficha/lista la carga
// aqui, cacheable por etag.
// ---------------------------------------------------------------------------

// photoCacheControl: privada (por-usuario) y revalidable. La URL lleva ?v=<etag>,
// asi que un cambio de la tarjeta invalida la cache del navegador.
const photoCacheControl = "private, max-age=3600, must-revalidate"

func uidFromQuery(r *http.Request) (string, error) {
	uid := strings.TrimSpace(r.URL.Query().Get("uid"))
	if uid == "" {
		return "", httpx.ErrBadRequest.WithMessage("The contact UID is required.")
	}
	return uid, nil
}

type setPhotoRequest struct {
	UID  string `json:"uid"`
	Data string `json:"data"` // data:image/...;base64,...
}

// SetPhoto define/reemplaza la foto de un contacto (PHOTO embebida). Valida el
// data URI (solo raster JPEG/PNG/WebP/GIF, <= 256 KiB) antes de incrustarlo.
func (h *Handlers) SetPhoto(w http.ResponseWriter, r *http.Request) {
	var req setPhotoRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.UID) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The contact UID is required."))
		return
	}
	dataURI, err := domain.ValidatePhotoDataURI(req.Data)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(photoErrMessage(err)))
		return
	}
	if err := h.store.SetContactPhoto(r.Context(), subFromCtx(r), req.UID, dataURI); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"uid": domain.CleanUID(req.UID), "photo": true})
}

// RemovePhoto quita la foto de un contacto (elimina la PHOTO de su vCard).
func (h *Handlers) RemovePhoto(w http.ResponseWriter, r *http.Request) {
	uid, err := uidFromQuery(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.RemoveContactPhoto(r.Context(), subFromCtx(r), uid); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// GetPhoto sirve los bytes de la foto de un contacto (para el <img> del avatar),
// decodificados de la PHOTO embebida, con etag y cache condicional. 404 si el
// contacto no tiene foto (el frontend cae a las iniciales).
func (h *Handlers) GetPhoto(w http.ResponseWriter, r *http.Request) {
	uid, err := uidFromQuery(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	dataURI, etag, err := h.store.GetContactPhoto(r.Context(), subFromCtx(r), uid)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	raw, mime, err := domain.PhotoBytes(dataURI)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	tag := `"` + etag.String() + `"`
	// Revalidacion condicional barata: si el navegador ya tiene esta version, 304.
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag.String()) {
		w.Header().Set("ETag", tag)
		w.Header().Set("Cache-Control", photoCacheControl)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", photoCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

// photoErrMessage traduce un ErrInvalidPhoto a un mensaje de usuario.
func photoErrMessage(err error) string {
	if errors.Is(err, domain.ErrInvalidPhoto) {
		return "Invalid image: use a JPEG, PNG, WebP or GIF of up to 256 KB."
	}
	return "Invalid image."
}
