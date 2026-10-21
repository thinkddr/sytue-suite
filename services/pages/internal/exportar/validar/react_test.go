// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"errors"
	"testing"
)

func archivosReactBase() map[string][]byte {
	return map[string][]byte{
		"package.json": []byte(`{
  "name": "sytue-pages-react",
  "dependencies": {
    "react": "19.0.0",
    "react-dom": "19.0.0"
  },
  "devDependencies": {
    "vite": "6.2.0",
    "@vitejs/plugin-react": "4.3.4",
    "typescript": "5.7.3",
    "@types/react": "19.0.10",
    "@types/react-dom": "19.0.4",
    "@types/node": "22.13.5"
  }
}`),
		"tsconfig.json":        []byte(`{"compilerOptions":{}}`),
		"vite.config.ts":       []byte(`export default {};`),
		"index.html":           []byte(`<!DOCTYPE html><html><body><div id="root"></div></body></html>`),
		"src/main.tsx":         []byte(`console.log("main");`),
		"src/App.tsx":          []byte(`export default function App() { return null; }`),
		"src/api/pages-api.ts": []byte(`export const PAGES_BASE_URL = "https://ejemplo.sytue.eu"; export async function resolverAltcha() {}`),
		"README.md":            []byte(`# React`),
	}
}

func depsReactBase() DependenciasReactEsperadas {
	return DependenciasReactEsperadas{
		React:           "19.0.0",
		ReactDom:        "19.0.0",
		Vite:            "6.2.0",
		VitePluginReact: "4.3.4",
		TypeScript:      "5.7.3",
		TypesReact:      "19.0.10",
		TypesReactDom:   "19.0.4",
		TypesNode:       "22.13.5",
	}
}

func TestReactValido(t *testing.T) {
	archivos := archivosReactBase()
	if err := React(archivos, depsReactBase()); err != nil {
		t.Fatalf("React devolvió error inesperado: %v", err)
	}
}

func TestReactArchivoFaltante(t *testing.T) {
	archivos := archivosReactBase()
	delete(archivos, "src/api/pages-api.ts")

	err := React(archivos, depsReactBase())
	if !errors.Is(err, ErrReactArchivoFaltante) {
		t.Fatalf("esperaba ErrReactArchivoFaltante, obtuvo: %v", err)
	}
}

func TestReactDependenciaInvalida(t *testing.T) {
	archivos := archivosReactBase()
	archivos["package.json"] = []byte(`{
  "dependencies": {
    "react": "18.2.0",
    "react-dom": "19.0.0"
  }
}`)

	err := React(archivos, depsReactBase())
	if !errors.Is(err, ErrReactDependenciaInvalida) {
		t.Fatalf("esperaba ErrReactDependenciaInvalida, obtuvo: %v", err)
	}
}

func TestReactScriptExterno(t *testing.T) {
	archivos := archivosReactBase()
	archivos["index.html"] = []byte(`<html><head><script src="https://cdn.malicioso.com/tracker.js"></script></head></html>`)

	err := React(archivos, depsReactBase())
	if !errors.Is(err, ErrReactScriptExterno) {
		t.Fatalf("esperaba ErrReactScriptExterno, obtuvo: %v", err)
	}
}

func TestReactPagesAPIFaltante(t *testing.T) {
	archivos := archivosReactBase()
	archivos["src/api/pages-api.ts"] = []byte(`console.log("vacio");`)

	err := React(archivos, depsReactBase())
	if !errors.Is(err, ErrReactPagesAPIFaltante) {
		t.Fatalf("esperaba ErrReactPagesAPIFaltante, obtuvo: %v", err)
	}
}
