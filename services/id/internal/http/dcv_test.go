// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import "testing"

// EL DOMINIO SALE DEL HOST PEDIDO, y normalizarlo mal rompe la validación de dos
// maneras: si sobra el puerto no encuentra el reto y la emisión falla; si se
// cuela otra cosa, se serviría el reto de un dominio desde otro y acreditaríamos
// un control no demostrado.
func TestHostSinPuerto(t *testing.T) {
	casos := map[string]string{
		"acme.sytue.eu":     "acme.sytue.eu",
		"ACME.Sytue.EU":     "acme.sytue.eu",
		"acme.sytue.eu:443": "acme.sytue.eu",
		"acme.sytue.eu.":    "acme.sytue.eu", // FQDN absoluto
		"  acme.sytue.eu  ": "acme.sytue.eu",
		"[2001:db8::1]":     "2001:db8::1",
		"[2001:db8::1]:443": "2001:db8::1",
		"":                  "",
	}
	for entra, quiero := range casos {
		if got := hostSinPuerto(entra); got != quiero {
			t.Errorf("hostSinPuerto(%q) = %q, quiero %q", entra, got, quiero)
		}
	}
}

// EL NOMBRE VA EN UNA RUTA, así que lo que no esté acotado aquí acaba componiendo
// una. La tabla lo comprueba también (migración 0061): en los dos sitios.
func TestNombreDeReto(t *testing.T) {
	validos := []string{"ABC123.txt", "sectigo-dcv", "a", "reto_2026-09.txt"}
	for _, n := range validos {
		if !nombreDeReto.MatchString(n) {
			t.Errorf("%q debería valer", n)
		}
	}
	invalidos := []string{
		"",                 // vacío
		"../../etc/passwd", // travesía
		"a/b",              // barra
		"con espacio",
		"acento-ñ",
		"a\nb", // salto de línea: partiría una cabecera
	}
	for _, n := range invalidos {
		if nombreDeReto.MatchString(n) {
			t.Errorf("%q NO debería valer", n)
		}
	}
}
