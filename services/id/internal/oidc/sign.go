// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package oidc

import (
	"fmt"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// sign serializa y firma un token con la clave activa (RS256), incluyendo el
// `kid` en la cabecera JOSE para que los verificadores seleccionen la clave.
func (i *Issuer) sign(tok jwt.Token) (string, error) {
	key, err := i.km.signingKey()
	if err != nil {
		return "", err
	}
	kid, _ := key.Get(jwk.KeyIDKey)
	hdr := jws.NewHeaders()
	if k, ok := kid.(string); ok {
		_ = hdr.Set(jws.KeyIDKey, k)
	}
	_ = hdr.Set(jws.TypeKey, "JWT")

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key, jws.WithProtectedHeaders(hdr)))
	if err != nil {
		return "", fmt.Errorf("oidc: firmar token: %w", err)
	}
	return string(signed), nil
}
