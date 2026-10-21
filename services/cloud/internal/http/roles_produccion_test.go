// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cloudhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// CLOUD-04 y CLOUD-16 (auditoría 2026-10-06), con la decisión del dueño del
// 2026-10-17: solo admin y owner despliegan a producción y crean servicios con
// comando; un member despliega previews; billing no escribe.

func comoPersona(r *http.Request) *http.Request {
	return r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "user-1"}))
}

func comoToken(r *http.Request, scope domain.TokenScope) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), tokenCtxKey{}, &apiCaller{Scope: scope}))
}

func conRol(role domain.Role) *fakeSvc {
	return &fakeSvc{effRole: role, effFromMember: true}
}

func TestBillingNoEscribe(t *testing.T) {
	for _, c := range []struct {
		role   domain.Role
		method string
		want   int
	}{
		{domain.RoleBilling, http.MethodPost, http.StatusForbidden},
		{domain.RoleBilling, http.MethodPut, http.StatusForbidden},
		{domain.RoleBilling, http.MethodDelete, http.StatusForbidden},
		{domain.RoleBilling, http.MethodGet, http.StatusOK},
		{domain.RoleMember, http.MethodPost, http.StatusOK},
	} {
		next, _ := spyNext()
		rr := httptest.NewRecorder()
		newHandlers(conRol(c.role)).requireWriter()(next).ServeHTTP(rr, humanReq(c.method, "/api/v1/projects/x/services"))
		if rr.Code != c.want {
			t.Errorf("%s %s: code=%d, se esperaba %d", c.role, c.method, rr.Code, c.want)
		}
	}
	// Los tokens tienen su propio suelo por scope: requireWriter no los mira.
	next, called := spyNext()
	rr := httptest.NewRecorder()
	newHandlers(&fakeSvc{}).requireWriter()(next).ServeHTTP(rr, tokenReq(http.MethodPost, "/x", domain.ScopeWrite))
	if !*called {
		t.Fatalf("token write: code=%d", rr.Code)
	}
}

func TestRollbackYPromocionSoloAdmin(t *testing.T) {
	for _, c := range []struct {
		nombre string
		f      *fakeSvc
		r      *http.Request
		pasa   bool
	}{
		{"member", conRol(domain.RoleMember), humanReq(http.MethodPost, "/x"), false},
		{"admin", conRol(domain.RoleAdmin), humanReq(http.MethodPost, "/x"), true},
		{"owner", conRol(domain.RoleOwner), humanReq(http.MethodPost, "/x"), true},
		{"token write", &fakeSvc{}, tokenReq(http.MethodPost, "/x", domain.ScopeWrite), false},
		{"token deploy", &fakeSvc{}, tokenReq(http.MethodPost, "/x", domain.ScopeDeploy), true},
	} {
		next, called := spyNext()
		rr := httptest.NewRecorder()
		newHandlers(c.f).requireProductionDeployer()(next).ServeHTTP(rr, c.r)
		if *called != c.pasa {
			t.Errorf("%s: pasa=%v code=%d", c.nombre, *called, rr.Code)
		}
	}
}

func TestDesplegarProduccionSoloAdmin(t *testing.T) {
	for _, c := range []struct {
		role   domain.Role
		target string
		want   int
	}{
		{domain.RoleMember, "", http.StatusForbidden},
		{domain.RoleMember, "production", http.StatusForbidden},
		{domain.RoleMember, "preview", http.StatusAccepted},
		{domain.RoleAdmin, "production", http.StatusAccepted},
		{domain.RoleOwner, "", http.StatusAccepted},
	} {
		f := conRol(c.role)
		rr := httptest.NewRecorder()
		newHandlers(f).CreateDeployment(rr, req(http.MethodPost, "/x/deployments", validUUID, `{"git_ref":"main","target":"`+c.target+`"}`))
		if rr.Code != c.want {
			t.Errorf("%s → %q: code=%d, se esperaba %d (%s)", c.role, c.target, rr.Code, c.want, rr.Body.String())
		}
		if c.want == http.StatusForbidden && f.deployIn != nil {
			t.Errorf("%s → %q: llegó a desplegar", c.role, c.target)
		}
	}
}

func TestServicioConComandoSoloAdmin(t *testing.T) {
	for _, c := range []struct {
		role domain.Role
		body string
		want int
	}{
		{domain.RoleMember, `{"kind":"worker","name":"w","config":{"command":"sh"}}`, http.StatusForbidden},
		{domain.RoleMember, `{"kind":"job","name":"j"}`, http.StatusForbidden},
		{domain.RoleMember, `{"kind":"cron","name":"c"}`, http.StatusForbidden},
		{domain.RoleMember, `{"kind":"functions","name":"f"}`, http.StatusForbidden},
		{domain.RoleMember, `{"kind":"worker","name":"w","target":"preview"}`, http.StatusAccepted},
		{domain.RoleMember, `{"kind":"postgres","name":"db"}`, http.StatusAccepted},
		{domain.RoleAdmin, `{"kind":"worker","name":"w"}`, http.StatusAccepted},
	} {
		f := conRol(c.role)
		rr := httptest.NewRecorder()
		newHandlers(f).ProvisionService(rr, req(http.MethodPost, "/x/services", validUUID, c.body))
		if rr.Code != c.want {
			t.Errorf("%s %s: code=%d, se esperaba %d", c.role, c.body, rr.Code, c.want)
		}
	}
	// Un token write tampoco crea un worker en producción: es desplegar.
	rr := httptest.NewRecorder()
	r := comoToken(req(http.MethodPost, "/x/services", validUUID, `{"kind":"worker","name":"w"}`), domain.ScopeWrite)
	newHandlers(&fakeSvc{}).ProvisionService(rr, r)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("token write con worker: code=%d", rr.Code)
	}
}

func TestVariablesDeProduccionSoloAdmin(t *testing.T) {
	for _, c := range []struct {
		role  domain.Role
		scope string
		want  int
	}{
		{domain.RoleMember, "all", http.StatusForbidden},
		{domain.RoleMember, "production", http.StatusForbidden},
		{domain.RoleMember, "", http.StatusForbidden},
		{domain.RoleMember, "preview", http.StatusOK},
		{domain.RoleAdmin, "all", http.StatusOK},
	} {
		rr := httptest.NewRecorder()
		newHandlers(conRol(c.role)).SetEnv(rr, req(http.MethodPut, "/x/env", validUUID, `{"key":"A","value":"v","scope":"`+c.scope+`"}`))
		if rr.Code != c.want {
			t.Errorf("SetEnv %s %q: code=%d, se esperaba %d", c.role, c.scope, rr.Code, c.want)
		}
	}
	for _, c := range []struct {
		role  domain.Role
		scope domain.EnvScope
		want  int
	}{
		{domain.RoleMember, domain.ScopeAll, http.StatusForbidden},
		{domain.RoleMember, domain.ScopePreview, http.StatusNoContent},
		{domain.RoleAdmin, domain.ScopeProduction, http.StatusNoContent},
	} {
		f := conRol(c.role)
		f.envScope = c.scope
		rr := httptest.NewRecorder()
		newHandlers(f).DeleteEnv(rr, req(http.MethodDelete, "/env/x", validUUID, ""))
		if rr.Code != c.want {
			t.Errorf("DeleteEnv %s %q: code=%d, se esperaba %d", c.role, c.scope, rr.Code, c.want)
		}
	}
}

func memberReq(method, sub, body string) *http.Request {
	r := httptest.NewRequest(method, "/members", strings.NewReader(body))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("sub", sub)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	return comoPersona(r)
}

func TestSoloUnOwnerTocaAOtroOwner(t *testing.T) {
	roles := map[string]domain.Role{"user-1": domain.RoleAdmin, "jefa": domain.RoleOwner, "otro": domain.RoleMember}

	rr := httptest.NewRecorder()
	newHandlers(&fakeSvc{rolesBySub: roles}).AddMember(rr, memberReq(http.MethodPost, "", `{"sub":"otro","role":"owner"}`))
	if rr.Code != http.StatusForbidden {
		t.Errorf("admin nombra owner: code=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	newHandlers(&fakeSvc{rolesBySub: roles}).AddMember(rr, memberReq(http.MethodPost, "", `{"sub":"jefa","role":"member"}`))
	if rr.Code != http.StatusForbidden {
		t.Errorf("admin degrada al owner: code=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	newHandlers(&fakeSvc{rolesBySub: roles}).RemoveMember(rr, memberReq(http.MethodDelete, "jefa", ""))
	if rr.Code != http.StatusForbidden {
		t.Errorf("admin quita al owner: code=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	newHandlers(&fakeSvc{rolesBySub: roles}).AddMember(rr, memberReq(http.MethodPost, "", `{"sub":"otro","role":"admin"}`))
	if rr.Code != http.StatusCreated {
		t.Errorf("admin sube a un member a admin: code=%d", rr.Code)
	}

	owner := map[string]domain.Role{"user-1": domain.RoleOwner, "jefa": domain.RoleOwner}
	rr = httptest.NewRecorder()
	newHandlers(&fakeSvc{rolesBySub: owner}).RemoveMember(rr, memberReq(http.MethodDelete, "jefa", ""))
	if rr.Code != http.StatusNoContent {
		t.Errorf("owner quita a otro owner: code=%d", rr.Code)
	}
}
