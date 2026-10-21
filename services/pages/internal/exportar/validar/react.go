// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrReactArchivoFaltante     = errors.New("react validador: falta archivo obligatorio")
	ErrReactJSONInvalido        = errors.New("react validador: archivo JSON malformado")
	ErrReactDependenciaInvalida = errors.New("react validador: versión de dependencia no coincide con la tabla normativa")
	ErrReactScriptExterno       = errors.New("react validador: no se permiten scripts o estilos externos de CDNs no autorizadas")
	ErrReactPagesAPIFaltante    = errors.New("react validador: src/api/pages-api.ts debe definir PAGES_BASE_URL y resolverAltcha")
)

var archivosObligatoriosReact = []string{
	"package.json",
	"tsconfig.json",
	"vite.config.ts",
	"index.html",
	"src/main.tsx",
	"src/App.tsx",
	"src/api/pages-api.ts",
	"README.md",
}

var (
	reReactExternalScript = regexp.MustCompile(`(?i)<script[^>]+src=["']https?://`)
	reReactExternalCSS    = regexp.MustCompile(`(?i)<link[^>]+href=["']https?://[^"']*\.css`)
)

// DependenciasReactEsperadas define las versiones autoritativas exactas que deben estar presentes.
type DependenciasReactEsperadas struct {
	React           string
	ReactDom        string
	Vite            string
	VitePluginReact string
	TypeScript      string
	TypesReact      string
	TypesReactDom   string
	TypesNode       string
}

// React valida la capa 1 (estructural, sin red) de una exportación a React (Vite + TS).
func React(archivos map[string][]byte, deps DependenciasReactEsperadas) error {
	// 1. Validar rutas seguras
	for ruta := range archivos {
		if err := validarRutaSegura(ruta); err != nil {
			return fmt.Errorf("%w: %s", ErrRutaPeligrosa, ruta)
		}
	}

	// 2. Comprobar archivos obligatorios
	for _, req := range archivosObligatoriosReact {
		if _, ok := archivos[req]; !ok {
			return fmt.Errorf("%w: %s", ErrReactArchivoFaltante, req)
		}
	}

	// 3. Validar JSONs
	for ruta, contenido := range archivos {
		if strings.HasSuffix(ruta, ".json") {
			var js any
			if err := json.Unmarshal(contenido, &js); err != nil {
				return fmt.Errorf("%w en %s: %w", ErrReactJSONInvalido, ruta, err)
			}
		}
	}

	// 4. Validar package.json y dependencias exactas
	if err := validarPackageJSONReact(archivos["package.json"], deps); err != nil {
		return err
	}

	// 5. Validar index.html sin scripts externos peligrosos
	if html, ok := archivos["index.html"]; ok {
		if reReactExternalScript.Match(html) || reReactExternalCSS.Match(html) {
			return fmt.Errorf("%w en index.html", ErrReactScriptExterno)
		}
	}

	// 6. Validar pages-api.ts
	apiTS := string(archivos["src/api/pages-api.ts"])
	if !strings.Contains(apiTS, "PAGES_BASE_URL") || !strings.Contains(apiTS, "resolverAltcha") {
		return ErrReactPagesAPIFaltante
	}

	return nil
}

func validarPackageJSONReact(data []byte, deps DependenciasReactEsperadas) error {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"` //nolint:tagliatelle // package.json usa camelCase
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("%w en package.json: %w", ErrReactJSONInvalido, err)
	}

	comprobar := func(mapa map[string]string, nombre, esperada string) error {
		if esperada == "" {
			return nil
		}
		actual, ok := mapa[nombre]
		if !ok {
			return fmt.Errorf("%w: falta dependencia %q", ErrReactDependenciaInvalida, nombre)
		}
		if actual != esperada {
			return fmt.Errorf("%w: %s tiene versión %q pero se esperaba exacta %q",
				ErrReactDependenciaInvalida, nombre, actual, esperada)
		}
		return nil
	}

	if err := comprobar(pkg.Dependencies, "react", deps.React); err != nil {
		return err
	}
	if err := comprobar(pkg.Dependencies, "react-dom", deps.ReactDom); err != nil {
		return err
	}
	if err := comprobar(pkg.DevDependencies, "vite", deps.Vite); err != nil {
		return err
	}
	if err := comprobar(pkg.DevDependencies, "@vitejs/plugin-react", deps.VitePluginReact); err != nil {
		return err
	}
	if err := comprobar(pkg.DevDependencies, "typescript", deps.TypeScript); err != nil {
		return err
	}
	if err := comprobar(pkg.DevDependencies, "@types/react", deps.TypesReact); err != nil {
		return err
	}
	if err := comprobar(pkg.DevDependencies, "@types/react-dom", deps.TypesReactDom); err != nil {
		return err
	}
	if err := comprobar(pkg.DevDependencies, "@types/node", deps.TypesNode); err != nil {
		return err
	}

	return nil
}
