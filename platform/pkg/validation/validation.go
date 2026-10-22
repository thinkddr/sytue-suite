// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package validation ofrece validación y saneado de entrada reutilizable por
// todos los servicios Go de Sytue One. deny-by-default: los helpers rechazan lo
// no explícitamente permitido (longitudes, formatos, caracteres de control).
//
// No sustituye a las consultas parametrizadas de pkg/pg (el saneado NO es una
// defensa anti-inyección: la parametrización lo es); su cometido es garantizar
// datos bien formados, acotados y sin PII innecesaria antes de persistir.
package validation

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// FieldError describe un error de validación asociado a un campo concreto.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }

// Errors agrupa errores por campo; implementa error y se serializa como los
// detalles estructurados del sobre de error de httpx.
type Errors struct {
	Fields []FieldError `json:"fields"`
}

func (e *Errors) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return "validación fallida"
	}
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Error())
	}
	return "validación fallida: " + strings.Join(parts, "; ")
}

// HasErrors indica si se acumuló algún error.
func (e *Errors) HasErrors() bool { return e != nil && len(e.Fields) > 0 }

// OrNil devuelve nil si no hubo errores, o *Errors en caso contrario.
func (e *Errors) OrNil() error {
	if e.HasErrors() {
		return e
	}
	return nil
}

// Validator acumula errores por campo (patrón builder). Encadena checks y al
// final se consulta OrNil().
type Validator struct {
	errs Errors
}

// New crea un Validator vacío.
func New() *Validator { return &Validator{} }

// Add registra un error para un campo.
func (v *Validator) Add(field, message string) *Validator {
	v.errs.Fields = append(v.errs.Fields, FieldError{Field: field, Message: message})
	return v
}

// Check registra el mensaje si cond es falso (para condiciones ad-hoc).
func (v *Validator) Check(cond bool, field, message string) *Validator {
	if !cond {
		v.Add(field, message)
	}
	return v
}

// Required exige que un valor de texto no esté vacío (tras recortar espacios).
func (v *Validator) Required(field, value string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.Add(field, "es obligatorio")
	}
	return v
}

// MaxLen limita la longitud en runes (no bytes) para acotar el tamaño real.
func (v *Validator) MaxLen(field, value string, maxLen int) *Validator {
	if utf8.RuneCountInString(value) > maxLen {
		v.Add(field, fmt.Sprintf("supera la longitud máxima de %d", maxLen))
	}
	return v
}

// MinLen exige una longitud mínima en runes.
func (v *Validator) MinLen(field, value string, minLen int) *Validator {
	if utf8.RuneCountInString(value) < minLen {
		v.Add(field, fmt.Sprintf("requiere al menos %d caracteres", minLen))
	}
	return v
}

// Email valida sintácticamente una dirección de correo (RFC 5322 básico).
func (v *Validator) Email(field, value string) *Validator {
	if !IsEmail(value) {
		v.Add(field, "no es un correo válido")
	}
	return v
}

// Slug valida un slug DNS-compatible (minúsculas, dígitos, guiones internos).
func (v *Validator) Slug(field, value string) *Validator {
	if !IsSlug(value) {
		v.Add(field, "no es un identificador válido")
	}
	return v
}

// Matches valida contra una expresión regular precompilada.
func (v *Validator) Matches(field, value string, re *regexp.Regexp, msg string) *Validator {
	if !re.MatchString(value) {
		v.Add(field, msg)
	}
	return v
}

// In exige que el valor esté en la allowlist dada (deny-by-default).
func (v *Validator) In(field, value string, allowed ...string) *Validator {
	for _, a := range allowed {
		if value == a {
			return v
		}
	}
	v.Add(field, "valor no permitido")
	return v
}

// NoControl rechaza caracteres de control (excepto tab/nueva línea/retorno),
// que suelen indicar payloads maliciosos o corruptos.
func (v *Validator) NoControl(field, value string) *Validator {
	if HasControlChars(value) {
		v.Add(field, "contiene caracteres de control no permitidos")
	}
	return v
}

// Err devuelve el conjunto de errores acumulados (nil si no hubo).
func (v *Validator) Err() error { return v.errs.OrNil() }

// ---- Funciones puras de validación/saneado ----

var slugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// IsSlug indica si s es un slug DNS-compatible.
func IsSlug(s string) bool { return slugRe.MatchString(s) }

// IsEmail valida sintácticamente un correo y acota su longitud (RFC 5321: 254).
func IsEmail(s string) bool {
	if s == "" || len(s) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}
	// mail.ParseAddress admite "Nombre <a@b>"; exigimos la dirección desnuda.
	return addr.Address == s
}

// HasControlChars indica si s contiene caracteres de control no permitidos
// (se toleran \t, \n, \r que son legítimos en texto multilínea).
func HasControlChars(s string) bool {
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// SanitizeText normaliza texto de una línea: recorta espacios de los extremos,
// elimina caracteres de control (incluidos los invisibles de dirección BiDi) y
// colapsa espacios internos. No es una defensa anti-XSS (eso es escapado en la
// capa de presentación), sino normalización de entrada.
func SanitizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case unicode.IsControl(r), isBiDiOrZeroWidth(r):
			// descartar
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// TruncateRunes recorta s a como mucho n runes (sin partir un rune).
func TruncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// CSVSafe neutraliza la inyección de fórmulas CSV (CWE-1236): si una celda
// empieza por un carácter que Excel, LibreOffice o Google Sheets interpretarían
// como el comienzo de una fórmula, la prefija con una comilla simple para que la
// hoja de cálculo la trate como texto. Sin esto, un valor de usuario guardado en
// la base de datos (el nombre de un contacto, la descripción de un evento) se
// ejecuta al abrir el CSV exportado: =HYPERLINK(...) exfiltra, +cmd|... llega a
// ejecutar en Windows.
//
// Es el saneado de SALIDA de toda exportación CSV de la suite (registros de
// auditoría, contactos, gobernanza). El round-trip se revierte quitando la
// comilla sólo si protege uno de estos caracteres.
func CSVSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// isBiDiOrZeroWidth detecta caracteres de control BiDi y de ancho cero usados en
// ataques de homoglifos/spoofing (Trojan Source, CVE-2021-42574). Se usan
// escapes Unicode para no incrustar caracteres invisibles en el propio fuente.
func isBiDiOrZeroWidth(r rune) bool {
	switch r {
	case 0x200B, 0x200C, 0x200D, 0xFEFF, // zero-width space/NJ/J/BOM
		0x202A, 0x202B, 0x202C, 0x202D, 0x202E, // LRE/RLE/PDF/LRO/RLO
		0x2066, 0x2067, 0x2068, 0x2069: // LRI/RLI/FSI/PDI
		return true
	}
	return false
}

// AsErrors intenta extraer *Errors de un error de validación.
func AsErrors(err error) (*Errors, bool) {
	var e *Errors
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
