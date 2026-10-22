// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"errors"
	"testing"
)

func temaShopifyMinimoValido() map[string][]byte {
	return map[string][]byte{
		"layout/theme.liquid": []byte(`<!DOCTYPE html>
<html>
<head>
  {{ content_for_header }}
</head>
<body>
  {{ content_for_layout }}
</body>
</html>`),
		"config/settings_schema.json": []byte(`[
  {
    "name": "theme_info",
    "theme_name": "Sytue Export",
    "theme_version": "1.0.0",
    "theme_author": "Sytue"
  }
]`),
		"config/settings_data.json": []byte(`{
  "current": {}
}`),
		"locales/en.default.json": []byte(`{
  "general": {
    "welcome": "Welcome",
    "cart": "Cart"
  },
  "sections": {
    "hero": {
      "title": "Welcome to our store"
    }
  }
}`),
		"templates/index.json": []byte(`{
  "sections": {
    "hero": {
      "type": "pages-hero"
    }
  },
  "order": ["hero"]
}`),
		"sections/pages-hero.liquid": []byte(`
<section class="hero">
  <h1>{{ 'sections.hero.title' | t }}</h1>
</section>

{% schema %}
{
  "name": "Hero Section",
  "blocks": [
    {
      "type": "cta_button",
      "name": "CTA Button"
    }
  ]
}
{% endschema %}
`),
	}
}

func TestShopifyValido(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	if err := Shopify(archivos); err != nil {
		t.Fatalf("Shopify() devolvió error en tema válido: %v", err)
	}
}

func TestShopifyFaltaArchivoObligatorio(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	delete(archivos, "layout/theme.liquid")

	err := Shopify(archivos)
	if !errors.Is(err, ErrArchivoFaltante) {
		t.Fatalf("esperaba ErrArchivoFaltante, obtuvo: %v", err)
	}
}

func TestShopifyJSONInvalido(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["config/settings_data.json"] = []byte(`{ "roto": `)

	err := Shopify(archivos)
	if !errors.Is(err, ErrJSONInvalido) {
		t.Fatalf("esperaba ErrJSONInvalido, obtuvo: %v", err)
	}
}

func TestShopifyFaltaContentForHeader(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["layout/theme.liquid"] = []byte(`<html><body>{{ content_for_layout }}</body></html>`)

	err := Shopify(archivos)
	if !errors.Is(err, ErrFaltaHeaderLayout) {
		t.Fatalf("esperaba ErrFaltaHeaderLayout, obtuvo: %v", err)
	}
}

func TestShopifyVersionThemeInfoInvalida(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["config/settings_schema.json"] = []byte(`[
  {
    "name": "theme_info",
    "theme_name": "Sytue Export",
    "theme_version": "9.9.9"
  }
]`)

	err := Shopify(archivos, "1.0.0")
	if !errors.Is(err, ErrThemeInfoInvalido) {
		t.Fatalf("esperaba ErrThemeInfoInvalido, obtuvo: %v", err)
	}
}

func TestShopifySeccionInexistente(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["templates/index.json"] = []byte(`{
  "sections": {
    "inexistente": {
      "type": "pages-fantasma"
    }
  },
  "order": ["inexistente"]
}`)

	err := Shopify(archivos)
	if !errors.Is(err, ErrSeccionInexistente) {
		t.Fatalf("esperaba ErrSeccionInexistente, obtuvo: %v", err)
	}
}

func TestShopifyTraduccionFaltante(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["sections/pages-hero.liquid"] = []byte(`
<section>
  <p>{{ 'clave.que.no.existe' | t }}</p>
</section>
`)

	err := Shopify(archivos)
	if !errors.Is(err, ErrTraduccionFaltante) {
		t.Fatalf("esperaba ErrTraduccionFaltante, obtuvo: %v", err)
	}
}

func TestShopifyScriptExterno(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["layout/theme.liquid"] = []byte(`
<html>
<head>
  <script src="https://cdn.jsdelivr.net/npm/malicioso.js"></script>
  {{ content_for_header }}
</head>
<body>
  {{ content_for_layout }}
</body>
</html>`)

	err := Shopify(archivos)
	if !errors.Is(err, ErrScriptExterno) {
		t.Fatalf("esperaba ErrScriptExterno, obtuvo: %v", err)
	}
}

func TestShopifyRutaPeligrosa(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["../escape.txt"] = []byte("peligro")

	err := Shopify(archivos)
	if !errors.Is(err, ErrRutaPeligrosa) {
		t.Fatalf("esperaba ErrRutaPeligrosa, obtuvo: %v", err)
	}
}

func TestShopifyLimiteLongitudBloque(t *testing.T) {
	archivos := temaShopifyMinimoValido()
	archivos["sections/pages-hero.liquid"] = []byte(`
{% schema %}
{
  "name": "Hero",
  "blocks": [
    {
      "type": "este_tipo_de_bloque_es_demasiado_largo_para_shopify",
      "name": "Boton"
    }
  ]
}
{% endschema %}
`)

	err := Shopify(archivos)
	if !errors.Is(err, ErrLimiteSuperado) {
		t.Fatalf("esperaba ErrLimiteSuperado, obtuvo: %v", err)
	}
}
