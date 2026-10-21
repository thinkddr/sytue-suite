// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginLosColoresSonLosDeBranding(t *testing.T) {
	canon, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "branding", "tokens", "colors.css"))
	if err != nil {
		t.Fatal(err)
	}
	if coloresCSS != string(canon) {
		t.Fatal("colores.css no coincide con branding/tokens/colors.css; ejecutar scripts/sync-branding.sh")
	}
	if !strings.HasPrefix(loginCSS, string(canon)) {
		t.Fatal("el login no sirve los tokens canónicos de la suite")
	}
}

// La hoja ahora contiene los tokens vendorizados además de las reglas del login.
// Si la CSP siguiera calculando el hash de la hoja anterior, el navegador ocultaría
// todos los estilos aunque la igualdad de los ficheros estuviera en verde.
func TestLoginLaCSPAutorizaLaHojaServida(t *testing.T) {
	rec := httptest.NewRecorder()
	h := securityHeaders(Config{Env: "prod"})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(loginPage("es", "Entrar", loginFormHTML("es", "", "", "", false))))
	}))
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login/", http.NoBody))

	_, despues, ok := strings.Cut(rec.Body.String(), "<style>")
	if !ok {
		t.Fatal("la página no declara su hoja de estilos")
	}
	css, _, ok := strings.Cut(despues, "</style>")
	if !ok {
		t.Fatal("la hoja de estilos no se cierra")
	}
	sum := sha256.Sum256([]byte(css))
	quiero := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	for _, directiva := range strings.Split(rec.Header().Get("Content-Security-Policy"), ";") {
		if !strings.HasPrefix(strings.TrimSpace(directiva), "style-src ") {
			continue
		}
		if !strings.Contains(directiva, quiero) {
			t.Fatal("style-src no autoriza el CSS realmente servido por el login")
		}
		if strings.Contains(directiva, "'unsafe-inline'") {
			t.Fatal("la CSP debe autorizar la hoja por hash, sin unsafe-inline")
		}
		return
	}
	t.Fatal("la CSP no declara style-src")
}

// El inicializador debe ejecutarse antes de pintar y estar autorizado por la CSP.
func TestLoginElTemaCompartidoEstaAutorizado(t *testing.T) {
	html := loginPage("es", "Entrar", "")
	if !strings.Contains(html, "<script>"+themeInitJS+"</script></head>") {
		t.Fatal("falta el tema compartido antes del cuerpo")
	}
	sum := sha256.Sum256([]byte(themeInitJS))
	hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(cspScriptHashes, hash) {
		t.Fatal("la CSP bloquea el tema inicial")
	}
	if !strings.Contains(themeInitJS, "translucent") {
		t.Fatal("el IdP no conserva el tercer tema")
	}
}

func TestFontSirveLaLicenciaYRechazaOtrosRecursos(t *testing.T) {
	for _, caso := range []struct {
		ruta   string
		codigo int
		mime   string
	}{
		{"/fonts/Figtree-Regular.ttf", 200, "font/ttf"},
		{"/fonts/OFL.txt", 200, "text/plain; charset=utf-8"},
		{"/fonts/../login.go", 404, ""},
	} {
		rec := httptest.NewRecorder()
		handleFont(rec, httptest.NewRequest(http.MethodGet, caso.ruta, http.NoBody))
		if rec.Code != caso.codigo {
			t.Fatalf("%s: estado %d", caso.ruta, rec.Code)
		}
		if caso.mime != "" && rec.Header().Get("Content-Type") != caso.mime {
			t.Fatalf("%s: tipo incorrecto", caso.ruta)
		}
		if caso.ruta == "/fonts/OFL.txt" && !strings.Contains(rec.Body.String(), "SIL OPEN FONT LICENSE") {
			t.Fatal("no se distribuye la licencia de la fuente")
		}
	}
}
