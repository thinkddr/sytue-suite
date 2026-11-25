// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Cambiar el @usuario y el móvil desde Mi cuenta (ADR 0040 y 0041, mig 0126).
//
// Las dos piden la contraseña actual, como el correo de recuperación: son
// identificadores de acceso, y con una sesión robada cambiarlos sería empezar a
// quedarse la cuenta. El móvil, además, no se fija hasta confirmar el código que llega
// por SMS al número nuevo; es también como las cuentas de antes de la 0066 añaden uno.
// Y como en el alta, que ese número ya es de otra cuenta solo se dice después del
// código, a quien ha demostrado que el número es suyo.
package http

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/i18n"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// Errores con código propio: la tarjeta de Mi cuenta los traduce por el código.
var (
	errUsernameInvalido  = httpx.NewError(http.StatusBadRequest, "username_invalid", "The username is not valid.")
	errUsernameOcupado   = httpx.NewError(http.StatusConflict, "username_taken", "That username is not available.")
	errUsernameReciente  = httpx.NewError(http.StatusTooManyRequests, "username_recent", "The username was changed less than 30 days ago.")
	errUsernameDireccion = httpx.NewError(http.StatusConflict, "username_is_address", "This username is your Sytue address and cannot be changed here.")
	errMovilInvalido     = httpx.NewError(http.StatusBadRequest, "phone_invalid", "The mobile number is not valid.")
	errMovilIgual        = httpx.NewError(http.StatusConflict, "phone_same", "That is already your mobile number.")
	errSMSLimite         = httpx.NewError(http.StatusTooManyRequests, "sms_limit", "Too many codes requested. Try again later.")
	errCodigoEspera      = httpx.NewError(http.StatusTooManyRequests, "code_wait", "Wait a minute before asking for another code.")
	errSMSNoDisponible   = httpx.NewError(http.StatusServiceUnavailable, "sms_unavailable", "Text messages are not available right now.")
	errSMSFallido        = httpx.NewError(http.StatusBadGateway, "sms_failed", "The text message could not be sent.")
	errCodigoCaducado    = httpx.NewError(http.StatusGone, "code_expired", "The code has expired. Ask for a new one.")
	errCodigoMal         = httpx.NewError(http.StatusBadRequest, "code_wrong", "The code is not correct.")
	errMovilConCuenta    = httpx.NewError(http.StatusConflict, "phone_has_account", "That mobile number already belongs to another account.")
)

// usuarioConContrasena lee a quien llama y comprueba su contraseña actual. Una cuenta
// sin contraseña (solo SSO) no cambia identificadores de acceso desde aquí.
func (s *Server) usuarioConContrasena(w http.ResponseWriter, r *http.Request, password, fallo string) (domain.User, bool) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return domain.User{}, false
	}
	user, err := s.store.GetUserByID(r.Context(), c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return domain.User{}, false
	}
	if user.PasswordHash == "" {
		httpx.Fail(w, r, mapPasswordErr(errWrongCurrentPassword))
		return domain.User{}, false
	}
	if ok, err := domain.VerifyPassword(password, user.PasswordHash); err != nil || !ok {
		_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, c.Tenant, fallo, "", r))
		httpx.Fail(w, r, mapPasswordErr(errWrongCurrentPassword))
		return domain.User{}, false
	}
	return user, true
}

// esSuDireccion: el @usuario es la parte local de SU dirección del dominio raíz (la
// de una cuenta personal, ana@sytue.eu). Cambiarlo sería cambiar de dirección, y eso
// no se hace desde aquí.
func (s *Server) esSuDireccion(ctx context.Context, u domain.User) bool {
	if u.Username == "" {
		return false
	}
	dir := u.Username + "@" + strings.ToLower(suiteDomain(s.cfg.Issuer))
	if strings.EqualFold(u.Email, dir) {
		return true
	}
	otro, err := s.store.GetUserByAddress(ctx, dir)
	return err == nil && otro.ID == u.ID
}

type cambioUsernameRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// handleMeUsername — PUT /api/v1/me/username — cambia el @usuario. El anterior queda
// reservado para la misma persona 30 días, y no se puede volver a cambiar antes.
func (s *Server) handleMeUsername(w http.ResponseWriter, r *http.Request) {
	var req cambioUsernameRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	nuevo := domain.NormalizeUsername(req.Username)
	if !domain.ValidUsername(nuevo) {
		httpx.Fail(w, r, errUsernameInvalido)
		return
	}
	user, ok := s.usuarioConContrasena(w, r, req.Password, "user.username.change_failed")
	if !ok {
		return
	}
	ctx := r.Context()
	if strings.EqualFold(nuevo, user.Username) {
		httpx.JSON(w, r, http.StatusOK, map[string]any{"username": user.Username})
		return
	}
	if s.esSuDireccion(ctx, user) {
		httpx.Fail(w, r, errUsernameDireccion)
		return
	}
	// Reservado se dice igual que ocupado, como en el alta.
	if domain.UsernameReservado(nuevo) {
		httpx.Fail(w, r, errUsernameOcupado)
		return
	}
	libre, err := s.store.UsernameLibrePara(ctx, user.ID, nuevo)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !libre {
		httpx.Fail(w, r, errUsernameOcupado)
		return
	}
	anterior, err := s.store.CambiarUsername(ctx, user.ID, nuevo, time.Now())
	switch {
	case errors.Is(err, store.ErrCambioDeUsernameReciente):
		httpx.Fail(w, r, errUsernameReciente)
		return
	case errors.Is(err, domain.ErrConflict):
		httpx.Fail(w, r, errUsernameOcupado)
		return
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	c, _ := claimsFrom(ctx)
	_ = s.store.Audit(ctx, nil, auditEntry(user.ID, c.Tenant, "user.username.changed", anterior+" → "+nuevo, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"username": nuevo})
}

type cambioMovilRequest struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

// handleMePhone — PUT /api/v1/me/phone — manda por SMS el código al móvil nuevo (202).
// No lo fija: eso es POST /api/v1/me/phone/verify con el código.
func (s *Server) handleMePhone(w http.ResponseWriter, r *http.Request) {
	var req cambioMovilRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	phone, ok := domain.NormalizePhone(strings.TrimSpace(req.Phone))
	if !ok {
		httpx.Fail(w, r, errMovilInvalido)
		return
	}
	if s.sms == nil {
		httpx.Fail(w, r, errSMSNoDisponible)
		return
	}
	user, ok := s.usuarioConContrasena(w, r, req.Password, "user.phone.change_failed")
	if !ok {
		return
	}
	if phone == user.Phone {
		httpx.Fail(w, r, errMovilIgual)
		return
	}
	ctx := r.Context()
	if !s.smsPermitido(r, phone) {
		httpx.Fail(w, r, errSMSLimite)
		return
	}
	code, err := domain.NewSignupCode()
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.store.PedirCambioDeMovil(ctx, user.ID, phone, domain.HashSignupCode(user.ID, code), time.Now()); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			httpx.Fail(w, r, errCodigoEspera)
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.sms.Send(ctx, phone, s.textoSMS(i18n.From(r), code)); err != nil {
		s.log.ErrorContext(ctx, "enviar el código del móvil nuevo por SMS falló", "error", err)
		_ = s.store.OlvidarCambioDeMovil(ctx, user.ID)
		httpx.Fail(w, r, errSMSFallido)
		return
	}
	c, _ := claimsFrom(ctx)
	_ = s.store.Audit(ctx, nil, auditEntry(user.ID, c.Tenant, "user.phone.code_sent", "", r))
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"pending": phone})
}

type verificarMovilRequest struct {
	Code string `json:"code"`
}

// handleMePhoneVerify — POST /api/v1/me/phone/verify — confirma el código y fija el
// móvil nuevo. Cinco intentos: después, hay que pedir otro código.
func (s *Server) handleMePhoneVerify(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req verificarMovilRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	cambio, err := s.store.ProbarCambioDeMovil(ctx, c.Subject, time.Now())
	if errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, errCodigoCaducado)
		return
	}
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	code := strings.TrimSpace(req.Code)
	if subtle.ConstantTimeCompare([]byte(domain.HashSignupCode(c.Subject, code)), []byte(cambio.CodeHash)) != 1 {
		if cambio.Attempts >= domain.SignupCodeMaxAttempts {
			_ = s.store.OlvidarCambioDeMovil(ctx, c.Subject)
			httpx.Fail(w, r, errCodigoCaducado)
			return
		}
		httpx.Fail(w, r, errCodigoMal)
		return
	}
	_ = s.store.OlvidarCambioDeMovil(ctx, c.Subject)
	if err := s.store.SetPhone(ctx, c.Subject, cambio.Phone); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			httpx.Fail(w, r, errMovilConCuenta)
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, c.Tenant, "user.phone.changed", "", r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"phone": cambio.Phone})
}
