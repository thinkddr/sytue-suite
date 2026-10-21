// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// resetTokenRandomLen es la entropía del token de reseteo (32 bytes = 256 bits):
// inadivinable por sí solo; la firma HMAC es defensa en profundidad añadida.
const resetTokenRandomLen = 32

// NewResetToken genera un token de reseteo de contraseña FIRMADO: 32 bytes
// aleatorios seguidos de una etiqueta HMAC-SHA256 con la clave del servidor,
// ambos en base64url ("<random>.<hmac>"). Devuelve el token en claro —que viaja
// SOLO en el enlace del email— y su hash SHA-256, lo ÚNICO que se persiste (nunca
// el token en claro). La firma permite rechazar en tiempo constante, y sin tocar
// la BD, los tokens manipulados o forjados; la fila en BD gobierna el un-solo-uso
// (se borra al consumir) y la caducidad.
func NewResetToken(key []byte) (token, hash string, err error) {
	r := make([]byte, resetTokenRandomLen)
	if _, err := rand.Read(r); err != nil {
		return "", "", fmt.Errorf("domain: leer aleatoriedad: %w", err)
	}
	b64 := base64.RawURLEncoding.EncodeToString
	token = b64(r) + "." + b64(resetMAC(key, r))
	return token, HashToken(token), nil
}

// VerifyResetTokenSig comprueba la firma HMAC del token en tiempo constante. NO
// consulta la BD: es el filtro previo (barato) contra tokens manipulados/forjados
// antes del consumo atómico. Un token con formato o firma inválidos ⇒ false.
func VerifyResetTokenSig(key []byte, token string) bool {
	i := strings.IndexByte(token, '.')
	if i <= 0 || i >= len(token)-1 {
		return false
	}
	r, err := base64.RawURLEncoding.DecodeString(token[:i])
	if err != nil || len(r) != resetTokenRandomLen {
		return false
	}
	mac, err := base64.RawURLEncoding.DecodeString(token[i+1:])
	if err != nil {
		return false
	}
	return hmac.Equal(mac, resetMAC(key, r))
}

// resetMAC calcula HMAC-SHA256(key, msg).
func resetMAC(key, msg []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(msg)
	return h.Sum(nil)
}
