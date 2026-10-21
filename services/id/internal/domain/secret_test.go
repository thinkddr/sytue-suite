// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	// Política por defecto: min 12, sin complejidad.
	def := DefaultPasswordPolicy
	// Menos del mínimo (incluye una con acento de code points) ⇒ rechazada.
	for _, weak := range []string{"", "a", "1234567", "café123", "solo-once-c"} {
		if err := ValidatePassword(def, weak); !errors.Is(err, ErrWeakPassword) {
			t.Errorf("ValidatePassword(default, %q) = %v, quiero ErrWeakPassword", weak, err)
		}
	}
	// ≥ min runes ⇒ aceptada (sin reglas de composición con la política por defecto).
	for _, ok := range []string{"123456789012", "correcto-caballo", "contraseña-válida-ñ"} {
		if err := ValidatePassword(def, ok); err != nil {
			t.Errorf("ValidatePassword(default, %q) = %v, quiero nil", ok, err)
		}
	}
	// Suelo duro: una política laxa nunca admite menos de PasswordMinFloor.
	if err := ValidatePassword(PasswordPolicy{MinLength: 1}, "1234567"); !errors.Is(err, ErrWeakPassword) {
		t.Error("min_length por debajo del suelo no debe rebajar la validación")
	}
	if err := ValidatePassword(PasswordPolicy{MinLength: 1}, "12345678"); err != nil {
		t.Errorf("8 caracteres deben pasar el suelo duro: %v", err)
	}
	// Complejidad: exige dígito + otra clase.
	cx := PasswordPolicy{MinLength: 8, RequireComplexity: true}
	// solo letras / solo dígitos / dígitos+símbolos SIN letra (el caso que el review destapó).
	for _, bad := range []string{"solopalabras", "123456789", "1234!@#$%^"} {
		if err := ValidatePassword(cx, bad); !errors.Is(err, ErrPasswordComplexity) {
			t.Errorf("ValidatePassword(complex, %q) = %v, quiero ErrPasswordComplexity", bad, err)
		}
	}
	for _, good := range []string{"palabra12", "abc123!!", "Passw0rd"} {
		if err := ValidatePassword(cx, good); err != nil {
			t.Errorf("ValidatePassword(complex, %q) = %v, quiero nil", good, err)
		}
	}
	// Cota superior anti-DoS (Argon2 sobre entrada patológica).
	if err := ValidatePassword(def, strings.Repeat("x", maxPasswordLen+1)); !errors.Is(err, ErrWeakPassword) {
		t.Error("una contraseña que excede la cota debe rechazarse")
	}
	// Los errores se mapean a 400 vía ErrInvalidInput.
	if !errors.Is(ErrWeakPassword, ErrInvalidInput) || !errors.Is(ErrPasswordComplexity, ErrInvalidInput) {
		t.Error("los errores de política deben envolver ErrInvalidInput")
	}
}

func TestHashPasswordRoundTrip(t *testing.T) {
	const pw = "correcto-caballo-batería-grapa"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("hash no es argon2id PHC: %q", h)
	}
	if strings.Contains(h, pw) {
		t.Fatal("el hash contiene la contraseña en claro")
	}
	ok, err := VerifyPassword(pw, h)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(correcta) = %v, %v; quería true, nil", ok, err)
	}
	ok, _ = VerifyPassword("otra-cosa", h)
	if ok {
		t.Fatal("VerifyPassword aceptó una contraseña incorrecta")
	}
}

func TestHashPasswordUniqueSalt(t *testing.T) {
	a, _ := HashPassword("misma")
	b, _ := HashPassword("misma")
	if a == b {
		t.Fatal("dos hashes de la misma contraseña son iguales (¿sal fija?)")
	}
}

func TestVerifyPasswordEmpty(t *testing.T) {
	if ok, _ := VerifyPassword("", "$argon2id$v=19$m=1,t=1,p=1$AAAA$AAAA"); ok {
		t.Fatal("contraseña vacía no debe verificar")
	}
	if ok, _ := VerifyPassword("x", ""); ok {
		t.Fatal("hash vacío no debe verificar")
	}
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestVerifyPKCE_S256(t *testing.T) {
	verifier := "verificador-de-prueba-con-suficiente-entropia-0123456789"
	challenge := s256(verifier)

	if !VerifyPKCE("S256", challenge, verifier) {
		t.Fatal("S256 válido debería verificar")
	}
	// Método vacío se trata como S256 (default OIDC).
	if !VerifyPKCE("", challenge, verifier) {
		t.Fatal("método vacío debería tratarse como S256")
	}
	if VerifyPKCE("S256", challenge, "verificador-incorrecto") {
		t.Fatal("verifier incorrecto no debería verificar")
	}
}

func TestVerifyPKCE_PlainRejected(t *testing.T) {
	// 'plain' está deshabilitado por política: incluso con challenge==verifier,
	// debe rechazarse.
	if VerifyPKCE("plain", "abc123", "abc123") {
		t.Fatal("el método plain debe estar deshabilitado")
	}
	if VerifyPKCE("S128", "abc", "abc") {
		t.Fatal("métodos desconocidos deben rechazarse")
	}
}

func TestVerifyPKCE_Empty(t *testing.T) {
	if VerifyPKCE("S256", "", "algo") {
		t.Fatal("challenge vacío no debe verificar")
	}
	if VerifyPKCE("S256", "algo", "") {
		t.Fatal("verifier vacío no debe verificar")
	}
}

func TestVerifyDecoyDoesNotPanic(t *testing.T) {
	// Solo verifica que la verificación señuelo no entra en pánico y no valida
	// ningún secreto (siempre descarta el resultado).
	VerifyDecoy("")
	VerifyDecoy("cualquier-secreto")
	if decoyHash == "" || !strings.HasPrefix(decoyHash, "$argon2id$") {
		t.Fatal("el hash señuelo debe ser un PHC Argon2id válido")
	}
}

func TestHashTokenDeterministicAndOpaque(t *testing.T) {
	tok := "un-token-opaco"
	h1 := HashToken(tok)
	h2 := HashToken(tok)
	if h1 != h2 {
		t.Fatal("HashToken no es determinista")
	}
	if strings.Contains(h1, tok) {
		t.Fatal("el hash contiene el token en claro")
	}
	if HashToken("otro") == h1 {
		t.Fatal("colisión inesperada de HashToken")
	}
}
