// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"fmt"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// GenerarCompartidosPrestaShop produce los archivos deterministas de un tema hijo de Hummingbird en PrestaShop (§9.4).
func GenerarCompartidosPrestaShop(irSitio *ir.Sitio, productos []domain.Product) (map[string][]byte, error) {
	archivos := make(map[string][]byte)

	vTema, _ := ObtenerVersion("prestashop_theme_version")
	vPresta, _ := ObtenerVersion("prestashop_version")

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

	// 1. config/theme.yml obligatoriamente con padre Hummingbird (§9.4)
	archivos["config/theme.yml"] = []byte(fmt.Sprintf(`name: sytue_store
display_name: '%s'
version: '%s'
author:
  name: 'Sytue Web'
  email: 'soporte@sytue.eu'
  url: 'https://sytue.eu'
meta:
  compatibility:
    from: '8.0.0'
    to: '%s'
parent: hummingbird
preview: preview.png
assets:
  css:
    all:
      - id: custom-theme-css
        path: assets/css/custom.css
        priority: 50
`, nombreSitio, vTema, vPresta))

	// 2. preview.png generado deterministamente con image/png (180x180)
	pngBytes, err := GenerarImagenPNG(180, 180, colorPrimario)
	if err != nil {
		return nil, fmt.Errorf("prestashop: error generando preview.png: %w", err)
	}
	archivos["preview.png"] = pngBytes

	// 3. templates/index.tpl (plantilla Smarty que extiende del layout padre)
	archivos["templates/index.tpl"] = []byte(fmt.Sprintf(`{extends file='page.tpl'}

{block name='page_content_container'}
  <section id="content" class="page-home">
    <div class="container my-4">
      <h1 class="h1 text-center">%s</h1>
      <p class="text-center lead">Tema exportado desde Sytue Web basado en Hummingbird.</p>
    </div>
  </section>
{/block}
`, nombreSitio))

	// 4. templates/layout.tpl
	archivos["templates/layout.tpl"] = []byte(`{extends file='parent:layout.tpl'}
`)

	// 5. assets/css/custom.css
	archivos["assets/css/custom.css"] = []byte(fmt.Sprintf(`:root {
  --primary-color: %s;
}

.page-home h1 {
  color: var(--primary-color);
}
`, colorPrimario))

	// 6. products_export_prestashop.csv
	if len(productos) > 0 {
		csvBytes, err := CatalogoPrestaShop(productos, idioma)
		if err != nil {
			return nil, fmt.Errorf("prestashop: error generando catálogo CSV: %w", err)
		}
		archivos["products_export_prestashop.csv"] = csvBytes
	}

	// 7. README.md
	archivos["README.md"] = []byte(fmt.Sprintf(`# Tema PrestaShop: %s

Tema hijo basado en el tema oficial **Hummingbird** exportado desde **Sytue Web**.

- Versión: %s
- Compatible con PrestaShop: >= 8.0.0 (hasta %s)
- Tema padre requerido: `+"`hummingbird`"+`

## Instalación

1. Asegúrate de tener instalado el tema oficial `+"`hummingbird`"+` en tu tienda PrestaShop.
2. Sube esta carpeta a `+"`themes/sytue_store/`"+`.
3. Activa el tema desde el backoffice en **Diseño > Tema y logotipo**.
4. Importa el catálogo desde **Parámetros avanzados > Importar** seleccionando `+"`products_export_prestashop.csv`"+`.
`, nombreSitio, vTema, vPresta))

	return archivos, nil
}
