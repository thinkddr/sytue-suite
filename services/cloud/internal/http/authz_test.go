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

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
	"git.sytue.eu/sytue/one/services/cloud/internal/store"
)

// --- helpers ---

// tokenReq construye una petición autenticada como token de API con el scope dado.
func tokenReq(method, target string, scope domain.TokenScope) *http.Request {
	r := httptest.NewRequest(method, target, http.NoBody)
	return r.WithContext(context.WithValue(r.Context(), tokenCtxKey{}, &apiCaller{Scope: scope}))
}

// humanReq construye una petición autenticada como humano con los roles de claim.
func humanReq(method, target string, roles ...string) *http.Request {
	r := httptest.NewRequest(method, target, http.NoBody)
	return r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "user-1", Roles: roles}))
}

// cloudHumanReq construye una sesión humana con organización y correo
// verificados, tal y como llega después del middleware OIDC.
func cloudHumanReq(method, target string, roles ...string) *http.Request {
	r := httptest.NewRequest(method, target, http.NoBody)
	claims := authn.Claims{
		Subject: "user-1",
		Email:   "user@example.eu",
		Tenant:  "acme",
		Roles:   roles,
	}
	return r.WithContext(authn.Into(r.Context(), claims))
}

// subReq añade el parámetro de ruta chi {sub}.
func subReq(method, target, sub string) *http.Request {
	r := httptest.NewRequest(method, target, http.NoBody)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("sub", sub)
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rc))
	return r.WithContext(authn.Into(r.Context(), authn.Claims{Subject: "user-1"}))
}

// projTokenReq construye una petición autenticada como token de API confinado al
// proyecto pid, con la ruta completa target (como la ve chi en un middleware).
func projTokenReq(target string, pid *uuid.UUID) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	return r.WithContext(context.WithValue(r.Context(), tokenCtxKey{}, &apiCaller{Scope: domain.ScopeDeploy, ProjectID: pid}))
}

// spyNext es un handler que registra si se le llamó (para probar middlewares).
func spyNext() (http.Handler, *bool) {
	called := false
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}), &called
}

// --- requireScope (escalera de scope de tokens; no-op para humanos) ---

func TestRequireScope(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	mw := h.requireScope()

	// Token read en ruta de deploy => 403.
	next, called := spyNext()
	rr := httptest.NewRecorder()
	mw(next).ServeHTTP(rr, tokenReq(http.MethodPost, "/deploy", domain.ScopeRead))
	if rr.Code != http.StatusForbidden || *called {
		t.Fatalf("token read en deploy: code=%d called=%v", rr.Code, *called)
	}

	// Token deploy => pasa.
	next, called = spyNext()
	rr = httptest.NewRecorder()
	mw(next).ServeHTTP(rr, tokenReq(http.MethodPost, "/deploy", domain.ScopeDeploy))
	if rr.Code != http.StatusOK || !*called {
		t.Fatalf("token deploy: code=%d called=%v", rr.Code, *called)
	}

	// Humano (sin token en contexto) => no-op, pasa (autoriza por rol en otra capa).
	next, called = spyNext()
	rr = httptest.NewRecorder()
	mw(next).ServeHTTP(rr, humanReq(http.MethodPost, "/deploy"))
	if rr.Code != http.StatusOK || !*called {
		t.Fatalf("humano en requireScope: code=%d called=%v", rr.Code, *called)
	}
}

// --- requireCloudMembership (límite de acceso y materialización de puestos) ---

func TestRequireCloudMembership(t *testing.T) {
	t.Run("api token bypasses human membership", func(t *testing.T) {
		h := newHandlers(&fakeSvc{})
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.requireCloudMembership()(next).ServeHTTP(rr, tokenReq(http.MethodGet, "/api/v1/projects", domain.ScopeRead))
		if rr.Code != http.StatusOK || !*called {
			t.Fatalf("token API: code=%d called=%v", rr.Code, *called)
		}
	})

	t.Run("explicit member passes", func(t *testing.T) {
		h := newHandlers(&fakeSvc{effRole: domain.RoleMember, effFromMember: true})
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.requireCloudMembership()(next).ServeHTTP(rr, cloudHumanReq(http.MethodGet, "/api/v1/projects", "member"))
		if rr.Code != http.StatusOK || !*called {
			t.Fatalf("miembro explícito: code=%d called=%v", rr.Code, *called)
		}
	})

	t.Run("accepted invitation materializes access", func(t *testing.T) {
		f := &fakeSvc{effFromMember: true, acceptPending: true}
		h := newHandlers(f)
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.requireCloudMembership()(next).ServeHTTP(rr, cloudHumanReq(http.MethodGet, "/api/v1/projects", "member"))
		if rr.Code != http.StatusOK || !*called {
			t.Fatalf("invitación aceptada: code=%d called=%v", rr.Code, *called)
		}
		if f.acceptSub != "user-1" || f.acceptEmail != "user@example.eu" {
			t.Fatalf("identidad materializada incorrecta: sub=%q email=%q", f.acceptSub, f.acceptEmail)
		}
	})

	t.Run("missing membership is denied", func(t *testing.T) {
		h := newHandlers(&fakeSvc{effFromMember: true})
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.requireCloudMembership()(next).ServeHTTP(rr, cloudHumanReq(http.MethodGet, "/api/v1/projects", "member"))
		if rr.Code != http.StatusForbidden || *called {
			t.Fatalf("sin membresía: code=%d called=%v", rr.Code, *called)
		}
	})

	t.Run("owner bootstraps an empty tenant", func(t *testing.T) {
		f := &fakeSvc{effFromMember: false}
		h := newHandlers(f)
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.requireCloudMembership()(next).ServeHTTP(rr, cloudHumanReq(http.MethodGet, "/api/v1/projects", "owner"))
		if rr.Code != http.StatusOK || !*called {
			t.Fatalf("bootstrap owner: code=%d called=%v", rr.Code, *called)
		}
		if f.addMemberSub != "user-1" || f.addMemberRole != domain.RoleOwner {
			t.Fatalf("bootstrap incorrecto: sub=%q role=%q", f.addMemberSub, f.addMemberRole)
		}
	})

	t.Run("non owner cannot bootstrap an empty tenant", func(t *testing.T) {
		h := newHandlers(&fakeSvc{effFromMember: false})
		next, called := spyNext()
		rr := httptest.NewRecorder()
		h.requireCloudMembership()(next).ServeHTTP(rr, cloudHumanReq(http.MethodGet, "/api/v1/projects", "admin"))
		if rr.Code != http.StatusForbidden || *called {
			t.Fatalf("bootstrap no-owner: code=%d called=%v", rr.Code, *called)
		}
	})
}

// --- confineProjectToken (token de API acotado a un proyecto) ---

func TestConfineProjectToken(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	mw := h.confineProjectToken()
	p := uuid.New()
	other := uuid.New()

	// serve corre el middleware con la petición dada y devuelve (código, llamado).
	serve := func(r *http.Request) (int, bool) {
		next, called := spyNext()
		rr := httptest.NewRecorder()
		mw(next).ServeHTTP(rr, r)
		return rr.Code, *called
	}

	// CONFINAMIENTO: token acotado a P.
	allowed := []string{
		"/api/v1/projects/" + p.String(),                  // el propio proyecto
		"/api/v1/projects/" + p.String() + "/deployments", // sus deployments
		"/api/v1/projects/" + p.String() + "/env",         // su env
		"/api/v1/projects/" + p.String() + "/services",    // sus servicios
	}
	for _, path := range allowed {
		if code, called := serve(projTokenReq(path, &p)); code != http.StatusOK || !called {
			t.Fatalf("confinado debe PASAR en %s: code=%d called=%v", path, code, called)
		}
	}
	// Nota: los bypass por normalización de path (`..`, `//`, `%2f`) NO se defienden
	// aquí (pathUnderProject ve el r.URL.Path decodificado y el primer segmento sigue
	// siendo P) sino AGUAS ABAJO en chi, que enruta sin limpiar `..` (→ 404) y sobre
	// RawPath para `%2f` (→ GetProject(id-inválido) → 400). Este test fija el LÍMITE de
	// pathUnderProject (prefijo exacto + primer segmento == UUID de P, deny-by-default).
	denied := []string{
		"/api/v1/projects/" + other.String() + "/deployments",   // otro proyecto
		"/api/v1/projects/" + other.String() + "/" + p.String(), // P como 2º segmento (no cuela)
		"/api/v1/projects",                                     // lista de proyectos (tenant-wide)
		"/api/v1/deployments/" + p.String(),                    // recurso desnudo
		"/api/v1/builds/" + p.String(),                         // recurso desnudo
		"/api/v1/services/" + p.String() + "/restore",          // recurso desnudo (restore)
		"/api/v1/org",                                          // tenant-wide
		"/api/v1/members",                                      // tenant-wide
		"/api/v1/billing/account",                              // tenant-wide
		"/api/v1/tokens",                                       // tenant-wide
		"/api/v1/usage",                                        // tenant-wide
		"/api/v1/github/callback",                              // tenant-wide
		"/api/v1/projects/not-a-uuid/deployments",              // segmento no-UUID
		"/api/v1/projects/" + p.String() + "extra/deployments", // UUID concatenado (no parsea)
		"/api/v1/projectsX/" + p.String() + "/deployments",     // prefijo lookalike (sin barra)
		"/API/V1/PROJECTS/" + p.String() + "/deployments",      // prefijo en mayúsculas (case-sensitive)
	}
	for _, path := range denied {
		if code, called := serve(projTokenReq(path, &p)); code != http.StatusForbidden || called {
			t.Fatalf("confinado debe DENEGAR (403) %s: code=%d called=%v", path, code, called)
		}
	}

	// NO-REGRESIÓN: token tenant-wide (ProjectID nil) pasa en todo, como hoy.
	for _, path := range []string{"/api/v1/org", "/api/v1/projects/" + other.String() + "/deployments", "/api/v1/tokens"} {
		if code, called := serve(projTokenReq(path, nil)); code != http.StatusOK || !called {
			t.Fatalf("token tenant-wide debe PASAR en %s: code=%d called=%v", path, code, called)
		}
	}

	// El humano (sin token en contexto) NO se ve afectado: pasa (autoriza por rol).
	if code, called := serve(humanReq(http.MethodGet, "/api/v1/org")); code != http.StatusOK || !called {
		t.Fatalf("humano no debe verse afectado: code=%d called=%v", code, called)
	}
}

// --- requireRole (rol efectivo de humanos; rechaza tokens) ---

func TestRequireRoleRejectsTokens(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	next, called := spyNext()
	rr := httptest.NewRecorder()
	h.requireRole("admin", "owner")(next).ServeHTTP(rr, tokenReq(http.MethodPost, "/tokens", domain.ScopeDeploy))
	if rr.Code != http.StatusForbidden || *called {
		t.Fatalf("token en requireRole debe ser 403: code=%d called=%v", rr.Code, *called)
	}
}

func TestRequireRoleEffectiveMembership(t *testing.T) {
	// Membership explícito admin => pasa.
	h := newHandlers(&fakeSvc{effRole: domain.RoleAdmin, effFromMember: true})
	next, called := spyNext()
	rr := httptest.NewRecorder()
	h.requireRole("admin", "owner")(next).ServeHTTP(rr, humanReq(http.MethodPatch, "/org"))
	if rr.Code != http.StatusOK || !*called {
		t.Fatalf("admin efectivo: code=%d called=%v", rr.Code, *called)
	}

	// Membership explícito member => 403 (aunque el claim dijera otra cosa).
	h = newHandlers(&fakeSvc{effRole: domain.RoleMember, effFromMember: true})
	next, called = spyNext()
	rr = httptest.NewRecorder()
	h.requireRole("admin", "owner")(next).ServeHTTP(rr, humanReq(http.MethodPatch, "/org", "owner"))
	if rr.Code != http.StatusForbidden || *called {
		t.Fatalf("member efectivo debe denegar pese al claim: code=%d called=%v", rr.Code, *called)
	}
}

func TestRequireRoleClaimFallback(t *testing.T) {
	// Sin membership (fromMembership=false): fallback al claim del JWT (owner inicial).
	h := newHandlers(&fakeSvc{effFromMember: false})
	next, called := spyNext()
	rr := httptest.NewRecorder()
	h.requireRole("admin", "owner")(next).ServeHTTP(rr, humanReq(http.MethodPatch, "/org", "owner"))
	if rr.Code != http.StatusOK || !*called {
		t.Fatalf("fallback owner por claim: code=%d called=%v", rr.Code, *called)
	}

	// Sin membership y claim insuficiente => 403.
	next, called = spyNext()
	rr = httptest.NewRecorder()
	h.requireRole("admin", "owner")(next).ServeHTTP(rr, humanReq(http.MethodPatch, "/org", "member"))
	if rr.Code != http.StatusForbidden || *called {
		t.Fatalf("claim insuficiente: code=%d called=%v", rr.Code, *called)
	}
}

// --- authenticateToken (resuelve + suelo read/write por método) ---

func TestAuthenticateTokenReadOnlyFloor(t *testing.T) {
	resolved := store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeRead, ID: uuid.New()}
	h := newHandlers(&fakeSvc{resolvedTok: resolved})

	// GET con token read => pasa.
	next, called := spyNext()
	rr := httptest.NewRecorder()
	h.authenticateToken(rr, httptest.NewRequest(http.MethodGet, "/projects", http.NoBody), next, "scl_x")
	if rr.Code != http.StatusOK || !*called {
		t.Fatalf("GET read: code=%d called=%v", rr.Code, *called)
	}

	// POST con token read => 403 (no muta sin write).
	next, called = spyNext()
	rr = httptest.NewRecorder()
	h.authenticateToken(rr, httptest.NewRequest(http.MethodPost, "/projects", http.NoBody), next, "scl_x")
	if rr.Code != http.StatusForbidden || *called {
		t.Fatalf("POST read debe ser 403: code=%d called=%v", rr.Code, *called)
	}

	// POST con token write => pasa.
	h = newHandlers(&fakeSvc{resolvedTok: store.ResolvedToken{Tenant: "acme", Scope: domain.ScopeWrite, ID: uuid.New()}})
	next, called = spyNext()
	rr = httptest.NewRecorder()
	h.authenticateToken(rr, httptest.NewRequest(http.MethodPost, "/projects", http.NoBody), next, "scl_x")
	if rr.Code != http.StatusOK || !*called {
		t.Fatalf("POST write: code=%d called=%v", rr.Code, *called)
	}
}

func TestAuthenticateTokenInvalid(t *testing.T) {
	h := newHandlers(&fakeSvc{resolveTokErr: domain.ErrNotFound})
	next, called := spyNext()
	rr := httptest.NewRecorder()
	h.authenticateToken(rr, httptest.NewRequest(http.MethodGet, "/projects", http.NoBody), next, "scl_bad")
	if rr.Code != http.StatusUnauthorized || *called {
		t.Fatalf("token inválido debe ser 401: code=%d called=%v", rr.Code, *called)
	}
}

// --- Handlers de tokens ---

func TestCreateTokenReturnsSecretOnce(t *testing.T) {
	f := &fakeSvc{apiTok: domain.APIToken{Name: "ci", Scope: domain.ScopeDeploy, Plaintext: "scl_secreto"}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.CreateToken(rr, req(http.MethodPost, "/tokens", "", `{"name":"ci","scope":"deploy"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if m := decodeEnvelope(t, rr); m["token"] != "scl_secreto" {
		t.Fatalf("el secreto debe devolverse una vez: %v", m)
	}
	if f.createTokIn == nil || f.createTokIn.Scope != domain.ScopeDeploy || f.createTokIn.CreatedBy != "user-1" {
		t.Fatalf("entrada no propagada: %+v", f.createTokIn)
	}
}

func TestCreateTokenBadExpiry(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.CreateToken(rr, req(http.MethodPost, "/tokens", "", `{"name":"ci","expires_at":"ayer"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rr.Code)
	}
}

func TestListTokensEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListTokens(rr, req(http.MethodGet, "/tokens", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if _, ok := decodeEnvelope(t, rr)["tokens"]; !ok {
		t.Fatalf("sin clave tokens: %s", rr.Body.String())
	}
}

func TestRevokeToken(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.RevokeToken(rr, req(http.MethodDelete, "/tokens/x", validUUID, ""))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d", rr.Code)
	}
}

// --- Handlers de miembros ---

func TestAddMember(t *testing.T) {
	f := &fakeSvc{member: domain.Membership{Sub: "sub-9", Role: domain.RoleAdmin}}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.AddMember(rr, req(http.MethodPost, "/members", "", `{"sub":"sub-9","role":"admin"}`))
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if f.addMemberSub != "sub-9" || f.addMemberRole != domain.RoleAdmin {
		t.Fatalf("no propagado: sub=%q role=%q", f.addMemberSub, f.addMemberRole)
	}
}

func TestListMembersEmpty(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.ListMembers(rr, req(http.MethodGet, "/members", "", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if _, ok := decodeEnvelope(t, rr)["members"]; !ok {
		t.Fatalf("sin clave members: %s", rr.Body.String())
	}
}

func TestRemoveMember(t *testing.T) {
	h := newHandlers(&fakeSvc{})
	rr := httptest.NewRecorder()
	h.RemoveMember(rr, subReq(http.MethodDelete, "/members/sub-9", "sub-9"))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

// --- Revelación del secreto de webhook en GetProject (solo admin/owner) ---

func TestGetProjectRevealsSecretToAdmin(t *testing.T) {
	f := &fakeSvc{
		project:       domain.Project{Slug: "web"},
		webhookSecret: "scl_wh",
		effRole:       domain.RoleAdmin,
		effFromMember: true,
	}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetProject(rr, req(http.MethodGet, "/projects/x", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if m := decodeEnvelope(t, rr); m["webhook_secret"] != "scl_wh" {
		t.Fatalf("admin debe ver el secreto: %v", m)
	}
}

func TestGetProjectHidesSecretFromMember(t *testing.T) {
	f := &fakeSvc{
		project:       domain.Project{Slug: "web"},
		webhookSecret: "scl_wh",
		effRole:       domain.RoleMember,
		effFromMember: true,
	}
	h := newHandlers(f)
	rr := httptest.NewRecorder()
	h.GetProject(rr, req(http.MethodGet, "/projects/x", validUUID, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if _, ok := decodeEnvelope(t, rr)["webhook_secret"]; ok {
		t.Fatalf("un miembro raso NO debe ver el secreto: %s", rr.Body.String())
	}
}
