// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
)

// Encabezados estándar de exportación de catálogo para Shopify (CSV).
var EncabezadosShopifyCSV = []string{
	"Handle",
	"Title",
	"Body (HTML)",
	"Vendor",
	"Product Category",
	"Type",
	"Tags",
	"Published",
	"Option1 Name",
	"Option1 Value",
	"Option2 Name",
	"Option2 Value",
	"Option3 Name",
	"Option3 Value",
	"Variant SKU",
	"Variant Grams",
	"Variant Inventory Qty",
	"Variant Inventory Policy",
	"Variant Fulfillment Service",
	"Variant Price",
	"Variant Compare At Price",
	"Variant Requires Shipping",
	"Variant Taxable",
	"Image Src",
	"Image Position",
	"Image Alt Text",
	"Status",
}

// CatalogoShopify convierte una lista de productos en un CSV compatible con Shopify Admin.
func CatalogoShopify(productos []domain.Product, idioma, vendor string) ([]byte, error) {
	if idioma == "" {
		idioma = "es"
	}
	if vendor == "" {
		vendor = "Sytue Store"
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := w.Write(EncabezadosShopifyCSV); err != nil {
		return nil, fmt.Errorf("shopify csv: error escribiendo encabezados: %w", err)
	}

	for _, p := range productos {
		dp := extraerDatosProducto(p, idioma, vendor)
		if len(p.Variants) == 0 {
			if err := w.Write(filaVariantePorDefecto(dp)); err != nil {
				return nil, err
			}
			continue
		}

		for _, v := range p.Variants {
			if err := w.Write(filaVariante(dp, v, p, idioma)); err != nil {
				return nil, err
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("shopify csv: error en flush: %w", err)
	}

	return buf.Bytes(), nil
}

type datosProductoCSV struct {
	handle    string
	title     string
	bodyHTML  string
	vendor    string
	category  string
	prodType  string
	tags      string
	published string
	status    string
	optNames  [3]string
}

func extraerDatosProducto(p domain.Product, idioma, vendor string) datosProductoCSV {
	handle := obtenerTextoIdioma(p.Slug, idioma)
	if handle == "" {
		handle = p.ID.String()
	}
	title := obtenerTextoIdioma(p.Name, idioma)
	if title == "" {
		title = "Producto sin título"
	}
	published := "TRUE"
	status := "active"
	if !p.Active {
		published = "FALSE"
		status = "draft"
	}

	var optNames [3]string
	for i := 0; i < len(p.Options) && i < 3; i++ {
		nombreOpt := obtenerTextoIdioma(p.Options[i].Name, idioma)
		if nombreOpt == "" {
			nombreOpt = p.Options[i].Key
		}
		optNames[i] = nombreOpt
	}

	return datosProductoCSV{
		handle:    handle,
		title:     title,
		bodyHTML:  obtenerTextoIdioma(p.Description, idioma),
		vendor:    vendor,
		category:  p.Category,
		prodType:  p.Category,
		tags:      p.Kind + ", " + p.Category,
		published: published,
		status:    status,
		optNames:  optNames,
	}
}

func filaVariantePorDefecto(dp datosProductoCSV) []string {
	return []string{
		dp.handle,
		dp.title,
		dp.bodyHTML,
		dp.vendor,
		dp.category,
		dp.prodType,
		dp.tags,
		dp.published,
		"Title",         // Option1 Name
		"Default Title", // Option1 Value
		"", "",          // Option2 Name, Value
		"", "", // Option3 Name, Value
		dp.handle + "-default", // Variant SKU
		"0",                    // Variant Grams
		"10",                   // Variant Inventory Qty
		"deny",                 // Variant Inventory Policy
		"manual",               // Variant Fulfillment Service
		"0.00",                 // Variant Price
		"",                     // Variant Compare At Price
		"TRUE",                 // Variant Requires Shipping
		"TRUE",                 // Variant Taxable
		"",                     // Image Src
		"",                     // Image Position
		"",                     // Image Alt Text
		dp.status,
	}
}

func filaVariante(dp datosProductoCSV, v domain.Variant, p domain.Product, idioma string) []string {
	optNames := dp.optNames
	optVals := extraerValoresOpciones(p, v, idioma)

	if optNames[0] == "" {
		optNames[0] = "Title"
		optVals[0] = "Default Title"
	}

	priceStr := fmt.Sprintf("%.2f", float64(v.PriceCents)/100.0)
	compareStr := ""
	if v.CompareAtCents != nil && *v.CompareAtCents > 0 {
		compareStr = fmt.Sprintf("%.2f", float64(*v.CompareAtCents)/100.0)
	}

	stockStr := "0"
	if v.Stock != nil {
		stockStr = fmt.Sprintf("%d", *v.Stock)
	}

	sku := v.SKU
	if sku == "" {
		sku = fmt.Sprintf("%s-%s", dp.handle, v.ID.String()[:8])
	}

	return []string{
		dp.handle,
		dp.title,
		dp.bodyHTML,
		dp.vendor,
		dp.category,
		dp.prodType,
		dp.tags,
		dp.published,
		optNames[0], optVals[0],
		optNames[1], optVals[1],
		optNames[2], optVals[2],
		sku,
		"0",
		stockStr,
		"deny",
		"manual",
		priceStr,
		compareStr,
		"TRUE",
		"TRUE",
		"",
		"",
		"",
		dp.status,
	}
}

func extraerValoresOpciones(p domain.Product, v domain.Variant, idioma string) [3]string {
	var optVals [3]string
	for i := 0; i < len(p.Options) && i < 3; i++ {
		optKey := p.Options[i].Key
		valKey := v.Options[optKey]
		label := valKey
		for _, ov := range p.Options[i].Values {
			if ov.Key == valKey {
				l := obtenerTextoIdioma(ov.Label, idioma)
				if l != "" {
					label = l
				}
				break
			}
		}
		optVals[i] = label
	}
	return optVals
}

func obtenerTextoIdioma(m map[string]string, lang string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[lang]; ok && v != "" {
		return v
	}
	if v, ok := m["es"]; ok && v != "" {
		return v
	}
	if v, ok := m["en"]; ok && v != "" {
		return v
	}
	for _, v := range m {
		if v != "" {
			return v
		}
	}
	return ""
}
