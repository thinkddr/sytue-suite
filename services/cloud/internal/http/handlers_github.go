// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"net/http"
	"strconv"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
)

// GitHubAppCallback es el Setup/callback URL de la GitHub App. Al instalar/autorizar
// la App, GitHub redirige aquí con ?installation_id=X&code=Y&state=<emitido>. El
// endpoint liga la instalación al proyecto SOLO tras probar la propiedad (el service
// canjea el `code` por un user token y exige que la instalación pertenezca al usuario).
//
// Auth: admin/owner (montado bajo /api/v1). CSRF / confusión de proyecto: el `state`
// es opaco, de un solo uso y atado a quien lo pidió con StartGitHubInstall; además
// ConnectGitHubAppWithOwnership resuelve el proyecto BAJO RLS. El `code` es de un solo uso y no se
// registra ni se refleja en la respuesta.
func (h *Handlers) GitHubAppCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	installationID, err := strconv.ParseInt(q.Get("installation_id"), 10, 64)
	if err != nil || installationID <= 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid installation_id parameter."))
		return
	}
	code := q.Get("code")
	if code == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The GitHub authorisation code is missing."))
		return
	}

	// El `state` lo emitió este servidor para esta persona (CLOUD-18): un id de
	// proyecto puesto a mano en el enlace ya no liga nada.
	projectID, err := h.svc.ConsumeGitHubInstall(r.Context(), q.Get("state"), subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := h.svc.ConnectGitHubAppWithOwnership(r.Context(), projectID, installationID, code); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"connected": true, "installation_id": installationID})
}

// StartGitHubInstall emite el `state` con el que el panel manda a instalar la
// GitHub App en un proyecto. Se canjea una sola vez en GitHubAppCallback.
func (h *Handlers) StartGitHubInstall(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	state, err := h.svc.StartGitHubInstall(r.Context(), id, subFromCtx(r))
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"state": state})
}
