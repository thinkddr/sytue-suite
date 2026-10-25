//go:build integration

// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
)

func (l *liveServer) orgAppPassword(t *testing.T, slug string) domain.Organization {
	t.Helper()
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: slug, Name: slug})
	if err != nil {
		t.Fatal(err)
	}
	return org
}

func (l *liveServer) canjearAppPassword(t *testing.T, user, password string, token bool) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	body, err := json.Marshal(verifyAppPasswordReq{Identity: user, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/internal/app-passwords/verify", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	if token {
		l.srv.handleInternalAppPasswordToken(w, r)
	} else {
		l.srv.handleInternalVerifyAppPassword(w, r)
	}
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

// La sesión en B debe abrir B aunque A sea la primera membresía; retirar B no
// puede redirigir la credencial a A ni revivirla al reinvitar a su dueño.
func TestAppPasswordTenantFijo(t *testing.T) {
	l := newLiveServer(t)
	a, b := l.orgAppPassword(t, "password-a"), l.orgAppPassword(t, "password-b")
	u := l.addMember(t, a, "multi@id.test", domain.RoleMember)
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: b.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	rr, out := l.do(t, http.MethodPost, "/api/v1/me/app-passwords", l.token(t, u.ID, b.Slug, nil), map[string]string{"label": "Archivos"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta: %d %s", rr.Code, rr.Body)
	}
	password, _ := out["password"].(string)
	rr, verified := l.canjearAppPassword(t, u.Email, password, false)
	if rr.Code != http.StatusOK || verified["tenant"] != b.Slug {
		t.Fatalf("verificación eligió otra org: %d %v", rr.Code, verified)
	}
	rr, tokens := l.canjearAppPassword(t, u.Email, password, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("canje: %d %s", rr.Code, rr.Body)
	}
	keys, err := l.srv.keys.PublicJWKS()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := tokens["access_token"].(string)
	claims, err := oidc.Verify(raw, itIssuer, audienciaAppPasswordToken, keys)
	if err != nil {
		t.Fatal(err)
	}
	tenant, _ := claims.Get("tenant")
	scope, _ := claims.Get("scope")
	if tenant != b.Slug || claims.Subject() != u.ID || scope != "openid drive" {
		t.Fatalf("identidad canjeada: sub=%s tenant=%v scope=%v", claims.Subject(), tenant, scope)
	}
	if out["org_id"] != b.ID || out["tenant"] != b.Slug {
		t.Fatalf("alta sin binding exacto: %v", out)
	}
	if _, err := l.st.UpdateOrgSettings(l.ctx, nil, b.ID, b.Name, domain.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if rr, _ := l.canjearAppPassword(t, u.Email, password, true); rr.Code != http.StatusUnauthorized {
		t.Fatalf("org suspendida: %d", rr.Code)
	}
	if _, err := l.st.UpdateOrgSettings(l.ctx, nil, b.ID, b.Name, domain.StatusActive); err != nil {
		t.Fatal(err)
	}
	if err := l.st.RemoveMembership(l.ctx, nil, b.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []bool{false, true} {
		if rr, _ := l.canjearAppPassword(t, u.Email, password, token); rr.Code != http.StatusUnauthorized {
			t.Fatalf("membresía retirada: %d", rr.Code)
		}
	}
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: b.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	if rr, _ := l.canjearAppPassword(t, u.Email, password, true); rr.Code != http.StatusUnauthorized {
		t.Fatalf("reinvitación revivió la credencial: %d", rr.Code)
	}
}

func TestAppPasswordTenantRechazaDestinoNoAutorizado(t *testing.T) {
	l := newLiveServer(t)
	a, ajena := l.orgAppPassword(t, "password-own"), l.orgAppPassword(t, "password-other")
	u := l.addMember(t, a, "own@id.test", domain.RoleMember)
	for _, tenant := range []string{"", ajena.Slug} {
		rr, _ := l.do(t, http.MethodPost, "/api/v1/me/app-passwords", l.token(t, u.ID, tenant, nil), map[string]string{"label": "Inválida"})
		if rr.Code != http.StatusForbidden {
			t.Fatalf("tenant %q: %d %s", tenant, rr.Code, rr.Body)
		}
	}
	tok := l.token(t, u.ID, a.Slug, nil)
	if _, err := l.st.DB().Exec(l.ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, a.ID, u.ID); err != nil {
		t.Fatal(err)
	}
	rr, _ := l.do(t, http.MethodPost, "/api/v1/me/app-passwords", tok, map[string]string{"label": "Token anterior a baja"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("token sin membresía: %d %s", rr.Code, rr.Body)
	}
	if list, err := l.st.ListAppPasswords(l.ctx, u.ID); err != nil || len(list) != 0 {
		t.Fatalf("se crearon claves sin membresía: %v %v", list, err)
	}
}

// La migración conserva una credencial antigua inequívoca, pero las multi-org
// quedan inertes aunque posteriormente solo les quede una membresía.
func TestAppPasswordTenantMigracionLegacy(t *testing.T) {
	l := newLiveServer(t)
	a, b := l.orgAppPassword(t, "legacy-a"), l.orgAppPassword(t, "legacy-b")
	single := l.addMember(t, a, "single@id.test", domain.RoleMember)
	multi := l.addMember(t, a, "legacy-multi@id.test", domain.RoleMember)
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: b.ID, UserID: multi.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	owner, err := pgx.Connect(l.ctx, l.dbOwnerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(l.ctx) })
	down, err := db.Migrations.ReadFile("migrations/0119_app_password_org.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(l.ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	for _, u := range []domain.User{single, multi} {
		if _, err = l.st.DB().Exec(l.ctx, `INSERT INTO app_passwords(user_id,secret_hash,label) VALUES($1,$2,'legacy')`, u.ID, domain.HashToken(NormalizeAppPassword("legacy-"+u.ID))); err != nil {
			t.Fatal(err)
		}
	}
	up, err := db.Migrations.ReadFile("migrations/0119_app_password_org.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Exec(l.ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	if rr, out := l.canjearAppPassword(t, single.Email, "legacy-"+single.ID, false); rr.Code != http.StatusOK || out["tenant"] != a.Slug {
		t.Fatalf("legacy inequívoca: %d %v", rr.Code, out)
	}
	if rr, _ := l.canjearAppPassword(t, multi.Email, "legacy-"+multi.ID, true); rr.Code != http.StatusUnauthorized {
		t.Fatalf("legacy ambigua: %d", rr.Code)
	}
	if _, err := l.st.DB().Exec(l.ctx, `DELETE FROM memberships WHERE org_id=$1 AND user_id=$2`, b.ID, multi.ID); err != nil {
		t.Fatal(err)
	}
	if rr, _ := l.canjearAppPassword(t, multi.Email, "legacy-"+multi.ID, true); rr.Code != http.StatusUnauthorized {
		t.Fatalf("legacy se reasignó tras baja: %d", rr.Code)
	}
}

// Las otras dos altas deben conservar exactamente la misma org que la API JSON.
func TestAppPasswordTenantFormularioYPerfilApple(t *testing.T) {
	l := newLiveServer(t)
	a, b := l.orgAppPassword(t, "paths-a"), l.orgAppPassword(t, "paths-b")
	u := l.addMember(t, a, "paths@id.test", domain.RoleMember)
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: b.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	cookie := l.sesionBorrado(t, u.ID, b.ID, time.Minute, "pwd", "otp")
	r := httptest.NewRequest(http.MethodPost, "/account/app-passwords", strings.NewReader("label=Formulario"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	l.mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("formulario: %d %s", w.Code, w.Body)
	}
	l.srv.cfg.MailIMAPAddr, l.srv.cfg.MailSMTPAddr = "imap.id.test:993", "smtp.id.test:465"
	l.srv.cfg.CalDAVURL, l.srv.cfg.CardDAVURL = "https://calendar.id.test/dav", "https://contacts.id.test/dav"
	rr, _ := l.do(t, http.MethodPost, "/api/v1/me/apple-profile", l.token(t, u.ID, b.Slug, nil), map[string]string{"label": "Apple"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("perfil: %d %s", rr.Code, rr.Body)
	}
	rows, err := l.st.ListAppPasswords(l.ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("altas: %d", len(rows))
	}
	for _, row := range rows {
		if row.OrgID != b.ID || row.Tenant != b.Slug {
			t.Fatalf("%s ligada a otra org: %+v", row.Label, row)
		}
	}
}
