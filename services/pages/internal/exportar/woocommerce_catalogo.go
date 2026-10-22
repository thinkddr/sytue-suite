// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// Encabezados estándar de exportación de catálogo para WooCommerce (CSV).
var EncabezadosWooCommerceCSV = []string{
	"ID",
	"Type",
	"SKU",
	"Name",
	"Published",
	"Is featured?",
	"Visibility in catalog",
	"Short description",
	"Description",
	"In stock?",
	"Stock",
	"Regular price",
	"Sale price",
	"Categories",
	"Images",
}

// CatalogoWooCommerce convierte una lista de productos en un CSV compatible con WooCommerce Importer.
func CatalogoWooCommerce(productos []domain.Product, idioma string) ([]byte, error) {
	if idioma == "" {
		idioma = "es"
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := w.Write(EncabezadosWooCommerceCSV); err != nil {
		return nil, fmt.Errorf("woocommerce csv: error escribiendo encabezados: %w", err)
	}

	for idx, p := range productos {
		filas := generarFilasProductoWooCommerce(idx, p, idioma)
		for _, f := range filas {
			if err := w.Write(f); err != nil {
				return nil, err
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("woocommerce csv: error en flush: %w", err)
	}

	return buf.Bytes(), nil
}

func generarFilasProductoWooCommerce(idx int, p domain.Product, idioma string) [][]string {
	nombre := obtenerTextoIdioma(p.Name, idioma)
	if nombre == "" {
		nombre = fmt.Sprintf("Producto %d", idx+1)
	}
	desc := obtenerTextoIdioma(p.Description, idioma)
	categoria := p.Category
	if categoria == "" {
		categoria = "General"
	}
	strImgs := make([]string, 0, len(p.Images))
	for _, img := range p.Images {
		strImgs = append(strImgs, img.String())
	}
	imagenes := strings.Join(strImgs, ", ")

	if len(p.Variants) == 0 {
		return [][]string{
			{
				fmt.Sprintf("%d", idx+1),
				"simple",
				fmt.Sprintf("SKU-%d", idx+1),
				nombre,
				"1",
				"0",
				"visible",
				"",
				desc,
				"1",
				"100",
				"0.00",
				"",
				categoria,
				imagenes,
			},
		}
	}

	filas := make([][]string, 0, len(p.Variants))
	for vIdx, v := range p.Variants {
		filas = append(filas, generarFilaVarianteWooCommerce(idx, vIdx, v, nombre, desc, categoria, imagenes))
	}
	return filas
}

func generarFilaVarianteWooCommerce(idx, vIdx int, v domain.Variant, nombre, desc, categoria, imagenes string) []string {
	var regularPrice, salePrice string
	if v.CompareAtCents != nil && *v.CompareAtCents > v.PriceCents {
		regularPrice = fmt.Sprintf("%.2f", float64(*v.CompareAtCents)/100.0)
		salePrice = fmt.Sprintf("%.2f", float64(v.PriceCents)/100.0)
	} else {
		regularPrice = fmt.Sprintf("%.2f", float64(v.PriceCents)/100.0)
		salePrice = ""
	}

	stockQty := "100"
	inStock := "1"
	if v.Stock != nil {
		stockQty = fmt.Sprintf("%d", *v.Stock)
		if *v.Stock <= v.Reserved {
			inStock = "0"
		}
	}

	varNombre := nombre
	if len(v.Options) > 0 {
		partes := make([]string, 0, len(v.Options))
		for k, val := range v.Options {
			partes = append(partes, fmt.Sprintf("%s: %s", k, val))
		}
		varNombre = fmt.Sprintf("%s (%s)", nombre, strings.Join(partes, ", "))
	}

	return []string{
		fmt.Sprintf("%d%02d", idx+1, vIdx+1),
		"simple",
		v.SKU,
		varNombre,
		"1",
		"0",
		"visible",
		"",
		desc,
		inStock,
		stockQty,
		regularPrice,
		salePrice,
		categoria,
		imagenes,
	}
}
