// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Correo de recuperación (ADR 0041): opcional, se pone en «Mi cuenta» y es por
// donde vuelve la contraseña a una cuenta cuyo correo de acceso es su propio
// buzón de Sytue. Además es uno de los cuatro identificadores de acceso.
//
// Cambiarlo pide la contraseña actual, igual que cambiar la contraseña: con una
// sesión robada, poner un correo propio como recuperación y pedir después el
// reseteo es quedarse la cuenta. Y no se fija hasta que se confirma el enlace
// que llega a esa dirección (email_verification.go, propósito «recovery»).
package http

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

type recoveryEmailRequest struct {
	// Email vacío retira el correo de recuperación.
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleMeRecoveryEmail — PUT /api/v1/me/recovery-email — manda el enlace de
// confirmación a la dirección nueva (202, aún no fijada) o, con `email` vacío,
// retira la actual (200).
func (s *Server) handleMeRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	c, ok := claimsFrom(r.Context())
	if !ok {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	var req recoveryEmailRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	user, err := s.store.GetUserByID(ctx, c.Subject)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// Una cuenta sin contraseña (solo SSO) no tiene nada que recuperar por correo.
	if user.PasswordHash == "" {
		httpx.Fail(w, r, mapPasswordErr(errWrongCurrentPassword))
		return
	}
	if ok, err := domain.VerifyPassword(req.Password, user.PasswordHash); err != nil || !ok {
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, c.Tenant, "user.recovery_email.change_failed", "", r))
		httpx.Fail(w, r, mapPasswordErr(errWrongCurrentPassword))
		return
	}

	if strings.TrimSpace(req.Email) == "" {
		if err := s.store.SetRecoveryEmail(ctx, nil, user.ID, ""); err != nil {
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, c.Tenant, "user.recovery_email.removed", "", r))
		httpx.JSON(w, r, http.StatusOK, map[string]any{"recovery_email": ""})
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	if err != nil || addr.Name != "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The recovery email is not a valid address."))
		return
	}
	email := strings.ToLower(addr.Address)
	// La de recuperación es para cuando no se puede entrar en Sytue: una dirección
	// del dominio de la suite, o el propio correo de acceso, no recupera nada.
	if strings.EqualFold(email, user.Email) || strings.HasSuffix(email, "@"+strings.ToLower(suiteDomain(s.cfg.Issuer))) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The recovery email must be outside Sytue."))
		return
	}
	if err := s.sendEmailLink(ctx, user.ID, email, store.VerifyRecovery); err != nil {
		s.log.ErrorContext(ctx, "enviar la confirmación del correo de recuperación falló", "error", err)
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage("The confirmation email could not be sent."))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, c.Tenant, "user.recovery_email.verification_sent", "", r))
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"pending": email})
}

// fijarCorreoDeRecuperacion pone el correo ya confirmado. ErrConflict si esa
// dirección ya da acceso a OTRA cuenta: como buzón o correo de acceso (que
// ganan en el login, ver userByLogin) o como recuperación de otra persona.
func (s *Server) fijarCorreoDeRecuperacion(ctx context.Context, q pg.Querier, userID, email string) error {
	otro, err := s.store.GetUserByAddress(ctx, email)
	switch {
	case err == nil && otro.ID != userID:
		return domain.ErrConflict
	case err != nil && !errors.Is(err, domain.ErrNotFound):
		return err
	}
	return s.store.SetRecoveryEmail(ctx, q, userID, email)
}
