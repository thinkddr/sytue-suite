// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ciclo de vida de credenciales self-service (API JSON, JWT bearer): cambio de
// contraseña, reset por admin y enrolamiento/desactivación de TOTP. Reusa la
// criptografía existente (Argon2id de domain, HMAC-SHA1 de totp) y la política de
// contraseñas de dominio; NUNCA se registra ni se refleja una contraseña, un
// secreto TOTP ni un código. La contrapartida no-JS (formularios de /account)
// vive en account_security.go y comparte este mismo núcleo.
package http

import (
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/qr"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/totp"
)

// errWrongCurrentPassword: la contraseña actual aportada no verifica (o la cuenta
// no tiene contraseña, p. ej. solo-SSO). Mensaje genérico al usuario.
var errWrongCurrentPassword = errors.New("id: contraseña actual incorrecta")

// changeUserPassword es el núcleo del cambio de contraseña, compartido por el
// endpoint JSON (/api/v1/me/password) y el formulario de /account. Verifica la
// contraseña actual en tiempo constante (VerifyPassword), aplica la política de
// dominio a la nueva, la re-hashea con el MISMO algoritmo (Argon2id) y —clave— en
// la MISMA transacción revoca sesiones y refresh tokens para invalidar cualquier
// credencial robada (modelo Okta). keepSessionID != "" conserva esa sesión (el
// dispositivo desde el que se cambia, caso navegador); "" revoca TODAS (caso API
// bearer, sin sesión de navegador asociada). Devuelve cuántas sesiones cerró.
func (s *Server) changeUserPassword(r *http.Request, userID, current, newPassword, keepSessionID string) (int, error) {
	ctx := r.Context()
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return 0, err
	}
	// Sin hash (cuenta solo-SSO/MFA) no hay "contraseña actual" que cambiar.
	if user.PasswordHash == "" {
		return 0, errWrongCurrentPassword
	}
	ok, err := domain.VerifyPassword(current, user.PasswordHash)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errWrongCurrentPassword
	}
	policy, err := s.store.EffectivePasswordPolicyForUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	if err := domain.ValidatePassword(policy, newPassword); err != nil {
		return 0, err
	}
	// La nueva debe diferir de la actual (higiene básica; ambas son entradas del
	// propio usuario, no hay canal de temporización que proteger).
	if newPassword == current {
		return 0, domain.ErrWeakPassword
	}
	hash, err := domain.HashPassword(newPassword)
	if err != nil {
		return 0, err
	}
	var revoked int
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		if e := s.store.SetPassword(ctx, q, userID, hash); e != nil {
			return e
		}
		var e error
		if keepSessionID != "" {
			revoked, e = s.store.RevokeOtherSessions(ctx, q, userID, keepSessionID)
		} else {
			revoked, e = s.store.RevokeAllUserSessions(ctx, q, userID)
		}
		return e
	})
	return revoked, err
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleMePassword — POST /api/v1/me/password — cambia la contraseña del usuario
// del access token. Revoca TODAS sus sesiones/refresh (el token bearer no lleva
// sesión de navegador que conservar); el access token vivo caduca solo y ya no
// se puede refrescar. Rate-limitado (ver Router).
func (s *Server) handleMePassword(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req changePasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	revoked, err := s.changeUserPassword(r, c.Subject, req.CurrentPassword, req.NewPassword, "")
	if err != nil {
		httpx.Fail(w, r, mapPasswordErr(err))
		if errors.Is(err, errWrongCurrentPassword) {
			_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.password.change_failed", "", r))
		}
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "user.password.changed", "", r))
	s.avisarSeguridad(r.Context(), c.Tenant, c.Subject, "security.password.changed", nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"sessions_revoked": revoked})
}

type adminSetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// handleAdminSetPassword — POST /api/v1/admin/users/{id}/password — un owner/admin
// fija una contraseña a un usuario de su organización (soporte / recuperación) y
// revoca sus sesiones. Deny-by-default: requireRole ya exige owner/admin, y
// adminMayManage añade las invariantes (no sobre uno mismo → usa /account; a un
// owner solo otro owner). El aislamiento por tenant lo garantiza adminTargetInOrg
// (el objetivo debe ser miembro de la org activa). La contraseña NO se audita.
func (s *Server) handleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, id, mem, apiErr := s.adminTargetInOrg(r)
	if apiErr != nil {
		httpx.Fail(w, r, apiErr)
		return
	}
	if e := adminMayManage(c.Subject, id, c.Roles, mem.Role); e != nil {
		httpx.Fail(w, r, e)
		return
	}
	var req adminSetPasswordRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	policy, err := s.store.EffectivePasswordPolicyForUser(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if err := domain.ValidatePassword(policy, req.NewPassword); err != nil {
		httpx.Fail(w, r, mapPasswordErr(err))
		return
	}
	hash, err := domain.HashPassword(req.NewPassword)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	var revoked int
	ctx := r.Context()
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		if e := s.store.SetPassword(ctx, q, id, hash); e != nil {
			return e
		}
		var e error
		revoked, e = s.store.RevokeAllUserSessions(ctx, q, id)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "user.password.reset", id, r))
	s.avisarSeguridad(r.Context(), org.ID, id, "security.password.changed", nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": id, "sessions_revoked": revoked})
}

// mfaIssuerLabel es el emisor visible del URI otpauth:// (nombre en la app
// autenticadora). Reusa el display name del Relying Party (WEBAUTHN_RP_DISPLAY_NAME).
func (s *Server) mfaIssuerLabel() string {
	if s.cfg.RPDisplayName != "" {
		return s.cfg.RPDisplayName
	}
	return "Sytue"
}

// handleMFATOTPEnroll — POST /api/v1/me/mfa/totp/enroll — genera un secreto TOTP
// (crypto/rand), lo guarda SIN confirmar y devuelve el otpauth:// URI + el secreto
// base32 para el QR / entrada manual. NO activa MFA todavía (eso lo hace confirm).
// Si ya hay un TOTP CONFIRMADO se rechaza (evita pisar el segundo factor vigente
// dejándolo sin confirmar, lo que bloquearía el login); para regenerar hay que
// desactivarlo antes. Un secreto pendiente anterior sí se sobreescribe (reinicio).
func (s *Server) handleMFATOTPEnroll(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if _, err := s.store.GetTOTPSecret(r.Context(), user.ID); err == nil {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("App verification is already on; turn it off before setting it up again."))
		return
	} else if !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.store.UpsertTOTPSecret(r.Context(), nil, user.ID, secret, false); err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, c.Tenant, "mfa.totp.enroll_begin", "", r))
	uri := totp.ProvisioningURI(s.mfaIssuerLabel(), user.Email, secret)
	out := map[string]any{"otpauth_uri": uri, "secret": totp.EncodeSecret(secret)}
	// El QR ya pintado, como en /account: el codificador vive aquí (platform/pkg/qr,
	// clean-room y con tests) y llevarlo al navegador sería reimplementarlo o
	// añadir una dependencia para algo que ya está resuelto. Si no cabe se OMITE y
	// no se falla: la clave y el URI en texto siguen ahí y bastan para enrolar.
	//
	// PNG y no SVG: varios gestores de TOTP dejan «subir una imagen del código» y
	// filtran el selector por PNG/JPEG, donde un SVG ni aparece. Va como data URI
	// para que el cliente lo ponga en un <img> tal cual.
	if b, e := qr.PNG(uri); e == nil {
		out["qr_png"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(b)
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

type totpConfirmRequest struct {
	Code string `json:"code"`
}

// handleMFATOTPConfirm — POST /api/v1/me/mfa/totp/confirm — valida un código
// contra el secreto PENDIENTE (misma validación que el login, totp.Validate) y,
// si es correcto, ACTIVA el TOTP (confirmed=true + mfa_enabled=true) en una tx.
func (s *Server) handleMFATOTPConfirm(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req totpConfirmRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	secret, err := s.store.GetPendingTOTPSecret(r.Context(), c.Subject)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("There is no verification setup in progress. Start again."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !totp.Validate(secret, req.Code, time.Now()) {
		_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "mfa.totp.confirm_failed", "", r))
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("That code is not correct. Check your device clock and try again."))
		return
	}
	codes, err := s.activateTOTP(r, c.Subject)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "mfa.totp.enabled", "", r))
	s.avisarSeguridad(r.Context(), c.Tenant, c.Subject, "security.mfa.enabled", nil)
	// Los códigos de recuperación se devuelven UNA vez (el llamante los muestra y no
	// vuelven a poder consultarse: solo se guarda su hash).
	httpx.JSON(w, r, http.StatusOK, map[string]any{"mfa_enabled": true, "recovery_codes": codes})
}

// activateTOTP marca confirmado el enrolamiento, eleva mfa_enabled y genera el lote
// de códigos de recuperación (de un solo uso), todo en una tx. Devuelve los códigos
// en claro para mostrarlos UNA sola vez (solo se persiste su hash). Compartido por
// el endpoint JSON y el formulario de /account.
func (s *Server) activateTOTP(r *http.Request, userID string) ([]string, error) {
	display, hashes, err := domain.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		if _, e := s.store.ConfirmTOTPSecret(ctx, q, userID); e != nil {
			return e
		}
		if e := s.store.SetMFAEnabled(ctx, q, userID, true); e != nil {
			return e
		}
		return s.store.InsertRecoveryCodes(ctx, q, userID, hashes)
	})
	if err != nil {
		return nil, err
	}
	return display, nil
}

// consumeRecoveryCode intenta gastar (un solo uso, atómico) un código de
// recuperación ya normalizado. Devuelve true si existía y se ha consumido. Se
// contrasta solo el hash (el código es de alta entropía: el timing del Index Scan
// no lo filtra), igual que los tokens de sesión/reseteo.
func (s *Server) consumeRecoveryCode(r *http.Request, userID, normalized string) bool {
	used, err := s.store.ConsumeRecoveryCode(r.Context(), nil, userID, domain.HashToken(normalized))
	return err == nil && used
}

// regenerateRecoveryCodes reemplaza los códigos de recuperación del usuario por un
// lote nuevo (invalidando los anteriores) y devuelve los nuevos en claro para
// mostrarlos una vez. El TOTP debe estar activo (lo garantiza el llamante).
func (s *Server) regenerateRecoveryCodes(r *http.Request, userID string) ([]string, error) {
	display, hashes, err := domain.NewRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.store.InsertRecoveryCodes(r.Context(), nil, userID, hashes); err != nil {
		return nil, err
	}
	return display, nil
}

// handleMFARecoveryRegenerate — POST /api/v1/me/mfa/totp/recovery-codes — regenera
// los códigos de recuperación (exige TOTP activo) y los devuelve una vez. Los
// anteriores dejan de servir.
func (s *Server) handleMFARecoveryRegenerate(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if _, err := s.store.GetTOTPSecret(r.Context(), c.Subject); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("App verification is not on."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	codes, err := s.regenerateRecoveryCodes(r, c.Subject)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, "mfa.totp.recovery_regenerated", "", r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"recovery_codes": codes})
}

type totpDisableRequest struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

// handleMFATOTPDisable — DELETE /api/v1/me/mfa/totp — desactiva el TOTP exigiendo
// una prueba de identidad: un código TOTP vigente O la contraseña de la cuenta
// (evita que una sesión secuestrada apague el segundo factor sin más). Solo baja
// mfa_enabled si NO quedan passkeys WebAuthn (que también elevan el login); si las
// hay, se conserva el step-up.
func (s *Server) handleMFATOTPDisable(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req totpDisableRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	secret, err := s.store.GetTOTPSecret(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrConflict.WithMessage("App verification is not on."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !s.verifyTOTPDisable(user, secret, req) {
		_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, c.Tenant, "mfa.totp.disable_failed", "", r))
		httpx.Fail(w, r, httpx.ErrUnauthorized.WithMessage("Your identity could not be verified."))
		return
	}
	if err := s.disableTOTP(r, user.ID); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(r.Context(), nil, auditEntry(user.ID, c.Tenant, "mfa.totp.disabled", "", r))
	s.avisarSeguridad(r.Context(), c.Tenant, user.ID, "security.mfa.disabled", nil)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"mfa_enabled": false})
}

// verifyTOTPDisable confirma la identidad para desactivar el TOTP: un código
// vigente (comparación en tiempo constante en totp.Validate) o la contraseña de
// la cuenta (VerifyPassword). Basta con uno.
func (s *Server) verifyTOTPDisable(user domain.User, secret []byte, req totpDisableRequest) bool {
	if req.Code != "" && totp.Validate(secret, req.Code, time.Now()) {
		return true
	}
	if req.Password != "" && user.PasswordHash != "" {
		ok, err := domain.VerifyPassword(req.Password, user.PasswordHash)
		return err == nil && ok
	}
	return false
}

// disableTOTP borra el secreto TOTP y baja mfa_enabled en una tx. Hoy el step-up
// del login SOLO valida TOTP (la ceremonia de login WebAuthn es follow-up): dejar
// mfa_enabled=true sin secreto TOTP dejaría al usuario sin poder superar el segundo
// paso (lockout), por eso se baja siempre.
// ponytail: cuando exista el login por passkey, recomputar mfa_enabled = "queda
// algún factor" (TOTP o WebAuthn) en vez de bajarlo incondicionalmente.
func (s *Server) disableTOTP(r *http.Request, userID string) error {
	ctx := r.Context()
	return s.store.InTx(ctx, func(q pg.Querier) error {
		if e := s.store.DeleteTOTPSecret(ctx, q, userID); e != nil {
			return e
		}
		// Los códigos de recuperación son un respaldo del segundo factor: sin TOTP no
		// tienen sentido y dejarlos sería un cabo suelto.
		if e := s.store.DeleteRecoveryCodes(ctx, q, userID); e != nil {
			return e
		}
		return s.store.SetMFAEnabled(ctx, q, userID, false)
	})
}

// passwordErrMsg da un mensaje HTML del error de política, distinguiendo el
// requisito incumplido sin filtrar la contraseña. Usado por los flujos HTML de
// invitación y reset.
func passwordErrMsg(err error) string {
	if errors.Is(err, domain.ErrPasswordComplexity) {
		return "The password must include letters and at least one number."
	}
	return "La contraseña no cumple la longitud mínima requerida."
}

// mapPasswordErr traduce los errores del cambio/reset de contraseña a HTTP. La
// contraseña actual incorrecta es 401 con mensaje genérico; la política, 400.
func mapPasswordErr(err error) error {
	switch {
	case errors.Is(err, errWrongCurrentPassword):
		return httpx.ErrUnauthorized.WithMessage("The current password is not correct.")
	case errors.Is(err, domain.ErrPasswordComplexity):
		return httpx.ErrBadRequest.WithMessage("The password must include letters and at least one number.")
	case errors.Is(err, domain.ErrWeakPassword):
		return httpx.ErrBadRequest.WithMessage("The password is shorter than the required minimum and must differ from the current one.")
	default:
		return mapDomainErr(err)
	}
}
