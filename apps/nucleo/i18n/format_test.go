// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import "testing"

// ParseFixed es la puerta por la que entra el dinero que alguien TECLEA, así que se
// prueba con lo que de verdad se escribe: los dos separadores decimales, los millares de
// cada idioma y lo que no es un número. Un céntimo perdido aquí es una factura mal
// emitida, y eso no lo ve nadie hasta el cierre.
func TestParseFixedLeeLosImportesQueSeTeclean(t *testing.T) {
	casos := []struct {
		l      Locale
		texto  string
		quiere int64
		ok     bool
	}{
		{"es", "1.234,56", 123456, true}, // millar con punto, decimal con coma
		{"en", "1,234.56", 123456, true}, // y al revés
		{"es", "12,50", 1250, true},
		{"en", "12.50", 1250, true},
		{"es", "12.50", 1250, true},   // teclado numérico: el punto también hace de coma
		{"es", "1.234", 123400, true}, // tres cifras detrás: es millar, no decimal
		{"es", "1234", 123400, true},
		{"es", "0,5", 50, true},
		{"es", ",5", 50, true},
		{"es", "-12,34", -1234, true},
		{"fr", "1 234,56", 123456, true}, // el francés agrupa con espacio fino
		{"es", " 99 ", 9900, true},
		{"es", "", 0, false},
		{"es", "abc", 0, false},
		{"es", "1.23.456,78", 0, false}, // un millar de dos cifras no es un número
		{"es", "12,345", 0, false},      // la coma ES el decimal del español: tres decimales no son céntimos
		{"en", "12,345", 1234500, true}, // y en inglés la misma cadena son doce mil trescientos cuarenta y cinco
		{"es", ".", 0, false},
		{"es", "1,2,3", 0, false},
	}
	for _, c := range casos {
		n, ok := ParseFixed(c.l, c.texto, 2)
		if ok != c.ok || (ok && n != c.quiere) {
			t.Errorf("ParseFixed(%s, %q) = %d, %v; quiere %d, %v", c.l, c.texto, n, ok, c.quiere, c.ok)
		}
	}
	// Y el viaje de ida y vuelta en los 26 idiomas: lo que FormatFixed escribe,
	// ParseFixed lo tiene que leer céntimo a céntimo.
	for _, l := range Locales {
		for _, centimos := range []int64{0, 5, 99, 100, 123456, 100000000, -4250} {
			texto := FormatFixed(l, float64(centimos)/100, 2)
			if n, ok := ParseFixed(l, texto, 2); !ok || n != centimos {
				t.Errorf("%s: FormatFixed(%d céntimos) = %q, y ParseFixed devuelve %d, %v", l, centimos, texto, n, ok)
			}
		}
	}
}
