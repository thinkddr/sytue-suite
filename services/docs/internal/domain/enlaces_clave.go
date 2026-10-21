// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"unicode/utf8"

	pkgclave "git.sytue.eu/sytue/one/platform/pkg/clave"
)

// Contraseña opcional de un enlace de compartir (docs/security.md#matriz-acl).
//
// El hash es el de `platform/pkg/clave` (Argon2id con sal, PHC), el mismo de los
// enlaces de Drive. Quien acierta la contraseña recibe una PRUEBA que presenta después
// en cada petición, para no pagar un Argon2id por página.

const (
	// ClaveDeEnlaceMin y ClaveDeEnlaceMax acotan la contraseña, en caracteres. El
	// máximo evita que un cuerpo enorme sea trabajo gratis para Argon2id.
	ClaveDeEnlaceMin = 8
	ClaveDeEnlaceMax = 128
)

// HashClaveDeEnlace valida y hashea la contraseña de un enlace. Vacía = sin contraseña
// (devuelve ""), que no es un error.
func HashClaveDeEnlace(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	if n := utf8.RuneCountInString(password); n < ClaveDeEnlaceMin || n > ClaveDeEnlaceMax {
		return "", ErrClaveDeEnlaceInvalida
	}
	return pkgclave.Hash(password)
}

// VerificarClaveDeEnlace compara en tiempo constante. Una contraseña fuera de rango no
// llega a Argon2id.
func VerificarClaveDeEnlace(password, hash string) bool {
	if n := utf8.RuneCountInString(password); n == 0 || n > ClaveDeEnlaceMax {
		return false
	}
	ok, err := pkgclave.Verificar(password, hash)
	return err == nil && ok
}

// PruebaDeClave es lo que recibe quien acertó la contraseña de un enlace, y lo que
// presenta después. Sale del hash guardado: cambiar o quitar la contraseña la invalida.
//
// ponytail: sin secreto del servidor, como en Drive: quien pudiera leer el hash de la
// base ya puede calcularla. Si algún día hay un secreto global, HMAC(secreto, id del enlace).
func PruebaDeClave(hash string) string {
	sum := sha256.Sum256([]byte("docs-enlace-v1:" + hash))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// PruebaValida dice si `prueba` es la del enlace con ese hash, en tiempo constante.
func PruebaValida(prueba, hash string) bool {
	return prueba != "" && subtle.ConstantTimeCompare([]byte(prueba), []byte(PruebaDeClave(hash))) == 1
}
