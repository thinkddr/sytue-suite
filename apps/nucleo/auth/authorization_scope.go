// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"

	"git.sytue.eu/sytue/one/apps/nucleo/autorizacion"
)

// Los testigos de una autorización viven en `nucleo/autorizacion`, una hoja sin OIDC ni
// llavero, para que el cliente `api`, la caché `offline` y las pantallas compartidas no
// enlacen este paquete (Sytue Office no tiene cuenta). Aquí quedan sus nombres de
// siempre y lo que es de la sesión: emitirlos y dejar que se comprueben con su cerrojo.

type (
	// IdentitySnapshot is the session's existing non-secret identity, read under one
	// lock. It is not evidence that an arbitrary bearer token belongs to that identity.
	IdentitySnapshot = autorizacion.IdentitySnapshot
	// AuthorizationScope is an in-memory witness for one issued authorization. Its
	// revision is invalidated even by A→B→A; no token or header is retained in it.
	AuthorizationScope = autorizacion.AuthorizationScope
	// AuthorizationIntent binds a user's action before token acquisition. Unlike an
	// issued AuthorizationScope it contains no header, token or token fingerprint.
	// Its revision is read and updated only under its session's mutex.
	AuthorizationIntent = autorizacion.AuthorizationIntent
)

// ErrAuthorizationChanged rejects an operation begun in a different session.
var ErrAuthorizationChanged = autorizacion.ErrAuthorizationChanged

// La sesión es la autorizacion.Sesion de los testigos que emite.
var _ autorizacion.Sesion = (*Session)(nil)

func (s *Session) identityLocked() IdentitySnapshot {
	return IdentitySnapshot{Issuer: s.tokens.Issuer, Client: s.tokens.Cliente, Subject: s.tokens.Subject, Organization: organizacionDe(s.tokens)}
}

// SnapshotIdentity never mixes fields from two concurrent session adoptions.
func (s *Session) SnapshotIdentity() IdentitySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identityLocked()
}

// Comprobar (autorizacion.Sesion) ejecuta la comprobación de un testigo con el cerrojo
// de la sesión tomado y su revisión e identidad de ese instante.
func (s *Session) Comprobar(c autorizacion.Comprobacion) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.Con(s.revision, s.identityLocked())
}

// AccessTokenScoped obtains authorization and its witness in the same critical
// section, including renewal. AccessToken callers keep their existing API.
func (s *Session) AccessTokenScoped(ctx context.Context) (string, *AuthorizationScope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent := RequestAuthorizationIntent(ctx)
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if intent != nil && !intent.VigenteCon(s, s.revision, s.identityLocked()) {
		return "", nil, ErrAuthorizationChanged
	}
	antes := s.revision
	token, err := s.accessToken(ctx)
	if err != nil {
		return "", nil, err
	}
	if intent != nil {
		if intent.Identity() != s.identityLocked() {
			return "", nil, ErrAuthorizationChanged
		}
		// This acquisition may have renewed the same identity under this lock.
		// Other adoptions cannot run between the check and this update.
		if !intent.Renovada(antes, s.revision) {
			return "", nil, ErrAuthorizationChanged
		}
	}
	if token == "" {
		return token, nil, nil
	}
	return token, autorizacion.EmitirAmbito(s, s.revision, s.identityLocked(), "Bearer "+token), nil
}

// WithAuthorizationScope carries the witness across arbitrary HTTP wrappers.
func WithAuthorizationScope(ctx context.Context, scope *AuthorizationScope) context.Context {
	return autorizacion.WithAuthorizationScope(ctx, scope)
}

// RequestAuthorizationScope returns nil for legacy/fake sessions without a witness.
func RequestAuthorizationScope(ctx context.Context) *AuthorizationScope {
	return autorizacion.RequestAuthorizationScope(ctx)
}

// WithAuthorizationIntent captures this session now, before any asynchronous
// work. Reusing the resulting context cannot adopt a later account on a 401.
func (s *Session) WithAuthorizationIntent(ctx context.Context) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return autorizacion.EmitirIntencion(ctx, s, s.revision, s.identityLocked())
}

func RequestAuthorizationIntent(ctx context.Context) *AuthorizationIntent {
	return autorizacion.RequestAuthorizationIntent(ctx)
}
