// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package autorizacion

import (
	"context"
	"crypto/sha256"
)

// Sesion es la sesión que emite los testigos de este paquete: en la suite,
// `*auth.Session`. Un testigo guarda la sesión, la revisión y la identidad con que se
// emitió, y comprobarlo es compararlas con las de la sesión BAJO SU CERROJO, el mismo con
// que renueva o adopta otra cuenta. Tiene que ser comparable (un puntero): un testigo
// solo vale para la sesión que lo emitió.
type Sesion interface {
	// Comprobar ejecuta c (c.Con) con el cerrojo de la sesión tomado y con su revisión y
	// su identidad de ese instante.
	Comprobar(c Comprobacion)
}

// Comprobacion es lo que un testigo mira de su sesión bajo el cerrojo de esta. Solo la
// construye este paquete: quien implementa Sesion la ejecuta, no la escribe, así que
// nadie de fuera corre código suyo con el cerrojo de la sesión tomado.
type Comprobacion struct {
	f func(revision uint64, identidad IdentitySnapshot)
}

// Con ejecuta la comprobación. La llama Sesion.Comprobar, con el cerrojo tomado; la
// Comprobacion vacía no hace nada.
func (c Comprobacion) Con(revision uint64, identidad IdentitySnapshot) {
	if c.f != nil {
		c.f(revision, identidad)
	}
}

// AuthorizationScope is an in-memory witness for one issued authorization. Its
// revision is invalidated even by A→B→A; no token or header is retained in it.
type AuthorizationScope struct {
	session  Sesion
	revision uint64
	identity IdentitySnapshot
	header   [sha256.Size]byte
}

// EmitirAmbito es el testigo de la autorización que s acaba de dar con la cabecera
// `header` (`Bearer <token>`), en su revisión e identidad de ese instante. Lo llama la
// sesión con su cerrojo tomado (auth.Session.AccessTokenScoped); del token solo queda la
// huella de la cabecera.
func EmitirAmbito(s Sesion, revision uint64, identidad IdentitySnapshot, header string) *AuthorizationScope {
	return &AuthorizationScope{s, revision, identidad, sha256.Sum256([]byte(header))}
}

// Current checks the actual header and the session revision, without deriving
// identity from JWT claims or exporting the authorization to persistent storage.
func (s *AuthorizationScope) Current(header string) bool {
	if s == nil || s.session == nil || sha256.Sum256([]byte(header)) != s.header {
		return false
	}
	vigente := false
	s.session.Comprobar(Comprobacion{func(revision uint64, identidad IdentitySnapshot) {
		vigente = s.revision == revision && s.identity == identidad
	}})
	return vigente
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

// AuthorizationIntent binds a user's action before token acquisition. Unlike an
// issued AuthorizationScope it contains no header, token or token fingerprint.
// Its revision is read and updated only under its session's mutex.
type AuthorizationIntent struct {
	session  Sesion
	revision uint64
	identity IdentitySnapshot
}

type authorizationIntentKey struct{}

// EmitirIntencion es ctx con la intención de una acción sobre s, en su revisión e
// identidad de ese instante. La emite la sesión con su cerrojo tomado
// (auth.Session.WithAuthorizationIntent, que es lo que llaman las pantallas).
func EmitirIntencion(ctx context.Context, s Sesion, revision uint64, identidad IdentitySnapshot) context.Context {
	intent := &AuthorizationIntent{s, revision, identidad}
	return context.WithValue(ctx, authorizationIntentKey{}, intent)
}

// WithoutAuthorizationIntent removes only the short action intent. Native
// application lifetimes call it AFTER capturing a persisted work witness and
// binding their HTTP session to that identity and witness. It grants no authority
// itself: every request must still acquire and validate its AuthorizationScope.
// Cancellation, deadlines, ScopeProof and unrelated values remain inherited.
func WithoutAuthorizationIntent(ctx context.Context) context.Context {
	return context.WithValue(ctx, authorizationIntentKey{}, (*AuthorizationIntent)(nil))
}

func RequestAuthorizationIntent(ctx context.Context) *AuthorizationIntent {
	intent, _ := ctx.Value(authorizationIntentKey{}).(*AuthorizationIntent)
	return intent
}

// VigenteCon es Current con el cerrojo de s YA tomado y su revisión e identidad de ese
// instante: la intención es de s, de esa revisión y de esa identidad, que es alguien.
// Lo llama la sesión (auth.Session.AccessTokenScoped).
func (i *AuthorizationIntent) VigenteCon(s Sesion, revision uint64, identidad IdentitySnapshot) bool {
	return i != nil && i.session == s && i.revision == revision &&
		i.identity.Subject != "" && i.identity == identidad
}

// Renovada pone la intención en la revisión con que su sesión acaba de renovar el token
// para la misma identidad, sin soltar el cerrojo (auth.Session.AccessTokenScoped): una
// renovación propia no la invalida; una adopción, sí. Solo avanza desde `anterior`, la
// revisión en que la sesión acaba de comprobarla vigente: una intención vieja (de antes de
// un cambio de cuenta, A→B→A) no se puede revivir así, ni por error. Dice si avanzó.
func (i *AuthorizationIntent) Renovada(anterior, revision uint64) bool {
	if i == nil || i.revision != anterior {
		return false
	}
	i.revision = revision
	return true
}

// Current excludes appearance/presence changes, and rejects adoption including ABA.
func (i *AuthorizationIntent) Current() bool {
	if i == nil || i.session == nil {
		return false
	}
	vigente := false
	i.session.Comprobar(Comprobacion{func(revision uint64, identidad IdentitySnapshot) {
		vigente = i.VigenteCon(i.session, revision, identidad)
	}})
	return vigente
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
