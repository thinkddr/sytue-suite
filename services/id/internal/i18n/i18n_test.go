// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package i18n

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCatalogoCompleto es el que evita el fallo más probable de un catálogo a
// mano: añadir una cadena y traducirla solo a algunos idiomas. La que falte cae
// al inglés en medio de una pantalla en otro idioma, y eso no se detecta
// mirando el código, solo abriendo esa pantalla en ese idioma concreto.
func TestCatalogoCompleto(t *testing.T) {
	for clave, trads := range catalogo {
		for _, l := range Locales {
			if strings.TrimSpace(trads[l]) == "" {
				t.Errorf("la clave %q no está traducida al %q", clave, l)
			}
		}
		if len(trads) != len(Locales) {
			t.Errorf("la clave %q tiene %d idiomas y hay %d soportados",
				clave, len(trads), len(Locales))
		}
	}
	if len(catalogo) == 0 {
		t.Fatal("el catálogo está vacío")
	}
}

// TestMesesCompletos: un idioma sin nombres de mes pintaría fechas en inglés en
// medio de una pantalla traducida.
func TestMesesCompletos(t *testing.T) {
	for _, l := range Locales {
		m, ok := meses[l]
		if !ok {
			t.Errorf("el idioma %q no tiene nombres de mes", l)
			continue
		}
		for i, nombre := range m {
			if strings.TrimSpace(nombre) == "" {
				t.Errorf("%q: el mes %d está vacío", l, i+1)
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	casos := map[string]string{
		"es": "es", "ES": "es", "es-ES": "es", "en-GB": "en",
		"de-AT": "de", "it": "it",
		"": "en", "ja-JP": "en", "klingon": "en",
	}
	for dentro, quiero := range casos {
		if got := Normalize(dentro); got != quiero {
			t.Errorf("Normalize(%q) = %q, quiero %q", dentro, got, quiero)
		}
	}
	// Los espacios sobran: una cookie escrita a mano o un Accept-Language mal
	// recortado no deberían dejar sin idioma a nadie.
	conEspacios := "  fr-FR  "
	if got := Normalize(conEspacios); got != "fr" {
		t.Errorf("Normalize con espacios = %q, quiero fr", got)
	}
}

// TestFrom cubre la precedencia: la elección explícita del usuario (cookie) pesa
// más que la preferencia del navegador, porque quien fijó la cookie quiere ese
// idioma aunque su sistema esté en otro.
func TestFrom(t *testing.T) {
	casos := []struct {
		nombre string
		cookie string
		accept string
		quiero string
	}{
		{"cookie manda sobre el navegador", "fr", "de-DE,de;q=0.9", "fr"},
		{"sin cookie, gana el navegador", "", "it-IT,it;q=0.9", "it"},
		{"cookie inválida, cae al navegador", "klingon", "de-DE", "de"},
		{"cookie en inglés se respeta", "en", "es-ES", "en"},
		{"sin nada, inglés", "", "", "en"},
		{"idioma no soportado en la cabecera", "", "ja-JP,ja;q=0.9", "en"},
		{"factor de calidad decide", "", "ja;q=0.9,de;q=0.8,es;q=0.3", "de"},
		{"el primero sin q gana por defecto", "", "es,fr;q=0.5", "es"},
		{"comodín se ignora", "", "*", "en"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if c.cookie != "" {
				r.AddCookie(&http.Cookie{Name: CookieName, Value: c.cookie})
			}
			if c.accept != "" {
				r.Header.Set("Accept-Language", c.accept)
			}
			if got := From(r); got != c.quiero {
				t.Fatalf("From() = %q, quiero %q", got, c.quiero)
			}
		})
	}
}

// TestTFallback: una clave sin traducir tiene que verse, no desaparecer. Una
// cadena vacía pasa por hueco de maquetación y sobrevive años.
func TestTFallback(t *testing.T) {
	if got := T("es", "clave.que.no.existe"); got != "clave.que.no.existe" {
		t.Errorf("una clave desconocida debería devolverse tal cual, dio %q", got)
	}
	if got := T("es", "login.title"); got != "Iniciar sesión" {
		t.Errorf("T(es, login.title) = %q", got)
	}
	// Idioma inexistente: cae al inglés, no a vacío.
	if got := T("klingon", "login.title"); got != "Sign in" {
		t.Errorf("un idioma desconocido debería caer al inglés, dio %q", got)
	}
}

// TestFechaDiaPrimero: el orden día-mes en los cinco idiomas, incluido el
// inglés. En un listado de accesos, «4 Aug» y «8 Apr» son días distintos y nada
// en pantalla dice qué convención se está usando.
func TestFechaDiaPrimero(t *testing.T) {
	cuando := time.Date(2026, time.August, 4, 20, 34, 0, 0, time.UTC)
	quiero := map[string]string{
		"en": "4 Aug, 20:34", "es": "4 ago, 20:34", "fr": "4 août, 20:34",
		"de": "4. Aug., 20:34", "it": "4 ago, 20:34",
	}
	for l, esperado := range quiero {
		if got := Fecha(l, cuando, true); got != esperado {
			t.Errorf("Fecha(%q) = %q, quiero %q", l, got, esperado)
		}
	}
	if got := Fecha("es", cuando, false); got != "4 ago 2026" {
		t.Errorf("Fecha sin hora = %q", got)
	}
}
