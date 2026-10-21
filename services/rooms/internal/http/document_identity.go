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

type documentIdentityService interface {
	DocumentIdentityAccessChanged(context.Context, string, string, int64) error
	DocumentIdentityAccessVersion(context.Context, string, string) (int64, error)
}

func documentIdentityHandler(s documentIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Subject string `json:"subject"`
			Tenant  string `json:"tenant"`
			Version int64  `json:"version"`
		}
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if !validDocumentIdentityScope(req.Subject, req.Tenant) || req.Version < 1 {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		if err := s.DocumentIdentityAccessChanged(r.Context(), req.Subject, req.Tenant, req.Version); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func documentIdentityVersionHandler(s documentIdentityService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, tenant := chi.URLParam(r, "subject"), chi.URLParam(r, "tenant")
		if !validDocumentIdentityScope(subject, tenant) {
			httpx.Fail(w, r, httpx.ErrBadRequest)
			return
		}
		version, err := s.DocumentIdentityAccessVersion(r.Context(), subject, tenant)
		if err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.JSON(w, r, http.StatusOK, map[string]int64{"version": version})
	}
}

func validDocumentIdentityScope(subject, tenant string) bool {
	id, err := uuid.Parse(subject)
	return err == nil && id != uuid.Nil && id.String() == subject && tenancy.ValidSlug(tenant)
}
