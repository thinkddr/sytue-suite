// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"context"
	"errors"
	"net/http"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/log"
)

// Server envuelve un *http.Server con arranque y apagado graceful estándar.
type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
}

// ServerConfig configura el servidor HTTP.
type ServerConfig struct {
	Addr            string
	Handler         http.Handler
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// SinTimeout apaga un timeout de ServerConfig. Hace falta un valor aparte porque
// 0 significa «el valor por defecto»: quien escribía `WriteTimeout: 0` creyendo
// quitar el límite se quedaba con los 30 s de abajo, que cortan una subida o una
// descarga grande a medias.
const SinTimeout time.Duration = -1

// NewServer construye un Server con timeouts seguros por defecto. Un timeout a
// SinTimeout (o cualquier negativo) queda sin límite: net/http trata ≤ 0 así.
func NewServer(cfg ServerConfig) *Server {
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 15 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 60 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 15 * time.Second
	}
	return &Server{
		httpServer: &http.Server{
			Addr:              cfg.Addr,
			Handler:           cfg.Handler,
			ReadTimeout:       cfg.ReadTimeout,
			ReadHeaderTimeout: 10 * time.Second,
			WriteTimeout:      cfg.WriteTimeout,
			IdleTimeout:       cfg.IdleTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

// Run arranca el servidor y bloquea hasta que el contexto se cancele (señal de
// apagado), momento en el que ejecuta un apagado graceful. Devuelve nil en un
// apagado limpio.
//
//nolint:contextcheck // el drenaje usa un context.Background con plazo propio A PROPÓSITO: el ctx padre ya está cancelado.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		log.From(ctx).Info("servidor HTTP escuchando", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.From(ctx).Info("apagando servidor HTTP", "timeout", s.shutdownTimeout.String())
		// El ctx padre ya está cancelado: el drenaje necesita un contexto NUEVO
		// con su propio plazo, no uno heredado (moriría al instante).
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
