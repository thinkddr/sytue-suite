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

	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// mxBackend es el backend del MX entrante (puerto 25): acepta correo de otros
// servidores hacia los dominios servidos. No autentica (correo entre MTA).
type mxBackend struct{ d Deps }

func (b *mxBackend) NewSession(c *gosmtp.Conn) (gosmtp.Session, error) {
	remoteIP := remoteIP(c)
	// Rate-limit anti-abuso por IP (ventana de minuto sobre Postgres).
	if b.d.Cfg.MaxInboundPerMinPerIP > 0 {
		n, err := b.d.Inbound.HitRate(context.Background(), remoteIP.String())
		if err == nil && n > b.d.Cfg.MaxInboundPerMinPerIP {
			b.d.Logger.Warn("mx: rate-limit excedido", "ip", remoteIP.String(), "count", n)
			return nil, &gosmtp.SMTPError{Code: 421, EnhancedCode: gosmtp.EnhancedCode{4, 7, 0}, Message: "Too many messages, slow down"}
		}
	}
	return &mxSession{d: b.d, remoteIP: remoteIP}, nil
}

// mxSession lleva el estado de una transacción SMTP entrante.
type mxSession struct {
	d        Deps
	remoteIP net.IP
	from     string
	rcpts    []string
}

func (s *mxSession) Mail(from string, _ *gosmtp.MailOptions) error {
	addr, err := domain.ParseAddress(from)
	if from != "" && err != nil {
		return &gosmtp.SMTPError{Code: 501, EnhancedCode: gosmtp.EnhancedCode{5, 1, 7}, Message: "Bad sender address"}
	}
	s.from = strings.ToLower(addr.String())
	if from == "" {
		s.from = "" // null sender (bounces)
	}
	return nil
}

func (s *mxSession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	addr, err := domain.ParseAddress(to)
	if err != nil {
		return &gosmtp.SMTPError{Code: 501, EnhancedCode: gosmtp.EnhancedCode{5, 1, 3}, Message: "Bad recipient address"}
	}
	// Solo aceptamos correo para direcciones locales conocidas (anti-relay).
	if _, err := s.d.Inbound.Resolve(context.Background(), addr.String()); err != nil {
		if errors.Is(err, domain.ErrNoSuchUser) {
			return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 1, 1}, Message: "No such user here"}
		}
		return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 3, 0}, Message: "Temporary lookup failure"}
	}
	if len(s.rcpts) >= s.d.Cfg.MaxRecipients {
		return &gosmtp.SMTPError{Code: 452, EnhancedCode: gosmtp.EnhancedCode{4, 5, 3}, Message: "Too many recipients"}
	}
	s.rcpts = append(s.rcpts, addr.String())
	return nil
}

func (s *mxSession) Data(r io.Reader) error {
	if len(s.rcpts) == 0 {
		return &gosmtp.SMTPError{Code: 554, EnhancedCode: gosmtp.EnhancedCode{5, 5, 1}, Message: "No valid recipients"}
	}
	ctx := context.Background()
	in := delivery.InboundMessage{
		RemoteIP:   s.remoteIP,
		MailFrom:   s.from,
		Recipients: s.rcpts,
		Body:       r,
	}
	if err := s.d.Inbound.Deliver(ctx, in); err != nil {
		switch {
		case errors.Is(err, domain.ErrMessageTooLarge):
			return &gosmtp.SMTPError{Code: 552, EnhancedCode: gosmtp.EnhancedCode{5, 3, 4}, Message: "Message too large"}
		case errors.Is(err, delivery.ErrRejectedPolicy):
			return &gosmtp.SMTPError{Code: 550, EnhancedCode: gosmtp.EnhancedCode{5, 7, 1}, Message: "Rejected by DMARC policy"}
		case errors.Is(err, domain.ErrQuotaExceeded):
			return &gosmtp.SMTPError{Code: 452, EnhancedCode: gosmtp.EnhancedCode{4, 2, 2}, Message: "Mailbox over quota"}
		default:
			s.d.Logger.Error("mx: entrega fallida", "error", err)
			return &gosmtp.SMTPError{Code: 451, EnhancedCode: gosmtp.EnhancedCode{4, 3, 0}, Message: "Temporary delivery failure"}
		}
	}
	return nil
}

func (s *mxSession) Reset() { s.from = ""; s.rcpts = nil }

func (s *mxSession) Logout() error { return nil }

// AuthMechanisms: el MX entrante no ofrece AUTH (no debe ser un relay abierto
// ni un punto de fuerza bruta de credenciales).
func (s *mxSession) AuthMechanisms() []string { return nil }

func (s *mxSession) Auth(string) (sasl.Server, error) {
	return nil, gosmtp.ErrAuthUnsupported
}

func remoteIP(c *gosmtp.Conn) net.IP {
	if c == nil {
		return nil
	}
	host, _, err := net.SplitHostPort(c.Conn().RemoteAddr().String())
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}
