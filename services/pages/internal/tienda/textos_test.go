// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tienda

import (
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/pages/internal/textos"
)

var idiomasOficiales26 = []string{
	"bg", "cs", "da", "de", "el", "en", "es", "et", "fi", "fr",
	"ga", "hr", "hu", "is", "it", "lt", "lv", "mt", "nb", "nl",
	"pl", "pt", "ro", "sk", "sl", "sv",
}

func TestCatalogoTienda_TieneLos26Idiomas(t *testing.T) {
	c, err := textos.Cargar(textosFS, "textos")
	if err != nil {
		t.Fatalf("error al cargar el catálogo de textos de tienda: %v", err)
	}

	idiomas := c.Idiomas()
	if len(idiomas) != 26 {
		t.Fatalf("se esperaban 26 idiomas en el catálogo de tienda, hay %d: %v", len(idiomas), idiomas)
	}

	for _, id := range idiomasOficiales26 {
		if !slices.Contains(idiomas, id) {
			t.Errorf("falta el idioma oficial %q en el catálogo de tienda", id)
		}
	}

	// Comprobar que Texto sustituye correctamente las variables
	datos := map[string]string{"number": "P-000001", "total": "29,10 EUR"}
	for _, id := range idiomasOficiales26 {
		asunto := c.Texto(id, "order_paid_buyer_subject", datos)
		if asunto == "" || asunto == "order_paid_buyer_subject" {
			t.Errorf("asunto vacío o no traducido para %q", id)
		}
	}
}
