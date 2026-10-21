// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package smtp implementa los servidores SMTP de Sytue Mail sobre la librería
// permisiva emersion/go-smtp (MIT):
//
//   - MX entrante (puerto 25): recibe correo de otros MTA hacia los dominios
//     servidos. Verifica SPF/DKIM/DMARC, aplica rate-limit anti-abuso, parsea el
//     MIME, sube el .eml a S3 y guarda el sobre en PostgreSQL (entrega al Inbox).
//   - Submission/MSA (puertos 587 STARTTLS y 465 TLS implícito): clientes
//     autenticados (vía Sytue ID) envían correo saliente. Firma DKIM y encola en
//     PostgreSQL para entrega diferida con reintentos.
//
// Clean-room sobre RFC 5321 (SMTP), RFC 6409 (Submission), RFC 3207 (STARTTLS).
package smtp

import (
	"context"
	"crypto/tls"
	"log/slog"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
)

// TokenVerifier valida credenciales de submission. En Sytue, la autenticación
// SMTP AUTH se delega en Sytue ID: el usuario presenta su dirección y un token
// de acceso OAuth2 (o contraseña de aplicación) que se valida vía OIDC/JWKS.
type TokenVerifier interface {
	// Authenticate valida (identity, secret) y devuelve el subject y tenant.
	Authenticate(ctx context.Context, identity, secret string) (subject, tenant, address string, err error)
}

// Servers agrupa los listeners SMTP del servicio para su arranque/apagado.
type Servers struct {
	MX          *gosmtp.Server // :25 entrante
	Submission  *gosmtp.Server // :587 STARTTLS
	Submissions *gosmtp.Server // :465 TLS implícito
}

// Deps son las dependencias de los backends SMTP.
type Deps struct {
	Cfg       config.Config
	Logger    *slog.Logger
	Inbound   *delivery.Inbound  // entrega de correo entrante (MX)
	Outbound  *delivery.Outbound // encolado de correo saliente (submission)
	Verifier  TokenVerifier
	TLSConfig *tls.Config // STARTTLS / TLS implícito (nil en dev: Traefik termina)
}

// NewServers construye los tres servidores SMTP con timeouts/limites de hardening.
func NewServers(d Deps) *Servers {
	common := func(s *gosmtp.Server, addr string) *gosmtp.Server {
		s.Addr = addr
		s.Domain = d.Cfg.Hostname
		s.ReadTimeout = d.Cfg.SMTPReadTimeout
		s.WriteTimeout = d.Cfg.SMTPWriteTimeout
		s.MaxMessageBytes = d.Cfg.MaxMessageBytes
		s.MaxRecipients = d.Cfg.MaxRecipients
		s.MaxLineLength = 2000
		s.AllowInsecureAuth = !d.Cfg.Env.IsProd() // dev: Traefik/local sin TLS
		s.TLSConfig = d.TLSConfig
		return s
	}

	mx := common(gosmtp.NewServer(&mxBackend{d: d}), d.Cfg.SMTPMXAddr)
	mx.AllowInsecureAuth = true // el MX entrante NO autentica (recibe de otros MTA)

	sub := common(gosmtp.NewServer(&submissionBackend{d: d}), d.Cfg.SMTPSubmissionAddr)

	subs := common(gosmtp.NewServer(&submissionBackend{d: d}), d.Cfg.SMTPSImplicitAddr)

	return &Servers{MX: mx, Submission: sub, Submissions: subs}
}

// ListenAndServe arranca un servidor; los listeners de TLS implícito (465/993)
// usan ListenAndServeTLS. Cada uno se lanza en su propia goroutine desde main.
func ListenAndServe(s *gosmtp.Server, implicitTLS bool) error {
	if implicitTLS && s.TLSConfig != nil {
		return s.ListenAndServeTLS()
	}
	return s.ListenAndServe()
}

// Shutdown apaga todos los servidores con un plazo.
func (s *Servers) Shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for _, srv := range []*gosmtp.Server{s.MX, s.Submission, s.Submissions} {
		if srv != nil {
			_ = srv.Shutdown(ctx)
		}
	}
}
