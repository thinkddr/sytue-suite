// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package plantillas

import (
	"strings"
	"testing"
)

func TestTemasCatalogo(t *testing.T) {
	temas, err := CargarTemas()
	if err != nil {
		t.Fatalf("CargarTemas() error = %v", err)
	}

	if len(temas.Palettes) < 4 {
		t.Fatalf("paletas = %d; want >= 4", len(temas.Palettes))
	}
	for _, p := range temas.Palettes {
		if p.ID == "" || p.Name == "" {
			t.Fatalf("paleta con id o nombre vacío: %#v", p)
		}
		for _, colorKey := range []string{"primary", "on_primary", "background", "text", "muted"} {
			v, ok := p.Colors[colorKey]
			if !ok || !strings.HasPrefix(v, "#") {
				t.Fatalf("paleta %q falta color válido %q: %q", p.ID, colorKey, v)
			}
		}
	}

	if len(temas.FontPairs) < 4 {
		t.Fatalf("font_pairs = %d; want >= 4", len(temas.FontPairs))
	}
	for _, fp := range temas.FontPairs {
		if fp.ID == "" || fp.Name == "" || fp.Heading == "" || fp.Body == "" {
			t.Fatalf("pareja con campos vacíos: %#v", fp)
		}
	}
}
