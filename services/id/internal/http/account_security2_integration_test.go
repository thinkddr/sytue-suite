// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de las dos features de seguridad de cuenta (contra un
// PostgreSQL real, build-tag `integration`): reset de MFA por admin (limpia TOTP +
// passkeys + recovery, aislamiento cross-tenant → 404, idempotente) y política de
// contraseñas por-org (endpoint GET/PATCH con cotas, y ENFORCEMENT en CADA punto
// donde se fija/cambia una contraseña). Reusa el harness newLiveServer.
package http

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// enrollMFA da a un usuario los tres artefactos de segundo factor (TOTP confirmado,
// una passkey y códigos de recuperación) + mfa_enabled, para comprobar que el reset
// los limpia TODOS.
func (l *liveServer) enrollMFA(t *testing.T, userID string) {
	t.Helper()
	if err := l.st.UpsertTOTPSecret(l.ctx, nil, userID, []byte("0123456789abcdef01234567"), true); err != nil {
		t.Fatalf("UpsertTOTPSecret: %v", err)
	}
	if err := l.st.SetMFAEnabled(l.ctx, nil, userID, true); err != nil {
		t.Fatalf("SetMFAEnabled: %v", err)
	}
	if err := l.st.InsertCredential(l.ctx, nil, domain.WebAuthnCredential{
		UserID: userID, CredID: []byte("cred-" + userID), PublicKey: []byte("pk"),
		AAGUID: make([]byte, 16), SignCount: 1, Name: "Llave",
	}); err != nil {
		t.Fatalf("InsertCredential: %v", err)
	}
	if err := l.st.InsertRecoveryCodes(l.ctx, nil, userID, []string{"h1", "h2", "h3"}); err != nil {
		t.Fatalf("InsertRecoveryCodes: %v", err)
	}
}

func TestAdminResetMFA(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	target := l.addMember(t, org, "victim@acme.test", domain.RoleMember)
	l.enrollMFA(t, target.ID)
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	path := "/api/v1/admin/users/" + target.ID + "/reset-mfa"

	// Reset → 200 {ok:true} y los tres artefactos + el flag desaparecen.
	rr, out := l.do(t, http.MethodPost, path, adminTok, nil)
	if rr.Code != http.StatusOK || out["ok"] != true {
		t.Fatalf("reset-mfa: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if _, err := l.st.GetTOTPSecret(l.ctx, target.ID); err == nil {
		t.Fatal("el secreto TOTP debía borrarse")
	}
	if creds, _ := l.st.ListCredentialsForUser(l.ctx, target.ID); len(creds) != 0 {
		t.Fatalf("quedaron %d passkeys, quiero 0", len(creds))
	}
	if n, _ := l.st.CountRecoveryCodes(l.ctx, target.ID); n != 0 {
		t.Fatalf("quedaron %d códigos de recuperación, quiero 0", n)
	}
	if u, _ := l.st.GetUserByID(l.ctx, target.ID); u.MFAEnabled {
		t.Fatal("mfa_enabled debía quedar en false")
	}

	// Idempotente: repetir sin MFA no es error.
	if rr, _ := l.do(t, http.MethodPost, path, adminTok, nil); rr.Code != http.StatusOK {
		t.Fatalf("reset-mfa idempotente: status=%d", rr.Code)
	}

	// Aislamiento: un usuario de OTRO tenant → 404 (sin fuga), MFA intacto.
	other, _ := l.seedOrgAdmin(t, "other")
	stranger := l.addMember(t, other, "stranger@other.test", domain.RoleMember)
	l.enrollMFA(t, stranger.ID)
	rr, _ = l.do(t, http.MethodPost, "/api/v1/admin/users/"+stranger.ID+"/reset-mfa", adminTok, nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("reset-mfa cross-tenant: status=%d, quiero 404", rr.Code)
	}
	if _, err := l.st.GetTOTPSecret(l.ctx, stranger.ID); err != nil {
		t.Fatal("el MFA de otro tenant NO debía tocarse")
	}
}

func TestPasswordPolicyEndpoint(t *testing.T) {
	l := newLiveServer(t)
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	path := "/api/v1/admin/org/password-policy"

	// GET default (migración: min 12, sin complejidad).
	rr, out := l.do(t, http.MethodGet, path, adminTok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET policy: status=%d", rr.Code)
	}
	if n, _ := out["min_length"].(float64); n != 12 || out["require_complexity"] != false {
		t.Fatalf("default inesperado: %v", out)
	}

	// PATCH fuera de rango → 400 (ambas cotas).
	for _, bad := range []int{4, 200} {
		rr, _ := l.do(t, http.MethodPatch, path, adminTok, map[string]any{"min_length": bad})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("PATCH min_length=%d: status=%d, quiero 400", bad, rr.Code)
		}
	}

	// PATCH válido → 200 y GET refleja el cambio.
	rr, _ = l.do(t, http.MethodPatch, path, adminTok, map[string]any{"min_length": 16, "require_complexity": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH válido: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	_, out = l.do(t, http.MethodGet, path, adminTok, nil)
	if n, _ := out["min_length"].(float64); n != 16 || out["require_complexity"] != true {
		t.Fatalf("PATCH no persistió: %v", out)
	}
}

// TestPasswordPolicyEnforcement verifica que TODOS los puntos que fijan/cambian una
// contraseña respetan la política de la org: con min_length=16, una contraseña de
// 12 (que pasaría el default) es rechazada en cada punto.
func TestPasswordPolicyEnforcement(t *testing.T) {
	const weak = "abcdefgh1234"       // 12 runes: OK con default, viola min 16
	const strong = "abcdefghij123456" // 16 runes: cumple

	l := newLiveServer(t)
	mc := &captureMailer{}
	l.srv.mailer = mc
	org, admin := l.seedOrgAdmin(t, "acme")
	adminTok := l.token(t, admin.ID, org.Slug, []string{"admin"})
	// Sube la política a min 16.
	if rr, _ := l.do(t, http.MethodPatch, "/api/v1/admin/org/password-policy", adminTok,
		map[string]any{"min_length": 16}); rr.Code != http.StatusOK {
		t.Fatalf("PATCH policy: status=%d", rr.Code)
	}

	member := l.addMember(t, org, "member@acme.test", domain.RoleMember)
	if err := l.st.SetPassword(l.ctx, nil, member.ID, mustHash(t, "current-pass-123")); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	memberTok := l.token(t, member.ID, org.Slug, []string{"member"})

	// Punto 1: self-service change (POST /me/password) — cubre también la consola
	// /account, que enruta por el mismo changeUserPassword.
	t.Run("me/password", func(t *testing.T) {
		rr, _ := l.do(t, http.MethodPost, "/api/v1/me/password", memberTok,
			map[string]string{"current_password": "current-pass-123", "new_password": weak})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, quiero 400", rr.Code)
		}
		rr, _ = l.do(t, http.MethodPost, "/api/v1/me/password", memberTok,
			map[string]string{"current_password": "current-pass-123", "new_password": strong})
		if rr.Code != http.StatusOK {
			t.Fatalf("contraseña fuerte: status=%d cuerpo=%s", rr.Code, rr.Body)
		}
	})

	// Punto 2: admin set-password (POST /admin/users/{id}/password).
	t.Run("admin-set-password", func(t *testing.T) {
		path := "/api/v1/admin/users/" + member.ID + "/password"
		rr, _ := l.do(t, http.MethodPost, path, adminTok, map[string]string{"new_password": weak})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d, quiero 400", rr.Code)
		}
		rr, _ = l.do(t, http.MethodPost, path, adminTok, map[string]string{"new_password": strong})
		if rr.Code != http.StatusOK {
			t.Fatalf("fuerte: status=%d", rr.Code)
		}
	})

	// Punto 3: reset por email (POST /reset-password). Débil → 422 y el token NO se
	// consume (sigue sirviendo para reintentar con una fuerte).
	t.Run("reset-submit", func(t *testing.T) {
		token, hash, err := domain.NewResetToken(l.srv.resetKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.st.InTx(l.ctx, func(q pg.Querier) error {
			return l.st.InsertResetToken(l.ctx, q, hash, member.ID, time.Now().Add(30*time.Minute))
		}); err != nil {
			t.Fatalf("InsertResetToken: %v", err)
		}
		rr := l.postForm(t, "/reset-password", url.Values{
			"token": {token}, "new_password": {weak}, "confirm_password": {weak},
		})
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("débil: status=%d, quiero 422", rr.Code)
		}
		rr = l.postForm(t, "/reset-password", url.Values{
			"token": {token}, "new_password": {strong}, "confirm_password": {strong},
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("token no preservado tras fallo de política: status=%d", rr.Code)
		}
	})

	// Punto 4: alta por invitación (POST /accept-invitation). Débil → 422 sin crear
	// cuenta ni quemar el token; fuerte → cuenta creada.
	t.Run("invite-accept", func(t *testing.T) {
		rr, _ := l.do(t, http.MethodPost, "/api/v1/admin/invitations", adminTok,
			map[string]string{"email": "newbie@acme.test", "role": "member"})
		if rr.Code != http.StatusCreated {
			t.Fatalf("invitar: status=%d", rr.Code)
		}
		token := tokenFromLink(t, mc.inviteLink)
		rr = l.postForm(t, "/accept-invitation", url.Values{
			"token": {token}, "new_password": {weak}, "confirm_password": {weak},
		})
		if rr.Code != http.StatusUnprocessableEntity {
			t.Fatalf("débil: status=%d, quiero 422", rr.Code)
		}
		if _, err := l.st.GetUserByEmail(l.ctx, "newbie@acme.test"); err == nil {
			t.Fatal("una contraseña débil no debe crear la cuenta")
		}
		rr = l.postForm(t, "/accept-invitation", url.Values{
			"token": {token}, "new_password": {strong}, "confirm_password": {strong},
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("token no preservado / fuerte rechazada: status=%d", rr.Code)
		}
		if _, err := l.st.GetUserByEmail(l.ctx, "newbie@acme.test"); err != nil {
			t.Fatalf("la cuenta debía crearse con la contraseña fuerte: %v", err)
		}
	})
}

func mustHash(t *testing.T, pw string) string {
	t.Helper()
	h, err := domain.HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	return h
}
