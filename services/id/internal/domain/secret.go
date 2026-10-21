// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Cotas de la longitud mínima configurable por-org (NIST 800-63B §5.1.1.2:
// longitud sobre composición). PasswordMinFloor es también el suelo absoluto: por
// débil que se configure una org, nunca se admite menos. maxPasswordLen acota el
// coste de Argon2 ante entradas patológicas (DoS) admitiendo passphrases largas.
const (
	PasswordMinFloor = 8
	PasswordMinCeil  = 128
	maxPasswordLen   = 4096
)

// PasswordPolicy es la política de contraseñas de una organización. Mínima a
// propósito (longitud + un interruptor de complejidad); sin matriz de reglas
// enterprise (historial, rotación forzada, clases obligatorias configurables).
type PasswordPolicy struct {
	MinLength         int
	RequireComplexity bool
}

// DefaultPasswordPolicy es el default sensato que hereda toda org (y el fallback
// cuando un usuario no tiene ninguna membresía). Debe coincidir con el DEFAULT de
// la migración de `organizations`.
var DefaultPasswordPolicy = PasswordPolicy{MinLength: 12, RequireComplexity: false}

// ErrWeakPassword indica que una contraseña no alcanza la longitud mínima de la
// política. Envuelve ErrInvalidInput ⇒ el transporte la mapea a 400.
var ErrWeakPassword = fmt.Errorf("%w: la contraseña no cumple la longitud mínima", ErrInvalidInput)

// ErrPasswordComplexity indica que la política exige complejidad y la contraseña
// no la cumple (mezcla de clases + un dígito). Envuelve ErrInvalidInput ⇒ 400.
var ErrPasswordComplexity = fmt.Errorf("%w: la contraseña no cumple la complejidad requerida", ErrInvalidInput)

// ValidatePassword aplica la política de contraseñas de una org. Es el ÚNICO punto
// de dominio que decide qué contraseña es aceptable (choke point): TODO alta o
// cambio de contraseña pasa por aquí antes de hashear. Longitud en runes ≥
// max(policy.MinLength, PasswordMinFloor) y en bytes ≤ maxPasswordLen. Con
// RequireComplexity exige al menos un dígito y una segunda clase de carácter.
func ValidatePassword(policy PasswordPolicy, password string) error {
	minLen := policy.MinLength
	if minLen < PasswordMinFloor {
		minLen = PasswordMinFloor // suelo duro: una org no puede rebajar por debajo de 8
	}
	if utf8.RuneCountInString(password) < minLen || len(password) > maxPasswordLen {
		return ErrWeakPassword
	}
	if policy.RequireComplexity && !hasComplexity(password) {
		return ErrPasswordComplexity
	}
	return nil
}

// hasComplexity exige al menos una LETRA y al menos un DÍGITO (mezcla alfanumérica).
// Coincide con el mensaje de error ("letras y al menos un número") y con el copy de
// la consola, y rechaza p. ej. "1234!@#$" (dígitos+símbolos, sin letra). unicode.IsLetter
// cuenta letras Unicode (ñ, é…). ponytail: política mínima; no exigimos mayús/minús/símbolo.
func hasComplexity(pw string) bool {
	var hasLetter, hasDigit bool
	for _, r := range pw {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case unicode.IsLetter(r):
			hasLetter = true
		}
	}
	return hasLetter && hasDigit
}

// Parámetros Argon2id (OWASP Password Storage Cheat Sheet, perfil de servidor).
// Revisables en auditoría de seguridad previa a producción.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword devuelve un hash Argon2id codificado en formato PHC.
// Nunca se almacena la contraseña en claro (docs/CONVENTIONS.md §0.7).
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("%w: contraseña vacía", ErrInvalidInput)
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("domain: leer aleatoriedad: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		b64(salt), b64(key)), nil
}

// VerifyPassword compara una contraseña con un hash PHC en tiempo constante.
func VerifyPassword(password, encoded string) (bool, error) {
	if password == "" || encoded == "" {
		return false, nil
	}
	parts := strings.Split(encoded, "$")
	// ["", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("domain: hash de contraseña con formato desconocido")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, err
	}
	var mem uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &t, &p); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	// Un hash Argon2 legítimo mide 32 bytes; se acota para descartar PHC
	// corruptos y el overflow int→uint32 (gosec G115).
	if len(want) == 0 || len(want) > 512 {
		return false, errors.New("domain: hash de contraseña con longitud inválida")
	}
	got := argon2.IDKey([]byte(password), salt, t, mem, p, uint32(len(want))) //nolint:gosec // G115: len(want) acotado a (0,512] justo arriba.
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// decoyHash es un hash Argon2id fijo (de un secreto que nadie conoce) usado como
// "señuelo" para uniformar el tiempo de respuesta cuando el client no existe o no
// tiene secreto configurado, evitando enumeración por timing (auditoría F7).
var decoyHash = func() string {
	h, err := HashPassword("decoy-secret-never-matches-any-real-client-credential")
	if err != nil {
		// No debería ocurrir; en el peor caso VerifyDecoy no gastará tiempo.
		return ""
	}
	return h
}()

// VerifyDecoy ejecuta una verificación Argon2id contra un hash señuelo y descarta
// el resultado. Se llama en las ramas de fallo de autenticación de client para
// que el tiempo de respuesta sea indistinguible del de un secreto real inválido.
func VerifyDecoy(secret string) {
	if decoyHash == "" || secret == "" {
		// Aun sin secreto, gastamos algo de trabajo para no ser instantáneos.
		_, _ = VerifyPassword("x", decoyHash)
		return
	}
	_, _ = VerifyPassword(secret, decoyHash)
}

// GenerateToken produce un token opaco aleatorio (URL-safe, sin padding) con
// nBytes de entropía. Usado para códigos de autorización, sesiones y refresh.
func GenerateToken(nBytes int) (string, error) {
	if nBytes <= 0 {
		nBytes = 32
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("domain: leer aleatoriedad: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken calcula el SHA-256 (hex) de un token opaco. Solo el hash se persiste,
// nunca el token en claro (defensa ante filtración de la base de datos).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// VerifyPKCE valida el code_verifier frente al code_challenge (RFC 7636).
// SOLO se admite el método S256: `plain` queda deshabilitado por política de
// seguridad (RFC 9700 §2.1.1, OWASP ASVS L2 §3.5), ya que no protege ante fuga
// del code_challenge. Un método vacío se interpreta como S256 (default OIDC).
func VerifyPKCE(method, challenge, verifier string) bool {
	if challenge == "" || verifier == "" {
		return false
	}
	switch method {
	case "S256", "":
		sum := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(sum[:])
		return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
	default:
		// plain u otros métodos: rechazados.
		return false
	}
}
