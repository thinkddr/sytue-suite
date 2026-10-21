// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// hookReq arma la petición con el token en la ruta, como la sirve chi. Sin
// sesión ni cabeceras: la credencial es la propia URL, que es el sentido de esto.
func hookReq(token string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/hooks/deploy/"+token, http.NoBody)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("token", token)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
}

// El disparador acepta EXACTAMENTE una cosa: un token vivo con scope deploy y
// confinado a un proyecto. Las otras tres combinaciones son la puerta trasera que
// este endpoint no puede ser —desplegar con un token de lectura, o publicar
// cualquier proyecto del tenant con una sola URL— y todas responden 404, sin
// distinguir el motivo a quien está probando direcciones.
func TestDeployHookSoloTokenDeployConProyecto(t *testing.T) {
	pid := uuid.New()
	casos := []struct {
		nombre string
		tok    store.ResolvedToken
		err    error
		quiero int
	}{
		{"válido", store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeDeploy, ProjectID: &pid}, nil, http.StatusAccepted},
		{"hook", store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeHook, ProjectID: &pid}, nil, http.StatusAccepted},
		{"hook sin proyecto", store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeHook}, nil, http.StatusNotFound},
		{"scope write", store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeWrite, ProjectID: &pid}, nil, http.StatusNotFound},
		{"sin proyecto", store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeDeploy}, nil, http.StatusNotFound},
		{"token inválido", store.ResolvedToken{}, domain.ErrNotFound, http.StatusNotFound},
		{"tenant vacío", store.ResolvedToken{Scope: domain.ScopeDeploy, ProjectID: &pid}, nil, http.StatusNotFound},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			f := &fakeSvc{
				resolvedTok:   c.tok,
				resolveTokErr: c.err,
				dep:           domain.Deployment{ID: uuid.New(), Status: domain.DeployQueued},
			}
			rr := httptest.NewRecorder()
			newHandlers(f).DeployHook(rr, hookReq("scl_loquesea"))
			if rr.Code != c.quiero {
				t.Fatalf("status = %d, quiero %d", rr.Code, c.quiero)
			}
			if c.quiero == http.StatusAccepted {
				// Y despliega EL proyecto del token, no uno que venga de fuera: la
				// petición no lleva ningún identificador de proyecto.
				if f.deployIn == nil || f.deployIn.ProjectID != pid {
					t.Fatalf("desplegó %v, quiero el proyecto del token %s", f.deployIn, pid)
				}
				if f.deployIn.Target != domain.TargetProduction {
					t.Errorf("target = %q, quiero production", f.deployIn.Target)
				}
			} else if f.deployIn != nil {
				t.Error("se disparó un despliegue con un token que no debía valer")
			}
		})
	}
}

// Un token hook solo vale en /hooks/deploy/{token}: como Bearer no entra en la
// API, ni a leer (CLOUD-17).
func TestTokenHookNoEntraEnLaAPI(t *testing.T) {
	pid := uuid.New()
	h := newHandlers(&fakeSvc{resolvedTok: store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeHook, ProjectID: &pid, ID: uuid.New()}})
	for _, m := range []string{http.MethodGet, http.MethodPatch} {
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.authenticateToken(rr, httptest.NewRequest(m, "/api/v1/projects/"+pid.String(), http.NoBody), next, "scl_x")
		if rr.Code != http.StatusUnauthorized || *called {
			t.Fatalf("%s con token hook: code=%d called=%v", m, rr.Code, *called)
		}
	}
}
