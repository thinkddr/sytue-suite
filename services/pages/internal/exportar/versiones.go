// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// FechaRevision es la fecha de la última revisión autoritativa de versiones (§9.4, IR-25).
const FechaRevision = "2026-09-30"

// CaducidadDias es el plazo máximo (90 días) antes de exigir revisión obligatoria de versiones.
const CaducidadDias = 90

// Tabla define las versiones exactas y autoritativas de dependencias y herramientas para cada destino.
// Ningún fichero de especificación (fichas/*.md) o plantilla puede escribir versiones crudas; deben
// usar marcadores {{v.<clave>}}.
var Tabla = map[string]string{
	"shopify_theme_version":       "1.0.0",
	"shopify_cli_version":         "3.68.0",
	"shopify_theme_check_version": "2.18.0",
	// Estas tres versiones se reflejan desde el catálogo/bun.lock del frontal.
	// Se fijan sin rangos en los ZIP generados para que la exportación sea
	// reproducible y no cambie al instalarla semanas después.
	"astro_version":                  "7.3.3",
	"astro_preact_version":           "6.0.5",
	"preact_version":                 "10.29.8",
	"react_version":                  "19.0.0",
	"react_dom_version":              "19.0.0",
	"vite_version":                   "6.2.0",
	"vite_plugin_react_version":      "4.3.4",
	"typescript_version":             "5.7.3",
	"types_react_version":            "19.0.10",
	"types_react_dom_version":        "19.0.4",
	"types_node_version":             "22.13.5",
	"nextjs_version":                 "15.2.0",
	"tailwindcss_version":            "4.0.9",
	"woocommerce_version":            "9.6.0",
	"woocommerce_theme_version":      "1.0.0",
	"wordpress_version":              "6.7.2",
	"prestashop_version":             "8.2.0",
	"prestashop_theme_version":       "1.0.0",
	"prestashop_hummingbird_version": "1.0.0",
	"shopware_version":               "6.6.9",
	"magento_version":                "2.4.7",
	"bigcommerce_stencil_version":    "7.0.0",
	"flutter_version":                "3.29.0",
	"compose_version":                "1.7.8",
	"swiftui_version":                "6.0",
	"fyne_version":                   "2.5.4",
}

var reMarcador = regexp.MustCompile(`\{\{v\.([a-zA-Z0-9_-]+)\}\}`)

// ValidarCaducidad comprueba que la tabla de versiones no ha superado los 90 días desde su revisión.
func ValidarCaducidad() error {
	tRev, err := time.Parse("2006-01-02", FechaRevision)
	if err != nil {
		return fmt.Errorf("versiones: formato de FechaRevision inválido: %w", err)
	}
	caducaEn := tRev.AddDate(0, 0, CaducidadDias)
	ahora := time.Now()
	if ahora.After(caducaEn) {
		return fmt.Errorf("versiones: tabla caducada (revisión %s caducó en %s, han pasado más de %d días)",
			FechaRevision, caducaEn.Format("2006-01-02"), CaducidadDias)
	}
	return nil
}

// ReemplazarMarcadores sustituye todos los marcadores {{v.<clave>}} por sus valores de la tabla.
// Si un marcador no existe en la tabla, devuelve un error indicando la clave faltante.
func ReemplazarMarcadores(texto string) (string, error) {
	var errFalta error
	resultado := reMarcador.ReplaceAllStringFunc(texto, func(m string) string {
		submatch := reMarcador.FindStringSubmatch(m)
		if len(submatch) < 2 {
			return m
		}
		clave := submatch[1]
		val, ok := Tabla[clave]
		if !ok {
			if errFalta == nil {
				errFalta = fmt.Errorf("versiones: marcador desconocido {{v.%s}}", clave)
			}
			return m
		}
		return val
	})
	if errFalta != nil {
		return "", errFalta
	}
	return resultado, nil
}

// ObtenerVersion devuelve la versión exacta para una clave dada, o un error si no existe.
func ObtenerVersion(clave string) (string, error) {
	val, ok := Tabla[clave]
	if !ok {
		return "", fmt.Errorf("versiones: clave '%s' no encontrada", clave)
	}
	return val, nil
}

// Errores tipados.
var (
	ErrVersionCaducada = errors.New("versiones: la tabla de dependencias ha caducado")
)
