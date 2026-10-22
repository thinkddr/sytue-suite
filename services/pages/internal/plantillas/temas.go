// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed temas.json
var temasRaw []byte

// PaletaTemas define una paleta de colores con contraste accesible.
type PaletaTemas struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Colors map[string]string `json:"colors"`
}

// ParejaFuentesTemas define una pareja tipográfica de título y cuerpo.
type ParejaFuentesTemas struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Heading string `json:"heading"`
	Body    string `json:"body"`
}

// TemasCatalogo agrupa las paletas y parejas de fuentes para el panel Tema.
type TemasCatalogo struct {
	Palettes  []PaletaTemas        `json:"palettes"`
	FontPairs []ParejaFuentesTemas `json:"font_pairs"`
}

// CargarTemas devuelve el catálogo de temas embebido.
func CargarTemas() (TemasCatalogo, error) {
	var tc TemasCatalogo
	if err := json.Unmarshal(temasRaw, &tc); err != nil {
		return TemasCatalogo{}, fmt.Errorf("plantillas: decodificar temas.json: %w", err)
	}
	return tc, nil
}

// TemasRaw devuelve los bytes crudos de temas.json para respuestas HTTP directas.
func TemasRaw() []byte {
	return temasRaw
}
