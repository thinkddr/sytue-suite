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

// templateApplier es la parte de la capa de aplicación que da de alta un proyecto
// entero desde una plantilla. Se toma por aserción sobre h.svc en vez de ampliar
// cloudService porque handlers.go está tomado por otra sesión; el día que se
// añada allí la firma, esta interfaz y su aserción sobran.
type templateApplier interface {
	ApplyTemplate(ctx context.Context, in service.ApplyTemplateInput) (service.TemplateApplied, string, error)
}

// La aserción de h.svc es en caliente, así que esta comprobación en compilación
// es la que impide que una deriva de firma la convierta en un 500 silencioso.
var _ templateApplier = (*service.Service)(nil)

// createFromTemplateRequest es el cuerpo del alta desde plantilla. Nada de
// framework, comandos de build, puerto o sonda: eso lo dice el manifiesto, y
// dejar que el cliente los pisara convertiría la plantilla en un formulario más.
type createFromTemplateRequest struct {
	TemplateID string `json:"template_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	RepoURL    string `json:"repo_url"`
	// Answers responde SOLO a las variables con origin=ask de la plantilla. Una
	// clave que la plantilla no declara es un 400: si no, sería un canal para
	// escribir variables de entorno arbitrarias en el pod de la app.
	Answers map[string]string `json:"answers"`
	// InstanceSize pisa el tamaño sugerido por la plantilla ("" = el suyo).
	InstanceSize string `json:"instance_size"`
}

// templateAppliedResponse es el alta ya hecha más el secreto de webhook, que se
// revela UNA vez (y solo a un humano).
type templateAppliedResponse struct {
	service.TemplateApplied
	WebhookSecret string `json:"webhook_secret,omitempty"`
	WebhookURL    string `json:"webhook_url,omitempty"`
}

// CreateProjectFromTemplate da de alta un proyecto desde una plantilla del
// catálogo en UNA sola petición y de forma ATÓMICA: proyecto, servicios
// gestionados, variables de entorno y el primer despliegue encolado. Si algo
// falla no queda nada a medias (una transacción, ver store.InTemplateTx).
//
// RUTA — la línea de registro a insertar en server.go, JUSTO DEBAJO de
// `api.Post("/projects", h.CreateProject)`:
//
//	api.With(h.requireScope()).Post("/projects:from-template", h.CreateProjectFromTemplate)
//
// requireScope no es adorno: el alta encola un despliegue, así que un token de
// scope write entraría aquí a desplegar (escalada write→deploy) si la ruta se
// registrara desnuda. Los humanos pasan igual (autorizan por rol), y un token
// confinado a un proyecto ya lo rechaza confineProjectToken: la ruta no cuelga
// de /api/v1/projects/{P}/.
func (h *Handlers) CreateProjectFromTemplate(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.svc.(templateApplier)
	if !ok {
		httpx.Fail(w, r, httpx.ErrInternal)
		return
	}
	var req createFromTemplateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, webhookSecret, err := svc.ApplyTemplate(r.Context(), service.ApplyTemplateInput{
		TemplateID:   strings.TrimSpace(req.TemplateID),
		Slug:         strings.TrimSpace(req.Slug),
		Name:         strings.TrimSpace(req.Name),
		RepoURL:      strings.TrimSpace(req.RepoURL),
		Answers:      req.Answers,
		InstanceSize: strings.TrimSpace(req.InstanceSize),
		ActorSub:     subFromCtx(r),
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	// Misma regla que CreateProject: el secreto del webhook se le enseña al HUMANO
	// que crea el proyecto, nunca a un token de API (que con scope write lo leería
	// aquí y firmaría pushes contra /webhooks/git para disparar despliegues).
	resp := templateAppliedResponse{TemplateApplied: out, WebhookURL: h.svc.WebhookURL()}
	if tokenFromCtx(r.Context()) == nil {
		resp.WebhookSecret = webhookSecret
	}
	httpx.JSON(w, r, http.StatusCreated, resp)
}
