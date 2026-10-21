// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

type documentCallAccessService interface {
	DocumentCallAccessChanged(context.Context, uuid.UUID, string, int64) error
	DocumentCallAccessVersion(context.Context, uuid.UUID) (int64, error)
}

func documentCallAccessHandler(s documentCallAccessService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID      string `json:"document_id"`
			Tenant  string `json:"tenant"`
			Version int64  `json:"version"`
		}
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		id, err := uuid.Parse(req.ID)
		if err != nil || id == uuid.Nil || !tenancy.ValidSlug(req.Tenant) || req.Version < 1 {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		if err := s.DocumentCallAccessChanged(r.Context(), id, req.Tenant, req.Version); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func documentCallVersionHandler(s documentCallAccessService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "document"))
		if err != nil || id == uuid.Nil {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		version, err := s.DocumentCallAccessVersion(r.Context(), id)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, r, http.StatusOK, map[string]int64{"version": version})
	}
}
