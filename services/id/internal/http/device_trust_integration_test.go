// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// claimDelJWT lee un claim del payload de un JWT SIN verificarlo: el token lo acaba
// de emitir el propio servidor de la prueba, y lo que se mira es qué afirma.
func claimDelJWT(t *testing.T, raw, nombre string) any {
	t.Helper()
	partes := strings.Split(raw, ".")
	if len(partes) != 3 {
		t.Fatalf("no es un JWT: %q", raw)
	}
	payload, err := base64.RawURLEncoding.DecodeString(partes[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	return claims[nombre]
}

// TestDeviceTrustDeExtremoAExtremo: el claim sale del canje del código y de cada
// refresco, y sigue al dispositivo de la sesión en el momento de emitir: sin confianza
// es `none`; al marcarlo de confianza, el siguiente refresco dice `standard`; al
// revocarlo, el siguiente vuelve a `none`.
func TestDeviceTrustDeExtremoAExtremo(t *testing.T) {
	l := newLiveServer(t)
	org, adminTok := l.orgAdmin(t, "confianza", "admin@confianza.example")
	const redirectURI = "https://tercero.example/oauth/callback"
	rr, creado := l.do(t, http.MethodPost, "/api/v1/admin/apps/oidc-clients", adminTok, map[string]any{
		"name":          "Tercero",
		"redirect_uris": []string{redirectURI},
		"scopes":        []string{"openid", "profile", "email", "offline_access"},
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("alta del client: %d %s", rr.Code, rr.Body)
	}
	clientID, _ := creado["client_id"].(string)
	secret, _ := creado["client_secret"].(string)

	// La sesión de una persona que entró con passkey desde un dispositivo registrado.
	u := l.mkUser(t, "ana@confianza.example", "")
	if err := l.st.UpsertMembership(l.ctx, nil, domain.Membership{OrgID: org.ID, UserID: u.ID, Role: domain.RoleMember}); err != nil {
		t.Fatal(err)
	}
	dev, err := l.st.UpsertDevice(l.ctx, nil, domain.Device{OrgID: org.ID, UserID: u.ID, DeviceID: "dt-ana", Name: "Portátil"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := domain.GenerateToken(32)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken(raw), UserID: u.ID, OrgID: org.ID, AuthTime: now,
		AMR: []string{"webauthn"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour), DeviceID: dev.ID,
	}); err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: sessionCookieName, Value: raw}

	pedir := func(form url.Values) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(clientID, secret)
		rec := httptest.NewRecorder()
		l.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("/token: %d %s", rec.Code, rec.Body)
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	comprobar := func(tokens map[string]any, quiere, momento string) string {
		t.Helper()
		at, _ := tokens["access_token"].(string)
		if v := claimDelJWT(t, at, "device_trust"); v != quiere {
			t.Errorf("%s: device_trust del access token = %v, quiero %q", momento, v, quiere)
		}
		if idt, _ := tokens["id_token"].(string); idt != "" {
			if v := claimDelJWT(t, idt, "device_trust"); v != quiere {
				t.Errorf("%s: device_trust del ID token = %v, quiero %q", momento, v, quiere)
			}
		}
		rt, _ := tokens["refresh_token"].(string)
		if rt == "" {
			t.Fatalf("%s: sin refresh token", momento)
		}
		return rt
	}

	code, verifier := l.authorizeCode(t, clientID, redirectURI, cookie)
	rt := comprobar(pedir(url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}), domain.DeviceTrustNone, "canje, dispositivo aún sin confianza")

	if _, err := l.st.SetDeviceTrustedForUser(l.ctx, u.ID, dev.ID, true); err != nil {
		t.Fatal(err)
	}
	rt = comprobar(pedir(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rt}}),
		domain.DeviceTrustStandard, "refresco tras marcarlo de confianza")

	if ok, err := l.st.DeleteDeviceForUser(l.ctx, u.ID, dev.ID); err != nil || !ok {
		t.Fatalf("revocar el dispositivo: %v %v", ok, err)
	}
	comprobar(pedir(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {rt}}),
		domain.DeviceTrustNone, "refresco tras revocar el dispositivo")
}

// TestElLoginAtaLaSesionASuDispositivo: el login real (contraseña) registra el
// dispositivo y deja la sesión atada a él, que es de donde sale el claim.
func TestElLoginAtaLaSesionASuDispositivo(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "atada", Name: "Atada"})
	if err != nil {
		t.Fatal(err)
	}
	u := caOrgUser(t, l, org, "atada@id.test")
	if code, ok := caLogin(t, l, u.Email); !ok {
		t.Fatalf("login: %d sin cookie de sesión", code)
	}
	var atadas int
	if err := l.st.DB().QueryRow(l.ctx, `SELECT count(*) FROM sessions s JOIN devices d ON d.id = s.device_id
		WHERE s.user_id = $1 AND d.user_id = $1`, u.ID).Scan(&atadas); err != nil {
		t.Fatal(err)
	}
	if atadas != 1 {
		t.Fatalf("sesiones atadas a su dispositivo tras el login: %d, quiero 1", atadas)
	}
}
