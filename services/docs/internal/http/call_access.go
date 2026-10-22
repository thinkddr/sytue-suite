// SPDX-License-Identifier: Apache-2.0
package http

import (
	"context"
	"errors"
	"net/http"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
)

type documentCallAccessStore interface {
	DocumentCallAccess(context.Context, string, string) (domain.DocumentCallAccess, error)
}

func (h *Handler) documentCallAccess(w http.ResponseWriter, r *http.Request) {
	subject, ok := subjectFrom(r)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	st, ok := h.store.(documentCallAccessStore)
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	result, err := st.DocumentCallAccess(r.Context(), docID(r), subject.Sub)
	if errors.Is(err, domain.ErrCallAccessUnavailable) {
		httpx.Fail(w, r, httpx.NewError(http.StatusServiceUnavailable, "document_calls_unavailable", "Encrypted document calling is unavailable."))
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, result)
}
