// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package totp implementa TOTP (RFC 6238) sobre HOTP (RFC 4226) para el segundo
// factor de Sytue ID. Clean-room a partir de las RFC públicas; sin dependencias
// de terceros (solo stdlib crypto/hmac + crypto/sha1).
//
// SHA-1 se usa aquí SOLO como PRF dentro de HMAC (HMAC-SHA1), que es el algoritmo
// por defecto e interoperable de los autenticadores TOTP (Google Authenticator,
// etc.); no es un uso de SHA-1 como función de hash de seguridad. La comparación
// del código es en tiempo constante y se admite una ventana de ±1 paso.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // G505: TOTP exige HMAC-SHA1 (RFC 6238 §1.2, interop Google Authenticator); no se usa como hash de seguridad.
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// digits es la longitud del código (6, estándar de facto).
	digits = 6
	// period es el paso temporal en segundos (30, estándar de facto).
	period = 30
	// skewSteps es la ventana tolerada a cada lado (±1 paso ⇒ ±30s) para
	// absorber el desfase de reloj del autenticador sin ampliar en exceso.
	skewSteps = 1
)

// Code calcula el código TOTP para un secreto y un instante dados.
func Code(secret []byte, t time.Time) string {
	unix := t.Unix()
	if unix < 0 { // antes de 1970: reloj roto; trátalo como el instante cero
		unix = 0
	}
	return hotp(secret, uint64(unix)/period) //nolint:gosec // G115: unix se fuerza a >= 0 justo arriba.
}

// Validate comprueba un código TOTP contra el secreto en el instante `now`,
// admitiendo una ventana de ±skewSteps pasos. Comparación en tiempo constante.
func Validate(secret []byte, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(secret) == 0 || len(code) != digits {
		return false
	}
	counter := now.Unix() / period
	var ok bool
	for d := int64(-skewSteps); d <= skewSteps; d++ {
		c := counter + d
		if c < 0 {
			continue
		}
		expected := hotp(secret, uint64(c))
		// Comparación en tiempo constante; no cortocircuitar el bucle para no
		// filtrar por timing cuál paso coincidió.
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			ok = true
		}
	}
	return ok
}

// hotp calcula el valor HOTP (RFC 4226) de `digits` dígitos.
func hotp(secret []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	mod := bin % pow10(digits)
	return fmt.Sprintf("%0*d", digits, mod)
}

func pow10(n int) uint32 {
	p := uint32(1)
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}

// DecodeSecret decodifica un secreto TOTP en base32 (sin padding, mayúsculas),
// el formato del URI otpauth:// que consumen los autenticadores.
func DecodeSecret(b32 string) ([]byte, error) {
	s := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(b32, " ", "")))
	dec := base32.StdEncoding.WithPadding(base32.NoPadding)
	b, err := dec.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("totp: secreto base32 inválido: %w", err)
	}
	return b, nil
}

// SecretLen es la longitud en bytes del secreto TOTP generado (160 bits): el
// tamaño recomendado para HMAC-SHA1 (RFC 4226 §4) e interoperable con los
// autenticadores.
const SecretLen = 20

// GenerateSecret produce un secreto TOTP aleatorio de SecretLen bytes con
// crypto/rand (CSPRNG). Uno por usuario, nunca reutilizado.
func GenerateSecret() ([]byte, error) {
	b := make([]byte, SecretLen)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("totp: leer aleatoriedad: %w", err)
	}
	return b, nil
}

// EncodeSecret codifica un secreto en base32 (sin padding, mayúsculas), el
// formato del campo `secret` del URI otpauth:// (inverso de DecodeSecret).
func EncodeSecret(secret []byte) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret)
}

// ProvisioningURI construye el URI otpauth://totp (Key URI Format, Google
// Authenticator) que los autenticadores consumen por QR o entrada manual. issuer
// es el nombre visible del emisor (p. ej. "Sytue"); account, la cuenta del
// usuario (su email). Los parámetros van explícitos (SHA1/6/30) para no depender
// de los defaults del cliente.
func ProvisioningURI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", EncodeSecret(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", strconv.Itoa(digits))
	q.Set("period", strconv.Itoa(period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
