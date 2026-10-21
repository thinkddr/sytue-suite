// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"regexp"
	"strings"
	"testing"
)

// Lo que de verdad hay que impedir: que el valor de una cabecera escriba OTRA
// cabecera. Un `\r\n` colado ahí es inyección de respuesta, y el resto de la
// validación existe para tenerlo cerca.
func TestValidateHTTPRulesRechazaInyeccion(t *testing.T) {
	t.Parallel()
	casos := []struct {
		nombre string
		reglas HTTPRules
	}{
		{"salto de línea en el valor", HTTPRules{Headers: map[string]string{"X-A": "b\r\nSet-Cookie: robado=1"}}},
		{"nombre con dos puntos", HTTPRules{Headers: map[string]string{"X-A: b": "c"}}},
		{"cabecera del servidor", HTTPRules{Headers: map[string]string{"Content-Length": "0"}}},
		{"host", HTTPRules{Headers: map[string]string{"host": "otro.example"}}},
		{"origen sin barra", HTTPRules{Redirects: []Redirect{{From: "viejo", To: "/nuevo"}}}},
		{"comodín en medio", HTTPRules{Redirects: []Redirect{{From: "/a*/b", To: "/c"}}}},
		{"destino raro", HTTPRules{Redirects: []Redirect{{From: "/a", To: "javascript:alert(1)"}}}},
		{"código inventado", HTTPRules{Redirects: []Redirect{{From: "/a", To: "/b", Status: 418}}}},
		{"salto de línea en la redirección", HTTPRules{Redirects: []Redirect{{From: "/a", To: "/b\r\nX: y"}}}},
	}
	for _, c := range casos {
		if err := ValidateHTTPRules(c.reglas); err == nil {
			t.Errorf("%s: se aceptó una regla que no debería pasar", c.nombre)
		}
	}
}

func TestValidateHTTPRulesAceptaLoNormal(t *testing.T) {
	t.Parallel()
	err := ValidateHTTPRules(HTTPRules{
		Redirects: []Redirect{
			{From: "/viejo", To: "/nuevo"},
			{From: "/blog/*", To: "/noticias", Status: 301},
			{From: "/ir", To: "https://sytue.eu", Status: 302},
		},
		Headers: map[string]string{"X-Frame-Options": "DENY", "Referrer-Policy": "no-referrer"},
	})
	if err != nil {
		t.Fatalf("reglas corrientes rechazadas: %v", err)
	}
}

// El cliente escribe rutas, no expresiones: todo lo suyo se escapa. Sin esto, un
// punto en la ruta casaría con cualquier carácter y una redirección de «/v1.0»
// se llevaría por delante «/v1x0».
func TestRedirectRegexEscapaLoDelUsuario(t *testing.T) {
	t.Parallel()
	pattern, replacement := RedirectRegex(Redirect{From: "/v1.0/precios", To: "/precios"})
	re := regexp.MustCompile(pattern)

	if !re.MatchString("https://app.sytue.eu/v1.0/precios") {
		t.Error("la ruta exacta no casa con su propia regla")
	}
	if re.MatchString("https://app.sytue.eu/v1x0/precios") {
		t.Error("el punto se interpretó como comodín: la regex del usuario no está escapada")
	}
	if replacement != "/precios" {
		t.Errorf("reemplazo = %q", replacement)
	}
}

func TestRedirectRegexComodinConservaElResto(t *testing.T) {
	t.Parallel()
	pattern, replacement := RedirectRegex(Redirect{From: "/blog/*", To: "/noticias/"})
	re := regexp.MustCompile(pattern)

	got := re.ReplaceAllString("https://app.sytue.eu/blog/2026/agosto", replacement)
	if want := "/noticias/2026/agosto"; got != want {
		t.Errorf("con comodín: %q, quiero %q", got, want)
	}
	if !strings.Contains(pattern, "(.*)$") {
		t.Errorf("el comodín no produjo grupo de captura: %q", pattern)
	}
}

// 308 y no 301: el 301 deja que el navegador convierta un POST en GET, y eso
// aparece semanas después en un formulario que dejó de funcionar.
func TestRedirectStatusPorDefecto(t *testing.T) {
	t.Parallel()
	if got := RedirectStatus(Redirect{From: "/a", To: "/b"}); got != 308 {
		t.Errorf("código por defecto = %d, quiero 308", got)
	}
	if got := RedirectStatus(Redirect{From: "/a", To: "/b", Status: 302}); got != 302 {
		t.Errorf("código explícito = %d, quiero 302", got)
	}
}
