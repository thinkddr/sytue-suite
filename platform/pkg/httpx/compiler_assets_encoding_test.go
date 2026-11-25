// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"
)

func compilerFixtureBrotli(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := brotli.NewWriterLevel(&out, brotli.BestCompression)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func compilerFixtureGzip(t *testing.T, raw []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// Lo que el cliente hace con la respuesta: descomprimir según Content-Encoding (lo
// hace el navegador) y comparar la SHA-256 con la del manifiesto.
func compilerDecoded(t *testing.T, w *httptest.ResponseRecorder) []byte {
	t.Helper()
	var reader io.Reader = bytes.NewReader(w.Body.Bytes())
	switch w.Header().Get("Content-Encoding") {
	case "":
	case "br":
		reader = brotli.NewReader(reader)
	case "gzip":
		gz, err := gzip.NewReader(reader)
		if err != nil {
			t.Fatal(err)
		}
		reader = gz
	default:
		t.Fatalf("codificación inesperada %q", w.Header().Get("Content-Encoding"))
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCompilerAssetsServePrecompressedVariantsBoundToTheRawDigest(t *testing.T) {
	wasm := bytes.Repeat([]byte("vault-core-artifact "), 4096)
	runtime := bytes.Repeat([]byte("globalThis.Go = class {};\n"), 512)
	files := fstest.MapFS{
		"vault/core.wasm":       {Data: wasm},
		"vault/core.wasm.br":    {Data: compilerFixtureBrotli(t, wasm)},
		"vault/core.wasm.gz":    {Data: compilerFixtureGzip(t, wasm)},
		"vault/wasm_exec.js":    {Data: runtime},
		"vault/wasm_exec.js.br": {Data: compilerFixtureBrotli(t, runtime)},
		"vault/wasm_exec.js.gz": {Data: compilerFixtureGzip(t, runtime)},
	}
	a := NewCompilerAssets(files, map[string]string{"vault": "vault"})
	get := func(asset, query, acceptEncoding string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://fixture.test/api/client-core/"+asset+query, http.NoBody)
		if acceptEncoding != "" {
			r.Header.Set("Accept-Encoding", acceptEncoding)
		}
		w := httptest.NewRecorder()
		a.Serve(w, r, asset)
		return w
	}
	m := get("manifest.json", "?module=vault", "br, gzip")
	if m.Code != http.StatusOK || m.Header().Get("Content-Encoding") != "" {
		t.Fatal("manifest", m.Code, m.Header())
	}
	var manifest struct {
		ID      string `json:"module_id"`
		Wasm    string `json:"wasm_sha256"`
		Runtime string `json:"runtime_sha256"`
	}
	if json.Unmarshal(m.Body.Bytes(), &manifest) != nil || manifest.ID != "vault" {
		t.Fatal("manifest del núcleo de la bóveda", m.Body.String())
	}
	sum := sha256.Sum256(wasm)
	if manifest.Wasm != hex.EncodeToString(sum[:]) {
		t.Fatal("la huella del manifiesto no es la de los bytes sin comprimir")
	}
	casos := []struct{ accept, encoding string }{
		{"br, gzip, deflate, zstd", "br"},
		{"gzip, deflate, br;q=0.5", "gzip"},
		{"gzip", "gzip"},
		{"zstd, deflate", ""},
		{"", ""},
		{"identity", ""},
		{"br;q=0, gzip;q=0", ""},
		{"*", "br"},
		{"*;q=0.1, gzip", "gzip"},
		{"BR", "br"},
		{"br;q=nonsense, gzip", "gzip"},
	}
	for asset, digest := range map[string]string{"core.wasm": manifest.Wasm, "wasm_exec.js": manifest.Runtime} {
		etags := map[string]string{}
		for _, c := range casos {
			w := get(asset, "?module=vault&sha256="+digest, c.accept)
			if w.Code != http.StatusOK {
				t.Fatal(asset, c.accept, w.Code)
			}
			if got := w.Header().Get("Content-Encoding"); got != c.encoding {
				t.Fatalf("%s con Accept-Encoding %q: Content-Encoding %q, se esperaba %q", asset, c.accept, got, c.encoding)
			}
			if w.Header().Get("Vary") != "Accept-Encoding" {
				t.Fatal(asset, c.accept, "sin Vary: Accept-Encoding", w.Header())
			}
			if !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
				t.Fatal(asset, c.accept, "perdió la caché inmutable", w.Header())
			}
			if w.Header().Get("Content-Length") != strconv.Itoa(w.Body.Len()) {
				t.Fatal(asset, c.accept, "Content-Length no es el del cuerpo enviado")
			}
			decoded := compilerDecoded(t, w)
			got := sha256.Sum256(decoded)
			if hex.EncodeToString(got[:]) != digest {
				t.Fatalf("%s con %q: lo descomprimido no tiene la huella del manifiesto", asset, c.accept)
			}
			if c.encoding != "" && w.Body.Len() >= len(decoded) {
				t.Fatal(asset, c.accept, "la variante no es más pequeña")
			}
			if previous, ok := etags[c.encoding]; ok && previous != w.Header().Get("ETag") {
				t.Fatal("ETag distinto para la misma representación")
			}
			etags[c.encoding] = w.Header().Get("ETag")
		}
		if etags[""] == etags["br"] || etags[""] == etags["gzip"] || etags["br"] == etags["gzip"] {
			t.Fatal("un mismo ETag fuerte para representaciones distintas", etags)
		}
		// Una huella vieja sigue siendo un 412, comprima o no.
		if w := get(asset, "?module=vault&sha256="+strings.Repeat("0", 64), "br"); w.Code != http.StatusPreconditionFailed || w.Header().Get("Content-Encoding") != "" {
			t.Fatal("huella vieja servida", w.Code)
		}
	}
}

// Una variante que no devuelve los mismos bytes (de otra generación, truncada o
// corrupta) no se sirve nunca: el cliente rechazaría la huella y se quedaría sin
// núcleo. Sin variantes válidas, la respuesta es la de siempre, sin Vary.
func TestCompilerAssetsDropStaleOrCorruptVariants(t *testing.T) {
	wasm := bytes.Repeat([]byte("current-core "), 4096)
	stale := bytes.Repeat([]byte("previous-core "), 4096)
	files := fstest.MapFS{
		"core/core.wasm":       {Data: wasm},
		"core/core.wasm.br":    {Data: compilerFixtureBrotli(t, stale)},
		"core/core.wasm.gz":    {Data: []byte("not gzip at all")},
		"core/wasm_exec.js":    {Data: []byte("runtime")},
		"core/wasm_exec.js.gz": {Data: compilerFixtureGzip(t, []byte("runtime"))},
	}
	a := NewCompilerAssets(files, map[string]string{"core": "core"})
	sum := sha256.Sum256(wasm)
	r := httptest.NewRequest(http.MethodGet, "http://fixture.test/api/client-core/core.wasm?sha256="+hex.EncodeToString(sum[:]), http.NoBody)
	r.Header.Set("Accept-Encoding", "br, gzip")
	w := httptest.NewRecorder()
	a.Serve(w, r, "core.wasm")
	if w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != "" || !bytes.Equal(w.Body.Bytes(), wasm) {
		t.Fatal("se sirvió una variante que no casa con la huella", w.Code, w.Header())
	}
	if w.Header().Get("Vary") != "" {
		t.Fatal("Vary sin variantes", w.Header())
	}
	// Un fichero diminuto cuya variante «comprimida» no es más pequeña tampoco.
	r = httptest.NewRequest(http.MethodGet, "http://fixture.test/api/client-core/wasm_exec.js", http.NoBody)
	r.Header.Set("Accept-Encoding", "gzip")
	w = httptest.NewRecorder()
	a.Serve(w, r, "wasm_exec.js")
	if w.Header().Get("Content-Encoding") != "" || w.Body.String() != "runtime" {
		t.Fatal("variante mayor que el original", w.Header())
	}
}
