// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

const maxBytesFuentes = 6 << 20

type catalogoFuentes struct {
	Families []familiaFuente `json:"families"`
}

type familiaFuente struct {
	ID      string   `json:"id"`
	License string   `json:"license"`
	Source  string   `json:"source"`
	Scripts []string `json:"scripts"`
}

func TestCatalogoFuentesOFLCompletoYAcotado(t *testing.T) {
	data, err := os.ReadFile("fuentes.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalogo catalogoFuentes
	if err := json.Unmarshal(data, &catalogo); err != nil {
		t.Fatal(err)
	}
	esperadas := []string{"inter", "figtree", "source-serif-4", "lora", "playfair-display", "dm-sans", "work-sans", "manrope", "ibm-plex-sans", "noto-sans", "noto-serif", "montserrat"}
	if len(catalogo.Families) != len(esperadas) {
		t.Fatalf("familias = %d, want %d", len(catalogo.Families), len(esperadas))
	}
	var total int64
	for _, fuente := range catalogo.Families {
		if !slices.Contains(esperadas, fuente.ID) || fuente.License != "OFL-1.1" || fuente.Source == "" || !slices.Contains(fuente.Scripts, "latin") {
			t.Fatalf("manifiesto inválido para %q: %#v", fuente.ID, fuente)
		}
		for _, fichero := range append(append([]string(nil), fuente.Scripts...), "OFL") {
			extension := ".woff2"
			if fichero == "OFL" {
				fichero, extension = "OFL", ".txt"
			}
			info, err := os.Stat(filepath.Join("fuentes", fuente.ID, fichero+extension))
			if err != nil || info.Size() == 0 {
				t.Fatalf("%s: %v", filepath.Join(fuente.ID, fichero+extension), err)
			}
			total += info.Size()
		}
	}
	if total > maxBytesFuentes {
		t.Fatalf("catálogo de fuentes ocupa %d bytes; máximo %d", total, maxBytesFuentes)
	}
}
