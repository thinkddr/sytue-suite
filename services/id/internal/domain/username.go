// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/mailaddr"
)

// El @usuario: el nombre con el que alguien de OTRA organización te encuentra en
// Sytue One (ADR 0040). Es global, único y no distingue mayúsculas.
//
// COMPARTE ESPACIO CON LAS DIRECCIONES DEL DOMINIO RAÍZ: @ana es ana@sytue.eu. En
// una cuenta personal eso es literal —se registra eligiendo el @usuario y ese es
// su buzón—, y por eso sus reglas son exactamente las de una parte local de
// buzón: un @usuario válido es un punto fijo de normalizarLocal. Dos reglas
// distintas acabarían dando un @usuario que no se puede usar como dirección.

// minUsername es la longitud mínima. Con una o dos letras el espacio se agota en
// una tarde y el nombre no dice quién es nadie.
const minUsername = 3

// NormalizeUsername deja lo que alguien teclea como @usuario en su forma
// canónica: sin la arroba delante, sin espacios alrededor y en minúsculas. NO
// corrige nada más a propósito: si «Ana Pérez» se convirtiera en silencio en
// «ana.perez», la persona se registraría con un nombre que no ha elegido.
func NormalizeUsername(raw string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "@"))
}

// ValidUsername dice si el @usuario (ya normalizado) se puede usar: entre 3 y 64
// caracteres de letras, dígitos, punto y guion, sin separadores en los extremos
// ni repetidos. No mira si está reservado: eso es UsernameReservado.
func ValidUsername(u string) bool {
	return len(u) >= minUsername && normalizarLocal(u) == u
}

// UsernameReservado dice si el @usuario suplantaría a la plataforma
// (@soporte, @admin, @sytue-id…). Es la misma lista que protege las direcciones
// del dominio raíz, porque es la misma dirección.
func UsernameReservado(u string) bool { return mailaddr.Reservado(u) }

// UsernameBase es la raíz del @usuario que se le asigna a quien no lo eligió
// (invitados, SCIM, SSO): la misma parte local nombre.apellidos que su buzón, que
// es lo que quien le conoce teclearía primero.
func UsernameBase(givenName, familyName, fullName, email string) string {
	return MailboxLocalPart(givenName, familyName, fullName, email)
}

// UsernameCandidate compone el intento n-ésimo a partir de la raíz: n=0 la raíz
// limpia y después ana.perez2, ana.perez3… Mismo desempate que las direcciones.
func UsernameCandidate(base string, n int) string { return candidatoLocal(base, n) }

// UsernameDeLogin reconoce un @usuario en el campo de acceso: lo que empieza por
// arroba, o lo que no lleva ninguna. Todo lo demás es un correo.
func UsernameDeLogin(ident string) (string, bool) {
	ident = strings.TrimSpace(ident)
	if ident == "" || (strings.Contains(ident, "@") && !strings.HasPrefix(ident, "@")) {
		return "", false
	}
	return NormalizeUsername(ident), true
}
