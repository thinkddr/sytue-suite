// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
)

func ejemploSitioIR() *ir.Sitio {
	return &ir.Sitio{
		Version: 1,
		Languages: ir.IdiomasSitio{
			Default: "es",
			Others:  []string{"en"},
		},
		Theme: ir.Tema{
			Colors: map[string]string{
				"background": "#ffffff",
				"text":       "#111827",
				"primary":    "#ca1682",
			},
			Container: 1200,
		},
		SEO: ir.SeoSitio{
			SiteName: ir.Textos{"es": "Mi Tienda Sytue"},
		},
		Pages: []ir.Pagina{
			{
				ID: "home",
				Slug: ir.Textos{
					"es": "inicio",
					"en": "home",
				},
				Title: ir.Textos{
					"es": "Inicio",
					"en": "Home",
				},
				Sections: []ir.Nodo{
					{
						ID:   "hero-1",
						Type: "section",
						Kind: "hero",
						Children: []ir.Nodo{
							{
								ID:   "txt-1",
								Type: "text",
								Content: ir.TextosRicos{
									"es": []ir.Tramo{{Text: "Bienvenidos a nuestra tienda online"}},
								},
							},
						},
					},
					{
						ID:   "features-1",
						Type: "section",
						Kind: "features",
					},
				},
			},
		},
	}
}

func TestMedirSeccion(t *testing.T) {
	nodo := &ir.Nodo{
		ID:   "padre",
		Type: "section",
		Children: []ir.Nodo{
			{
				ID:   "hijo-1",
				Type: "frame",
				Children: []ir.Nodo{
					{
						ID:   "nieto-1",
						Type: "text",
					},
				},
			},
			{
				ID:   "hijo-2",
				Type: "button",
			},
		},
	}

	prof, bloques := MedirSeccion(nodo)
	if prof != 3 {
		t.Errorf("esperaba profundidad 3, obtuvo %d", prof)
	}
	if bloques != 4 {
		t.Errorf("esperaba 4 bloques en total, obtuvo %d", bloques)
	}
}

func TestGenerarShopify(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()
	productos := []domain.Product{
		{
			ID:          uuid.New(),
			Slug:        map[string]string{"es": "taza-sytue"},
			Name:        map[string]string{"es": "Taza de Cerámica"},
			Description: map[string]string{"es": "<p>Taza artesanal</p>"},
			Category:    "Hogar",
			Active:      true,
			Kind:        "general",
		},
	}

	archivos, err := Generar(ctx, DestinoShopify, sitio, productos, nil)
	if err != nil {
		t.Fatalf("Generar(DestinoShopify) falló: %v", err)
	}

	// Comprobar archivos obligatorios
	ficherosEsperados := []string{
		"layout/theme.liquid",
		"config/settings_schema.json",
		"config/settings_data.json",
		"locales/en.default.json",
		"templates/index.json",
		"templates/product.json",
		"sections/header-group.json",
		"sections/footer-group.json",
		"sections/pages-header.liquid",
		"sections/pages-footer.liquid",
		"sections/pages-hero.liquid",
		"sections/pages-features.liquid",
		"assets/theme.css",
		"products_export_shopify.csv",
		"README.md",
	}

	for _, f := range ficherosEsperados {
		if _, ok := archivos[f]; !ok {
			t.Errorf("archivo esperado %q no fue generado", f)
		}
	}
}

func TestGenerarZip(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()

	var buf bytes.Buffer
	err := GenerarZip(ctx, DestinoShopify, sitio, nil, nil, &buf)
	if err != nil {
		t.Fatalf("GenerarZip falló: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("el buffer zip está vacío")
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("error abriendo ZIP generado: %v", err)
	}

	encontrados := make(map[string]bool)
	for _, f := range zr.File {
		encontrados[f.Name] = true
	}

	if !encontrados["layout/theme.liquid"] {
		t.Error("el ZIP no contiene layout/theme.liquid")
	}
	if !encontrados["templates/index.json"] {
		t.Error("el ZIP no contiene templates/index.json")
	}
	if !encontrados["README.md"] {
		t.Error("el ZIP no contiene README.md")
	}
}

func TestGenerarReact(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()

	archivos, err := Generar(ctx, DestinoReact, sitio, nil, nil)
	if err != nil {
		t.Fatalf("Generar(DestinoReact) falló: %v", err)
	}

	esperados := []string{
		"package.json",
		"tsconfig.json",
		"tsconfig.node.json",
		"vite.config.ts",
		"index.html",
		"src/main.tsx",
		"src/App.tsx",
		"src/index.css",
		"src/api/pages-api.ts",
		"src/components/Formulario.tsx",
		"src/components/BotonComprar.tsx",
		"README.md",
	}

	for _, esp := range esperados {
		if _, ok := archivos[esp]; !ok {
			t.Errorf("React: falta archivo obligatorio %q", esp)
		}
	}
}

func TestGenerarNextjs(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()

	archivos, err := Generar(ctx, DestinoNextjs, sitio, nil, nil)
	if err != nil {
		t.Fatalf("Generar(DestinoNextjs) falló: %v", err)
	}

	esperados := []string{
		"package.json",
		"tsconfig.json",
		"next.config.ts",
		"app/layout.tsx",
		"app/page.tsx",
		"app/globals.css",
		"lib/pages-api.ts",
		"components/Formulario.tsx",
		"components/BotonComprar.tsx",
		"README.md",
	}

	for _, esp := range esperados {
		if _, ok := archivos[esp]; !ok {
			t.Errorf("Next.js: falta archivo obligatorio %q", esp)
		}
	}
}

func TestGenerarAstro(t *testing.T) {
	archivos, err := Generar(context.Background(), DestinoAstro, ejemploSitioIR(), nil, nil)
	if err != nil {
		t.Fatalf("Generar(DestinoAstro) falló: %v", err)
	}
	for _, ruta := range []string{
		"package.json", "astro.config.mjs", "src/pages/index.astro", "src/lib/pages-api.ts",
		"src/islands/Form.tsx", "src/islands/Cart.tsx", "README.md",
	} {
		if _, ok := archivos[ruta]; !ok {
			t.Errorf("Astro: falta archivo obligatorio %q", ruta)
		}
	}
	paquete := string(archivos["package.json"])
	for _, version := range []string{"\"astro\":\"7.3.3\"", "\"@astrojs/preact\":\"6.0.5\"", "\"preact\":\"10.29.8\""} {
		if !strings.Contains(paquete, version) {
			t.Errorf("Astro: package.json no está atado a la versión del lock %s: %s", version, paquete)
		}
	}
}

func TestGenerarComercioEntregaArbolesPorPlataforma(t *testing.T) {
	productos := []domain.Product{{
		ID:          uuid.New(),
		Name:        map[string]string{"es": "Producto de prueba"},
		Description: map[string]string{"es": "Descripción"},
		Category:    "Pruebas",
		Active:      true,
		Variants: []domain.Variant{{
			SKU:        "PRUEBA-1",
			PriceCents: 1299,
		}},
	}}
	casos := map[Destino][]string{
		DestinoShopware:    {"composer.json", "src/Resources/theme.json", "src/Resources/views/storefront/page/content/index.html.twig", "src/Resources/public/plugin.png", "products.csv"},
		DestinoMagento:     {"composer.json", "registration.php", "theme.xml", "Magento_Theme/layout/default.xml", "products.csv"},
		DestinoBigCommerce: {"config.json", "schema.json", "templates/pages/home.html", "assets/scss/theme.scss", "products.csv"},
	}
	for destino, esperados := range casos {
		archivos, err := Generar(context.Background(), destino, ejemploSitioIR(), productos, nil)
		if err != nil {
			t.Fatalf("Generar(%s) falló: %v", destino, err)
		}
		for _, ruta := range esperados {
			if len(archivos[ruta]) == 0 {
				t.Errorf("%s: falta o está vacío %q", destino, ruta)
			}
		}
		if !strings.Contains(string(archivos["products.csv"]), "PRUEBA-1") {
			t.Errorf("%s: el CSV no conserva el SKU de variante", destino)
		}
	}
}

func TestGenerarNativoEntregaProyectoYCliente(t *testing.T) {
	casos := map[Destino][]string{
		DestinoFlutter: {"pubspec.yaml", "lib/pages_api.dart", "lib/main.dart"},
		DestinoCompose: {"settings.gradle.kts", "app/src/main/java/pagesapi/PagesApi.kt", "app/src/main/java/pages/MainActivity.kt"},
		DestinoSwiftUI: {"Package.swift", "Sources/PagesAPI/PagesApi.swift", "Sources/PagesAPI/PagesView.swift", "Flujo.swift"},
		DestinoFyne:    {"go.mod", "pagesapi/pagesapi.go", "main.go", "flex.go"},
	}
	for destino, esperados := range casos {
		archivos, err := Generar(context.Background(), destino, ejemploSitioIR(), nil, nil)
		if err != nil {
			t.Fatalf("Generar(%s) falló: %v", destino, err)
		}
		for _, ruta := range esperados {
			if len(archivos[ruta]) == 0 {
				t.Errorf("%s: falta o está vacío %q", destino, ruta)
			}
		}
	}
}

func TestGenerarWooCommerce(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()
	productos := []domain.Product{
		{
			ID:          uuid.New(),
			Slug:        map[string]string{"es": "producto-1"},
			Name:        map[string]string{"es": "Producto Uno"},
			Description: map[string]string{"es": "Descripción uno"},
			Category:    "Pruebas",
			Active:      true,
			Kind:        "general",
		},
	}

	archivos, err := Generar(ctx, DestinoWooCommerce, sitio, productos, nil)
	if err != nil {
		t.Fatalf("Generar(DestinoWooCommerce) falló: %v", err)
	}

	esperados := []string{
		"style.css",
		"theme.json",
		"functions.php",
		"parts/header.html",
		"parts/footer.html",
		"templates/index.html",
		"screenshot.png",
		"products_export_woocommerce.csv",
		"README.md",
	}

	for _, esp := range esperados {
		if _, ok := archivos[esp]; !ok {
			t.Errorf("WooCommerce: falta archivo obligatorio %q", esp)
		}
	}
}

func TestGenerarPrestaShop(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()
	productos := []domain.Product{
		{
			ID:          uuid.New(),
			Slug:        map[string]string{"es": "producto-presta"},
			Name:        map[string]string{"es": "Producto PrestaShop"},
			Description: map[string]string{"es": "Descripción PrestaShop"},
			Category:    "Pruebas",
			Active:      true,
			Kind:        "general",
		},
	}

	archivos, err := Generar(ctx, DestinoPrestaShop, sitio, productos, nil)
	if err != nil {
		t.Fatalf("Generar(DestinoPrestaShop) falló: %v", err)
	}

	esperados := []string{
		"config/theme.yml",
		"preview.png",
		"templates/index.tpl",
		"templates/layout.tpl",
		"assets/css/custom.css",
		"products_export_prestashop.csv",
		"README.md",
	}

	for _, esp := range esperados {
		if _, ok := archivos[esp]; !ok {
			t.Errorf("PrestaShop: falta archivo obligatorio %q", esp)
		}
	}
}

func TestGenerarZipNuevosDestinos(t *testing.T) {
	ctx := context.Background()
	sitio := ejemploSitioIR()

	destinos := []Destino{DestinoReact, DestinoNextjs, DestinoWooCommerce, DestinoPrestaShop}
	for _, dest := range destinos {
		var buf bytes.Buffer
		if err := GenerarZip(ctx, dest, sitio, nil, nil, &buf); err != nil {
			t.Fatalf("GenerarZip(%s) falló: %v", dest, err)
		}
		if buf.Len() == 0 {
			t.Errorf("GenerarZip(%s) produjo un zip vacío", dest)
		}
	}
}
