// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNegotiateLanguage(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"cabecera ausente", "", "en"},
		{"exacto", "fr", "fr"},
		{"mayúsculas", "DE", "de"},
		{"región cae a la primaria", "fr-CA", "fr"},
		{"región con caso mixto", "es-419", "es"},
		{"q ausente vale 1 y gana al q explícito", "de, fr;q=0.9", "de"},
		{"ordena por q, no por aparición", "fr;q=0.2, it;q=0.9, de;q=0.5", "it"},
		{"a igual q gana el primero", "it;q=0.5, de;q=0.5", "it"},
		{"no soportado cae al siguiente candidato", "pt-BR, fr;q=0.5", "fr"},
		{"todos no soportados → base", "pt-BR, ja, zh-Hans", "en"},
		{"q=0 descarta ese idioma", "fr;q=0, it;q=0.1", "it"},
		{"q=0 en el único candidato → base", "es;q=0", "en"},
		{"comodín → base", "*", "en"},
		{"comodín tras uno no soportado", "pt-BR;q=0.9, *;q=0.1", "en"},
		{"comodín no gana a un soportado con más q", "*;q=0.1, it;q=0.8", "it"},
		{"espacios y parámetros extra", " it ; charset=utf-8 ; q=0.7 , de;q=0.6", "it"},
		{"q en mayúscula", "de;Q=0.1, es;q=0.2", "es"},
		{"basura entera → base", "???, ;;;, ,,,", "en"},
		{"basura mezclada con válido", "!!!, fr", "fr"},
		{"q inválido descarta el elemento", "fr;q=alto, de", "de"},
		{"q fuera de rango descarta el elemento", "fr;q=9, de", "de"},
		{"subetiqueta demasiado larga", "abcdefghij, es", "es"},
		{"solo comas", ",,,", "en"},
		{"cabecera real de navegador", "es-ES,es;q=0.9,en;q=0.8,fr;q=0.7", "es"},
		{"cabecera real con base preferida", "en-GB,en;q=0.9,es;q=0.8", "en"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NegotiateLanguage(tc.header); got != tc.want {
				t.Fatalf("NegotiateLanguage(%q) = %q, quiero %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestNegotiateLanguageEsPura(t *testing.T) {
	// Misma entrada, misma salida, sin estado compartido entre llamadas.
	const header = "pt-BR;q=0.9, it;q=0.8, fr;q=0.85"
	first := NegotiateLanguage(header)
	for range 10 {
		if got := NegotiateLanguage(header); got != first {
			t.Fatalf("resultado inestable: %q vs %q", got, first)
		}
	}
	if first != "fr" {
		t.Fatalf("= %q, quiero fr", first)
	}
}

func TestLanguageFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if got := LanguageFromRequest(req); got != BaseLanguage {
		t.Fatalf("sin cabecera = %q, quiero %q", got, BaseLanguage)
	}
	req.Header.Set("Accept-Language", "de-AT;q=0.9, en;q=0.4")
	if got := LanguageFromRequest(req); got != "de" {
		t.Fatalf("= %q, quiero de", got)
	}
	if got := LanguageFromRequest(nil); got != BaseLanguage {
		t.Fatalf("petición nil = %q, quiero %q", got, BaseLanguage)
	}
}

func TestLanguagesEsCopiaConBasePrimero(t *testing.T) {
	langs := Languages()
	if len(langs) != 5 || langs[0] != BaseLanguage {
		t.Fatalf("Languages() = %v", langs)
	}
	langs[0] = "xx"
	if Languages()[0] != BaseLanguage {
		t.Fatal("Languages() expone el slice interno")
	}
}
