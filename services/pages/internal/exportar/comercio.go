// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// generarComercio crea árboles importables, no un tema HTML genérico. El pago
// sigue siendo el de cada plataforma; Pages solo transporta catálogo y diseño.
func generarComercio(destino Destino, sitio *ir.Sitio, productos []domain.Product) (map[string][]byte, error) {
	idioma := sitio.Languages.Default
	if idioma == "" {
		idioma = "es"
	}
	nombre := obtenerTextoIdioma(sitio.SEO.SiteName, idioma)
	if nombre == "" {
		nombre = "Sytue Store"
	}
	primary := sitio.Theme.Colors["primary"]
	if primary == "" {
		primary = "#2563eb"
	}
	catalogo, err := catalogoComercioCSV(productos, idioma)
	if err != nil {
		return nil, err
	}

	switch destino {
	case DestinoShopware:
		icono, err := GenerarImagenPNG(512, 512, primary)
		if err != nil {
			return nil, err
		}
		v, _ := ObtenerVersion("shopware_version")
		return map[string][]byte{
			"README.md":                []byte(fmt.Sprintf("# %s para Shopware\n\nImporta `products.csv` desde Administración > Catálogo. El checkout lo gestiona Shopware.\n", nombre)),
			"composer.json":            []byte(fmt.Sprintf("{\n  \"name\": \"sytue/pages-shopware\",\n  \"require\": {\"shopware/core\": %q}\n}\n", v)),
			"src/Resources/theme.json": []byte("{\n  \"name\": \"Sytue Web\",\n  \"author\": \"Sytue\",\n  \"views\": [\"@Storefront\", \"@Plugins\", \"@SytuePages\"]\n}\n"),
			"src/Resources/views/storefront/page/content/index.html.twig": []byte(fmt.Sprintf("{%% sw_extends '@Storefront/storefront/page/content/index.html.twig' %%}\n{%% block base_main %%}<main><h1>%s</h1></main>{%% endblock %%}\n", escaparTwig(nombre))),
			"src/Resources/public/plugin.png":                             icono,
			"products.csv":                                                catalogo,
		}, nil
	case DestinoMagento:
		v, _ := ObtenerVersion("magento_version")
		return map[string][]byte{
			"README.md":                           []byte(fmt.Sprintf("# %s para Magento\n\nImporta `products.csv` desde System > Import. El checkout lo gestiona Magento.\n", nombre)),
			"composer.json":                       []byte(fmt.Sprintf("{\n  \"name\": \"sytue/pages-magento\",\n  \"require\": {\"magento/framework\": %q}\n}\n", v)),
			"registration.php":                    []byte("<?php\nuse Magento\\Framework\\Component\\ComponentRegistrar;\nComponentRegistrar::register(ComponentRegistrar::THEME, 'frontend/Sytue/pages', __DIR__);\n"),
			"theme.xml":                           []byte("<theme xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" xsi:noNamespaceSchemaLocation=\"urn:magento:framework:Config/etc/theme.xsd\"><title>Sytue Web</title><parent>Magento/blank</parent></theme>\n"),
			"Magento_Theme/layout/default.xml":    []byte("<page xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\" xsi:noNamespaceSchemaLocation=\"urn:magento:framework:View/Layout/etc/page_configuration.xsd\"><body><referenceContainer name=\"content\"><block class=\"Magento\\Framework\\View\\Element\\Template\" name=\"sytue.pages\" template=\"Magento_Theme::pages.phtml\"/></referenceContainer></body></page>\n"),
			"Magento_Theme/templates/pages.phtml": []byte(fmt.Sprintf("<main><h1>%s</h1></main>\n", escaparHTML(nombre))),
			"products.csv":                        catalogo,
		}, nil
	case DestinoBigCommerce:
		config, _ := json.MarshalIndent(map[string]any{"name": "Sytue Web", "version": "1.0.0", "meta": map[string]string{"author_name": "Sytue"}}, "", "  ")
		schema, _ := json.MarshalIndent([]map[string]any{{"name": "Sytue Web", "settings": []any{}}}, "", "  ")
		return map[string][]byte{
			"README.md":                 []byte(fmt.Sprintf("# %s para BigCommerce\n\nCarga `products.csv` en Products > Import. El checkout lo gestiona BigCommerce.\n", nombre)),
			"config.json":               append(config, '\n'),
			"schema.json":               append(schema, '\n'),
			"templates/pages/home.html": []byte(fmt.Sprintf("<main class=\"container\"><h1>%s</h1></main>\n", escaparHTML(nombre))),
			"assets/scss/theme.scss":    []byte(fmt.Sprintf(":root { --sytue-primary: %s; }\n", primary)),
			"products.csv":              catalogo,
		}, nil
	default:
		return nil, fmt.Errorf("comercio: destino inválido %q", destino)
	}
}

func catalogoComercioCSV(productos []domain.Product, idioma string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"sku", "name", "description", "category", "price", "stock", "active"}); err != nil {
		return nil, err
	}
	for i, producto := range productos {
		nombre := obtenerTextoIdioma(producto.Name, idioma)
		if nombre == "" {
			nombre = fmt.Sprintf("Producto %d", i+1)
		}
		if len(producto.Variants) == 0 {
			if err := w.Write([]string{fmt.Sprintf("SKU-%d", i+1), nombre, obtenerTextoIdioma(producto.Description, idioma), producto.Category, "0.00", "0", boolCSV(producto.Active)}); err != nil {
				return nil, err
			}
			continue
		}
		for _, variante := range producto.Variants {
			stock := ""
			if variante.Stock != nil {
				stock = fmt.Sprintf("%d", *variante.Stock-variante.Reserved)
			}
			if err := w.Write([]string{variante.SKU, nombre, obtenerTextoIdioma(producto.Description, idioma), producto.Category, fmt.Sprintf("%.2f", float64(variante.PriceCents)/100), stock, boolCSV(producto.Active)}); err != nil {
				return nil, err
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("comercio csv: %w", err)
	}
	return buf.Bytes(), nil
}

func boolCSV(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func escaparHTML(texto string) string {
	reemplazador := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")
	return reemplazador.Replace(texto)
}

func escaparTwig(texto string) string { return escaparHTML(texto) }
