// SPDX-License-Identifier: Apache-2.0
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

func (h *Handler) mountResourceSource(router chi.Router) {
	if h.resourceSourceSecret == "" || h.resourceSourceSessions == nil {
		return
	}
	source, _ := h.store.(httpx.ResourceBootstrapSourcePort)
	handler, err := httpx.NewResourceBootstrapSourceHandler(httpx.ResourceBootstrapSourceConfig{ServiceID: "docs", Secret: h.resourceSourceSecret, Port: source, Provider: h.resourceSourceSessions})
	if err != nil {
		router.Method(http.MethodPost, httpx.ResourceBootstrapSourcePath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { httpx.Fail(w, r, httpx.ErrUnavailable) }))
		return
	}
	router.Method(http.MethodPost, httpx.ResourceBootstrapSourcePath, handler)
}
