// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "strings"

// VariantesNIF devuelve las formas con las que un NIF puede aparecer escrito en
// otra parte: en mayúsculas, sin espacios ni guiones, y también sin el prefijo
// de país del NIF-IVA. «ES B-12345678» da ["ESB12345678", "B12345678"].
//
// El prefijo solo se quita si son DOS LETRAS: un NIF español empieza por una
// letra y una cifra (B1…, X1…), así que no se confunde con él.
func VariantesNIF(nif string) []string {
	var b strings.Builder
	for _, r := range strings.ToUpper(nif) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	n := b.String()
	if n == "" {
		return nil
	}
	if len(n) > 3 && n[0] >= 'A' && n[0] <= 'Z' && n[1] >= 'A' && n[1] <= 'Z' {
		return []string{n, n[2:]}
	}
	return []string{n}
}

// MismoNIF dice si dos NIF escritos de forma distinta son el mismo.
func MismoNIF(a, b string) bool {
	for _, x := range VariantesNIF(a) {
		for _, y := range VariantesNIF(b) {
			if x == y {
				return true
			}
		}
	}
	return false
}

// miembrosUE son los Estados de la Unión: sus empresas se identifican con el
// NIF-IVA.
var miembrosUE = func() map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Fields("AT BE BG CY CZ DE DK EE ES FI FR GR HR HU IE IT LT LU LV MT NL PL PT RO SE SI SK") {
		m[p] = true
	}
	return m
}()

// MiembroUE dice si el país (ISO 3166-1 alfa-2) es de la Unión Europea.
func MiembroUE(pais string) bool { return miembrosUE[pais] }

// PrefijoIVA es el prefijo del NIF-IVA de un país de la Unión: el de su código,
// salvo Grecia, que usa EL.
func PrefijoIVA(pais string) string {
	if pais == "GR" {
		return "EL"
	}
	return pais
}
