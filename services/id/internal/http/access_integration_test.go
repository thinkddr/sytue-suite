// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del ENFORCEMENT de Acceso Condicional en el flujo de login
// (contra un PostgreSQL real, build-tag `integration`). Verifican el punto exacto de
// enforcement — tras verificar la credencial, antes de emitir la sesión — con el
// harness newLiveServer: un usuario bajo `block` no obtiene sesión; un usuario
// excluido (break-glass) sí; bajo `require_mfa` sin segundo factor se le deniega y con
// segundo factor se le fuerza el step-up MFA. Reusa postForm/sessionCookieSet.
package http

import (
	"net/http"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// caOrgUser crea una org, un usuario con contraseña y su membresía; devuelve ambos.
func caOrgUser(t *testing.T, l *liveServer, org domain.Organization, email string) domain.User {
	t.Helper()
	u := l.mkUser(t, email, "login-secret-123")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{
		OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember,
	}); err != nil {
		t.Fatalf("UpsertMembership(%s): %v", email, err)
	}
	return u
}

func caLogin(t *testing.T, l *liveServer, email string) (int, bool) {
	t.Helper()
	rr := l.postForm(t, "/login/", map[string][]string{"email": {email}, "password": {"login-secret-123"}})
	return rr.Code, sessionCookieSet(rr)
}

func TestConditionalAccess_BlockAndBreakGlass(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "ca-block", Name: "CA Block"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	caOrgUser(t, l, org, "blocked@id.test")
	admin := caOrgUser(t, l, org, "admin@id.test")

	// Política block para TODOS, salvo el admin (exclusión break-glass).
	if _, err := l.st.CreateAccessPolicy(l.ctx, nil, domain.AccessPolicy{
		OrgID: org.ID, Name: "Bloquear todo", Enabled: true, Action: domain.AccessBlock,
		ExcludeUsers: []string{admin.ID}, StartMinute: -1, EndMinute: -1,
	}); err != nil {
		t.Fatalf("CreateAccessPolicy: %v", err)
	}

	// Usuario bajo block → login DENEGADO (401) y SIN sesión.
	if code, sess := caLogin(t, l, "blocked@id.test"); code != http.StatusUnauthorized || sess {
		t.Fatalf("bloqueado: code=%d session=%v, quiero 401 y sin sesión", code, sess)
	}
	// Admin excluido (break-glass) → login PERMITIDO, con sesión.
	if code, sess := caLogin(t, l, "admin@id.test"); !sess || code != http.StatusFound {
		t.Fatalf("excluido: code=%d session=%v, quiero 302 y con sesión", code, sess)
	}
}

func TestConditionalAccess_RequireMFA(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "ca-mfa", Name: "CA MFA"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	caOrgUser(t, l, org, "nomfa@id.test")
	withMFA := caOrgUser(t, l, org, "withmfa@id.test")
	// Habilitar MFA del segundo usuario (basta la columna: el step-up solo consulta
	// user.MFAEnabled para emitir la continuación; no valida el secreto hasta el paso 2).
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET mfa_enabled = true WHERE id = $1`, withMFA.ID); err != nil {
		t.Fatalf("enable mfa: %v", err)
	}

	// BASELINE (sin política): el usuario sin MFA entra con sesión.
	if code, sess := caLogin(t, l, "nomfa@id.test"); !sess || code != http.StatusFound {
		t.Fatalf("baseline sin política: code=%d session=%v, quiero 302 con sesión", code, sess)
	}

	// Política require_mfa para todos.
	if _, err := l.st.CreateAccessPolicy(l.ctx, nil, domain.AccessPolicy{
		OrgID: org.ID, Name: "Exigir MFA", Enabled: true, Action: domain.AccessRequireMFA,
		StartMinute: -1, EndMinute: -1,
	}); err != nil {
		t.Fatalf("CreateAccessPolicy: %v", err)
	}

	// Usuario SIN MFA bajo require_mfa → DENEGADO (no puede enrolar antes de la sesión).
	if code, sess := caLogin(t, l, "nomfa@id.test"); code != http.StatusUnauthorized || sess {
		t.Fatalf("require_mfa sin factor: code=%d session=%v, quiero 401 sin sesión", code, sess)
	}

	// Usuario CON MFA bajo require_mfa → se le FUERZA el step-up (302 a step_up=1), sin
	// sesión todavía (la sesión se abre tras verificar el segundo factor).
	rr := l.postForm(t, "/login/", map[string][]string{
		"email": {"withmfa@id.test"}, "password": {"login-secret-123"},
	})
	if rr.Code != http.StatusFound || sessionCookieSet(rr) {
		t.Fatalf("require_mfa con factor: code=%d session=%v, quiero 302 sin sesión (step-up)", rr.Code, sessionCookieSet(rr))
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "step_up=1") {
		t.Fatalf("require_mfa con factor: Location=%q, quiero redirección a step_up=1", loc)
	}
}
