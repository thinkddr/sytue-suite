// SPDX-License-Identifier: Apache-2.0
package meethttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/service"
)

func freshDocumentIdentity(s *service.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := uuid.Parse(chi.URLParam(r, "id"))
			if err != nil || id == uuid.Nil {
				httpx.Fail(w, r, httpx.ErrBadRequest)
				return
			}
			ctx, err := s.FreshDocumentCallContext(r.Context(), id)
			if err != nil {
				httpx.Fail(w, r, toAPIError(err))
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
