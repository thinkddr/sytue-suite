// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"html"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// loginSinMFA recorre el formulario real: la sesión no se fabrica ni se eleva.
func (l *liveServer) loginSinMFA(t *testing.T, email string) *http.Cookie {
	t.Helper()
	rr := l.postForm(t, "/login/", map[string][]string{
		"email": {email}, "password": {"login-secret-123"}, "return_to": {"/account"},
	})
	sess := cookieNamed(rr.Result().Cookies(), sessionCookieName)
	if rr.Code != http.StatusFound || sess == nil {
		t.Fatalf("login sin MFA: status=%d Location=%q, quiero sesión", rr.Code, rr.Header().Get("Location"))
	}
	stored, err := l.st.GetSessionByToken(l.ctx, domain.HashToken(sess.Value))
	if err != nil || hasMFA(stored.AMR) {
		t.Fatalf("sesión de contraseña: amr=%v err=%v", stored.AMR, err)
	}
	return sess
}

func TestRolAdministrativoNoObligaMFAEnProductosNiMiCuenta(t *testing.T) {
	l := newLiveServer(t)
	for _, caso := range []struct {
		nombre   string
		rol      domain.Role
		platform bool
	}{
		{"owner", domain.RoleOwner, false},
		{"admin", domain.RoleAdmin, false},
		{"platform-admin", domain.RoleMember, true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: caso.nombre, Name: caso.nombre})
			if err != nil {
				t.Fatal(err)
			}
			u := caOrgUser(t, l, org, caso.nombre+"@producto.test")
			if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: caso.rol}); err != nil {
				t.Fatal(err)
			}
			if caso.platform {
				if err := l.st.UpsertPlatformAdmin(l.ctx, nil, u.ID, u.ID, "prueba"); err != nil {
					t.Fatal(err)
				}
			}
			sess := l.loginSinMFA(t, u.Email)
			for _, app := range []struct{ client, vuelta string }{
				{"sytue-contacts", "https://contacts.sytue.eu/contacts/auth/callback"},
				{"sytue-account", "https://id.sytue.eu/account/auth/callback"},
			} {
				l.authorizeCode(t, app.client, app.vuelta, sess)
			}
			for _, app := range []nativa{
				{client: "sytue-mobile", vuelta: "sytue-contacts://auth/callback", secreto: "secreto-del-iphone-con-32-bytes!!!"},
				{client: "sytue-desktop", vuelta: "http://127.0.0.1:8921/auth/callback", secreto: "secreto-del-mac-con-32-bytes!!!!!!"},
			} {
				rr, verifier := l.pedirAutorizacion(t, app, sess, "")
				enlace := enlacePermitir.FindStringSubmatch(rr.Body.String())
				if rr.Code != http.StatusOK || enlace == nil {
					t.Fatalf("%s sin MFA no muestra consentimiento: status=%d Location=%q", app.client, rr.Code, rr.Header().Get("Location"))
				}
				rr, _ = l.pedirAutorizacion(t, app, sess, html.UnescapeString(enlace[1]))
				if code := codigoDe(rr); code == "" {
					t.Fatalf("%s no emite código tras consentir: status=%d Location=%q", app.client, rr.Code, rr.Header().Get("Location"))
				} else if status := l.canjear(t, app, code, verifier, app.secreto); status != http.StatusOK {
					t.Fatalf("%s canje sin MFA: status=%d", app.client, status)
				}
			}
			// El mismo rol sigue necesitando MFA al abrir la consola privilegiada.
			client, vuelta := "sytue-console", "https://admin.sytue.eu/admin/auth/callback"
			if caso.platform {
				client, vuelta = platformClient, "https://platform.sytue.eu/auth/callback"
			}
			if loc, _ := l.authorizeLocForClient(t, sess, client, vuelta); loc != "/account?mfa=required" {
				t.Fatalf("%s sin MFA: Location=%q, quiero enrolamiento", client, loc)
			}
			if user, err := l.st.GetUserByID(l.ctx, u.ID); err != nil || user.MFAEnabled {
				t.Fatalf("entrar en productos no debe activar MFA: enabled=%v err=%v", user.MFAEnabled, err)
			}
		})
	}
}

func TestTokenContactsSinMFANoAbreAdministracion(t *testing.T) {
	l := newLiveServer(t)
	const secret = "contacts-secret-prueba-1"
	l.setClientSecret(t, "sytue-contacts", secret)
	for _, role := range []domain.Role{domain.RoleOwner, domain.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: string(role), Name: string(role)})
			if err != nil {
				t.Fatal(err)
			}
			u := caOrgUser(t, l, org, string(role)+"@token.test")
			if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: role}); err != nil {
				t.Fatal(err)
			}
			sess := l.loginSinMFA(t, u.Email)
			token := l.tokenDeApp(t, "sytue-contacts", secret, "https://contacts.sytue.eu/contacts/auth/callback", sess)
			for _, ruta := range []string{"/api/v1/me", "/api/v1/me/sessions", "/api/v1/onboarding"} {
				if rr, _ := l.do(t, http.MethodGet, ruta, token, nil); rr.Code != http.StatusOK {
					t.Fatalf("self-service %s sin MFA: status=%d body=%s", ruta, rr.Code, rr.Body)
				}
			}
			for _, req := range []struct{ method, path string }{
				{http.MethodGet, "/api/v1/admin/users"},
				{http.MethodGet, "/api/v1/admin/audit"},
				{http.MethodGet, "/api/v1/admin/roles"},
				{http.MethodPost, "/api/v1/admin/users/" + u.ID + "/reset-password"},
				{http.MethodPost, "/api/v1/orgs"},
				{http.MethodPost, "/api/v1/orgs/" + org.Slug + "/members"},
			} {
				rr, _ := l.do(t, req.method, req.path, token, map[string]any{})
				if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Header().Get("WWW-Authenticate"), "insufficient_user_authentication") {
					t.Errorf("%s %s con token Contacts sin MFA: status=%d reto=%q, quiero RFC9470", req.method, req.path, rr.Code, rr.Header().Get("WWW-Authenticate"))
				}
			}
			// El mismo usuario sí administra con segundo factor; no es un veto al rol.
			consola := l.token(t, u.ID, org.Slug, []string{string(role)})
			if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/users", consola, nil); rr.Code != http.StatusOK {
				t.Fatalf("admin con MFA: status=%d body=%s", rr.Code, rr.Body)
			}
		})
	}
	// El alcance de plataforma es independiente del rol de organización. Un
	// miembro globalmente autorizado tampoco eleva su token de Contacts sin MFA.
	u := l.mkUser(t, "global@token.test", "login-secret-123")
	if err := l.st.UpsertPlatformAdmin(l.ctx, nil, u.ID, u.ID, "prueba"); err != nil {
		t.Fatal(err)
	}
	token := l.tokenDeApp(t, "sytue-contacts", secret, "https://contacts.sytue.eu/contacts/auth/callback", l.loginSinMFA(t, u.Email))
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/platform/activation", token, nil); rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "mfa_required") {
		t.Fatalf("plataforma con token Contacts sin MFA: status=%d body=%s", rr.Code, rr.Body)
	}
}

func TestCuentaPersonalConservaIntegracionesSinMFA(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: domain.PersonalSlug("personal@producto.test"), Name: "Personal", Kind: domain.OrgKindPersonal})
	if err != nil {
		t.Fatal(err)
	}
	u := caOrgUser(t, l, org, "personal@producto.test")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleOwner}); err != nil {
		t.Fatal(err)
	}
	l.setClientSecret(t, "sytue-contacts", "personal-secret-prueba-1")
	token := l.tokenDeApp(t, "sytue-contacts", "personal-secret-prueba-1", "https://contacts.sytue.eu/contacts/auth/callback", l.loginSinMFA(t, u.Email))
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/apps/oidc-clients", token, nil); rr.Code != http.StatusOK {
		t.Fatalf("integraciones personales sin MFA: status=%d body=%s", rr.Code, rr.Body)
	}
}

func TestMFAOptativoSePuedeActivarYSeExigeEnSiguienteLogin(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "optativo", Name: "Optativo"})
	if err != nil {
		t.Fatal(err)
	}
	u := caOrgUser(t, l, org, "optativo@producto.test")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleOwner}); err != nil {
		t.Fatal(err)
	}
	sess := l.loginSinMFA(t, u.Email)
	l.setClientSecret(t, "sytue-account", "account-secret-prueba-1")
	token := l.tokenDeApp(t, "sytue-account", "account-secret-prueba-1", "https://id.sytue.eu/account/auth/callback", sess)
	rr, out := l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/enroll", token, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("activar MFA desde Mi cuenta sin MFA: status=%d body=%s", rr.Code, rr.Body)
	}
	b32, _ := out["secret"].(string)
	secret, err := totp.DecodeSecret(b32)
	if err != nil {
		t.Fatal(err)
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/confirm", token, map[string]string{"code": totp.Code(secret, time.Now())})
	if rr.Code != http.StatusOK {
		t.Fatalf("confirmar MFA voluntario: status=%d body=%s", rr.Code, rr.Body)
	}
	rr = l.postForm(t, "/login/", map[string][]string{
		"email": {u.Email}, "password": {"login-secret-123"}, "return_to": {"/account"},
	})
	if rr.Code != http.StatusFound || sessionCookieSet(rr) || cookieNamed(rr.Result().Cookies(), mfaCookieName) == nil || !strings.Contains(rr.Header().Get("Location"), "step_up=1") {
		t.Fatalf("MFA activado debe desafiar antes de abrir sesión: status=%d Location=%q session=%v", rr.Code, rr.Header().Get("Location"), sessionCookieSet(rr))
	}
}

func TestRolesOmitidosNoEludenMFAConPermisosDelegados(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "roles-omitidos", Name: "Roles omitidos"})
	if err != nil {
		t.Fatal(err)
	}
	delegado, err := l.st.CreateCustomRole(l.ctx, nil, org.ID, "Leer usuarios", []string{string(domain.PermUsersRead)})
	if err != nil {
		t.Fatal(err)
	}
	for _, caso := range []struct {
		rol    domain.Role
		status int
	}{
		{domain.RoleOwner, http.StatusUnauthorized},
		{domain.RoleAdmin, http.StatusUnauthorized},
		{domain.RoleMember, http.StatusOK},
	} {
		t.Run(string(caso.rol), func(t *testing.T) {
			u := caOrgUser(t, l, org, string(caso.rol)+"@omitidos.test")
			if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: caso.rol}); err != nil {
				t.Fatal(err)
			}
			if err := l.st.AssignCustomRole(l.ctx, nil, org.ID, delegado.ID, u.ID); err != nil {
				t.Fatal(err)
			}
			client, err := l.st.GetClient(l.ctx, "sytue-contacts")
			if err != nil {
				t.Fatal(err)
			}
			// Claims del emisor real: permisos leídos de PostgreSQL, pero roles
			// omitidos como cuando solo esa consulta falla durante la emisión.
			claims := l.srv.userClaims(l.ctx, client, u, org.ID, "openid profile", time.Now())
			if len(claims.Perms) != 1 || claims.Perms[0] != string(domain.PermUsersRead) {
				t.Fatalf("permisos del delegado: %v", claims.Perms)
			}
			claims.Roles, claims.AMR, claims.AuthTime = nil, []string{"pwd"}, time.Now()
			token, err := l.iss.IssueAccessToken(claims, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			rr, _ := l.do(t, http.MethodGet, "/api/v1/admin/users", token, nil)
			if rr.Code != caso.status {
				t.Fatalf("roles omitidos de %s: status=%d, quiero %d; body=%s", caso.rol, rr.Code, caso.status, rr.Body)
			}
			if caso.status == http.StatusUnauthorized && !strings.Contains(rr.Header().Get("WWW-Authenticate"), "insufficient_user_authentication") {
				t.Fatal("falta el desafío RFC9470")
			}
		})
	}
}
