// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"errors"
	"testing"
)

func archivosNextjsBase() map[string][]byte {
	return map[string][]byte{
		"package.json": []byte(`{
  "name": "sytue-pages-nextjs",
  "dependencies": {
    "next": "15.2.0",
    "react": "19.0.0",
    "react-dom": "19.0.0"
  },
  "devDependencies": {
    "typescript": "5.7.3",
    "@types/react": "19.0.10",
    "@types/react-dom": "19.0.4",
    "@types/node": "22.13.5"
  }
}`),
		"tsconfig.json":    []byte(`{"compilerOptions":{}}`),
		"next.config.ts":   []byte(`export default {};`),
		"app/layout.tsx":   []byte(`export default function Root({children}) { return <html><body>{children}</body></html>; }`),
		"app/page.tsx":     []byte(`export default function Page() { return <h1>Home</h1>; }`),
		"lib/pages-api.ts": []byte(`export const PAGES_BASE_URL = "https://ejemplo.sytue.eu"; export async function resolverAltcha() {}`),
		"README.md":        []byte(`# Next.js`),
	}
}

func depsNextjsBase() DependenciasNextjsEsperadas {
	return DependenciasNextjsEsperadas{
		Next:          "15.2.0",
		React:         "19.0.0",
		ReactDom:      "19.0.0",
		TypeScript:    "5.7.3",
		TypesReact:    "19.0.10",
		TypesReactDom: "19.0.4",
		TypesNode:     "22.13.5",
	}
}

func TestNextjsValido(t *testing.T) {
	archivos := archivosNextjsBase()
	if err := Nextjs(archivos, depsNextjsBase()); err != nil {
		t.Fatalf("Nextjs devolvió error inesperado: %v", err)
	}
}

func TestNextjsArchivoFaltante(t *testing.T) {
	archivos := archivosNextjsBase()
	delete(archivos, "next.config.ts")

	err := Nextjs(archivos, depsNextjsBase())
	if !errors.Is(err, ErrNextjsArchivoFaltante) {
		t.Fatalf("esperaba ErrNextjsArchivoFaltante, obtuvo: %v", err)
	}
}

func TestNextjsDependenciaInvalida(t *testing.T) {
	archivos := archivosNextjsBase()
	archivos["package.json"] = []byte(`{
  "dependencies": {
    "next": "14.0.0",
    "react": "19.0.0",
    "react-dom": "19.0.0"
  }
}`)

	err := Nextjs(archivos, depsNextjsBase())
	if !errors.Is(err, ErrNextjsDependenciaInvalida) {
		t.Fatalf("esperaba ErrNextjsDependenciaInvalida, obtuvo: %v", err)
	}
}

func TestNextjsPagesAPIFaltante(t *testing.T) {
	archivos := archivosNextjsBase()
	archivos["lib/pages-api.ts"] = []byte(`// vacio`)

	err := Nextjs(archivos, depsNextjsBase())
	if !errors.Is(err, ErrNextjsPagesAPIFaltante) {
		t.Fatalf("esperaba ErrNextjsPagesAPIFaltante, obtuvo: %v", err)
	}
}
