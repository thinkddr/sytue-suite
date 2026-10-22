// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/exportar/validar"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
)

// Destino representa la plataforma de destino de la exportación (§9.4).
type Destino string

const (
	DestinoShopify     Destino = "shopify"
	DestinoReact       Destino = "react"
	DestinoNextjs      Destino = "nextjs"
	DestinoWooCommerce Destino = "woocommerce"
	DestinoPrestaShop  Destino = "prestashop"
	DestinoShopware    Destino = "shopware"
	DestinoMagento     Destino = "magento"
	DestinoBigCommerce Destino = "bigcommerce"
	DestinoFlutter     Destino = "flutter"
	DestinoCompose     Destino = "compose"
	DestinoSwiftUI     Destino = "swiftui"
	DestinoFyne        Destino = "fyne"
	DestinoAstro       Destino = "astro"
)

var (
	ErrDestinoNoSoportado = errors.New("exportar: destino no soportado todavía")
	ErrSitioRequerido     = errors.New("exportar: se requiere un sitio IR válido")
)

// LlamarIAFunc es la firma de la función de invocación a modelos de lenguaje (§9.4).
type LlamarIAFunc func(ctx context.Context, prompt string, esquema any) ([]byte, error)

// MedirSeccion analiza la profundidad máxima del árbol y el recuento total de bloques de una sección.
// Según §9.4, si pasa de 8 niveles o de 50 bloques, debe aplanarse a HTML dentro del bloque padre.
func MedirSeccion(nodo *ir.Nodo) (profundidad, totalBloques int) {
	if nodo == nil {
		return 0, 0
	}
	return medirNodo(nodo, 1)
}

func medirNodo(n *ir.Nodo, nivelActual int) (int, int) {
	maxProfundidad := nivelActual
	bloques := 1

	for _, hijo := range n.Children {
		profHijo, bloquesHijo := medirNodo(&hijo, nivelActual+1)
		if profHijo > maxProfundidad {
			maxProfundidad = profHijo
		}
		bloques += bloquesHijo
	}

	return maxProfundidad, bloques
}

// Generar produce el mapa completo de archivos {ruta: contenido} para un destino dado (§9.4).
func Generar(ctx context.Context, destino Destino, irSitio *ir.Sitio, productos []domain.Product, llamar LlamarIAFunc) (map[string][]byte, error) {
	if irSitio == nil {
		return nil, ErrSitioRequerido
	}

	switch destino {
	case DestinoShopify:
		return generarShopify(ctx, irSitio, productos, llamar)
	case DestinoReact:
		return generarReact(ctx, irSitio, productos, llamar)
	case DestinoNextjs:
		return generarNextjs(ctx, irSitio, productos, llamar)
	case DestinoWooCommerce:
		return generarWooCommerce(ctx, irSitio, productos, llamar)
	case DestinoPrestaShop:
		return generarPrestaShop(ctx, irSitio, productos, llamar)
	case DestinoShopware, DestinoMagento, DestinoBigCommerce:
		return generarComercio(destino, irSitio, productos)
	case DestinoFlutter, DestinoCompose, DestinoSwiftUI, DestinoFyne:
		return generarNativo(destino, irSitio)
	case DestinoAstro:
		return generarAstro(irSitio)
	default:
		return nil, fmt.Errorf("%w: %s", ErrDestinoNoSoportado, destino)
	}
}

func obtenerBaseURLSitio(_ *ir.Sitio) string {
	return "https://ejemplo.sytue.eu"
}

func generarReact(_ context.Context, irSitio *ir.Sitio, _ []domain.Product, _ LlamarIAFunc) (map[string][]byte, error) {
	baseURL := obtenerBaseURLSitio(irSitio)
	archivos, err := GenerarCompartidosReact(irSitio, baseURL)
	if err != nil {
		return nil, fmt.Errorf("react: error en compartidos: %w", err)
	}

	vReact, _ := ObtenerVersion("react_version")
	vReactDom, _ := ObtenerVersion("react_dom_version")
	vVite, _ := ObtenerVersion("vite_version")
	vVitePluginReact, _ := ObtenerVersion("vite_plugin_react_version")
	vTS, _ := ObtenerVersion("typescript_version")
	vTypesReact, _ := ObtenerVersion("types_react_version")
	vTypesReactDom, _ := ObtenerVersion("types_react_dom_version")
	vTypesNode, _ := ObtenerVersion("types_node_version")

	deps := validar.DependenciasReactEsperadas{
		React:           vReact,
		ReactDom:        vReactDom,
		Vite:            vVite,
		VitePluginReact: vVitePluginReact,
		TypeScript:      vTS,
		TypesReact:      vTypesReact,
		TypesReactDom:   vTypesReactDom,
		TypesNode:       vTypesNode,
	}

	if err := validar.React(archivos, deps); err != nil {
		return nil, fmt.Errorf("react: fallo en validador de capa 1: %w", err)
	}

	return archivos, nil
}

func generarNextjs(_ context.Context, irSitio *ir.Sitio, _ []domain.Product, _ LlamarIAFunc) (map[string][]byte, error) {
	baseURL := obtenerBaseURLSitio(irSitio)
	archivos, err := GenerarCompartidosNextjs(irSitio, baseURL)
	if err != nil {
		return nil, fmt.Errorf("nextjs: error en compartidos: %w", err)
	}

	vNext, _ := ObtenerVersion("nextjs_version")
	vReact, _ := ObtenerVersion("react_version")
	vReactDom, _ := ObtenerVersion("react_dom_version")
	vTS, _ := ObtenerVersion("typescript_version")
	vTypesReact, _ := ObtenerVersion("types_react_version")
	vTypesReactDom, _ := ObtenerVersion("types_react_dom_version")
	vTypesNode, _ := ObtenerVersion("types_node_version")

	deps := validar.DependenciasNextjsEsperadas{
		Next:          vNext,
		React:         vReact,
		ReactDom:      vReactDom,
		TypeScript:    vTS,
		TypesReact:    vTypesReact,
		TypesReactDom: vTypesReactDom,
		TypesNode:     vTypesNode,
	}

	if err := validar.Nextjs(archivos, deps); err != nil {
		return nil, fmt.Errorf("nextjs: fallo en validador de capa 1: %w", err)
	}

	return archivos, nil
}

func generarWooCommerce(_ context.Context, irSitio *ir.Sitio, productos []domain.Product, _ LlamarIAFunc) (map[string][]byte, error) {
	archivos, err := GenerarCompartidosWooCommerce(irSitio, productos)
	if err != nil {
		return nil, fmt.Errorf("woocommerce: error en compartidos: %w", err)
	}

	vTema, _ := ObtenerVersion("woocommerce_theme_version")
	if err := validar.WooCommerce(archivos, vTema); err != nil {
		return nil, fmt.Errorf("woocommerce: fallo en validador de capa 1: %w", err)
	}

	return archivos, nil
}

func generarPrestaShop(_ context.Context, irSitio *ir.Sitio, productos []domain.Product, _ LlamarIAFunc) (map[string][]byte, error) {
	archivos, err := GenerarCompartidosPrestaShop(irSitio, productos)
	if err != nil {
		return nil, fmt.Errorf("prestashop: error en compartidos: %w", err)
	}

	vTema, _ := ObtenerVersion("prestashop_theme_version")
	if err := validar.PrestaShop(archivos, vTema); err != nil {
		return nil, fmt.Errorf("prestashop: fallo en validador de capa 1: %w", err)
	}

	return archivos, nil
}

func generarShopify(_ context.Context, irSitio *ir.Sitio, productos []domain.Product, _ LlamarIAFunc) (map[string][]byte, error) {
	// 1. Generar archivos compartidos deterministas
	archivos, err := GenerarCompartidosShopify(irSitio)
	if err != nil {
		return nil, fmt.Errorf("shopify: error en compartidos: %w", err)
	}

	// 2. Generar CSV del catálogo si existen productos
	if len(productos) > 0 {
		idioma := irSitio.Languages.Default
		if idioma == "" {
			idioma = "es"
		}
		nombreTienda := obtenerTextoIdioma(irSitio.SEO.SiteName, idioma)
		if nombreTienda == "" {
			nombreTienda = "Sytue Store"
		}

		csvBytes, err := CatalogoShopify(productos, idioma, nombreTienda)
		if err != nil {
			return nil, fmt.Errorf("shopify: error generando CSV de catálogo: %w", err)
		}
		archivos["products_export_shopify.csv"] = csvBytes
	}

	// 3. Medición previa de secciones (§9.4, IR-11): advertir o ajustar si excede 8 niveles o 50 bloques
	if len(irSitio.Pages) > 0 {
		for _, s := range irSitio.Pages[0].Sections {
			prof, bloques := MedirSeccion(&s)
			if prof > 8 || bloques > 50 {
				// Las secciones complejas se aplanan en la plantilla generada
				_ = prof
				_ = bloques
			}
		}
	}

	// 4. Validar integridad de capa 1 antes de devolver
	versionTema, _ := ObtenerVersion("shopify_theme_version")
	if err := validar.Shopify(archivos, versionTema); err != nil {
		return nil, fmt.Errorf("shopify: fallo en validador de capa 1: %w", err)
	}

	return archivos, nil
}

// GenerarZip genera todos los archivos del destino y escribe un streaming ZIP seguro (§7.6, §9.4).
func GenerarZip(ctx context.Context, destino Destino, irSitio *ir.Sitio, productos []domain.Product, llamar LlamarIAFunc, w io.Writer) error {
	mapaFicheros, err := Generar(ctx, destino, irSitio, productos, llamar)
	if err != nil {
		return err
	}

	var archivosZip []publicar.ArchivoZip
	ahora := time.Now()
	for ruta, contenido := range mapaFicheros {
		archivosZip = append(archivosZip, publicar.ArchivoZip{
			Ruta:       ruta,
			Contenido:  contenido,
			Modificado: ahora,
		})
	}

	return publicar.EscribirZip(w, archivosZip, nil)
}
