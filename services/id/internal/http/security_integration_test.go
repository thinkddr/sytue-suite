// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del ciclo de vida de credenciales (Ola de fundación · ID)
// contra un PostgreSQL real (pgtest sobre testcontainers, build-tag `integration`,
// fuera del CI por defecto). Ejercen los handlers HTTP de extremo a extremo con un
// access token real firmado: cambio de contraseña (actual incorrecta → 401;
// correcta → 200 + sesiones revocadas + hash cambiado), política (corta → 400),
// enrolamiento TOTP (código inválido → no activa; válido → activa) y reset por
// admin (no-admin → 403; admin → 200 + contraseña fijada).
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/id/db"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/oidc"
	"git.sytue.eu/sytue/one/services/id/internal/store"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

const (
	itIssuer = "https://id.test"
	itAud    = "https://id.test/api"
	// Aprovisionamiento SCIM: token estático del provisioner y org a la que queda
	// ligado (los usa el test de SCIM Groups; inertes para el resto de tests).
	itSCIMToken = "scim-provisioning-token"
	itSCIMOrg   = "scimorg"
)

type liveServer struct {
	srv *Server
	st  *store.Store
	iss *oidc.Issuer
	ctx context.Context
	mux http.Handler
}

func newLiveServer(t *testing.T) *liveServer {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := context.Background()
	km := oidc.NewKeyManager()
	if err := km.Bootstrap(ctx, st); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	iss := oidc.NewIssuer(km, itIssuer, itAud, 15*time.Minute, time.Hour)
	cfg := Config{
		Issuer: itIssuer, ResourceAudience: itAud, RPDisplayName: "Sytue",
		// WebAuthn + reseteo: el host del issuer como RP-ID y su origen permitido.
		RPID: "id.test", RPOrigins: []string{itIssuer}, ResetTokenTTL: 30 * time.Minute,
		// Vida del token de invitación (sin ella expiraría al instante).
		InviteTokenTTL: 72 * time.Hour,
		// Sin esto vale 0 y TODA sesión abierta por el propio /login/ nace
		// caducada, así que ningún test podía usarla: los que necesitaban una la
		// insertaban a mano. El tramo «login real → sesión → /authorize» se
		// quedaba sin probar entero, que es justo donde cabe un bucle de step-up.
		SessionTTL: time.Hour,
		// Vida del token de verificación de email (sin ella expiraría al instante).
		EmailVerifyTokenTTL: 24 * time.Hour,
		// Vida del token de continuación MFA (paso 1→2): sin él, expira al instante.
		MFAContinuationTTL: 5 * time.Minute,
		// Authorization Code + refresh: sin TTL el código expiraría al instante (el
		// flujo /authorize→/token del test DCR no podría canjearlo).
		AuthCodeTTL:     60 * time.Second,
		RefreshTokenTTL: 720 * time.Hour,
		// SCIM: bearer estático del provisioner + org ligada (recurso Group).
		SCIMBearerToken: itSCIMToken,
		SCIMOrgSlug:     itSCIMOrg,
		// La organización que OPERA la plataforma. El cargador de configuración le
		// da el mismo valor por defecto (`config.go`, PLATFORM_ORG_SLUG), pero este
		// `Config` se construye a mano, así que aquí quedaba vacío: el test que
		// prueba que esa organización no se puede suspender la creaba con slug ""
		// y moría en el CHECK de `organizations.slug` antes de llegar a probar
		// nada. Un slug vacío tampoco valía como sentinela: `platform.go` compara
		// `org.Slug == cfg.PlatformOrgSlug` para decidir a quién proteger.
		PlatformOrgSlug: "corporate",
		SignupEnabled:   true,
	}
	srv := NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st, km, iss)
	return &liveServer{srv: srv, st: st, iss: iss, ctx: ctx, mux: srv.Router()}
}

// token emite un access token de quien ACABA de identificarse con segundo
// factor, que es la sesión normal de un administrador: las rutas sensibles
// exigen step-up (`requireStepUp`) y lo comprueban con `amr` y `auth_time`.
//
// Sin esos dos claims, doce tests de esta carpeta empezaron a contestar 401
// `insufficient_user_authentication` en cuanto entró el step-up. Quien quiera
// probar el RECHAZO no debe usar este ayudante: `stepup_test.go` monta sus
// claims a mano, que es donde se vigilan las dos condiciones por separado.
func (l *liveServer) token(t *testing.T, sub, tenant string, roles []string) string {
	t.Helper()
	raw, err := l.iss.IssueAccessToken(oidc.TokenClaims{
		Subject: sub, Audience: "sytue-console", Scope: "openid", Tenant: tenant, Roles: roles,
		AuthTime: time.Now().UTC(), AMR: []string{"pwd", "otp"},
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	return raw
}

func (l *liveServer) do(t *testing.T, method, path, token string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, rdr)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	l.mux.ServeHTTP(rr, r)
	out := map[string]any{}
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr, out
}

func (l *liveServer) mkUser(t *testing.T, email, password string) domain.User {
	t.Helper()
	u, err := l.st.CreateUser(l.ctx, nil, domain.User{Email: email, EmailVerified: true, Name: "Test"})
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", email, err)
	}
	if password != "" {
		hash, err := domain.HashPassword(password)
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if err := l.st.SetPassword(l.ctx, nil, u.ID, hash); err != nil {
			t.Fatalf("SetPassword: %v", err)
		}
	}
	return u
}

func TestMePasswordChange(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "ada@id.test", "original-pass-123")
	tok := l.token(t, u.ID, "", nil)

	// Una sesión viva para comprobar que el cambio la revoca.
	now := time.Now()
	if _, err := l.st.InsertSession(l.ctx, nil, domain.Session{
		TokenHash: domain.HashToken("sess-token-1"), UserID: u.ID, AuthTime: now,
		AMR: []string{"pwd"}, UserAgent: "test", ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	// Actual incorrecta → 401, sin tocar la contraseña.
	rr, _ := l.do(t, http.MethodPost, "/api/v1/me/password", tok,
		map[string]string{"current_password": "WRONG", "new_password": "brand-new-pass-1"})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("actual incorrecta: status=%d, quiero 401", rr.Code)
	}

	// Política: nueva demasiado corta → 400.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/me/password", tok,
		map[string]string{"current_password": "original-pass-123", "new_password": "short"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("nueva corta: status=%d, quiero 400", rr.Code)
	}

	// Correcta → 200 + al menos una sesión revocada.
	rr, out := l.do(t, http.MethodPost, "/api/v1/me/password", tok,
		map[string]string{"current_password": "original-pass-123", "new_password": "brand-new-pass-1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("cambio correcto: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if n, _ := out["sessions_revoked"].(float64); n < 1 {
		t.Fatalf("sessions_revoked = %v, quiero >= 1", out["sessions_revoked"])
	}
	// El hash refleja la nueva contraseña.
	got, err := l.st.GetUserByID(l.ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if ok, _ := domain.VerifyPassword("brand-new-pass-1", got.PasswordHash); !ok {
		t.Fatal("la contraseña no se actualizó al nuevo valor")
	}
}

func TestMeTOTPEnrollConfirm(t *testing.T) {
	l := newLiveServer(t)
	u := l.mkUser(t, "otp@id.test", "a-password-123")
	tok := l.token(t, u.ID, "", nil)

	// Enroll: devuelve secreto + otpauth, NO activa MFA.
	rr, out := l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/enroll", tok, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("enroll: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	b32, _ := out["secret"].(string)
	if b32 == "" || out["otpauth_uri"] == "" {
		t.Fatalf("enroll no devolvió secreto/otpauth: %v", out)
	}
	if u2, _ := l.st.GetUserByID(l.ctx, u.ID); u2.MFAEnabled {
		t.Fatal("enroll no debe activar MFA todavía")
	}
	secret, err := totp.DecodeSecret(b32)
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}

	// Código inválido → 400 y sigue sin activar.
	wrong := "000000"
	if totp.Code(secret, time.Now()) == wrong {
		wrong = "111111"
	}
	rr, _ = l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/confirm", tok, map[string]string{"code": wrong})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("código inválido: status=%d, quiero 400", rr.Code)
	}
	if u2, _ := l.st.GetUserByID(l.ctx, u.ID); u2.MFAEnabled {
		t.Fatal("un código inválido no debe activar el TOTP")
	}

	// Código válido → 200 y MFA activo + secreto confirmado.
	rr, _ = l.do(t, http.MethodPost, "/api/v1/me/mfa/totp/confirm", tok,
		map[string]string{"code": totp.Code(secret, time.Now())})
	if rr.Code != http.StatusOK {
		t.Fatalf("confirm válido: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	if u2, _ := l.st.GetUserByID(l.ctx, u.ID); !u2.MFAEnabled {
		t.Fatal("el código válido debe activar el TOTP (mfa_enabled)")
	}
	if _, err := l.st.GetTOTPSecret(l.ctx, u.ID); err != nil {
		t.Fatalf("el secreto debe quedar confirmado: %v", err)
	}
}

func TestAdminSetPasswordAuthz(t *testing.T) {
	l := newLiveServer(t)
	org, err := l.st.CreateOrg(l.ctx, nil, domain.Organization{Slug: "acme", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	admin := l.mkUser(t, "admin@id.test", "admin-pass-123")
	target := l.mkUser(t, "member@id.test", "old-pass-123")
	for _, m := range []domain.Membership{
		{OrgID: org.ID, UserID: admin.ID, Role: domain.RoleAdmin},
		{OrgID: org.ID, UserID: target.ID, Role: domain.RoleMember},
	} {
		if err := l.st.UpsertMembership(l.ctx, nil, m); err != nil {
			t.Fatalf("UpsertMembership: %v", err)
		}
	}
	path := "/api/v1/admin/users/" + target.ID + "/password"

	// No-admin (rol member) → 403 (deny-by-default de requireRole).
	memberTok := l.token(t, target.ID, "acme", []string{"member"})
	rr, _ := l.do(t, http.MethodPost, path, memberTok, map[string]string{"new_password": "reset-pass-123"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("no-admin: status=%d, quiero 403", rr.Code)
	}

	// Admin → 200 y contraseña fijada.
	adminTok := l.token(t, admin.ID, "acme", []string{"admin"})
	rr, _ = l.do(t, http.MethodPost, path, adminTok, map[string]string{"new_password": "reset-pass-123"})
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: status=%d cuerpo=%s", rr.Code, rr.Body)
	}
	got, err := l.st.GetUserByID(l.ctx, target.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if ok, _ := domain.VerifyPassword("reset-pass-123", got.PasswordHash); !ok {
		t.Fatal("el reset por admin no fijó la contraseña dada")
	}
}
