// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"encoding/json"
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// GenerarCompartidosWooCommerce produce los archivos deterministas de un tema de bloques de WooCommerce (§9.4).
func GenerarCompartidosWooCommerce(irSitio *ir.Sitio, productos []domain.Product) (map[string][]byte, error) {
	archivos := make(map[string][]byte)

	vTema, _ := ObtenerVersion("woocommerce_theme_version")
	vWP, _ := ObtenerVersion("wordpress_version")
	vWoo, _ := ObtenerVersion("woocommerce_version")

	nombreSitio := "Sytue Store"
	idioma := "es"
	colorPrimario := "#2563eb"
	if irSitio != nil {
		if irSitio.Languages.Default != "" {
			idioma = irSitio.Languages.Default
		}
		if irSitio.SEO.SiteName != nil {
			if n := obtenerTextoIdioma(irSitio.SEO.SiteName, idioma); n != "" {
				nombreSitio = n
			}
		}
		if irSitio.Theme.Colors != nil {
			if c, ok := irSitio.Theme.Colors["primary"]; ok && c != "" {
				colorPrimario = c
			}
		}
	}

	// 1. theme.json (especificación versión 3 de WordPress FSE)
	themeJSON := map[string]any{
		"$schema": "https://schemas.wp.org/trunk/theme.json",
		"version": 3,
		"settings": map[string]any{
			"color": map[string]any{
				"palette": []map[string]string{
					{
						"slug":  "primary",
						"color": colorPrimario,
						"name":  "Primary",
					},
					{
						"slug":  "base",
						"color": "#ffffff",
						"name":  "Base",
					},
					{
						"slug":  "contrast",
						"color": "#18181b",
						"name":  "Contrast",
					},
				},
			},
			"layout": map[string]string{
				"contentSize": "840px",
				"wideSize":    "1200px",
			},
		},
		"styles": map[string]any{
			"color": map[string]string{
				"background": "var(--wp--preset--color--base)",
				"text":       "var(--wp--preset--color--contrast)",
			},
			"typography": map[string]string{
				"fontFamily": "system-ui, -apple-system, sans-serif",
				"lineHeight": "1.5",
			},
		},
	}
	tjBytes, err := json.MarshalIndent(themeJSON, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("woocommerce: error serializando theme.json: %w", err)
	}
	archivos["theme.json"] = append(tjBytes, '\n')

	// 2. style.css con cabeceras requeridas de WordPress
	archivos["style.css"] = []byte(fmt.Sprintf(`/*
Theme Name: %s
Theme URI: https://sytue.eu/product/web
Author: Sytue Web
Author URI: https://sytue.eu
Description: Tema de bloques FSE exportado automáticamente desde Sytue Web con soporte WooCommerce.
Version: %s
Requires at least: %s
Tested up to: %s
Requires PHP: 8.1
License: GNU General Public License v2 or later
License URI: http://www.gnu.org/licenses/gpl-2.0.html
Text Domain: sytue-store
Tags: full-site-editing, e-commerce, block-patterns
*/
`, nombreSitio, vTema, vWP, vWP))

	// 3. functions.php
	archivos["functions.php"] = []byte(`<?php
/**
 * Funciones del tema exportado desde Sytue Web.
 */

if (!defined('ABSPATH')) {
    exit;
}

function sytue_pages_setup() {
    add_theme_support('woocommerce');
    add_theme_support('block-templates');
    add_theme_support('align-wide');
    add_theme_support('wp-block-styles');
    add_theme_support('editor-styles');
}
add_action('after_setup_theme', 'sytue_pages_setup');
`)

	// 4. parts/header.html con comentarios equilibrados
	archivos["parts/header.html"] = []byte(`<!-- wp:group {"tagName":"header","layout":{"type":"flex","justifyContent":"space-between"}} -->
<header class="wp-block-group">
  <!-- wp:site-title /-->
  <!-- wp:navigation /-->
</header>
<!-- /wp:group -->
`)

	// 5. parts/footer.html con comentarios equilibrados
	archivos["parts/footer.html"] = []byte(`<!-- wp:group {"tagName":"footer","layout":{"type":"flex","justifyContent":"center"}} -->
<footer class="wp-block-group">
  <!-- wp:paragraph -->
  <p>© 2026 Sytue Web Store</p>
  <!-- /wp:paragraph -->
</footer>
<!-- /wp:group -->
`)

	// 6. templates/index.html con comentarios equilibrados
	archivos["templates/index.html"] = []byte(fmt.Sprintf(`<!-- wp:template-part {"slug":"header","tagName":"header"} /-->
<!-- wp:group {"tagName":"main","layout":{"type":"constrained"}} -->
<main class="wp-block-group">
  <!-- wp:heading {"level":1} -->
  <h1>%s</h1>
  <!-- /wp:heading -->
  <!-- wp:paragraph -->
  <p>Bienvenido a nuestra tienda WooCommerce exportada desde Sytue Web.</p>
  <!-- /wp:paragraph -->
</main>
<!-- /wp:group -->
<!-- wp:template-part {"slug":"footer","tagName":"footer"} /-->
`, nombreSitio))

	// 7. screenshot.png generado deterministamente con image/png
	pngBytes, err := GenerarImagenPNG(1200, 900, colorPrimario)
	if err != nil {
		return nil, fmt.Errorf("woocommerce: error generando screenshot.png: %w", err)
	}
	archivos["screenshot.png"] = pngBytes

	// 8. products_export_woocommerce.csv
	if len(productos) > 0 {
		csvBytes, err := CatalogoWooCommerce(productos, idioma)
		if err != nil {
			return nil, fmt.Errorf("woocommerce: error generando catálogo CSV: %w", err)
		}
		archivos["products_export_woocommerce.csv"] = csvBytes
	}

	// 9. README.md
	archivos["README.md"] = []byte(fmt.Sprintf(`# Tema WooCommerce: %s

Tema de bloques FSE (Full Site Editing) exportado desde **Sytue Web**.

- Versión del tema: %s
- Compatible con WordPress: >= %s
- Compatible con WooCommerce: >= %s

## Instalación

1. Sube esta carpeta o archivo ZIP a `+"`wp-content/themes/`"+`.
2. Activa el tema desde **Apariencia > Temas**.
3. Importa el archivo `+"`products_export_woocommerce.csv`"+` desde **WooCommerce > Productos > Importar**.
`, nombreSitio, vTema, vWP, vWoo))

	return archivos, nil
}
