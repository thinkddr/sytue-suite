// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// /e2e/passkey redirige a `return_to` en cuanto algo falla (sin sesión, passkey
// ajena…): el Location que sale por el cable nunca es otro sitio. Un `return_to`
// que no vale vuelve a «Mi cuenta».
func TestLaPasskeyNuevaNoRedirigeFueraDelIdP(t *testing.T) {
	s := &Server{cfg: Config{Env: pcfg.EnvProd, Issuer: "https://id.sytue.eu"}}
	for vuelta, quiero := range map[string]string{
		"//evil.example":                      "/account",
		`/\evil.example`:                      "/account",
		"/\t/evil.example":                    "/account",
		"https://evil.example/":               "/account",
		"https://id.sytue.eu//evil.example/x": "/account",
		"javascript:alert(1)":                 "/account",
		"/account?pk=ok":                      "/account?pk=ok",
		"https://mail.sytue.eu/mail/":         "https://mail.sytue.eu/mail/",
	} {
		r := httptest.NewRequest(http.MethodGet, "/e2e/passkey?"+url.Values{
			"cred": {"AAAA"}, "return_to": {vuelta},
		}.Encode(), http.NoBody)
		rec := httptest.NewRecorder()
		s.handleUnlockPasskeyEnroll(rec, r)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != quiero {
			t.Errorf("return_to=%q ⇒ %d Location=%q; quiero 302 %q", vuelta, rec.Code, rec.Header().Get("Location"), quiero)
		}
	}
}

// El relevo pone el destino en enlaces y lo sigue desde el guion, en el origen de
// Sytue ID: solo con una vuelta http(s) o del esquema de una app de la suite. Con
// cualquier otra (un `javascript:` que ejecutaría al pulsar «Volver a la app»), la
// redirección de siempre, sin pintar nada.
func TestElRelevoSoloPintaUnaVueltaNavegable(t *testing.T) {
	for destino, ok := range map[string]bool{
		"https://mail.sytue.eu/auth/callback?code=c&state=s": true,
		"http://127.0.0.1:8921/auth/callback?code=c":         true,
		"sytue-mail://auth/callback?code=c":                  true,
		"sytue://auth/callback?code=c":                       true,
		"javascript://mail.sytue.eu/%0aalert(1)":             false,
		"JavaScript://x/%0aalert(1)":                         false,
		"javascript:alert(1)":                                false,
		"data:text/html,<script>alert(1)</script>":           false,
		"vbscript://x/msgbox":                                false,
		"sytue-mail:/sin-autoridad":                          false,
		"https://a@mail.sytue.eu/":                           false,
		"/relativa":                                          false,
	} {
		if got := vueltaDelRelevo(destino); got != ok {
			t.Errorf("vueltaDelRelevo(%q) = %v; quiero %v", destino, got, ok)
		}
	}

	// En el flujo: la app de la suite pide el relevo, pero el destino no es
	// navegable, así que sale la redirección y no se consulta nada más (el
	// servidor de la prueba no tiene almacén: pintar el relevo lo consultaría).
	s := &Server{}
	web := domain.Client{ID: "sytue-mail", Type: domain.ClientConfidential}
	for _, destino := range []string{"javascript://mail.sytue.eu/%0aalert(1)", "data:text/html,<script>alert(1)</script>"} {
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("destino %q: intentó pintar el relevo (%v)", destino, p)
				}
			}()
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/oauth2/authorize", http.NoBody)
			s.entregarCodigo(rec, r, authzRequest{Unlock: true}, web, domain.Session{UserID: "u"}, destino)
			if rec.Code != http.StatusFound || strings.Contains(rec.Body.String(), "e2e-relay") {
				t.Errorf("destino %q: %d, quiero la redirección sin relevo", destino, rec.Code)
			}
		}()
	}
}

// Los bloques de datos de las páginas del desbloqueo salen de html/template: un
// valor de la petición no cierra el <script> ni mete marcado.
func TestLosDatosDelDesbloqueoVanEscapados(t *testing.T) {
	carga := `</script><script>alert(1)</script><!--`
	got, err := bloqueDeDatos("e2e-passkey", passkeyDatos{Sub: "u", Cred: "c", Next: "/account?x=" + carga})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "</script>") != 1 || strings.Contains(got, "<!--") || strings.Contains(got, "<script>") {
		t.Fatalf("la carga sale sin escapar: %s", got)
	}
	if !strings.HasPrefix(got, `<script type="application/json" id="e2e-passkey">`) {
		t.Fatalf("bloque inesperado: %s", got)
	}
}
