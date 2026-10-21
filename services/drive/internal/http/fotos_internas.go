// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drivehttp

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// RutaCopiaFotos es la puerta de servicio a servicio por la que Sytue Photos copia al
// Drive de la dueña de un álbum la foto que le añade quien lo edita
// (service.CopiaParaFotos). Solo desde dentro y con el secreto de la suite.
const RutaCopiaFotos = "/internal/photos/copies"

type copiador interface {
	CopiaParaFotos(ctx context.Context, src uuid.UUID, duenya, editora string) (domain.File, bool, error)
}

// copiaFotos — POST /internal/photos/copies {tenant, file_id, owner_sub, editor_sub}.
func copiaFotos(svc copiador) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var c struct {
			Tenant   string `json:"tenant"`
			FileID   string `json:"file_id"`
			OwnerSub string `json:"owner_sub"`
			// EditorSub es quien añade la foto: solo se copia un fichero suyo.
			EditorSub string `json:"editor_sub"`
		}
		if err := httpx.DecodeJSON(w, r, &c); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		id, err := uuid.Parse(c.FileID)
		c.Tenant, c.OwnerSub, c.EditorSub = strings.TrimSpace(c.Tenant), strings.TrimSpace(c.OwnerSub), strings.TrimSpace(c.EditorSub)
		if err != nil || c.Tenant == "" || c.OwnerSub == "" || c.EditorSub == "" || len(c.OwnerSub) > 255 || len(c.EditorSub) > 255 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("tenant, file_id, owner_sub and editor_sub are required."))
			return
		}
		ctx := tenancy.Into(r.Context(), tenancy.Tenant{Slug: c.Tenant})
		f, reusada, err := svc.CopiaParaFotos(ctx, id, c.OwnerSub, c.EditorSub)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusOK, map[string]any{"file": f, "reused": reusada})
	}
}
