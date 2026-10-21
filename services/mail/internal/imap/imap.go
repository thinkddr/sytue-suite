// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package imap expone los buzones de Sytue Mail por IMAP (RFC 3501/9051) sobre
// la librería permisiva emersion/go-imap v2 (MIT), para clientes de correo de
// escritorio/móvil (Thunderbird, Apple Mail, etc.). La autenticación se delega
// en Sytue ID (dirección + token de acceso / contraseña de aplicación).
//
// Clean-room sobre los RFC de IMAP; los UID/UIDVALIDITY se sirven desde el
// modelo persistido en PostgreSQL (ver db/migrations). Los cuerpos se sirven
// streameando el .eml desde Object Storage S3.
package imap

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"

	imapserver "github.com/emersion/go-imap/v2/imapserver"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// Authenticator valida credenciales IMAP contra Sytue ID y devuelve la sesión.
type Authenticator interface {
	Authenticate(ctx context.Context, identity, secret string) (subject, tenant, address string, err error)
}

// Server envuelve el servidor IMAP de Sytue Mail.
type Server struct {
	srv      *imapserver.Server
	addr     string
	tlsConf  *tls.Config
	logger   *slog.Logger
	implicit bool
}

// Deps son las dependencias del servidor IMAP.
type Deps struct {
	Addr        string
	Hostname    string
	Store       *store.Store
	Blobs       *objectstore.Client
	Auth        Authenticator
	TLSConfig   *tls.Config
	ImplicitTLS bool // true para :993 (IMAPS), false para :143 + STARTTLS
	Logger      *slog.Logger
	// MaxAuthFailuresPerMinPerIP limita los fallos de LOGIN por IP y minuto. A 0
	// se desactiva. El 993 está expuesto a internet y hasta ahora aceptaba
	// intentos ilimitados sin dejar rastro en el log.
	MaxAuthFailuresPerMinPerIP int
}

// New construye el servidor IMAP con el backend de Sytue Mail.
func New(d Deps) *Server {
	be := &backend{
		store: d.Store, blobs: d.Blobs, auth: d.Auth, hostname: d.Hostname, logger: d.Logger,
		maxAuthFailPerMin: d.MaxAuthFailuresPerMinPerIP,
	}
	srv := imapserver.New(&imapserver.Options{
		NewSession:   be.newSession,
		Logger:       newSlogAdapter(d.Logger),
		TLSConfig:    d.TLSConfig,
		InsecureAuth: d.TLSConfig == nil, // dev: Traefik/local sin TLS
	})
	return &Server{srv: srv, addr: d.Addr, tlsConf: d.TLSConfig, logger: d.Logger, implicit: d.ImplicitTLS}
}

// ListenAndServe arranca el listener IMAP (TLS implícito en :993).
func (s *Server) ListenAndServe() error {
	var ln net.Listener
	var err error
	if s.implicit && s.tlsConf != nil {
		ln, err = tls.Listen("tcp", s.addr, s.tlsConf)
	} else {
		var lc net.ListenConfig
		ln, err = lc.Listen(context.Background(), "tcp", s.addr)
	}
	if err != nil {
		return fmt.Errorf("imap: escuchar en %s: %w", s.addr, err)
	}
	s.logger.Info("servidor IMAP escuchando", "addr", s.addr, "implicit_tls", s.implicit)
	return s.srv.Serve(ln)
}

// Close apaga el servidor IMAP.
func (s *Server) Close() error { return s.srv.Close() }
