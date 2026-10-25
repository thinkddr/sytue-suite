// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// sesionBorrado abre una sesión con la antigüedad y los métodos dados y devuelve
// su cookie.
func (l *liveServer) sesionBorrado(t *testing.T, userID, orgID string, hace time.Duration, amr ...string) *http.Cookie {
	t.Helper()
	token := "tok-borrar-" + userID + "-" + hace.String() + "-" + strings.Join(amr, "-")
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(token), UserID: userID, OrgID: orgID,
		AuthTime: time.Now().UTC().Add(-hace), AMR: amr, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token}
}

func (l *liveServer) borrar(t *testing.T, c *http.Cookie, sitio string, campos url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/account/delete", strings.NewReader(campos.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Sec-Fetch-Site", sitio)
	r.AddCookie(c)
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	return rr
}

func (l *liveServer) estado(t *testing.T, userID string) (status string, propia bool) {
	t.Helper()
	if err := l.st.DB().QueryRow(l.ctx,
		`SELECT status, coalesce(closed_by = id, false) FROM users WHERE id = $1`, userID).Scan(&status, &propia); err != nil {
		t.Fatalf("leer estado: %v", err)
	}
	return status, propia
}

func (l *liveServer) cuenta(t *testing.T, sql, userID string) int {
	t.Helper()
	var n int
	if err := l.st.DB().QueryRow(l.ctx, sql, userID).Scan(&n); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return n
}

// Borrar mi cuenta: cada verja se ve cerrada antes de verla abierta.
func TestBorrarMiCuenta(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-borrar", Name: "Acme Borrar"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	duena := l.addMember(t, org, "duena@id.test", domain.RoleOwner)
	u := caOrgUser(t, l, org, "se-va@id.test")
	if _, err := l.st.SetAvatar(l.ctx, u.ID, "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="); err != nil {
		t.Fatalf("SetAvatar: %v", err)
	}
	if _, err := l.st.CreateAppPassword(l.ctx, u.ID, org.ID, "hash-app", "Correo"); err != nil {
		t.Fatalf("CreateAppPassword: %v", err)
	}
	if _, err := l.st.UpsertDevice(l.ctx, nil, domain.Device{OrgID: org.ID, UserID: u.ID, DeviceID: "disp-borrar", Name: "Portátil"}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	vieja := l.sesionBorrado(t, u.ID, org.ID, time.Hour, "pwd")
	bien := url.Values{"confirm": {"se-va@id.test"}, "password": {"login-secret-123"}}

	// Desde otro sitio, con la cuenta mal escrita o con la contraseña mala: nada.
	if rr := l.borrar(t, vieja, "cross-site", bien); rr.Code != http.StatusForbidden {
		t.Fatalf("desde otro sitio: %d, quiero 403", rr.Code)
	}
	if rr := l.borrar(t, vieja, "same-origin", url.Values{"confirm": {"otra@id.test"}, "password": {"login-secret-123"}}); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("cuenta mal escrita: %d, quiero 422", rr.Code)
	}
	if rr := l.borrar(t, vieja, "same-origin", url.Values{"confirm": {"se-va@id.test"}, "password": {"mala"}}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("contraseña mala: %d, quiero 401", rr.Code)
	}
	if s, _ := l.estado(t, u.ID); s != "active" {
		t.Fatalf("tras los intentos fallidos la cuenta está %q", s)
	}

	// La única propietaria no puede irse.
	sDuena := l.sesionBorrado(t, duena.ID, org.ID, time.Minute, "pwd")
	if rr := l.borrar(t, sDuena, "same-origin", url.Values{"confirm": {"duena@id.test"}}); rr.Code != http.StatusConflict ||
		!strings.Contains(rr.Body.String(), "Acme Borrar") {
		t.Fatalf("única propietaria: %d, quiero 409 con el nombre de la organización", rr.Code)
	}
	if s, _ := l.estado(t, duena.ID); s != "active" {
		t.Fatalf("la única propietaria quedó %q", s)
	}

	// Ahora sí.
	rr := l.borrar(t, vieja, "same-origin", bien)
	if rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("borrar: %d %s", rr.Code, rr.Body)
	}
	if s, propia := l.estado(t, u.ID); s != "deleted" || !propia {
		t.Fatalf("tras borrar: status=%q propia=%v", s, propia)
	}
	for _, sql := range []string{
		`SELECT count(*) FROM sessions WHERE user_id = $1`,
		`SELECT count(*) FROM devices WHERE user_id = $1`,
		`SELECT count(*) FROM app_passwords WHERE user_id = $1`,
		`SELECT count(*) FROM user_avatars WHERE user_id = $1`,
		`SELECT count(*) FROM users WHERE id = $1 AND password_hash IS NOT NULL`,
	} {
		if n := l.cuenta(t, sql, u.ID); n != 0 {
			t.Fatalf("%s = %d tras borrar", sql, n)
		}
	}
	// Con la cuenta borrada ya no hay sesión: la página manda a identificarse.
	g := httptest.NewRequest(http.MethodGet, "/account/delete", http.NoBody)
	g.AddCookie(vieja)
	gr := httptest.NewRecorder()
	l.mux.ServeHTTP(gr, g)
	if gr.Code != http.StatusFound {
		t.Fatalf("GET tras borrar: %d, quiero 302 al login", gr.Code)
	}
	// Con otra propietaria activa, la que era la única ya puede irse.
	_ = l.addMember(t, org, "otra-duena@id.test", domain.RoleOwner)
	if rr := l.borrar(t, sDuena, "same-origin", url.Values{"confirm": {"duena@id.test"}}); rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("propietaria con otra propietaria: %d %s", rr.Code, rr.Body)
	}
}

// Con segundo factor, la contraseña sola no basta: hay que volver a entrar con los
// dos, y una sesión así de reciente vale sin contraseña.
func TestBorrarMiCuentaConSegundoFactor(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-borrar-mfa", Name: "Acme MFA"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	_ = l.addMember(t, org, "duena-mfa@id.test", domain.RoleOwner)
	u := caOrgUser(t, l, org, "mfa-se-va@id.test")
	l.enrollMFA(t, u.ID)
	campos := url.Values{"confirm": {"mfa-se-va@id.test"}, "password": {"login-secret-123"}}

	for _, c := range []struct {
		nombre string
		cookie *http.Cookie
	}{
		{"contraseña buena y sesión vieja", l.sesionBorrado(t, u.ID, org.ID, time.Hour, "pwd", "otp")},
		{"sesión reciente sin segundo factor", l.sesionBorrado(t, u.ID, org.ID, time.Minute, "pwd")},
	} {
		if rr := l.borrar(t, c.cookie, "same-origin", campos); rr.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d, quiero 401", c.nombre, rr.Code)
		}
	}
	if s, _ := l.estado(t, u.ID); s != "active" {
		t.Fatalf("se borró sin reautenticar con el segundo factor: %q", s)
	}
	fresca := l.sesionBorrado(t, u.ID, org.ID, time.Minute, "pwd", "otp")
	if rr := l.borrar(t, fresca, "same-origin", url.Values{"confirm": {"mfa-se-va@id.test"}}); rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("sesión reciente con segundo factor: %d %s", rr.Code, rr.Body)
	}
	for _, sql := range []string{
		`SELECT count(*) FROM totp_secrets WHERE user_id = $1`,
		`SELECT count(*) FROM webauthn_credentials WHERE user_id = $1`,
		`SELECT count(*) FROM mfa_recovery_codes WHERE user_id = $1`,
		`SELECT count(*) FROM users WHERE id = $1 AND mfa_enabled`,
	} {
		if n := l.cuenta(t, sql, u.ID); n != 0 {
			t.Fatalf("%s = %d tras borrar", sql, n)
		}
	}
}

// Vencida la ventana, la ficha se anonimiza, se va de sus organizaciones y no
// recupera un @usuario; antes de vencer, o sin el paso de los otros servicios, no.
func TestPurgaDeCuentaBorrada(t *testing.T) {
	l := newLiveServer(t)
	u, org, err := l.srv.crearCuentaPersonal(l.ctx, domain.User{
		Email: "ana-borrada@id.test", Username: "ana-borrada", EmailVerified: true, Name: "Ana",
	})
	if err != nil {
		t.Fatalf("crearCuentaPersonal: %v", err)
	}
	c := l.sesionBorrado(t, u.ID, org.ID, time.Minute, "pwd")
	if rr := l.borrar(t, c, "same-origin", url.Values{"confirm": {"@Ana-Borrada"}}); rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("borrar la cuenta personal: %d %s", rr.Code, rr.Body)
	}
	ahora := time.Now().Add(-retencionBaja)
	if p, err := l.st.PurgarCuentaPropia(l.ctx, u.ID, ahora); err != nil || p != nil {
		t.Fatalf("purga dentro de la ventana: %v %v", p, err)
	}
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET closed_at = now() - interval '31 days' WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if p, err := l.st.PurgarCuentaPropia(l.ctx, u.ID, ahora); err != nil || p != nil {
		t.Fatalf("purga vencida pero sin el paso de los servicios: %v %v", p, err)
	}
	if err := l.st.MarcarDatosPersonalesBorrados(l.ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	p, err := l.st.PurgarCuentaPropia(l.ctx, u.ID, ahora)
	if err != nil || len(p) != 1 || p[0] != org.ID {
		t.Fatalf("purga vencida: %v %v, quiero [%s]", p, err, org.ID)
	}
	var email, nombre string
	var username, phone *string
	if err := l.st.DB().QueryRow(l.ctx, `SELECT email, name, username, phone FROM users WHERE id = $1`, u.ID).
		Scan(&email, &nombre, &username, &phone); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(email, "ana") || nombre != "" || username != nil || phone != nil {
		t.Fatalf("ficha sin anonimizar: %q %q %v %v", email, nombre, username, phone)
	}
	if n := l.cuenta(t, `SELECT count(*) FROM memberships WHERE user_id = $1`, u.ID); n != 0 {
		t.Fatalf("quedan %d membresías", n)
	}
	if _, err := l.st.AsignarUsernamesPendientes(l.ctx, 100); err != nil {
		t.Fatal(err)
	}
	if n := l.cuenta(t, `SELECT count(*) FROM users WHERE id = $1 AND username IS NOT NULL`, u.ID); n != 0 {
		t.Fatal("la cuenta purgada recibió otro @usuario")
	}
	// Y su dirección sigue ocupada: nadie puede quedarse con ella y recibir lo suyo.
	if n := l.cuenta(t, `SELECT count(*) FROM org_mailboxes WHERE user_id = $1 AND removed_at IS NULL`, u.ID); n == 0 {
		t.Fatal("la dirección de la cuenta purgada quedó libre")
	}
	if p, err := l.st.PurgarCuentaPropia(l.ctx, u.ID, ahora); err != nil || p != nil {
		t.Fatalf("segunda purga: %v %v", p, err)
	}
}

// Lo que señaló la revisión de seguridad (ADR 0054), cada cosa con su prueba.
func TestBorrarMiCuentaRevision(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-revision", Name: "Acme Revisión"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	a := l.addMember(t, org, "a-duena@id.test", domain.RoleOwner)
	b := l.addMember(t, org, "b-duena@id.test", domain.RoleOwner)
	tokA := l.token(t, a.ID, org.Slug, nil)
	if _, err := l.st.CreateAppPassword(l.ctx, a.ID, org.ID, domain.HashToken("appsecretoderevision"), "Correo"); err != nil {
		t.Fatal(err)
	}
	cA := l.sesionBorrado(t, a.ID, org.ID, time.Minute, "pwd")
	if rr := l.borrar(t, cA, "same-origin", url.Values{"confirm": {"a-duena@id.test"}}); rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("borrar A: %d", rr.Code)
	}

	// Un access token vivo de la cuenta borrada ya no vale para la API.
	if rr, _ := l.do(t, http.MethodGet, "/api/v1/me", tokA, nil); rr.Code != http.StatusUnauthorized {
		t.Fatalf("token de una cuenta borrada: %d, quiero 401", rr.Code)
	}
	// Ni una contraseña de aplicación que siguiera ahí (la cuenta no está activa).
	if _, err := l.st.DB().Exec(l.ctx, `INSERT INTO app_passwords (user_id, secret_hash, label) VALUES ($1, $2, 'x')`,
		a.ID, domain.HashToken("otrosecretoderevision")); err != nil {
		t.Fatal(err)
	}
	vr := httptest.NewRecorder()
	l.srv.handleInternalVerifyAppPassword(vr, httptest.NewRequest(http.MethodPost, "/internal/app-passwords/verify",
		strings.NewReader(`{"password":"otrosecretoderevision"}`)))
	if vr.Code != http.StatusUnauthorized {
		t.Fatalf("contraseña de aplicación de una cuenta borrada: %d, quiero 401", vr.Code)
	}

	// La propietaria borrada no cuenta: B ya es la única y no se puede ir.
	if n, err := l.st.CountOrgOwners(l.ctx, nil, org.ID, ""); err != nil || n != 1 {
		t.Fatalf("CountOrgOwners = %d, %v; quiero 1 (la borrada no cuenta)", n, err)
	}
	// Y al revés: con B activa, retirar a la propietaria borrada no es «quitar a la
	// última» (la cuenta afectada cuenta).
	if n, err := l.st.CountOrgOwners(l.ctx, nil, org.ID, a.ID); err != nil || n != 2 {
		t.Fatalf("CountOrgOwners con A de afectada = %d, %v; quiero 2", n, err)
	}
	cB := l.sesionBorrado(t, b.ID, org.ID, time.Minute, "pwd")
	if rr := l.borrar(t, cB, "same-origin", url.Values{"confirm": {"b-duena@id.test"}}); rr.Code != http.StatusConflict {
		t.Fatalf("la última propietaria activa: %d, quiero 409", rr.Code)
	}

	// Un administrador no deshace la baja que pidió la persona.
	if err := l.st.RestoreUser(l.ctx, nil, a.ID, time.Now().Add(-retencionBaja)); err == nil {
		t.Fatal("RestoreUser deshizo una baja propia")
	}

	// Si vuelve (a mano, por privacidad) y se va otra vez, sus datos de otros
	// servicios se vuelven a borrar: la marca no sobrevive a la nueva baja.
	if err := l.st.MarcarDatosPersonalesBorrados(l.ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := l.st.DB().Exec(l.ctx, `UPDATE users SET status = 'active', closed_at = NULL, closed_by = NULL WHERE id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	cA2 := l.sesionBorrado(t, a.ID, org.ID, 2*time.Minute, "pwd")
	if rr := l.borrar(t, cA2, "same-origin", url.Values{"confirm": {"a-duena@id.test"}}); rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("segunda baja: %d %s", rr.Code, rr.Body)
	}
	if n := l.cuenta(t, `SELECT count(*) FROM users WHERE id = $1 AND personal_data_erased_at IS NOT NULL`, a.ID); n != 0 {
		t.Fatal("la segunda baja heredó la marca de datos ya borrados")
	}

	// La última administradora de la plataforma no se puede ir.
	if _, err := l.st.DB().Exec(l.ctx, `INSERT INTO platform_admins (user_id) VALUES ($1)`, b.ID); err != nil {
		t.Fatal(err)
	}
	_ = l.addMember(t, org, "c-duena@id.test", domain.RoleOwner)
	if rr := l.borrar(t, cB, "same-origin", url.Values{"confirm": {"b-duena@id.test"}}); rr.Code != http.StatusConflict ||
		!strings.Contains(rr.Body.String(), "plataforma") {
		t.Fatalf("la última administradora de la plataforma: %d, quiero 409", rr.Code)
	}
}

// Con segundo factor, una llave sin verificación de usuario no basta.
func TestBorrarMiCuentaLlaveSinPIN(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme-llave", Name: "Acme Llave"})
	if err != nil {
		t.Fatal(err)
	}
	_ = l.addMember(t, org, "duena-llave@id.test", domain.RoleOwner)
	u := caOrgUser(t, l, org, "llave@id.test")
	l.enrollMFA(t, u.ID)
	c := l.sesionBorrado(t, u.ID, org.ID, time.Minute, "webauthn")
	if rr := l.borrar(t, c, "same-origin", url.Values{"confirm": {"llave@id.test"}}); rr.Code != http.StatusUnauthorized {
		t.Fatalf("llave sin PIN: %d, quiero 401", rr.Code)
	}
	c2 := l.sesionBorrado(t, u.ID, org.ID, 2*time.Minute, "webauthn", "mfa")
	if rr := l.borrar(t, c2, "same-origin", url.Values{"confirm": {"llave@id.test"}}); rr.Code != http.StatusOK && rr.Code != http.StatusSeeOther {
		t.Fatalf("llave con PIN: %d %s", rr.Code, rr.Body)
	}
}
