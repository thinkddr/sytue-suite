// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import "testing"

func TestValidatorAccumulates(t *testing.T) {
	v := New()
	v.Required("name", "").Email("email", "no-es-correo").Slug("tenant", "BAD").MaxLen("bio", "hola", 2)
	err := v.Err()
	if err == nil {
		t.Fatal("esperaba errores")
	}
	verrs, ok := AsErrors(err)
	if !ok || len(verrs.Fields) != 4 {
		t.Fatalf("esperaba 4 errores por campo, got %+v", verrs)
	}
}

func TestValidatorPasses(t *testing.T) {
	v := New()
	v.Required("name", "Ada").Email("email", "ada@sytue.eu").Slug("tenant", "acme").In("role", "admin", "admin", "member")
	if err := v.Err(); err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
}

func TestIsEmail(t *testing.T) {
	if !IsEmail("a@b.com") {
		t.Error("correo válido rechazado")
	}
	if IsEmail("Nombre <a@b.com>") {
		t.Error("no debe aceptar forma con nombre")
	}
	if IsEmail("no-arroba") {
		t.Error("sin arroba debe fallar")
	}
}

func TestSanitizeText(t *testing.T) {
	// Colapsa espacios y elimina caracteres de control/zero-width.
	in := "  hola\u200b\tmundo   \x00 "
	got := SanitizeText(in)
	if got != "hola mundo" {
		t.Fatalf("SanitizeText = %q", got)
	}
}

func TestHasControlChars(t *testing.T) {
	if !HasControlChars("a\x00b") {
		t.Error("debe detectar NUL")
	}
	if HasControlChars("línea1\nlínea2\ttab") {
		t.Error("tab/nl son legítimos")
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := TruncateRunes("áéíóú", 3); got != "áéí" {
		t.Fatalf("TruncateRunes = %q", got)
	}
	if got := TruncateRunes("hi", 10); got != "hi" {
		t.Fatalf("no debe recortar, got %q", got)
	}
}

func TestNoControlValidator(t *testing.T) {
	v := New()
	v.NoControl("field", "bad\x01value")
	if v.Err() == nil {
		t.Fatal("debe registrar error por control")
	}
}

// TestCSVSafe cubre el saneado de salida CSV: los arranques de fórmula se
// escapan con comilla simple y el resto del texto no se toca.
func TestCSVSafe(t *testing.T) {
	// Cada uno de los seis arranques de fórmula debe quedar escapado.
	for _, s := range []string{
		"=HYPERLINK(\"http://evil\",\"x\")",
		"+1234",
		"-1+2",
		"@SUM(A1)",
		"\tcmd",
		"\rcmd",
	} {
		got := CSVSafe(s)
		if got != "'"+s {
			t.Errorf("CSVSafe(%q) = %q; se esperaba el prefijo de comilla", s, got)
		}
	}
	// Lo que no arranca fórmula pasa intacto, incluido el vacío y un valor que
	// sólo CONTIENE (no empieza por) un carácter de fórmula.
	for _, s := range []string{"", "Ana López", "a=b", "correo@sytue.eu", "123", "'ya escapado"} {
		if got := CSVSafe(s); got != s {
			t.Errorf("CSVSafe(%q) = %q; se esperaba sin cambios", s, got)
		}
	}
}
