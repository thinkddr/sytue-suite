// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validation

import "strings"

// letrasDNI es la tabla de la letra de control del DNI y el NIE (resto módulo 23).
const letrasDNI = "TRWAGMYFPDXBNJZSQVHLCKE"

// NIF dice si s es un NIF español válido: DNI (8 cifras y letra), NIE (X, Y o Z,
// 7 cifras y letra), NIF especial de persona física (K, L o M, 7 cifras y letra) o
// NIF de persona jurídica (letra, 7 cifras y control). Comprueba la letra o el dígito
// de control, no que exista.
//
// Normaliza como `VariantesNIF` de SyERP: mayúsculas, sin espacios, puntos ni guiones,
// y sin el prefijo «ES» del NIF-IVA. Otro prefijo de país no es un NIF español.
func NIF(s string) bool {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	n := strings.TrimPrefix(b.String(), "ES")
	if len(n) != 9 || !soloCifras(n[1:8]) {
		return false
	}
	primera, control := n[0], n[8]
	switch {
	case primera >= '0' && primera <= '9':
		return soloCifras(n[:8]) && control == letraDNI(n[:8])
	case primera == 'X' || primera == 'Y' || primera == 'Z':
		return control == letraDNI(string("012"[strings.IndexByte("XYZ", primera)])+n[1:8])
	case primera == 'K' || primera == 'L' || primera == 'M':
		return control == letraDNI(n[1:8])
	case strings.IndexByte("ABCDEFGHJNPQRSUVW", primera) >= 0:
		return controlCIF(primera, n[1:8], control)
	}
	return false
}

// letraDNI es la letra que corresponde a un número de hasta 8 cifras.
func letraDNI(cifras string) byte {
	n := 0
	for i := range len(cifras) {
		n = n*10 + int(cifras[i]-'0')
	}
	return letrasDNI[n%23]
}

// controlCIF comprueba el control de un NIF de persona jurídica: las cifras pares se
// suman, las impares se doblan y se suman sus dígitos; el control es 10 menos la
// última cifra del total, como dígito o como letra de «JABCDEFGHI» según la forma
// jurídica (P, Q, R, S, W y N solo letra; A, B, E y H solo dígito; el resto, las dos).
func controlCIF(primera byte, cifras string, control byte) bool {
	suma := 0
	for i := range len(cifras) {
		d := int(cifras[i] - '0')
		if i%2 == 0 {
			d *= 2
			d = d/10 + d%10
		}
		suma += d
	}
	digito := (10 - suma%10) % 10
	porDigito := control == "0123456789"[digito]
	porLetra := control == "JABCDEFGHI"[digito]
	switch {
	case strings.IndexByte("PQRSWN", primera) >= 0:
		return porLetra
	case strings.IndexByte("ABEH", primera) >= 0:
		return porDigito
	}
	return porDigito || porLetra
}

func soloCifras(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
