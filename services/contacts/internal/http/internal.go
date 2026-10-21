// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactshttp

import (
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// ---------------------------------------------------------------------------
// Endpoint INTERNO service-to-service: POST /internal/avatar-sync.
//
// Sytue ID lo llama al guardar o quitar el avatar propio (services/id/internal
// /http/me_avatar.go), para reflejarlo en las fichas de contacto que coincidan
// por correo en TODA la organización — no solo las del propio usuario que lo
// cambió, también las de sus compañeros. Eso solo puede resolverlo Contacts:
// es el único servicio con permiso para escribir en agendas ajenas, y por eso
// no viaja un JWT de usuario (que RLS acotaría a SU propia agenda) sino el
// secreto compartido, comparado en tiempo CONSTANTE. Fuera de /api/v1: no
// hereda RequireAuth/RequireTenant, así que el tenant viaja en el CUERPO.
// ---------------------------------------------------------------------------

func internalAuth(secret string, next http.Handler) http.Handler {
	return httpx.SecretoInterno(secret)(next)
}

type avatarSyncRequest struct {
	Tenant string   `json:"tenant"`
	Emails []string `json:"emails"`
	// Photo vacío QUITA la foto de las fichas que coincidan.
	Photo string `json:"photo"`
}

// AvatarSync — POST /internal/avatar-sync.
func (h *Handlers) AvatarSync(w http.ResponseWriter, r *http.Request) {
	var req avatarSyncRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	t, err := tenancy.New(req.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid tenant."))
		return
	}
	if len(req.Emails) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("emails is required."))
		return
	}
	norm := ""
	if req.Photo != "" {
		norm, err = validation.RasterImageDataURI(req.Photo)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid image."))
			return
		}
	}
	ctx := tenancy.Into(r.Context(), t)
	n, err := h.store.SyncAvatarByEmail(ctx, req.Emails, norm)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"ok": true, "updated": n})
}
