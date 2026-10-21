// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrWooCommerceArchivoFaltante   = errors.New("woocommerce validador: falta archivo obligatorio")
	ErrWooCommerceThemeJSONInvalido = errors.New("woocommerce validador: theme.json debe ser versión 3 válida")
	ErrWooCommerceBloquesDesbalance = errors.New("woocommerce validador: comentarios de bloque WordPress desbalanceados")
	ErrWooCommercePNGInvalido       = errors.New("woocommerce validador: screenshot.png no es una imagen PNG válida")
	ErrWooCommerceStyleCSSInvalido  = errors.New("woocommerce validador: style.css debe contener cabeceras de tema de WordPress")
)

var archivosObligatoriosWooCommerce = []string{
	"style.css",
	"theme.json",
	"functions.php",
	"templates/index.html",
	"screenshot.png",
	"README.md",
}

var (
	pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	// Bloque auto-cerrado: <!-- wp:nombre {...}\s*/-->.
	reWPSelfClosing = regexp.MustCompile(`<!--\s+wp:([a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)?)\s*(\{.*?\})?\s*/-->`)
)

// WooCommerce valida la capa 1 (estructural, sin red) de un tema de bloques WooCommerce exportado (§9.4).
func WooCommerce(archivos map[string][]byte, versionEsperada ...string) error {
	// 1. Validar rutas seguras
	for ruta := range archivos {
		if err := validarRutaSegura(ruta); err != nil {
			return fmt.Errorf("%w: %s", ErrRutaPeligrosa, ruta)
		}
	}

	// 2. Comprobar archivos obligatorios
	for _, req := range archivosObligatoriosWooCommerce {
		if _, ok := archivos[req]; !ok {
			return fmt.Errorf("%w: %s", ErrWooCommerceArchivoFaltante, req)
		}
	}

	// 3. Validar theme.json (especificación v3)
	if err := validarThemeJSON(archivos["theme.json"]); err != nil {
		return err
	}

	// 4. Validar style.css
	var vEsperada string
	if len(versionEsperada) > 0 {
		vEsperada = versionEsperada[0]
	}
	if err := validarStyleCSS(archivos["style.css"], vEsperada); err != nil {
		return err
	}

	// 5. Validar screenshot.png
	if !bytes.HasPrefix(archivos["screenshot.png"], pngSignature) {
		return ErrWooCommercePNGInvalido
	}

	// 6. Validar comentarios de bloque equilibrados en todas las plantillas y partes HTML
	for ruta, contenido := range archivos {
		if strings.HasSuffix(ruta, ".html") && (strings.HasPrefix(ruta, "templates/") || strings.HasPrefix(ruta, "parts/")) {
			if err := validarBloquesWordPress(contenido); err != nil {
				return fmt.Errorf("%w en %s: %w", ErrWooCommerceBloquesDesbalance, ruta, err)
			}
		}
	}

	return nil
}

func validarThemeJSON(data []byte) error {
	var tj struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &tj); err != nil {
		return fmt.Errorf("%w: JSON malformado: %w", ErrWooCommerceThemeJSONInvalido, err)
	}
	if tj.Version != 3 {
		return fmt.Errorf("%w: versión esperada 3, obtenida %d", ErrWooCommerceThemeJSONInvalido, tj.Version)
	}
	return nil
}

func validarStyleCSS(data []byte, versionEsperada string) error {
	s := string(data)
	if !strings.Contains(s, "Theme Name:") {
		return fmt.Errorf("%w: falta 'Theme Name:'", ErrWooCommerceStyleCSSInvalido)
	}
	if !strings.Contains(s, "Version:") {
		return fmt.Errorf("%w: falta 'Version:'", ErrWooCommerceStyleCSSInvalido)
	}
	if versionEsperada != "" && !strings.Contains(s, "Version: "+versionEsperada) {
		return fmt.Errorf("%w: versión en style.css no coincide con %q", ErrWooCommerceStyleCSSInvalido, versionEsperada)
	}
	return nil
}

// validarBloquesWordPress verifica que los bloques abiertos tengan su correspondiente bloque de cierre.
func validarBloquesWordPress(data []byte) error {
	texto := string(data)

	// Eliminamos los bloques auto-cerrados para que no interfieran
	textoSinAutoCierre := reWPSelfClosing.ReplaceAllString(texto, "")

	// Tokenizamos aberturas y cierres en orden
	reToken := regexp.MustCompile(`<!--\s+(/)?wp:([a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)?)[^>]*-->`)
	tokens := reToken.FindAllStringSubmatch(textoSinAutoCierre, -1)

	var pila []string
	for _, tok := range tokens {
		esCierre := tok[1] == "/"
		nombreBloque := tok[2]

		if !esCierre {
			pila = append(pila, nombreBloque)
		} else {
			if len(pila) == 0 {
				return fmt.Errorf("cierre de bloque inesperado <!-- /wp:%s -->", nombreBloque)
			}
			ultimo := pila[len(pila)-1]
			pila = pila[:len(pila)-1]
			if ultimo != nombreBloque {
				return fmt.Errorf("bloque desbalanceado: se abrió <!-- wp:%s --> pero se cerró <!-- /wp:%s -->", ultimo, nombreBloque)
			}
		}
	}

	if len(pila) > 0 {
		return fmt.Errorf("bloques sin cerrar al final: %v", pila)
	}

	return nil
}
