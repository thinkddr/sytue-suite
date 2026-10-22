// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// TestNormalizeLocale: lo que llega del formulario, del navegador o de un IdP
// externo por SCIM se reduce a uno de los cinco idiomas de producto. Lo que no
// se reconoce cae al inglés en vez de guardarse tal cual, porque una columna de
// idioma con un valor que ningún catálogo entiende deja la interfaz sin traducir
// y sin explicación.
func TestNormalizeLocale(t *testing.T) {
	casos := map[string]string{
		"es":                "es",
		"ES":                "es",
		"es-ES":             "es",
		"en-GB":             "en",
		"  fr-FR  ":         "fr",
		"de-AT":             "de",
		"it":                "it",
		"":                  "en",
		"pt-BR":             "en", // no soportado todavía
		"klingon":           "en",
		"es'; DROP TABLE--": "en",
	}
	for dentro, quiero := range casos {
		if got := NormalizeLocale(dentro); got != quiero {
			t.Errorf("NormalizeLocale(%q) = %q, quiero %q", dentro, got, quiero)
		}
	}
}

// TestLocaleNamesCubreTodos: cada idioma soportado necesita su nombre en su
// propio idioma, que es lo que permite volver al suyo a quien no entiende el
// que está viendo. Un idioma sin nombre saldría como opción vacía en el
// selector.
func TestLocaleNamesCubreTodos(t *testing.T) {
	for _, l := range Locales {
		if LocaleNames[l] == "" {
			t.Errorf("el idioma %q no tiene nombre en LocaleNames", l)
		}
	}
	if len(LocaleNames) != len(Locales) {
		t.Errorf("LocaleNames tiene %d entradas y Locales %d: sobra o falta alguna",
			len(LocaleNames), len(Locales))
	}
}

// TestNormalizeIP cubre el fallo que dejaba el login en 500: RemoteAddr trae la
// dirección IPv6 entre corchetes ("[::1]:54321") y esa forma es sintaxis de URL,
// no de dirección, así que la columna `inet` de Postgres la rechaza (22P02). En
// local es el caso NORMAL, porque localhost resuelve a ::1 antes que a 127.0.0.1.
func TestNormalizeIP(t *testing.T) {
	casos := []struct {
		nombre string
		dentro string
		fuera  string
	}{
		{"ipv6 con corchetes y puerto", "[::1]:54321", "::1"},
		{"ipv6 con corchetes sin puerto", "[::1]", "::1"},
		{"ipv6 desnudo", "2001:db8::1", "2001:db8::1"},
		{"ipv4 con puerto", "192.0.2.10:443", "192.0.2.10"},
		{"ipv4 sin puerto", "192.0.2.10", "192.0.2.10"},
		{"ipv6 con zona", "[fe80::1%eth0]:80", "fe80::1"},
		{"vacío", "", ""},
		// Lo que no es una IP se descarta en vez de guardarse: la cabecera
		// X-Forwarded-For la pone quien esté delante y puede traer cualquier cosa.
		{"basura", "no-soy-una-ip", ""},
		{"inyección", "'; DROP TABLE sessions--", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := NormalizeIP(c.dentro); got != c.fuera {
				t.Fatalf("NormalizeIP(%q) = %q, quiero %q", c.dentro, got, c.fuera)
			}
		})
	}
}

// TestUbicacionString: la ubicación se compone con lo que se sepa. Sin ciudad se
// da solo el país en vez de un «, España» con la coma huérfana; sin nada, cadena
// vacía, que es lo que hace que la interfaz omita el dato en lugar de pintar un
// separador suelto entre la IP y el método de acceso.
func TestUbicacionString(t *testing.T) {
	casos := []struct {
		u      Ubicacion
		quiero string
	}{
		{Ubicacion{Pais: "España", Ciudad: "Valencia"}, "Valencia, España"},
		{Ubicacion{Pais: "España"}, "España"},
		{Ubicacion{Ciudad: "Valencia"}, "Valencia"},
		{Ubicacion{}, ""},
	}
	for _, c := range casos {
		if got := c.u.String(); got != c.quiero {
			t.Errorf("Ubicacion%+v = %q, quiero %q", c.u, got, c.quiero)
		}
	}
}
