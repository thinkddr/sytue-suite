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

// Encabezados estándar de exportación de catálogo para PrestaShop (CSV).
var EncabezadosPrestaShopCSV = []string{
	"Product ID",
	"Active (0/1)",
	"Name",
	"Categories",
	"Price tax excluded",
	"Tax rules ID",
	"Wholesale price",
	"On sale (0/1)",
	"Discount amount",
	"Discount percent",
	"Discount from",
	"Discount to",
	"Reference #",
	"Supplier reference",
	"Supplier",
	"Manufacturer",
	"EAN13",
	"UPC",
	"Ecotax",
	"Width",
	"Height",
	"Depth",
	"Weight",
	"Delivery time",
	"Quantity",
	"Minimal quantity",
	"Low stock level",
	"Visibility",
	"Additional shipping cost",
	"Summary",
	"Description",
	"Tags",
	"Meta title",
	"Meta keywords",
	"Meta description",
	"Rewritten URL",
	"Text when in stock",
	"Text if backorder allowed",
	"Available for order (0/1)",
	"Show price (0/1)",
	"Image URLs",
	"Image alt texts",
	"Delete existing images (0/1)",
	"Condition",
}

// CatalogoPrestaShop genera un archivo CSV listo para importar en PrestaShop.
func CatalogoPrestaShop(productos []domain.Product, idioma string) ([]byte, error) {
	if idioma == "" {
		idioma = "es"
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := w.Write(EncabezadosPrestaShopCSV); err != nil {
		return nil, fmt.Errorf("prestashop csv: error escribiendo encabezados: %w", err)
	}

	for idx, p := range productos {
		filas := generarFilasProductoPrestaShop(idx, p, idioma)
		for _, f := range filas {
			if err := w.Write(f); err != nil {
				return nil, err
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("prestashop csv: error en flush: %w", err)
	}

	return buf.Bytes(), nil
}

func generarFilasProductoPrestaShop(idx int, p domain.Product, idioma string) [][]string {
	nombre := obtenerTextoIdioma(p.Name, idioma)
	if nombre == "" {
		nombre = fmt.Sprintf("Producto %d", idx+1)
	}
	slug := obtenerTextoIdioma(p.Slug, idioma)
	if slug == "" {
		slug = fmt.Sprintf("producto-%d", idx+1)
	}
	desc := obtenerTextoIdioma(p.Description, idioma)
	categoria := p.Category
	if categoria == "" {
		categoria = "Inicio"
	}
	strImgs := make([]string, 0, len(p.Images))
	for _, img := range p.Images {
		strImgs = append(strImgs, img.String())
	}
	imagenes := strings.Join(strImgs, ",")

	if len(p.Variants) == 0 {
		return [][]string{
			{
				fmt.Sprintf("%d", idx+1),
				"1",
				nombre,
				categoria,
				"0.00",
				"1",
				"0.00",
				"0",
				"",
				"",
				"",
				"",
				fmt.Sprintf("REF-%d", idx+1),
				"",
				"",
				"",
				"",
				"",
				"0.00",
				"0",
				"0",
				"0",
				"0",
				"3-5 días",
				"100",
				"1",
				"5",
				"both",
				"0.00",
				"",
				desc,
				"",
				nombre,
				"",
				"",
				slug,
				"En existencias",
				"",
				"1",
				"1",
				imagenes,
				nombre,
				"0",
				"new",
			},
		}
	}

	filas := make([][]string, 0, len(p.Variants))
	for vIdx, v := range p.Variants {
		filas = append(filas, generarFilaVariantePrestaShop(idx, vIdx, v, nombre, slug, desc, categoria, imagenes))
	}
	return filas
}

func generarFilaVariantePrestaShop(idx, vIdx int, v domain.Variant, nombre, slug, desc, categoria, imagenes string) []string {
	precio := fmt.Sprintf("%.2f", float64(v.PriceCents)/100.0)
	onSale := "0"
	var discountAmount string
	if v.CompareAtCents != nil && *v.CompareAtCents > v.PriceCents {
		onSale = "1"
		discountAmount = fmt.Sprintf("%.2f", float64(*v.CompareAtCents-v.PriceCents)/100.0)
	}

	qty := "100"
	if v.Stock != nil {
		qty = fmt.Sprintf("%d", *v.Stock)
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
		"1",
		varNombre,
		categoria,
		precio,
		"1",
		"0.00",
		onSale,
		discountAmount,
		"",
		"",
		"",
		v.SKU,
		"",
		"",
		"",
		"",
		"",
		"0.00",
		"0",
		"0",
		"0",
		"0",
		"3-5 días",
		qty,
		"1",
		"5",
		"both",
		"0.00",
		"",
		desc,
		"",
		varNombre,
		"",
		"",
		fmt.Sprintf("%s-%d", slug, vIdx+1),
		"En existencias",
		"",
		"1",
		"1",
		imagenes,
		varNombre,
		"0",
		"new",
	}
}
