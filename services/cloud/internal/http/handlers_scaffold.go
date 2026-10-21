// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// scaffolder es la parte de la aplicación que crea un repositorio nuevo en la
// forja y da de alta el proyecto sobre él.
type scaffolder interface {
	StartScaffold(ctx context.Context, in service.ScaffoldIntent) (string, error)
	FinishScaffold(ctx context.Context, state, code, actorSub string) (service.TemplateApplied, string, error)
	ForgeEnabled() bool
}

var _ scaffolder = (*service.Service)(nil)

// startScaffoldRequest es lo que el panel pide crear. Ni framework ni comandos:
// los pone la plantilla, igual que en el alta desde plantilla.
type startScaffoldRequest struct {
	TemplateID string `json:"template_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	// RepoName es cómo se llamará el repositorio en la forja; vacío = el slug.
	RepoName string `json:"repo_name"`
}

// StartScaffold devuelve a dónde mandar al usuario a autorizar la creación de su
// repositorio en Sytue Git.
//
// Devuelve la URL en vez de redirigir (302) porque quien llama es una isla del
// panel por fetch, y un redirect ahí lo sigue el navegador POR DENTRO del fetch:
// la respuesta acabaría siendo el HTML de la pantalla de consentimiento en vez de
// una instrucción de navegar.
//
// requireScope no es adorno: al volver, esto encola un despliegue.
func (h *Handlers) StartScaffold(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(scaffolder)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	var req startScaffoldRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	url, err := svc.StartScaffold(r.Context(), service.ScaffoldIntent{
		ActorSub:   subFromCtx(r),
		TemplateID: strings.TrimSpace(req.TemplateID),
		Slug:       strings.TrimSpace(req.Slug),
		Name:       strings.TrimSpace(req.Name),
		RepoName:   strings.TrimSpace(req.RepoName),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"authorize_url": url})
}

// ForgeCallback recoge el consentimiento y deja el proyecto creado y desplegando.
//
// Aquí SÍ se redirige, porque a esta ruta llega el NAVEGADOR de vuelta de la
// forja: devolverle un JSON dejaría al usuario mirando un objeto en una pestaña
// en blanco después de haber pulsado «autorizar». Se vuelve al panel del proyecto
// —o a la lista con el motivo— para que el rodeo termine donde empezó.
//
// El `state` es de un solo uso y lo emitió el servidor (service.StartScaffold), y
// el sub de la sesión tiene que ser el mismo que lo pidió: sin eso, un `state`
// robado dejaría que otro completara el alta en el tenant de la víctima.
func (h *Handlers) ForgeCallback(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(scaffolder)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	q := r.URL.Query()
	panel := strings.TrimSuffix(h.svc.PublicURL(), "/")
	// El usuario puede denegar el permiso, y eso no es un error del sistema: se
	// vuelve al panel sin ruido.
	if q.Get("error") != "" || q.Get("code") == "" {
		http.Redirect(w, r, panel+"/?forge=denegado", http.StatusFound)
		return
	}
	out, _, err := svc.FinishScaffold(r.Context(), q.Get("state"), q.Get("code"), subFromCtx(r))
	if err != nil {
		// El motivo no viaja en la URL: puede llevar el nombre de otro tenant o el
		// detalle de por qué falló la forja, y eso acabaría en el historial del
		// navegador y en el Referer de la siguiente petición.
		http.Redirect(w, r, panel+"/?forge=error", http.StatusFound)
		return
	}
	http.Redirect(w, r, panel+"/projects/"+out.Project.Slug, http.StatusFound)
}
