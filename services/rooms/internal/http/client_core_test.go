// SPDX-License-Identifier: Apache-2.0
package chathttp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
)

func TestClientCoreManifestAndExactRuntime(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/v1/client-core/{asset}", ClientCore)
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		return w
	}
	for _, path := range []string{"/api/v1/client-core/NOTICE", "/api/v1/client-core/unknown"} {
		if w := get(path); w.Code != http.StatusNotFound {
			t.Fatal("unlisted compiler file served", path, w.Code)
		}
	}
	if w := get("/api/v1/client-core/manifest.json?q=private"); w.Code != http.StatusBadRequest || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("compiler route accepted private arguments", w.Code, w.Header())
	}
	w := get("/api/v1/client-core/manifest.json")
	if w.Code == http.StatusServiceUnavailable {
		t.Log("generated compiler artifacts absent: explicit 503, never an empty success")
		return
	}
	if w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	var manifest struct {
		Version int    `json:"version"`
		Wasm    string `json:"wasm_sha256"`
		Runtime string `json:"runtime_sha256"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 1 || len(manifest.Wasm) != 64 || len(manifest.Runtime) != 64 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid compiler manifest", manifest, w.Header())
	}
	for name, digest := range map[string]string{"core.wasm": manifest.Wasm, "wasm_exec.js": manifest.Runtime} {
		exact := get("/api/v1/client-core/" + name + "?sha256=" + digest)
		sum := sha256.Sum256(exact.Body.Bytes())
		if exact.Code != 200 || hex.EncodeToString(sum[:]) != digest || !strings.Contains(exact.Header().Get("Cache-Control"), "immutable") {
			t.Fatal("mixed compiler artifact", name, exact.Code)
		}
		stale := get("/api/v1/client-core/" + name + "?sha256=" + strings.Repeat("0", 64))
		if stale.Code != http.StatusPreconditionFailed || stale.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("stale compiler manifest served", name, stale.Code)
		}
	}
	t.Log("embedded combined core/runtime hashes and stale-manifest rejection verified")
}

func TestMessageMetadataRejectsTermsAndUnknownBody(t *testing.T) {
	h := newHandlers(&fakeService{})
	for _, target := range []string{"/?q=never-on-server", "/?query=never-on-server"} {
		w := httptest.NewRecorder()
		h.MessageMetadata(w, req(http.MethodGet, target, "", claims(), nil))
		if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("metadata accepted search query", w.Code, w.Body.String())
		}
	}
	for _, body := range []string{`{"messages":[],"q":"private"}`, `{"messages":[{"id":"opaque","tenant":"acme","body":"private"}]}`} {
		w := httptest.NewRecorder()
		h.MessageMetadataVisible(w, req(http.MethodPost, "/", body, claims(), nil))
		if w.Code != 400 {
			t.Fatal("metadata accepted plaintext fields", w.Code, w.Body.String())
		}
	}
}

func TestClientCorePublicRouteKeepsMetadataAuthenticated(t *testing.T) {
	router := NewRouter(Deps{Verifier: &authn.Verifier{}})
	for _, path := range []string{"/api/v1/messages/metadata", "/api/v1/channels", "/api/v1/matrix/bootstrap"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if w.Code != http.StatusUnauthorized {
			t.Fatal("private route lost authentication", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/client-core/manifest.json", http.NoBody))
	if w.Code != 200 && w.Code != 503 {
		t.Fatal("compiler route unexpectedly requires a Rooms account", w.Code)
	}
}
