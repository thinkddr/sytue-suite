// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package validation ofrece validación y saneado de entrada reutilizable por
// todos los servicios Go de Sytue One. deny-by-default: los helpers rechazan lo
// no explícitamente permitido (longitudes, formatos, caracteres de control).
//
// La implementación vive en `apps/nucleo/validation`, que comparten los servicios y
// los clientes (el ERP de Sytue Office valida con las mismas reglas que services/syerp,
// PLAN-EDICION-LOCAL §6.13 E-01). Aquí se reexporta tal cual: los tipos son alias, así
// que `errors.As(err, &*validation.Errors)` sigue reconociendo lo de un lado y del otro.
//
// No sustituye a las consultas parametrizadas de pkg/pg (el saneado NO es una
// defensa anti-inyección: la parametrización lo es); su cometido es garantizar
// datos bien formados, acotados y sin PII innecesaria antes de persistir.
package validation

import devicevalidation "git.sytue.eu/sytue/one/apps/nucleo/validation"

// FieldError describe un error de validación asociado a un campo concreto.
type FieldError = devicevalidation.FieldError

// Errors agrupa errores por campo; implementa error y se serializa como los
// detalles estructurados del sobre de error de httpx.
type Errors = devicevalidation.Errors

// Validator acumula errores por campo (patrón builder). Encadena checks y al
// final se consulta OrNil().
type Validator = devicevalidation.Validator

// New crea un Validator vacío.
func New() *Validator { return devicevalidation.New() }

// IsSlug indica si s es un slug DNS-compatible.
func IsSlug(s string) bool { return devicevalidation.IsSlug(s) }

// IsEmail valida sintácticamente un correo y acota su longitud (RFC 5321: 254).
func IsEmail(s string) bool { return devicevalidation.IsEmail(s) }

// HasControlChars indica si s contiene caracteres de control no permitidos
// (se toleran \t, \n, \r que son legítimos en texto multilínea).
func HasControlChars(s string) bool { return devicevalidation.HasControlChars(s) }

// SanitizeText normaliza texto de una línea: recorta, quita caracteres de control y
// de dirección BiDi y colapsa espacios internos.
func SanitizeText(s string) string { return devicevalidation.SanitizeText(s) }

// TruncateRunes recorta s a como mucho n runes (sin partir un rune).
func TruncateRunes(s string, n int) string { return devicevalidation.TruncateRunes(s, n) }

// CSVSafe shares the device-side export guard with all backend consumers.
func CSVSafe(s string) string { return devicevalidation.CSVSafe(s) }

// AsErrors intenta extraer *Errors de un error de validación.
func AsErrors(err error) (*Errors, bool) { return devicevalidation.AsErrors(err) }
