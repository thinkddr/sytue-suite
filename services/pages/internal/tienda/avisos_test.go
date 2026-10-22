// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"testing"
)

func TestExtraerDireccionComprador(t *testing.T) {
	t.Run("mapa con pais y codigo postal", func(t *testing.T) {
		inv := map[string]any{
			"address": map[string]any{
				"country":     "es",
				"postal_code": "28001",
			},
		}
		pais, cp := extraerDireccionComprador(inv, nil)
		if pais != "ES" || cp != "28001" {
			t.Fatalf("esperado ES / 28001, obtenido %q / %q", pais, cp)
		}
	})

	t.Run("texto con codigo postal incrustado", func(t *testing.T) {
		inv := map[string]any{
			"address": "Calle Gran Vía 28, 28013 Madrid",
		}
		pais, cp := extraerDireccionComprador(inv, nil)
		if pais != "ES" || cp != "28013" {
			t.Fatalf("esperado ES / 28013, obtenido %q / %q", pais, cp)
		}
	})

	t.Run("fallback a envio y valores por defecto", func(t *testing.T) {
		ship := map[string]any{
			"cp":   "47001",
			"pais": "es",
		}
		pais, cp := extraerDireccionComprador(nil, ship)
		if pais != "ES" || cp != "47001" {
			t.Fatalf("esperado ES / 47001, obtenido %q / %q", pais, cp)
		}
	})

	t.Run("sin direccion cae a ES", func(t *testing.T) {
		pais, cp := extraerDireccionComprador(nil, nil)
		if pais != "ES" || cp != "" {
			t.Fatalf("esperado ES / vacio, obtenido %q / %q", pais, cp)
		}
	})
}
