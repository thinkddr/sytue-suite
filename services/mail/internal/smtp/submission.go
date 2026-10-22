// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smtp

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// submissionBackend es el backend del MSA (587/465): clientes AUTENTICADOS
// (vía Sytue ID) envían correo saliente. Exige AUTH antes de MAIL FROM.
type submissionBackend struct{ d Deps }

func (b *submissionBackend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	return &submissionSession{d: b.d, remoteIP: remoteIP(c)}, nil
}

type submissionSession struct {
	d             Deps
	remoteIP      net.IP
	authenticated bool
	subject       string
	tenant        string
	address       string // dirección autenticada (envelope-from forzado)
	from          string
	rcpts         []string
}

// AuthMechanisms anuncia PLAIN (sobre TLS): el cliente envía dirección + token
// de Sytue ID (o contraseña de aplicación) que validamos contra el IdP.
func (s *submissionSession) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *submissionSession) Auth(mech string) (sasl.Server, error) {
	if mech != sasl.Plain {
		return nil, gosmtp.ErrAuthUnsupported
	}
	return sasl.NewPlainServer(func(identity, username, password string) error {
		ip := ""
		if s.remoteIP != nil {
			ip = s.remoteIP.String()
		}
		lim := s.d.Cfg.MaxAuthFailuresPerMinPerIP
		// Antes de molestar al IdP: si esta IP ya agotó sus fallos en el minuto en
		// curso, se corta aquí. 454 y no 535 a propósito: es un fallo TEMPORAL, y
		// eso le dice al cliente legítimo que reintente luego, en vez de empujar a
		// su usuario a rehacer la cuenta.
		if lim > 0 && ip != "" {
			if n, rerr := s.d.Inbound.AuthFailRate(context.Background(), ip); rerr == nil && n >= lim {
				s.d.Logger.Warn("submission: demasiados fallos de autenticación", "ip", ip, "count", n, "limite", lim)
				return &gosmtp.SMTPError{Code: 454, EnhancedCode: gosmtp.EnhancedCode{4, 7, 0}, Message: "Too many failed authentication attempts, try again later"}
			}
		}
		subject, tenant, address, err := s.d.Verifier.Authenticate(context.Background(), username, password)
		if err != nil {
			// Contar Y REGISTRAR. Hasta ahora un ataque de diccionario contra los
			// buzones no dejaba ni una línea en el log: no solo no se frenaba, es
			// que no había forma de enterarse de que había ocurrido.
			if lim > 0 && ip != "" {
				n, _ := s.d.Inbound.HitAuthFail(context.Background(), ip)
				s.d.Logger.Warn("submission: autenticación fallida", "ip", ip, "count", n, "limite", lim)
			}
			return gosmtp.ErrAuthFailed
		}
		s.authenticated = true
		s.subject, s.tenant, s.address = subject, tenant, strings.ToLower(address)
		return nil
	}), nil
}

func (s *submissionSession) Mail(from string, _ *gosmtp.MailOptions) error {
	if !s.authenticated {
		return &gosmtp.SMTPError{Code: 530, EnhancedCode: gosmtp.EnhancedCode{5, 7, 0}, Message: "Authentication required"}
	}
	addr, err := domain.ParseAddress(from)
	if err != nil {
		return &gosmtp.SMTPError{Code: 501, EnhancedCode: gosmtp.EnhancedCode{5, 1, 7}, Message: "Bad sender address"}
	}
	// El MAIL FROM debe coincidir con la identidad autenticada (anti-spoofing).
	if !strings.EqualFold(addr.String(), s.address) {
		return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "Sender does not match authenticated user"}
	}
	s.from = addr.String()
	return nil
}

func (s *submissionSession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	if !s.authenticated {
		return &gosmtp.SMTPError{Code: 530, EnhancedCode: gosmtp.EnhancedCode{5, 7, 0}, Message: "Authentication required"}
	}
	addr, err := domain.ParseAddress(to)
	if err != nil {
		return &gosmtp.SMTPError{Code: 501, EnhancedCode: gosmtp.EnhancedCode{5, 1, 3}, Message: "Bad recipient address"}
	}
	if len(s.rcpts) >= s.d.Cfg.MaxRecipients {
		return &gosmtp.SMTPError{Code: 452, EnhancedCode: gosmtp.EnhancedCode{4, 5, 3}, Message: "Too many recipients"}
	}
	s.rcpts = append(s.rcpts, addr.String())
	return nil
}

func (s *submissionSession) Data(r io.Reader) error {
	if !s.authenticated {
		return &gosmtp.SMTPError{Code: 530, Message: "Authentication required"}
	}
	if len(s.rcpts) == 0 {
		return &gosmtp.SMTPError{Code: 554, EnhancedCode: gosmtp.EnhancedCode{5, 5, 1}, Message: "No recipients"}
	}
	// Fijar tenant en el contexto para que el encolado respete RLS.
	ctx := context.Background()
	if t, err := tenancy.New(s.tenant); err == nil {
		ctx = tenancy.Into(ctx, t)
	}
	sub := delivery.SubmitMessage{
		Subject:    s.subject,
		From:       s.from,
		Recipients: s.rcpts,
		Body:       r,
	}
	if err := s.d.Outbound.Submit(ctx, sub); err != nil {
		switch {
		case errors.Is(err, domain.ErrMessageTooLarge):
			return &gosmtp.SMTPError{Code: 552, EnhancedCode: gosmtp.EnhancedCode{5, 3, 4}, Message: "Message too large"}
		case errors.Is(err, domain.ErrFromNoAutorizado):
			// Permanente: reintentar el mismo mensaje no cambia de quién dice ser.
			return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "From header must be exactly one address of the authenticated account"}
		default:
			s.d.Logger.Error("submission: encolar fallido", "error", err)
			return &gosmtp.SMTPError{Code: 451, Message: "Could not queue message"}
		}
	}
	return nil
}

func (s *submissionSession) Reset() { s.from = ""; s.rcpts = nil }

func (s *submissionSession) Logout() error { return nil }
