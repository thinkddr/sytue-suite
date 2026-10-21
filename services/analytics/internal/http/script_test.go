// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analyticshttp_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	analyticshttp "git.sytue.eu/sytue/one/services/analytics/internal/http"
)

// prohibidas son las APIs que el script NO puede tocar. Este test las busca en
// los BYTES QUE SE SIRVEN y rompe el build si aparece alguna.
//
// Sin él, «no hace falta banner de cookies» es una promesa comercial; con él es
// una propiedad verificada del artefacto. Las huellas activas están por el mismo
// motivo: identifican el equipo sin guardar nada en él, que es la vía por la que
// un producto «sin cookies» deja de serlo sin que nadie lo note. Y screen.*
// entra porque las Directrices EDPB 2/2023 tratan leer las características del
// terminal como acceso a ese terminal.
//
// Si alguna vez hay que quitar una entrada de esta lista, no es un cambio de
// test: es un cambio del ADR 0026.
var prohibidas = []string{
	// Almacenamiento en el equipo terminal.
	"cookie", "localstorage", "sessionstorage", "indexeddb",
	"opendatabase", "cachestorage", "caches.", "storagemanager",
	// Huella activa.
	"canvas", "getcontext", "webgl", "todataurl",
	"hardwareconcurrency", "devicememory", "maxtouchpoints",
	"audiocontext", "offlineaudiocontext", "getbatteryinfo", "getbattery",
	"document.fonts", "fontface", "navigator.plugins", "mimetypes",
	"webrtc", "rtcpeerconnection", "getusermedia",
	// Características del terminal (EDPB 2/2023).
	"screen.", "screenx", "screeny", "devicepixelratio",
	"window.outerwidth", "window.innerwidth",
	"gettimezoneoffset", "resolvedoptions",
	// Identificadores estables del navegador.
	"navigator.languages", "navigator.language",
}

// TestScriptNoTocaElEquipoTerminal es la primera de las dos garantías del ADR
// 0026 que son test y no promesa.
func TestScriptNoTocaElEquipoTerminal(t *testing.T) {
	t.Parallel()
	servido := strings.ToLower(sirveScript(t, false))
	for _, api := range prohibidas {
		if strings.Contains(servido, api) {
			t.Errorf("el script servido usa (o menciona) %q: eso es acceso al equipo terminal "+
				"y tira abajo la afirmación «sin banner de cookies» del ADR 0026", api)
		}
	}
}

// TestScriptCabeEnElPresupuesto es la segunda: «ligero» es un requisito del
// encargo, no una aspiración, así que pasarse rompe el build.
func TestScriptCabeEnElPresupuesto(t *testing.T) {
	t.Parallel()
	comprimido := len(sirveScriptCrudo(t, true))
	if comprimido == 0 {
		t.Fatal("el script no se sirvió comprimido")
	}
	if comprimido > analyticshttp.MaxScriptGzip {
		t.Fatalf("el script comprimido mide %d B y el presupuesto es %d B: "+
			"o se recorta, o se cambia el presupuesto en el ADR (no a la ligera)",
			comprimido, analyticshttp.MaxScriptGzip)
	}
	t.Logf("script: %d B en claro, %d B gzip (presupuesto %d B)",
		len(analyticshttp.Script), comprimido, analyticshttp.MaxScriptGzip)
}

// El script debe seguir haciendo lo suyo: sin esto, un fichero vacío pasaría los
// dos tests anteriores con nota.
func TestScriptHaceLoQueDebe(t *testing.T) {
	t.Parallel()
	servido := sirveScript(t, false)
	imprescindibles := []string{
		"sendBeacon",           // no bloquea la navegación y sobrevive al unload
		"/_sytue/e",            // el colector, en el propio dominio
		"globalPrivacyControl", // GPC sin interruptor
		"doNotTrack",           // DNT sin interruptor
		"utm_source",           // la allowlist de query string
		"pushState",            // SPAs
		"data-sytue-event",     // eventos declarados
		"pageview",             //
	}
	for _, s := range imprescindibles {
		if !strings.Contains(servido, s) {
			t.Errorf("el script servido ya no contiene %q", s)
		}
	}
	// Las seis claves de la allowlist, ni una más.
	for _, k := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "'ref'"} {
		if !strings.Contains(servido, k) {
			t.Errorf("falta la clave %q de la allowlist de query string", k)
		}
	}
	// Y ninguna otra clave de query string: si alguien añadiera 'email' aquí,
	// el servidor la aceptaría como UTM.
	if strings.Count(servido, "utm_") != 5 {
		t.Errorf("la allowlist de UTM cambió de tamaño (utm_ aparece %d veces)", strings.Count(servido, "utm_"))
	}
}

func TestScriptSeSirveConCacheYETag(t *testing.T) {
	t.Parallel()
	rec := pideScript(t, false)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("sin ETag: el script se re-descargaría en cada navegación")
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "max-age=") {
		t.Error("sin Cache-Control")
	}
}

// TestScriptSoloSeSirveAUnHostDadoDeAlta fija el interruptor del complemento.
//
// Sytue Cloud inyecta la etiqueta en TODO el HTML que construye, así que está
// puesta siempre; lo que enciende y apaga la medición es esta respuesta. Si el
// script se sirviera a cualquier host, activar la analítica dejaría de ser un
// clic —habría que reconstruir y redesplegar la app— y, mientras tanto, cada
// visitante de un sitio que no mide descargaría y ejecutaría un script para que
// sus beacons se tiraran al llegar.
func TestScriptSoloSeSirveAUnHostDadoDeAlta(t *testing.T) {
	t.Parallel()
	router := nuevoColector(t, colectorOpts{})
	req := httptest.NewRequest(http.MethodGet, "http://no-medido.ejemplo.com/_sytue/s.js", http.NoBody)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("host sin dar de alta: GET /_sytue/s.js = %d, quiero 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sendBeacon") {
		t.Error("el 404 devolvió el script igualmente")
	}
	// El caché del 404 es lo que separa «activar tarda un minuto» de «activar
	// tarda una hora»: es el mismo valor que sirve el 200 salvo por esto.
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control del 404 = %q: activar la analítica tardaría eso en notarse", cc)
	}
}

func pideScript(t *testing.T, gz bool) *httptest.ResponseRecorder {
	t.Helper()
	router := nuevoColector(t, colectorOpts{})
	req := httptest.NewRequest(http.MethodGet, "http://sitio.ejemplo.com/_sytue/s.js", http.NoBody)
	if gz {
		req.Header.Set("Accept-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET /_sytue/s.js = %d", rec.Code)
	}
	return rec
}

// sirveScriptCrudo devuelve los bytes tal cual salen por el cable.
func sirveScriptCrudo(t *testing.T, gz bool) []byte {
	t.Helper()
	return pideScript(t, gz).Body.Bytes()
}

// sirveScript devuelve el script servido ya descomprimido si hacía falta.
func sirveScript(t *testing.T, gz bool) string {
	t.Helper()
	raw := sirveScriptCrudo(t, gz)
	if !gz {
		return string(raw)
	}
	zr, err := gzip.NewReader(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer func() { _ = zr.Close() }()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("leer gzip: %v", err)
	}
	return string(out)
}
