// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// acceptableSkew tolera un pequeño desfase de reloj al validar exp/nbf/iat.
// Se mantiene corto (30s) para no ampliar la ventana de tokens expirados.
const acceptableSkew = 30 * time.Second

// AllowedSignatureAlgs es la allowlist de algoritmos de firma aceptados al
// validar tokens emitidos por este IdP. Se restringe deliberadamente a RS256
// (el único algoritmo con el que se firma) para cerrar la clase de ataques de
// confusión de algoritmo y, sobre todo, para rechazar SIEMPRE `alg=none`
// (OWASP ASVS L2 §3.5, JWT BCP / RFC 8725 §2.1).
var AllowedSignatureAlgs = []jwa.SignatureAlgorithm{jwa.RS256}

// ErrInvalidToken indica que un token no es válido (firma, alg, claims o expiración).
var ErrInvalidToken = errors.New("oidc: token inválido")

// allowedAlgSet es la representación como conjunto para comprobaciones O(1).
var allowedAlgSet = func() map[jwa.SignatureAlgorithm]struct{} {
	m := make(map[jwa.SignatureAlgorithm]struct{}, len(AllowedSignatureAlgs))
	for _, a := range AllowedSignatureAlgs {
		m[a] = struct{}{}
	}
	return m
}()

// Verify valida un JWT emitido por este IdP contra el JWKS público, forzando:
//   - allowlist de algoritmos (RS256) — rechaza `alg=none` y algoritmos débiles
//     ANTES de intentar verificar la firma (RFC 8725);
//   - selección de clave por `kid` presente en la cabecera JOSE;
//   - validación estándar de claims (exp/nbf/iat) con el issuer esperado;
//   - si audience != "", validación de la audiencia (RFC 9068 §4, ASVS L2 §3.5):
//     el token debe llevar `aud` con el identificador del resource server que lo
//     consume, de modo que un access token emitido para otro recurso se rechace.
//
// Devuelve ErrInvalidToken (envuelto) ante cualquier fallo, para no filtrar
// detalles al llamante.
func Verify(raw, issuer, audience string, set jwk.Set) (jwt.Token, error) {
	// 1) Inspeccionar la cabecera JOSE y validar el `alg` contra la allowlist
	//    ANTES de verificar la firma. jws.Parse no ejecuta criptografía, solo
	//    decodifica; así rechazamos `alg=none` de forma explícita.
	msg, err := jws.Parse([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: cabecera JOSE ilegible", ErrInvalidToken)
	}
	sigs := msg.Signatures()
	if len(sigs) == 0 {
		return nil, fmt.Errorf("%w: token sin firma", ErrInvalidToken)
	}
	for _, s := range sigs {
		alg := s.ProtectedHeaders().Algorithm()
		if _, ok := allowedAlgSet[alg]; !ok {
			return nil, fmt.Errorf("%w: algoritmo no permitido %q", ErrInvalidToken, alg)
		}
	}

	// 2) Verificar firma y claims. WithKeySet exige `kid` y usa el `alg` de la
	//    clave; combinado con la comprobación anterior queda pinneado a RS256.
	opts := []jwt.ParseOption{
		jwt.WithKeySet(set, jws.WithInferAlgorithmFromKey(false), jws.WithRequireKid(true)),
		jwt.WithValidate(true),
		jwt.WithIssuer(issuer),
		jwt.WithAcceptableSkew(acceptableSkew),
	}
	if audience != "" {
		// El token debe declarar este resource server en `aud`.
		opts = append(opts, jwt.WithAudience(audience))
	}
	tok, err := jwt.ParseString(raw, opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	return tok, nil
}

// ActorOf devuelve el `act.sub` de un token (RFC 8693 §4.1): el client que lo
// obtuvo actuando en nombre del sujeto. delegated indica si el token LLEVA el
// claim `act` — es lo que importa para rechazarlo como subject_token de otro
// canje (la delegación no se encadena), y es true aunque el claim venga con una
// forma que no sepamos leer: ante la duda, delegado.
func ActorOf(tok jwt.Token) (actor string, delegated bool) {
	v, ok := tok.Get("act")
	if !ok {
		return "", false
	}
	if m, ok := v.(map[string]any); ok {
		s, _ := m["sub"].(string)
		return s, true
	}
	return "", true
}
