// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"context"
	"crypto/sha256"
	"errors"
)

// IdentitySnapshot is the session's existing non-secret identity, read under one
// lock. It is not evidence that an arbitrary bearer token belongs to that identity.
type IdentitySnapshot struct {
	Issuer, Client, Subject, Organization string
}

func (s *Session) identityLocked() IdentitySnapshot {
	return IdentitySnapshot{s.tokens.Issuer, s.tokens.Cliente, s.tokens.Subject, organizacionDe(s.tokens)}
}

// SnapshotIdentity never mixes fields from two concurrent session adoptions.
func (s *Session) SnapshotIdentity() IdentitySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identityLocked()
}

// AuthorizationScope is an in-memory witness for one issued authorization. Its
// revision is invalidated even by A→B→A; no token or header is retained in it.
type AuthorizationScope struct {
	session  *Session
	revision uint64
	identity IdentitySnapshot
	header   [sha256.Size]byte
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
	if intent != nil && !intent.currentLocked(s) {
		return "", nil, ErrAuthorizationChanged
	}
	token, err := s.accessToken(ctx)
	if err != nil {
		return "", nil, err
	}
	if intent != nil {
		if intent.identity != s.identityLocked() {
			return "", nil, ErrAuthorizationChanged
		}
		// This acquisition may have renewed the same identity under this lock.
		// Other adoptions cannot run between the check and this update.
		intent.revision = s.revision
	}
	if token == "" {
		return token, nil, nil
	}
	return token, &AuthorizationScope{s, s.revision, s.identityLocked(), sha256.Sum256([]byte("Bearer " + token))}, nil
}

// Current checks the actual header and the session revision, without deriving
// identity from JWT claims or exporting the authorization to persistent storage.
func (s *AuthorizationScope) Current(header string) bool {
	if s == nil || s.session == nil || sha256.Sum256([]byte(header)) != s.header {
		return false
	}
	s.session.mu.Lock()
	defer s.session.mu.Unlock()
	return s.revision == s.session.revision && s.identity == s.session.identityLocked()
}

type authorizationScopeKey struct{}

// WithAuthorizationScope carries the witness across arbitrary HTTP wrappers.
func WithAuthorizationScope(ctx context.Context, scope *AuthorizationScope) context.Context {
	return context.WithValue(ctx, authorizationScopeKey{}, scope)
}

// RequestAuthorizationScope returns nil for legacy/fake sessions without a witness.
func RequestAuthorizationScope(ctx context.Context) *AuthorizationScope {
	scope, _ := ctx.Value(authorizationScopeKey{}).(*AuthorizationScope)
	return scope
}

// ErrAuthorizationChanged rejects an operation begun in a different session.
var ErrAuthorizationChanged = errors.New("auth: authorization identity changed")

// AuthorizationIntent binds a user's action before token acquisition. Unlike an
// issued AuthorizationScope it contains no header, token or token fingerprint.
// Its revision is read and updated only under its session's mutex.
type AuthorizationIntent struct {
	session  *Session
	revision uint64
	identity IdentitySnapshot
}

type authorizationIntentKey struct{}

// WithAuthorizationIntent captures this session now, before any asynchronous
// work. Reusing the resulting context cannot adopt a later account on a 401.
func (s *Session) WithAuthorizationIntent(ctx context.Context) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent := &AuthorizationIntent{s, s.revision, s.identityLocked()}
	return context.WithValue(ctx, authorizationIntentKey{}, intent)
}

func RequestAuthorizationIntent(ctx context.Context) *AuthorizationIntent {
	intent, _ := ctx.Value(authorizationIntentKey{}).(*AuthorizationIntent)
	return intent
}

func (i *AuthorizationIntent) currentLocked(s *Session) bool {
	return i != nil && i.session == s && i.revision == s.revision &&
		i.identity.Subject != "" && i.identity == s.identityLocked()
}

// Current excludes appearance/presence changes, and rejects adoption including ABA.
func (i *AuthorizationIntent) Current() bool {
	if i == nil || i.session == nil {
		return false
	}
	i.session.mu.Lock()
	defer i.session.mu.Unlock()
	return i.currentLocked(i.session)
}

// Allows requires the issued authorization and the earlier intent to belong to
// the same Session instance and identity, including through SDK wrappers.
func (i *AuthorizationIntent) Allows(scope *AuthorizationScope, header string) bool {
	return i != nil && scope != nil && i.session == scope.session && i.identity == scope.identity &&
		i.Current() && scope.Current(header)
}

// Identity is the non-secret identity captured with this intent, never a new snapshot.
func (i *AuthorizationIntent) Identity() IdentitySnapshot {
	if i == nil {
		return IdentitySnapshot{}
	}
	return i.identity
}
