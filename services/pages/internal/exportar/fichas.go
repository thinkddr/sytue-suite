// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"embed"
	"fmt"
)

//go:embed fichas/*.md
var fichasFS embed.FS

// ObtenerFicha lee la ficha técnica de un destino y sustituye los marcadores {{v.<clave>}}.
func ObtenerFicha(destino string) (string, error) {
	nombre := fmt.Sprintf("fichas/%s.md", destino)
	b, err := fichasFS.ReadFile(nombre)
	if err != nil {
		return "", fmt.Errorf("fichas: destino '%s' no encontrado: %w", destino, err)
	}
	return ReemplazarMarcadores(string(b))
}

// ObtenerFichaCruda lee la ficha técnica sin sustituir marcadores (para tests de verificación).
func ObtenerFichaCruda(destino string) (string, error) {
	nombre := fmt.Sprintf("fichas/%s.md", destino)
	b, err := fichasFS.ReadFile(nombre)
	if err != nil {
		return "", fmt.Errorf("fichas: destino '%s' no encontrado: %w", destino, err)
	}
	return string(b), nil
}

// ListarFichas devuelve la lista de nombres de fichas disponibles.
func ListarFichas() ([]string, error) {
	entradas, err := fichasFS.ReadDir("fichas")
	if err != nil {
		return nil, err
	}
	var res []string
	for _, e := range entradas {
		if !e.IsDir() {
			res = append(res, e.Name())
		}
	}
	return res, nil
}
