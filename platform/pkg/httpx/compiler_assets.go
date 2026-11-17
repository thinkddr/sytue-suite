// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type compilerAsset struct {
	body         []byte
	digest, mime string
}
type compilerModule struct {
	load func() map[string]compilerAsset
}

// CompilerAssets serves fixed compiler artifacts. Roots are build-owned paths;
// requests select a literal module ID, never a filesystem path or document URL.
// Each module is loaded and hashed at most once, only when first requested.
type CompilerAssets struct{ modules map[string]compilerModule }

func NewCompilerAssets(source fs.FS, roots map[string]string) *CompilerAssets {
	out := &CompilerAssets{modules: map[string]compilerModule{}}
	for name, directory := range roots {
		maximum := 0
		switch name {
		case "core":
			maximum = 16 << 20
		case "office", "mail":
			maximum = 64 << 20
		default:
			panic("unlisted compiler module")
		}
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
				files[file] = compilerAsset{body: data, digest: hex.EncodeToString(sum[:]), mime: mime}
			}
			return files
		})}
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
	serveCompilerFile(w, files, asset, wanted)
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

func serveCompilerFile(w http.ResponseWriter, files map[string]compilerAsset, asset, wanted string) {
	file, ok := files[asset]
	if !ok {
		http.Error(w, "client core unavailable", http.StatusServiceUnavailable)
		return
	}
	if wanted != "" && wanted != file.digest {
		http.Error(w, "client core changed", http.StatusPreconditionFailed)
		return
	}
	w.Header().Set("Content-Type", file.mime)
	w.Header().Set("ETag", strconv.Quote(file.digest))
	if wanted != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	_, _ = w.Write(file.body)
}
