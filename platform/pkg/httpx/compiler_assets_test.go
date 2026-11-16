// SPDX-License-Identifier: Apache-2.0
package httpx

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

type observedCompilerFS struct {
	fs.FS
	reads *int
}

func (f observedCompilerFS) Open(name string) (fs.File, error) { *f.reads++; return f.FS.Open(name) }

func TestCompilerModulesAreAllowlistedLazyAndDigestBound(t *testing.T) {
	files := fstest.MapFS{"core/core.wasm": &fstest.MapFile{Data: []byte("core-artifact")}, "core/wasm_exec.js": &fstest.MapFile{Data: []byte("runtime")}, "office/core.wasm": &fstest.MapFile{Data: []byte("office-artifact")}, "office/wasm_exec.js": &fstest.MapFile{Data: []byte("runtime")}, "mail/core.wasm": &fstest.MapFile{Data: []byte("mail-artifact")}, "mail/wasm_exec.js": &fstest.MapFile{Data: []byte("runtime")}}
	reads := 0
	a := NewCompilerAssets(observedCompilerFS{FS: files, reads: &reads}, map[string]string{"core": "core", "office": "office", "mail": "mail"})
	get := func(query, asset string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		a.Serve(w, httptest.NewRequest(http.MethodGet, "http://fixture.test/api/client-core/"+asset+query, http.NoBody), asset)
		return w
	}
	for _, query := range []string{"?module=../../", "?module=office&module=core", "?module=", "?other=office"} {
		if w := get(query, "manifest.json"); w.Code != 400 {
			t.Fatal("invalid module reached assets", query, w.Code)
		}
	}
	if reads != 0 {
		t.Fatal("invalid module read compiler bytes")
	}
	w := get("", "manifest.json")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var manifest struct {
		ID   string `json:"module_id"`
		Wasm string `json:"wasm_sha256"`
	}
	if json.Unmarshal(w.Body.Bytes(), &manifest) != nil || manifest.ID != "core" || len(manifest.Wasm) != 64 {
		t.Fatal("core manifest binding")
	}
	coreReads := reads
	if w = get("?sha256="+manifest.Wasm, "core.wasm"); w.Code != 200 || w.Body.String() != "core-artifact" {
		t.Fatal("pinned artifact", w.Code)
	}
	if reads != coreReads {
		t.Fatal("compiler was loaded more than once")
	}
	if w = get("?module=office", "manifest.json"); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &manifest) != nil || manifest.ID != "office" {
		t.Fatal("lazy office manifest")
	}
	if reads <= coreReads {
		t.Fatal("office was not loaded lazily")
	}
	officeReads := reads
	if w = get("?module=mail", "manifest.json"); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &manifest) != nil || manifest.ID != "mail" {
		t.Fatal("lazy Mail manifest")
	}
	if reads <= officeReads {
		t.Fatal("Mail module was not loaded on demand")
	}
	if w = get("?module=mail&sha256="+manifest.Wasm, "core.wasm"); w.Code != 200 || w.Body.String() != "mail-artifact" {
		t.Fatal("Mail artifact is not pinned", w.Code)
	}
	if w = get("?module=core&sha256="+manifest.Wasm, "core.wasm"); w.Code != 412 {
		t.Fatal("cross-module digest accepted", w.Code)
	}
}
