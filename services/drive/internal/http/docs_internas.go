// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

// RutaFicheroDocs es la puerta por la que Sytue Docs pide tirar, restaurar o purgar
// el fichero de un documento cuando la persona lo hace desde Docs. La otra mitad
// —lo que pasa en Drive llega a Docs— es internal/docssync.
const RutaFicheroDocs = "/internal/docs/files/{id}"

type accionDocs interface {
	DocsAccion(ctx context.Context, id uuid.UUID, ownerSub, kind, accion string, borradoEn time.Time) (string, error)
}

// ficheroDocs — POST /internal/docs/files/{id} {tenant, owner_sub, kind, action}.
//
// `{id}` es el nil UUID para un documento SIN fichero: solo se pregunta, antes de
// purgarlo, si hay una retención legal sobre esa persona.
//
// Contesta 200 {"result": done|unchanged|not_applicable}; 409 name_conflict si al
// restaurar ya hay otro fichero con ese nombre en su carpeta, y 423 legal_hold si
// la purga la impide una retención legal. Docs necesita distinguir las dos para
// decírselo a la persona.
func ficheroDocs(svc accionDocs) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c struct {
			Tenant   string `json:"tenant"`
			OwnerSub string `json:"owner_sub"`
			Kind     string `json:"kind"`
			Action   string `json:"action"`
			// DeletedAt es cuándo se borró el documento (opcional): ver DocsAccion.
			DeletedAt time.Time `json:"deleted_at"`
		}
		if err := httpx.DecodeJSON(w, r, &c); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		c.Tenant, c.OwnerSub = strings.TrimSpace(c.Tenant), strings.TrimSpace(c.OwnerSub)
		if err != nil || !tenancy.ValidSlug(c.Tenant) || c.OwnerSub == "" || len(c.OwnerSub) > 255 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("tenant, owner_sub and a file id are required."))
			return
		}
		ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: c.Tenant})
		res, err := svc.DocsAccion(ctx, id, c.OwnerSub, c.Kind, c.Action, c.DeletedAt)
		switch {
		case err == nil:
			httpx.JSON(w, r, http.StatusOK, map[string]string{"result": res})
		case errors.Is(err, domain.ErrConflict):
			httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "name_conflict",
				"A file with that name already exists in its folder."))
		case errors.Is(err, service.ErrAccionDocs):
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("action must be trash, restore, purge or check."))
		case errors.Is(err, domain.ErrLegalHold):
			httpx.Fail(w, r, httpx.NewError(http.StatusLocked, "legal_hold",
				"This file is under legal hold and cannot be permanently deleted."))
		default:
			httpx.Fail(w, r, toAPIError(err))
		}
	}
}
