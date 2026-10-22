// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"bytes"
	"encoding/csv"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

func TestExportarCatalogoShopify(t *testing.T) {
	compareAt := 2999
	stockQty := 15

	productos := []domain.Product{
		{
			ID:          uuid.New(),
			Slug:        map[string]string{"es": "camiseta-sytue"},
			Name:        map[string]string{"es": "Camiseta Sytue 100% Algodón"},
			Description: map[string]string{"es": "<p>Camiseta de alta calidad con logo bordado.</p>"},
			Category:    "Ropa",
			Active:      true,
			Kind:        "general",
			Options: []domain.ProductOptionAxis{
				{
					Key:  "talla",
					Name: map[string]string{"es": "Talla"},
					Values: []domain.ProductOptionValue{
						{Key: "m", Label: map[string]string{"es": "M"}},
						{Key: "l", Label: map[string]string{"es": "L"}},
					},
				},
			},
			Variants: []domain.Variant{
				{
					ID:             uuid.New(),
					SKU:            "CAM-SYTUE-M",
					PriceCents:     2499,
					CompareAtCents: &compareAt,
					Stock:          &stockQty,
					Options:        map[string]string{"talla": "m"},
				},
				{
					ID:         uuid.New(),
					SKU:        "CAM-SYTUE-L",
					PriceCents: 2499,
					Stock:      &stockQty,
					Options:    map[string]string{"talla": "l"},
				},
			},
		},
		{
			ID:          uuid.New(),
			Slug:        map[string]string{"es": "taza-sytue"},
			Name:        map[string]string{"es": "Taza de Cerámica"},
			Description: map[string]string{"es": "<p>Taza resistente al microondas.</p>"},
			Category:    "Hogar",
			Active:      true,
			Kind:        "general",
		},
	}

	csvBytes, err := CatalogoShopify(productos, "es", "Sytue Tienda")
	if err != nil {
		t.Fatalf("CatalogoShopify falló: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(csvBytes))
	filas, err := r.ReadAll()
	if err != nil {
		t.Fatalf("error leyendo CSV generado: %v", err)
	}

	// Encabezado + 2 variantes del producto 1 + 1 fila del producto 2 = 4 filas
	if len(filas) != 4 {
		t.Fatalf("esperaba 4 filas en CSV, obtuvo %d", len(filas))
	}

	// Comprobar encabezados
	if filas[0][0] != "Handle" || filas[0][1] != "Title" || filas[0][19] != "Variant Price" {
		t.Errorf("encabezados incorrectos: %v", filas[0])
	}

	// Comprobar producto 1 variante M
	filaM := filas[1]
	if filaM[0] != "camiseta-sytue" {
		t.Errorf("esperaba handle 'camiseta-sytue', obtuvo %s", filaM[0])
	}
	if filaM[8] != "Talla" || filaM[9] != "M" {
		t.Errorf("opción incorrecta: %s=%s", filaM[8], filaM[9])
	}
	if filaM[14] != "CAM-SYTUE-M" {
		t.Errorf("SKU incorrecto: %s", filaM[14])
	}
	if filaM[19] != "24.99" {
		t.Errorf("precio incorrecto: %s", filaM[19])
	}
	if filaM[20] != "29.99" {
		t.Errorf("precio comparado incorrecto: %s", filaM[20])
	}
}
