// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// recoveryAlphabet es el alfabeto base32 (RFC 4648): A–Z2–7. Excluye 0/1/8/9, que
// se confunden con O/I/B/g al teclearlos — códigos legibles a mano. 32 divide 256
// exactamente ⇒ el mapeo byte%32 es uniforme (sin sesgo de módulo).
const recoveryAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

const (
	// RecoveryCodeLen es la longitud (en caracteres del alfabeto) de un código
	// normalizado: 10 chars × 5 bits = 50 bits de entropía. De un solo uso y tras
	// el rate-limit del login ⇒ inadivinable por fuerza bruta online.
	RecoveryCodeLen = 10
	// recoveryBatch es cuántos códigos se generan por lote (estándar de facto).
	recoveryBatch = 10
)

// NewRecoveryCodes genera un lote de códigos de recuperación TOTP con crypto/rand
// (CSPRNG). Devuelve los códigos en claro ya FORMATEADOS (para mostrarlos UNA vez)
// y sus hashes SHA-256 (lo único que se persiste, alineados por índice). Nunca se
// almacena el código en claro.
func NewRecoveryCodes() (display, hashes []string, err error) {
	display = make([]string, 0, recoveryBatch)
	hashes = make([]string, 0, recoveryBatch)
	for i := 0; i < recoveryBatch; i++ {
		b := make([]byte, RecoveryCodeLen)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, fmt.Errorf("domain: leer aleatoriedad: %w", err)
		}
		var sb strings.Builder
		for _, x := range b {
			sb.WriteByte(recoveryAlphabet[int(x)%len(recoveryAlphabet)])
		}
		code := sb.String()
		display = append(display, FormatRecoveryCode(code))
		hashes = append(hashes, HashToken(code))
	}
	return display, hashes, nil
}

// NormalizeRecoveryCode canoniza lo que teclea el usuario a la forma que se hashea:
// mayúsculas y solo los caracteres del alfabeto (ignora espacios, guiones y demás).
// Devuelve "" si no queda un código de la longitud EXACTA — el llamante no debe
// tratar "" como válido (evita consultar la BD con una entrada basura).
func NormalizeRecoveryCode(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToUpper(s) {
		if strings.ContainsRune(recoveryAlphabet, r) {
			sb.WriteRune(r)
		}
	}
	out := sb.String()
	if len(out) != RecoveryCodeLen {
		return ""
	}
	return out
}

// FormatRecoveryCode agrupa un código en dos bloques ("XXXXX-XXXXX") para leerlo y
// teclearlo mejor; el guion es cosmético (NormalizeRecoveryCode lo ignora).
func FormatRecoveryCode(code string) string {
	if len(code) != RecoveryCodeLen {
		return code
	}
	return code[:5] + "-" + code[5:]
}
