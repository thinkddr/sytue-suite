// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
)

type compilerAsset struct {
	body         []byte
	digest, mime string
	// encoded son las variantes precomprimidas por scripts/client-core-assets.sh
	// (tools/precomprimir), por Content-Encoding. Solo están las que, descomprimidas,
	// devuelven exactamente body: la huella del manifiesto es la de los bytes sin
	// comprimir, que es lo que el cliente comprueba.
	encoded map[string][]byte
}
type compilerModule struct {
	load func() map[string]compilerAsset
}

// compilerEncodings son las codificaciones que se sirven precomprimidas, en orden de
// preferencia a igual calidad: brotli 11 gana a gzip 9 en un wasm (el núcleo de la
// bóveda, 7,7 MB, queda en 1,58 MB con brotli y en 2,17 MB con gzip).
var compilerEncodings = []struct {
	name, suffix string
	reader       func(io.Reader) (io.Reader, error)
}{
	{"br", ".br", func(r io.Reader) (io.Reader, error) { return brotli.NewReader(r), nil }},
	{"gzip", ".gz", func(r io.Reader) (io.Reader, error) { return gzip.NewReader(r) }},
}

// compilerModuleMaximum es el techo de cada módulo; uno que lo pase no se sirve.
func compilerModuleMaximum(name string) int {
	switch name {
	case "core", "vault":
		// `vault` es el núcleo ligero de la bóveda (cmd/vault-wasm, ~7,7 MB) y `core`
		// el completo (cmd/pqc-wasm, ~11,4 MB).
		return 16 << 20
	case "office", "mail":
		return 64 << 20
	case "pdf":
		// El motor de PDF del estudio de la bóveda (cmd/pdf-wasm): ~17 MiB hoy.
		return 32 << 20
	default:
		panic("unlisted compiler module")
	}
}

// CompilerAssets serves fixed compiler artifacts. Roots are build-owned paths;
// requests select a literal module ID, never a filesystem path or document URL.
// Each module is loaded and hashed at most once, only when first requested.
type CompilerAssets struct{ modules map[string]compilerModule }

func NewCompilerAssets(source fs.FS, roots map[string]string) *CompilerAssets {
	out := &CompilerAssets{modules: map[string]compilerModule{}}
	for name, directory := range roots {
		maximum := compilerModuleMaximum(name)
		if !fs.ValidPath(directory) {
			panic("invalid compiler directory")
		}
		out.modules[name] = compilerModule{load: sync.OnceValue(func() map[string]compilerAsset {
			files := map[string]compilerAsset{}
			for file, mime := range map[string]string{"core.wasm": "application/wasm", "wasm_exec.js": "text/javascript; charset=utf-8", "GO-LICENSE": "text/plain; charset=utf-8"} {
				data, err := fs.ReadFile(source, directory+"/"+file)
				if err != nil || len(data) > maximum {
					continue
				}
				sum := sha256.Sum256(data)
				asset := compilerAsset{body: data, digest: hex.EncodeToString(sum[:]), mime: mime}
				if file != "GO-LICENSE" {
					asset.encoded = loadCompilerEncodings(source, directory+"/"+file, data)
				}
				files[file] = asset
			}
			return files
		})}
	}
	return out
}

// loadCompilerEncodings lee las variantes precomprimidas de un artefacto y se queda
// con las que devuelven sus mismos bytes. Una variante de otra generación (un wasm
// regenerado a mano sin pasar por el guion) se descarta: se serviría sin comprimir
// —y Traefik la comprimiría como antes— en vez de romper la huella en el cliente.
// Se descomprime una vez por módulo y proceso, la primera vez que alguien lo pide.
func loadCompilerEncodings(source fs.FS, name string, raw []byte) map[string][]byte {
	out := map[string][]byte{}
	for _, encoding := range compilerEncodings {
		compressed, err := fs.ReadFile(source, name+encoding.suffix)
		if err != nil || len(compressed) == 0 || len(compressed) >= len(raw) {
			continue
		}
		reader, err := encoding.reader(bytes.NewReader(compressed))
		if err != nil {
			continue
		}
		decoded, err := io.ReadAll(io.LimitReader(reader, int64(len(raw))+1))
		if err != nil || !bytes.Equal(decoded, raw) {
			continue
		}
		out[encoding.name] = compressed
	}
	return out
}

func (a *CompilerAssets) Serve(w http.ResponseWriter, r *http.Request, asset string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	module, selected, wanted, ok := a.resolveRequest(w, r)
	if !ok {
		return
	}
	if asset != "manifest.json" && asset != "core.wasm" && asset != "wasm_exec.js" && asset != "GO-LICENSE" {
		http.NotFound(w, r)
		return
	}
	files := selected.load()
	if asset == "manifest.json" {
		serveCompilerManifest(w, r, module, files)
		return
	}
	serveCompilerFile(w, r, files, asset, wanted)
}

func (a *CompilerAssets) resolveRequest(w http.ResponseWriter, r *http.Request) (string, compilerModule, string, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if (key != "module" && key != "sha256") || len(values) != 1 {
			http.Error(w, "invalid compiler request", http.StatusBadRequest)
			return "", compilerModule{}, "", false
		}
	}
	module := query.Get("module")
	if module == "" {
		if _, present := query["module"]; present {
			http.Error(w, "invalid compiler module", http.StatusBadRequest)
			return "", compilerModule{}, "", false
		}
		module = "core"
	}
	selected, ok := a.modules[module]
	if !ok {
		http.Error(w, "invalid compiler module", http.StatusBadRequest)
		return "", compilerModule{}, "", false
	}
	wanted := query.Get("sha256")
	if _, present := query["sha256"]; present && (len(wanted) != 64 || strings.Trim(wanted, "0123456789abcdef") != "") {
		http.Error(w, "invalid compiler digest", http.StatusBadRequest)
		return "", compilerModule{}, "", false
	}
	return module, selected, wanted, true
}

func serveCompilerManifest(w http.ResponseWriter, r *http.Request, module string, files map[string]compilerAsset) {
	wasm, wasmOK := files["core.wasm"]
	runtime, runtimeOK := files["wasm_exec.js"]
	if !wasmOK || !runtimeOK {
		http.Error(w, "client core unavailable", http.StatusServiceUnavailable)
		return
	}
	JSON(w, r, http.StatusOK, map[string]any{"version": 1, "module_id": module, "wasm_sha256": wasm.digest, "runtime_sha256": runtime.digest})
}

func serveCompilerFile(w http.ResponseWriter, r *http.Request, files map[string]compilerAsset, asset, wanted string) {
	file, ok := files[asset]
	if !ok {
		http.Error(w, "client core unavailable", http.StatusServiceUnavailable)
		return
	}
	if wanted != "" && wanted != file.digest {
		http.Error(w, "client core changed", http.StatusPreconditionFailed)
		return
	}
	body, etag := file.body, file.digest
	if len(file.encoded) > 0 {
		// La misma URL tiene varias representaciones: una caché que no mire
		// Accept-Encoding daría brotli a quien no lo entiende.
		w.Header().Set("Vary", "Accept-Encoding")
		if encoding := negotiateCompilerEncoding(r.Header.Values("Accept-Encoding"), file.encoded); encoding != "" {
			body = file.encoded[encoding]
			// Cada representación con su validador: el ETag fuerte no puede ser el
			// mismo para bytes distintos. La huella sigue siendo la del original.
			etag = file.digest + "." + encoding
			w.Header().Set("Content-Encoding", encoding)
		}
	}
	w.Header().Set("Content-Type", file.mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("ETag", strconv.Quote(etag))
	if wanted != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(body)
}

// negotiateCompilerEncoding elige, de las variantes disponibles, la de mayor
// calidad en Accept-Encoding (RFC 9110 §12.5.3); a igual calidad, brotli. `*` vale
// para las que no se nombran; `q=0` excluye. Sin cabecera, o si nada encaja, sin
// codificar.
func negotiateCompilerEncoding(header []string, available map[string][]byte) string {
	quality, wildcard := parseAcceptEncoding(header)
	best, bestQ := "", 0.0
	for _, encoding := range compilerEncodings {
		if _, ok := available[encoding.name]; !ok {
			continue
		}
		q, named := quality[encoding.name]
		if !named {
			q = wildcard
		}
		if q > bestQ {
			best, bestQ = encoding.name, q
		}
	}
	return best
}

// parseAcceptEncoding devuelve la calidad de cada codificación nombrada y la de `*`
// (−1 si no aparece). Una calidad ilegible cuenta como 0: no se adivina.
func parseAcceptEncoding(header []string) (map[string]float64, float64) {
	quality := map[string]float64{}
	wildcard := -1.0
	for _, line := range header {
		for _, part := range strings.Split(line, ",") {
			coding, params, _ := strings.Cut(part, ";")
			coding = strings.ToLower(strings.TrimSpace(coding))
			if coding == "" {
				continue
			}
			q := acceptQuality(params)
			if coding == "*" {
				wildcard = q
			} else if previous, seen := quality[coding]; !seen || q > previous {
				quality[coding] = q
			}
		}
	}
	return quality, wildcard
}

func acceptQuality(params string) float64 {
	q := 1.0
	for _, param := range strings.Split(params, ";") {
		key, value, found := strings.Cut(strings.TrimSpace(param), "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || parsed < 0 || parsed > 1 {
			parsed = 0
		}
		q = parsed
	}
	return q
}
