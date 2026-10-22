// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// lanzar hace el POST de formulario que manda un host WOPI.
func lanzar(t *testing.T, h *Handler, wopiSrc string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/wopi/view?WOPISrc="+url.QueryEscape(wopiSrc),
		strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Desde el host, que es OTRO origen: es el caso normal, no el raro.
	r.Header.Set("Origin", "https://drive.sytue.eu")
	rec := httptest.NewRecorder()
	h.wopiLanzar("view")(rec, r)
	return rec
}

func TestPuenteEntregaElEditorSinPonerElTokenEnLaURL(t *testing.T) {
	h := &Handler{wopiHosts: []string{"drive.sytue.eu"}}
	rec := lanzar(t, h, "https://drive.sytue.eu/wopi/files/abc", url.Values{"access_token": {"t0k3n"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("código = %d, quería 200", rec.Code)
	}
	cuerpo := rec.Body.String()
	// El destino es el editor y lleva src, token y modo DETRÁS de la almohadilla:
	// lo que va tras `#` no se envía al servidor, así que no acaba en el registro
	// de accesos del ingress ni en el `Referer` de la siguiente navegación.
	i := strings.Index(cuerpo, "/wopi/editor#")
	if i < 0 {
		t.Fatalf("no entrega el editor: %s", cuerpo)
	}
	if j := strings.Index(cuerpo, "t0k3n"); j < i {
		t.Fatalf("el token aparece antes del fragmento: %s", cuerpo)
	}
	for _, k := range []string{"src=", "token=", "mode=view"} {
		if !strings.Contains(cuerpo[i:], k) {
			t.Fatalf("falta %q en el fragmento: %s", k, cuerpo[i:])
		}
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("una página con un token dentro NO se cachea: %q", rec.Header().Get("Cache-Control"))
	}
}

func TestPuenteRechazaLoQueNoDebeAbrir(t *testing.T) {
	h := &Handler{wopiHosts: []string{"drive.sytue.eu"}}

	// Un host que no está en la lista: sin esto, cualquiera podría mandar a Docs a
	// buscar un fichero a un servidor suyo y usarlo de convertidor —o de proxy—.
	rec := lanzar(t, h, "https://malo.example/wopi/files/abc", url.Values{"access_token": {"t"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("host ajeno: código = %d, quería 403", rec.Code)
	}

	// Sin token no hay nada que abrir.
	rec = lanzar(t, h, "https://drive.sytue.eu/wopi/files/abc", url.Values{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("sin token: código = %d, quería 400", rec.Code)
	}

	// Y el token SOLO en la query no vale: aceptarlo ahí sería bendecir la vía que
	// deja credenciales escritas en los registros de todo lo que haya en medio.
	r := httptest.NewRequest(http.MethodPost,
		"/wopi/view?WOPISrc=https%3A%2F%2Fdrive.sytue.eu%2Fwopi%2Ffiles%2Fabc&access_token=t0k3n", http.NoBody)
	rec = httptest.NewRecorder()
	h.wopiLanzar("view")(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("token en la query: código = %d, quería 400", rec.Code)
	}
}

// TestCSPSoloDejaEnmarcarAQuienPuedeMandarFicheros: la previsualización de Drive
// exige pintar esta pantalla DENTRO de otra página, y esa es la única excepción
// al `frame-ancestors 'none'` de la suite. Abrirla de más es clickjacking sobre
// una superficie con el fichero de alguien dentro.
func TestCSPSoloDejaEnmarcarAQuienPuedeMandarFicheros(t *testing.T) {
	got := cspEnmarcado([]string{"drive.sytue.eu", "*.socios.example"})
	quiero := "object-src 'none'; base-uri 'self'; form-action 'self'; " +
		"frame-ancestors 'self' https://drive.sytue.eu https://*.socios.example"
	if got != quiero {
		t.Fatalf("csp = %q\nquería %q", got, quiero)
	}
	// Sin hosts la respuesta es 'none' y no un comodín: una lista vacía significa
	// «esta instalación no interopera con nadie», no «con todos».
	if got := cspEnmarcado(nil); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("sin hosts debe cerrarse: %q", got)
	}
}

// La web ya no vive en el host de la API (ADR 0076): el visor es el de Docus, en su host
// y bajo su prefijo. Visto en rojo devolviendo la ruta relativa de antes.
func TestElVisorWopiEsElDeDocus(t *testing.T) {
	for pub, quiero := range map[string]string{
		"https://docs.sytue.eu":            "https://docus.sytue.eu/docus/wopi/editor",
		"https://docs-api.sytue.localhost": "https://docus.sytue.localhost/docus/wopi/editor",
		"":                                 "/docus/wopi/editor",
	} {
		if got := visorWopi(pub); got != quiero {
			t.Errorf("visorWopi(%q) = %q, quiero %q", pub, got, quiero)
		}
	}
}
