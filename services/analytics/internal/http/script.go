// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package analyticshttp

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

// Script es el script del navegador tal cual se sirve en /_sytue/s.js. Es EL
// artefacto sobre el que se verifican las dos garantías del producto —lo que no
// toca y lo que ocupa— en script_test.go.
//
// # Lo prohibido (y por qué es un test y no una promesa)
//
// «Sin cookies no hace falta banner» solo se sostiene si de verdad no se toca el
// equipo terminal del visitante. Por eso la lista no vive en una política sino
// en un test que lee ESTOS BYTES y rompe el build si aparece cualquiera de:
// document.cookie, localStorage, sessionStorage, indexedDB, la Cache API, las
// huellas activas (canvas, WebGL, fuentes, hardwareConcurrency, deviceMemory)
// y screen.*, que las Directrices EDPB 2/2023 tratan como acceso al equipo
// terminal. El tipo de dispositivo se deduce del user-agent, que el navegador
// ya envía sin que nadie se lo pregunte.
//
// # El presupuesto
//
// «Ligero» es un requisito, no una aspiración: el mismo test falla si el script
// comprimido supera MaxScriptGzip.
//
//go:embed s.js
var Script string

// MaxScriptGzip es el presupuesto de tamaño del script servido, comprimido
// (ADR 0026 §8: < 1,5 KB gzip).
const MaxScriptGzip = 1536

// scriptGzip, scriptETag y scriptLen se calculan una vez: el script es estático
// y lo pide cada visitante.
var (
	scriptGzip = gzipOnce(Script)
	scriptETag = `"` + base64.RawURLEncoding.EncodeToString(sha256Sum(Script)[:12]) + `"`
)

func sha256Sum(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func gzipOnce(s string) []byte {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil
	}
	if _, err := zw.Write([]byte(s)); err != nil {
		return nil
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

// scriptOffMaxAge es lo que se cachea el 404 de un host no medido. Corto a
// propósito: es el retardo entre pulsar «activar» en el panel y que el sitio
// empiece a contar, y un minuto es lo que se tolera sin volver a preguntar por
// cada visita de un sitio que nunca va a medir.
const scriptOffMaxAge = "public, max-age=60"

// serveScript entrega /_sytue/s.js. Se sirve desde el DOMINIO DEL PROPIO SITIO,
// así que no hay petición cross-origin, no hay CORS y el cliente no tiene que
// relajar su CSP — que es lo que obliga a hacer cualquier script de analítica
// de terceros.
//
// # Por qué un host no dado de alta recibe 404
//
// El interruptor de la analítica es ESTA respuesta, y no una etiqueta en el
// HTML del cliente. Sytue Cloud inyecta el `<script>` en todo lo que construye
// (builder.GenerateDockerfile), así que la etiqueta está siempre; lo que decide
// si se mide es si existe la fila en `analytics_sites`. Sin este 404 habría que
// elegir entre inyectar solo cuando la analítica está activa —y entonces
// activarla exigiría reconstruir y redesplegar la app— o servir el script a todo
// el mundo y descartar sus beacons después, que es tráfico y ejecución que el
// visitante paga para nada.
//
// El colector ya trataba el host desconocido así en /e (`collect`): esto solo
// adelanta la misma decisión al script, donde además se nota en el navegador.
func (c *collector) serveScript(w http.ResponseWriter, r *http.Request) {
	if _, ok := c.sites.get(r.Context(), hostOnly(r.Host)); !ok {
		w.Header().Set("Cache-Control", scriptOffMaxAge)
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	// Una hora: suficiente para no reenviarlo en cada navegación y poco para que
	// un cambio llegue el mismo día.
	h.Set("Cache-Control", "public, max-age=3600")
	h.Set("ETag", scriptETag)
	h.Set("X-Content-Type-Options", "nosniff")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, scriptETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if len(scriptGzip) > 0 && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		h.Set("Content-Length", strconv.Itoa(len(scriptGzip)))
		_, _ = w.Write(scriptGzip)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(Script)))
	_, _ = w.Write([]byte(Script))
}
