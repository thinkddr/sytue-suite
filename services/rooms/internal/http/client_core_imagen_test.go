// SPDX-License-Identifier: Apache-2.0

//go:build imagen

package chathttp

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/go-chi/chi/v5"
)

// Lo que comprueba la compilación de la imagen: los módulos del núcleo de cliente
// se sirven, con un manifiesto cuyos hashes son los de los ficheros que se entregan,
// y en brotli y gzip cuando el navegador los acepta (descomprimidos, la misma huella). El 503 que TestClientCoreManifestAndExactRuntime acepta en desarrollo
// aquí es el fallo de la v3.20.3 (docs/operations.md#client-core-vacio). Se corre
// tras generar el núcleo:
//
//	scripts/client-core-assets.sh services/rooms/internal/http/client_core_assets
//	go test -tags imagen -run TestLaImagenSirveElNucleoDeCliente ./services/rooms/internal/http/
//
// Un módulo que pase de su límite (16 MiB core y vault, 64 MiB office y mail, 32 MiB pdf) también
// sale aquí: NewCompilerAssets lo descarta y el manifiesto vuelve a ser un 503.
func TestLaImagenSirveElNucleoDeCliente(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/v1/client-core/{asset}", ClientCore)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		return w
	}
	for _, modulo := range []string{"core", "vault", "office", "mail", "pdf"} {
		w := get("/api/v1/client-core/manifest.json?module=" + modulo)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: manifest.json → %d %q; la imagen saldría sin núcleo de cliente", modulo, w.Code, w.Body.String())
		}
		var manifest struct {
			Version int    `json:"version"`
			Module  string `json:"module_id"`
			Wasm    string `json:"wasm_sha256"`
			Runtime string `json:"runtime_sha256"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
			t.Fatal(modulo, err)
		}
		if manifest.Version != 1 || manifest.Module != modulo {
			t.Fatalf("%s: manifiesto inesperado %+v", modulo, manifest)
		}
		for fichero, digest := range map[string]string{"core.wasm": manifest.Wasm, "wasm_exec.js": manifest.Runtime} {
			ruta := "/api/v1/client-core/" + fichero + "?module=" + modulo + "&sha256=" + digest
			exacto := get(ruta)
			suma := sha256.Sum256(exacto.Body.Bytes())
			if exacto.Code != http.StatusOK || exacto.Body.Len() == 0 || hex.EncodeToString(suma[:]) != digest {
				t.Fatalf("%s/%s: %d, %d bytes; no es lo que anuncia el manifiesto", modulo, fichero, exacto.Code, exacto.Body.Len())
			}
			for _, codificacion := range []string{"br", "gzip"} {
				r := httptest.NewRequest(http.MethodGet, ruta, http.NoBody)
				r.Header.Set("Accept-Encoding", codificacion)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != codificacion {
					t.Fatalf("%s/%s: sin variante %s precomprimida (%d %v); Traefik volvería a comprimirlo en cada descarga", modulo, fichero, codificacion, w.Code, w.Header())
				}
				var lector io.Reader = brotli.NewReader(w.Body)
				if codificacion == "gzip" {
					gz, err := gzip.NewReader(w.Body)
					if err != nil {
						t.Fatal(modulo, fichero, err)
					}
					lector = gz
				}
				crudo, err := io.ReadAll(lector)
				suma := sha256.Sum256(crudo)
				if err != nil || hex.EncodeToString(suma[:]) != digest {
					t.Fatalf("%s/%s en %s: descomprimido no tiene la huella del manifiesto", modulo, fichero, codificacion)
				}
			}
		}
	}
}
