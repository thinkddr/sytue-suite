// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNextjsArchivoFaltante     = errors.New("nextjs validador: falta archivo obligatorio")
	ErrNextjsJSONInvalido        = errors.New("nextjs validador: archivo JSON malformado")
	ErrNextjsDependenciaInvalida = errors.New("nextjs validador: versión de dependencia no coincide con la tabla normativa")
	ErrNextjsPagesAPIFaltante    = errors.New("nextjs validador: lib/pages-api.ts debe definir PAGES_BASE_URL y resolverAltcha")
)

var archivosObligatoriosNextjs = []string{
	"package.json",
	"tsconfig.json",
	"next.config.ts",
	"app/layout.tsx",
	"app/page.tsx",
	"lib/pages-api.ts",
	"README.md",
}

// DependenciasNextjsEsperadas define las versiones autoritativas exactas que deben estar presentes.
type DependenciasNextjsEsperadas struct {
	Next          string
	React         string
	ReactDom      string
	TypeScript    string
	TypesReact    string
	TypesReactDom string
	TypesNode     string
}

// Nextjs valida la capa 1 (estructural, sin red) de una exportación a Next.js (App Router).
func Nextjs(archivos map[string][]byte, deps DependenciasNextjsEsperadas) error {
	// 1. Validar rutas seguras
	for ruta := range archivos {
		if err := validarRutaSegura(ruta); err != nil {
			return fmt.Errorf("%w: %s", ErrRutaPeligrosa, ruta)
		}
	}

	// 2. Comprobar archivos obligatorios
	for _, req := range archivosObligatoriosNextjs {
		if _, ok := archivos[req]; !ok {
			return fmt.Errorf("%w: %s", ErrNextjsArchivoFaltante, req)
		}
	}

	// 3. Validar JSONs
	for ruta, contenido := range archivos {
		if strings.HasSuffix(ruta, ".json") {
			var js any
			if err := json.Unmarshal(contenido, &js); err != nil {
				return fmt.Errorf("%w en %s: %w", ErrNextjsJSONInvalido, ruta, err)
			}
		}
	}

	// 4. Validar package.json y dependencias exactas
	if err := validarPackageJSONNextjs(archivos["package.json"], deps); err != nil {
		return err
	}

	// 5. Validar lib/pages-api.ts
	apiTS := string(archivos["lib/pages-api.ts"])
	if !strings.Contains(apiTS, "PAGES_BASE_URL") || !strings.Contains(apiTS, "resolverAltcha") {
		return ErrNextjsPagesAPIFaltante
	}

	return nil
}

func validarPackageJSONNextjs(data []byte, deps DependenciasNextjsEsperadas) error {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"` //nolint:tagliatelle // package.json usa camelCase
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("%w en package.json: %w", ErrNextjsJSONInvalido, err)
	}

	comprobar := func(mapa map[string]string, nombre, esperada string) error {
		if esperada == "" {
			return nil
		}
		actual, ok := mapa[nombre]
		if !ok {
			return fmt.Errorf("%w: falta dependencia %q", ErrNextjsDependenciaInvalida, nombre)
		}
		if actual != esperada {
			return fmt.Errorf("%w: %s tiene versión %q pero se esperaba exacta %q",
				ErrNextjsDependenciaInvalida, nombre, actual, esperada)
		}
		return nil
	}

	if err := comprobar(pkg.Dependencies, "next", deps.Next); err != nil {
		return err
	}
	if err := comprobar(pkg.Dependencies, "react", deps.React); err != nil {
		return err
	}
	if err := comprobar(pkg.Dependencies, "react-dom", deps.ReactDom); err != nil {
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
