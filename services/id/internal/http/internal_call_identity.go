// SPDX-License-Identifier: Apache-2.0
package http

import (
	"net/http"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

func (s *Server) handleInternalCallIdentity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
		Tenant  string `json:"tenant"`
	}
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	subject, err := uuid.Parse(req.Subject)
	if err != nil || subject == uuid.Nil || subject.String() != req.Subject || !tenancy.ValidSlug(req.Tenant) {
		httpx.Fail(w, r, httpx.ErrBadRequest)
		return
	}
	if !s.permiteDirectorio(w, r, s.organizacionesGlobal, "identidad-de-llamada:"+req.Subject+":"+req.Tenant) {
		return
	}
	proof, err := s.store.CallIdentity(r.Context(), req.Subject, req.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, proof)
}
