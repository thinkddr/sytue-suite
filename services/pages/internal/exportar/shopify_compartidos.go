// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"encoding/json"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

// GenerarCompartidosShopify produce todos los ficheros deterministas obligatorios para Shopify Online Store 2.0.
func GenerarCompartidosShopify(sitio *ir.Sitio) (map[string][]byte, error) {
	archivos := make(map[string][]byte)

	versionTema, err := ObtenerVersion("shopify_theme_version")
	if err != nil {
		return nil, err
	}

	// 1. layout/theme.liquid
	archivos["layout/theme.liquid"] = []byte(generarThemeLiquid(sitio))

	// 2. config/settings_schema.json
	archivos["config/settings_schema.json"] = []byte(generarSettingsSchema(sitio, versionTema))

	// 3. config/settings_data.json
	archivos["config/settings_data.json"] = []byte(generarSettingsData(sitio))

	// 4. locales/en.default.json
	archivos["locales/en.default.json"] = []byte(generarLocalesEn())

	// 5. sections/header-group.json y sections/footer-group.json
	archivos["sections/header-group.json"] = []byte(generarHeaderGroupJSON())
	archivos["sections/footer-group.json"] = []byte(generarFooterGroupJSON())

	// 6. templates/index.json
	archivos["templates/index.json"] = []byte(generarIndexJSON(sitio))

	// 7. templates estándar
	archivos["templates/product.json"] = []byte(generarTemplateEstandar("main-product"))
	archivos["templates/collection.json"] = []byte(generarTemplateEstandar("main-collection"))
	archivos["templates/cart.json"] = []byte(generarTemplateEstandar("main-cart"))
	archivos["templates/404.json"] = []byte(generarTemplateEstandar("main-404"))

	// 8. Secciones base para templates estándar
	archivos["sections/main-product.liquid"] = []byte(generarSeccionProductoLiquid())
	archivos["sections/main-collection.liquid"] = []byte(generarSeccionColeccionLiquid())
	archivos["sections/main-cart.liquid"] = []byte(generarSeccionCarritoLiquid())
	archivos["sections/main-404.liquid"] = []byte(generarSeccion404Liquid())
	archivos["sections/pages-header.liquid"] = []byte(generarSeccionHeaderLiquid(sitio))
	archivos["sections/pages-footer.liquid"] = []byte(generarSeccionFooterLiquid(sitio))

	// 9. Secciones dinámicas por cada kind en las páginas
	kindsVistos := map[string]bool{
		"header": true,
		"footer": true,
	}
	if len(sitio.Pages) > 0 {
		for _, s := range sitio.Pages[0].Sections {
			kind := s.Kind
			if kind == "" {
				kind = "content"
			}
			if !kindsVistos[kind] {
				kindsVistos[kind] = true
				rutaSec := fmt.Sprintf("sections/pages-%s.liquid", kind)
				archivos[rutaSec] = []byte(generarSeccionGenericaLiquid(kind))
			}
		}
	}

	// 10. assets/theme.css
	archivos["assets/theme.css"] = []byte(generarThemeCSS(sitio))

	// 11. README.md con instrucciones claras (§9.4, §20.8)
	archivos["README.md"] = []byte(generarReadmeShopify(sitio, versionTema))

	return archivos, nil
}

func generarThemeLiquid(sitio *ir.Sitio) string {
	bg := "#ffffff"
	text := "#111827"
	primary := "#ca1682"
	if sitio.Theme.Colors != nil {
		if c, ok := sitio.Theme.Colors["background"]; ok && c != "" {
			bg = c
		}
		if c, ok := sitio.Theme.Colors["text"]; ok && c != "" {
			text = c
		}
		if c, ok := sitio.Theme.Colors["primary"]; ok && c != "" {
			primary = c
		}
	}

	return fmt.Sprintf(`<!doctype html>
<html class="no-js" lang="{{ request.locale.iso_code }}">
<head>
  <meta charset="utf-8">
  <meta http-equiv="X-UA-Compatible" content="IE=edge">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <link rel="canonical" href="{{ canonical_url }}">
  <title>{{ page_title }} - {{ shop.name }}</title>

  <style>
    :root {
      --sp-bg: %s;
      --sp-text: %s;
      --sp-primary: %s;
      --sp-container: %dpx;
    }
  </style>

  {{ 'theme.css' | asset_url | stylesheet_tag }}

  {{ content_for_header }}
</head>
<body class="sp-body">
  {%% sections 'header-group' %%}

  <main id="MainContent" class="content-for-layout focus-none" role="main" tabindex="-1">
    {{ content_for_layout }}
  </main>

  {%% sections 'footer-group' %%}
</body>
</html>
`, bg, text, primary, sitio.Theme.Container)
}

func generarSettingsSchema(sitio *ir.Sitio, version string) string {
	themeName := "Sytue Web Theme"
	if sitio != nil && len(sitio.SEO.SiteName) > 0 {
		for _, name := range sitio.SEO.SiteName {
			if name != "" {
				themeName = name + " Theme"
				break
			}
		}
	}

	schema := []map[string]any{
		{
			"name":                    "theme_info",
			"theme_name":              themeName,
			"theme_version":           version,
			"theme_author":            "Sytue Web",
			"theme_documentation_url": "https://sytue.eu/docs",
			"theme_support_url":       "https://sytue.eu/support",
		},
		{
			"name": "General",
			"settings": []map[string]any{
				{
					"type":    "color",
					"id":      "color_primary",
					"label":   "Primary Color",
					"default": "#ca1682",
				},
				{
					"type":    "range",
					"id":      "container_width",
					"min":     100,
					"max":     250,
					"step":    5,
					"unit":    "px",
					"label":   "Container Multiplier",
					"default": 120,
				},
			},
		},
	}
	b, _ := json.MarshalIndent(schema, "", "  ")
	return string(b)
}

func generarSettingsData(sitio *ir.Sitio) string {
	primary := "#ca1682"
	if sitio != nil && sitio.Theme.Colors != nil {
		if c, ok := sitio.Theme.Colors["primary"]; ok && c != "" {
			primary = c
		}
	}

	data := map[string]any{
		"current": map[string]any{
			"color_primary": primary,
		},
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	return string(b)
}

func generarLocalesEn() string {
	locales := map[string]any{
		"general": map[string]any{
			"welcome":      "Welcome to our store",
			"cart":         "Cart",
			"search":       "Search",
			"back_to_home": "Back to Home",
		},
		"sections": map[string]any{
			"header": map[string]any{
				"title": "Navigation",
			},
			"footer": map[string]any{
				"copyright": "All rights reserved.",
			},
			"product": map[string]any{
				"add_to_cart":  "Add to Cart",
				"out_of_stock": "Out of Stock",
				"price":        "Price",
			},
			"cart": map[string]any{
				"title":    "Your Cart",
				"checkout": "Checkout",
				"empty":    "Your cart is currently empty.",
			},
			"page_404": map[string]any{
				"title":       "Page Not Found",
				"description": "The page you requested does not exist.",
			},
		},
	}
	b, _ := json.MarshalIndent(locales, "", "  ")
	return string(b)
}

func generarHeaderGroupJSON() string {
	obj := map[string]any{
		"name": "Header Group",
		"type": "header",
		"sections": map[string]any{
			"header": map[string]any{
				"type": "pages-header",
			},
		},
		"order": []string{"header"},
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b)
}

func generarFooterGroupJSON() string {
	obj := map[string]any{
		"name": "Footer Group",
		"type": "footer",
		"sections": map[string]any{
			"footer": map[string]any{
				"type": "pages-footer",
			},
		},
		"order": []string{"footer"},
	}
	b, _ := json.MarshalIndent(obj, "", "  ")
	return string(b)
}

func generarIndexJSON(sitio *ir.Sitio) string {
	sectionsMap := make(map[string]any)
	var order []string

	if len(sitio.Pages) > 0 {
		for i, s := range sitio.Pages[0].Sections {
			kind := s.Kind
			if kind == "" {
				kind = "content"
			}
			secID := fmt.Sprintf("sec_%d_%s", i, kind)
			sectionsMap[secID] = map[string]any{
				"type": fmt.Sprintf("pages-%s", kind),
				"settings": map[string]any{
					"heading": extraerTextoNodo(&s),
				},
			}
			order = append(order, secID)
		}
	}

	if len(order) == 0 {
		sectionsMap["hero"] = map[string]any{
			"type": "pages-hero",
			"settings": map[string]any{
				"heading": "Welcome to our store",
			},
		}
		order = []string{"hero"}
	}

	tpl := map[string]any{
		"sections": sectionsMap,
		"order":    order,
	}
	b, _ := json.MarshalIndent(tpl, "", "  ")
	return string(b)
}

func generarTemplateEstandar(secType string) string {
	tpl := map[string]any{
		"sections": map[string]any{
			"main": map[string]any{
				"type": secType,
			},
		},
		"order": []string{"main"},
	}
	b, _ := json.MarshalIndent(tpl, "", "  ")
	return string(b)
}

func generarSeccionHeaderLiquid(sitio *ir.Sitio) string {
	return `
<header class="sp-header">
  <div class="sp-container sp-header-inner">
    <a href="{{ routes.root_url }}" class="sp-logo">
      {{ shop.name }}
    </a>
    <nav class="sp-nav" role="navigation" aria-label="{{ 'sections.header.title' | t }}">
      <ul class="sp-menu">
        {% for link in linklists.main-menu.links %}
          <li><a href="{{ link.url }}">{{ link.title }}</a></li>
        {% endfor %}
      </ul>
    </nav>
    <div class="sp-header-actions">
      <a href="{{ routes.cart_url }}" class="sp-cart-link">
        {{ 'general.cart' | t }}
        {% if cart.item_count > 0 %}
          <span class="sp-badge">({{ cart.item_count }})</span>
        {% endif %}
      </a>
    </div>
  </div>
</header>

{% schema %}
{
  "name": "Header",
  "settings": [
    {
      "type": "text",
      "id": "heading",
      "label": "Header Heading"
    }
  ]
}
{% endschema %}
`
}

func generarSeccionFooterLiquid(sitio *ir.Sitio) string {
	return `
<footer class="sp-footer">
  <div class="sp-container sp-footer-inner">
    <p class="sp-copyright">&copy; {{ 'now' | date: '%%Y' }} {{ shop.name }}. {{ 'sections.footer.copyright' | t }}</p>
  </div>
</footer>

{% schema %}
{
  "name": "Footer",
  "settings": []
}
{% endschema %}
`
}

func generarSeccionGenericaLiquid(kind string) string {
	return fmt.Sprintf(`
<section class="sp-section sp-section-%s">
  <div class="sp-container">
    {%% if section.settings.heading != blank %%}
      <h2 class="sp-section-heading">{{ section.settings.heading }}</h2>
    {%% endif %%}
    {%% for block in section.blocks %%}
      <div class="sp-block sp-block-{{ block.type }}" {{ block.shopify_attributes }}>
        {%% if block.settings.text != blank %%}
          <p>{{ block.settings.text }}</p>
        {%% endif %%}
      </div>
    {%% endfor %%}
  </div>
</section>

{%% schema %%}
{
  "name": "Section %s",
  "settings": [
    {
      "type": "text",
      "id": "heading",
      "label": "Heading"
    }
  ],
  "blocks": [
    {
      "type": "content_item",
      "name": "Content Block",
      "settings": [
        {
          "type": "textarea",
          "id": "text",
          "label": "Block Text"
        }
      ]
    }
  ],
  "presets": [
    {
      "name": "Section %s"
    }
  ]
}
{%% endschema %%}
`, kind, kind, kind)
}

func generarSeccionProductoLiquid() string {
	return `
<section class="sp-section sp-product-section">
  <div class="sp-container">
    <div class="sp-product-grid">
      <div class="sp-product-media">
        {% if product.featured_image %}
          <img src="{{ product.featured_image | image_url: width: 800 }}" alt="{{ product.featured_image.alt | escape }}" loading="lazy">
        {% endif %}
      </div>
      <div class="sp-product-info">
        <h1>{{ product.title }}</h1>
        <p class="sp-price">{{ product.price | money }}</p>
        <div class="sp-description">{{ product.description }}</div>
        {% form 'product', product %}
          <input type="hidden" name="id" value="{{ product.selected_or_first_available_variant.id }}">
          <button type="submit" class="sp-btn sp-btn-primary" {% unless product.available %}disabled{% endunless %}>
            {% if product.available %}
              {{ 'sections.product.add_to_cart' | t }}
            {% else %}
              {{ 'sections.product.out_of_stock' | t }}
            {% endif %}
          </button>
        {% endform %}
      </div>
    </div>
  </div>
</section>

{% schema %}
{
  "name": "Product Main",
  "settings": []
}
{% endschema %}
`
}

func generarSeccionColeccionLiquid() string {
	return `
<section class="sp-section sp-collection-section">
  <div class="sp-container">
    <h1>{{ collection.title }}</h1>
    <div class="sp-grid">
      {% for product in collection.products %}
        <div class="sp-card">
          <a href="{{ product.url }}">
            {% if product.featured_image %}
              <img src="{{ product.featured_image | image_url: width: 400 }}" alt="{{ product.title | escape }}" loading="lazy">
            {% endif %}
            <h3>{{ product.title }}</h3>
            <p>{{ product.price | money }}</p>
          </a>
        </div>
      {% endfor %}
    </div>
  </div>
</section>

{% schema %}
{
  "name": "Collection Main",
  "settings": []
}
{% endschema %}
`
}

func generarSeccionCarritoLiquid() string {
	return `
<section class="sp-section sp-cart-section">
  <div class="sp-container">
    <h1>{{ 'sections.cart.title' | t }}</h1>
    {% if cart.item_count > 0 %}
      <form action="{{ routes.cart_url }}" method="post">
        <ul class="sp-cart-items">
          {% for item in cart.items %}
            <li class="sp-cart-item">
              <span>{{ item.product.title }}</span>
              <span>{{ item.price | money }} &times; {{ item.quantity }}</span>
            </li>
          {% endfor %}
        </ul>
        <div class="sp-cart-actions">
          <button type="submit" name="checkout" class="sp-btn sp-btn-primary">{{ 'sections.cart.checkout' | t }}</button>
        </div>
      </form>
    {% else %}
      <p>{{ 'sections.cart.empty' | t }}</p>
    {% endif %}
  </div>
</section>

{% schema %}
{
  "name": "Cart Main",
  "settings": []
}
{% endschema %}
`
}

func generarSeccion404Liquid() string {
	return `
<section class="sp-section sp-404-section">
  <div class="sp-container text-center">
    <h1>{{ 'sections.page_404.title' | t }}</h1>
    <p>{{ 'sections.page_404.description' | t }}</p>
    <a href="{{ routes.root_url }}" class="sp-btn sp-btn-primary">{{ 'general.back_to_home' | t }}</a>
  </div>
</section>

{% schema %}
{
  "name": "404 Main",
  "settings": []
}
{% endschema %}
`
}

func generarThemeCSS(sitio *ir.Sitio) string {
	return `
/* Sytue Web - Shopify Theme CSS */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
  background-color: var(--sp-bg, #ffffff);
  color: var(--sp-text, #111827);
  line-height: 1.5;
}
.sp-container {
  width: 100%;
  max-width: var(--sp-container, 1200px);
  margin-left: auto;
  margin-right: auto;
  padding-left: 1.5rem;
  padding-right: 1.5rem;
}
.sp-header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
  padding: 1rem 0;
}
.sp-header-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.sp-logo {
  font-weight: 700;
  font-size: 1.25rem;
  color: inherit;
  text-decoration: none;
}
.sp-nav .sp-menu {
  display: flex;
  gap: 1.5rem;
  list-style: none;
}
.sp-nav a, .sp-cart-link {
  color: inherit;
  text-decoration: none;
  font-size: 0.9rem;
}
.sp-section {
  padding: 3rem 0;
}
.sp-btn {
  display: inline-block;
  padding: 0.625rem 1.25rem;
  font-size: 0.875rem;
  font-weight: 600;
  border-radius: 0.375rem;
  text-decoration: none;
  cursor: pointer;
  border: none;
}
.sp-btn-primary {
  background-color: var(--sp-primary, #ca1682);
  color: #ffffff;
}
.sp-product-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 2rem;
}
@media (min-width: 768px) {
  .sp-product-grid {
    grid-template-columns: 1fr 1fr;
  }
}
.sp-product-media img {
  width: 100%;
  height: auto;
  border-radius: 0.5rem;
}
.sp-footer {
  border-top: 1px solid rgba(0, 0, 0, 0.08);
  padding: 2rem 0;
  text-align: center;
  font-size: 0.875rem;
  color: #6b7280;
}
`
}

func generarReadmeShopify(sitio *ir.Sitio, version string) string {
	siteName := "Sytue Web"
	if sitio != nil && len(sitio.SEO.SiteName) > 0 {
		for _, name := range sitio.SEO.SiteName {
			if name != "" {
				siteName = name
				break
			}
		}
	}

	return fmt.Sprintf(`# Tema exportado de %s para Shopify Online Store 2.0

Versión del tema: %s

## Cómo importar este tema en Shopify

1. Accede a tu panel de administración de Shopify.
2. Ve a **Tienda online** > **Temas**.
3. En la sección **Biblioteca de temas**, pulsa en **Agregar tema** > **Subir archivo zip**.
4. Selecciona el archivo zip descargado desde Sytue Web y confirma la subida.
5. Una vez cargado, pulsa **Publicar** o **Personalizar** para ajustar colores, tipografía y secciones.

## Cómo importar el catálogo de productos

1. En el panel de Shopify, ve a **Productos**.
2. Pulsa en el botón **Importar** en la esquina superior derecha.
3. Selecciona el archivo `+"`products_export_shopify.csv`"+` incluido en esta exportación.
4. Revisa la asignación de columnas y pulsa **Importar productos**.

## Traducciones e Idiomas (§20.8)

El contenido principal de este tema está preparado en el idioma por defecto configurado en Sytue Web. Para añadir traducciones a otros idiomas:
- Instala la aplicación oficial **Shopify Translate & Adapt** desde la Shopify App Store.
- Los textos del sistema del tema se encuentran en `+"`locales/en.default.json`"+`.
- Podrás traducir cada sección y producto directamente desde la herramienta de traducción de Shopify.
`, siteName, version)
}

func extraerTextoNodo(n *ir.Nodo) string {
	if n == nil {
		return ""
	}
	if n.Content != nil {
		for _, tramos := range n.Content {
			var sb strings.Builder
			for _, tr := range tramos {
				sb.WriteString(tr.Text)
			}
			res := sb.String()
			if res != "" {
				return res
			}
		}
	}
	for _, h := range n.Children {
		txt := extraerTextoNodo(&h)
		if txt != "" {
			return txt
		}
	}
	return ""
}
