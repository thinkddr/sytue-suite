// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Disparadores de despliegue por URL («deploy hooks»).
//
// Es lo que en Vercel se llama Deploy Hook y en Coolify «webhook de redespliegue»:
// una dirección secreta que, al recibir un POST, publica el proyecto. Sirve para
// lo que no puede mandar una cabecera de autorización — un CMS al pulsar
// «publicar», un cron de un tercero, el botón de una hoja de cálculo—, que hoy
// era imposible: la API exige `Authorization: Bearer`.
//
// NO trae modelo nuevo. Un disparador es EXACTAMENTE un token de API con scope
// deploy confinado a un proyecto (los dos ejes existen desde la Ola de tokens),
// presentado como URL en vez de como cabecera. Así se hereda gratis lo que ya
// estaba escrito y probado: emisión, caducidad, revocación, listado y el
// confinamiento que impide que ese token toque otro proyecto.
package cloudhttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/service"
)

// DeployHook dispara un despliegue de producción desde una URL secreta.
//
// Fuera de /api/v1 y sin sesión de Sytue ID, como el webhook de git: la
// credencial es el propio token de la ruta. Todo fallo responde 404 —no 401 ni
// 403— para no confirmar qué disparadores existen a quien prueba URLs: la
// diferencia entre «este token no vale» y «este token no es de despliegue» solo
// le sirve a quien está probando.
func (h *Handlers) DeployHook(w http.ResponseWriter, r *http.Request) {
	raw := chi.URLParam(r, "token")
	if raw == "" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	rt, err := h.svc.ResolveAPIToken(r.Context(), raw)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	// Dos condiciones, y las dos importan: sin scope deploy esto sería una vía
	// para desplegar con un token de solo lectura, y sin proyecto confinado una
	// sola URL publicaría CUALQUIER proyecto del tenant.
	//
	// Los disparadores nuevos llevan scope hook, que no vale en la API (CLOUD-17).
	// Los anteriores son tokens deploy y siguen funcionando hasta que se revoquen.
	if (rt.Scope != domain.ScopeHook && !rt.Scope.AtLeast(domain.ScopeDeploy)) || rt.ProjectID == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	t, err := tenancy.New(rt.Tenant)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ctx := authn.Into(r.Context(), authn.Claims{Subject: "deployhook:" + rt.ID.String(), Tenant: rt.Tenant})
	ctx = tenancy.Into(ctx, t)

	// Rama por defecto y sin commit: es un «publica lo que haya en producción»,
	// que es lo que espera quien pulsa «publicar» en su CMS. El servicio aplica
	// las mismas puertas que un despliegue normal —tope de gasto, requisito de
	// suscripción, cuota—, así que este camino no es una puerta trasera.
	dep, _, err := h.svc.Deploy(ctx, service.DeployInput{
		ProjectID: *rt.ProjectID,
		Target:    domain.TargetProduction,
		CreatedBy: "deploy-hook",
	})
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"deployment_id": dep.ID, "status": dep.Status})
}
