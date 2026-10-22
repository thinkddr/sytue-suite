// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrArchivoFaltante    = errors.New("shopify validador: falta archivo obligatorio")
	ErrJSONInvalido       = errors.New("shopify validador: archivo JSON malformado")
	ErrFaltaHeaderLayout  = errors.New("shopify validador: layout/theme.liquid debe contener content_for_header y content_for_layout")
	ErrThemeInfoInvalido  = errors.New("shopify validador: settings_schema.json debe contener theme_info con versión exacta")
	ErrSeccionInexistente = errors.New("shopify validador: sección referenciada en plantilla no existe")
	ErrTraduccionFaltante = errors.New("shopify validador: clave de traducción referenciada no existe en en.default.json")
	ErrLimiteSuperado     = errors.New("shopify validador: límite de Shopify superado (longitud, anidamiento o bloques)")
	ErrScriptExterno      = errors.New("shopify validador: no se permiten scripts o estilos externos en CDNs")
	ErrRutaPeligrosa      = errors.New("shopify validador: ruta de archivo no permitida o con escape de directorio")
)

// Archivos obligatorios del tema Shopify (§9.4).
var archivosObligatorios = []string{
	"layout/theme.liquid",
	"config/settings_schema.json",
	"config/settings_data.json",
	"locales/en.default.json",
	"templates/index.json",
}

var (
	reContentForHeader = regexp.MustCompile(`\{\{\s*content_for_header\s*\}\}`)
	reContentForLayout = regexp.MustCompile(`\{\{\s*content_for_layout\s*\}\}`)
	reSchemaLiquid     = regexp.MustCompile(`(?s)\{%\s*schema\s*%\}(.*?)\{%\s*endschema\s*%\}`)
	reLiquidTrad       = regexp.MustCompile(`\{\{\s*['"]([a-zA-Z0-9_.-]+)['"]\s*\|\s*t\s*\}\}`)
	reLiquidSchemaTrad = regexp.MustCompile(`"t:([a-zA-Z0-9_.-]+)"`)
	reExternalScript   = regexp.MustCompile(`(?i)<script[^>]+src=["']https?://`)
	reExternalCSS      = regexp.MustCompile(`(?i)<link[^>]+href=["']https?://[^"']*\.css`)
)

// Shopify ejecuta la validación de capa 1 (estructural, sin red) para un tema exportado.
func Shopify(archivos map[string][]byte, versionEsperada ...string) error {
	if err := validarEstructuraArchivos(archivos); err != nil {
		return err
	}
	if err := validarThemeLiquid(archivos["layout/theme.liquid"]); err != nil {
		return err
	}

	var vEsperada string
	if len(versionEsperada) > 0 {
		vEsperada = versionEsperada[0]
	}
	if err := validarThemeInfo(archivos["config/settings_schema.json"], vEsperada); err != nil {
		return err
	}

	clavesLocales, err := extraerClavesLocales(archivos["locales/en.default.json"])
	if err != nil {
		return fmt.Errorf("error analizando locales: %w", err)
	}

	return validarContenidosArchivos(archivos, clavesLocales)
}

func validarEstructuraArchivos(archivos map[string][]byte) error {
	for ruta := range archivos {
		if err := validarRutaSegura(ruta); err != nil {
			return fmt.Errorf("%w: %s", ErrRutaPeligrosa, ruta)
		}
	}
	for _, req := range archivosObligatorios {
		if _, ok := archivos[req]; !ok {
			return fmt.Errorf("%w: %s", ErrArchivoFaltante, req)
		}
	}
	for ruta, contenido := range archivos {
		if strings.HasSuffix(ruta, ".json") {
			var js any
			if err := json.Unmarshal(contenido, &js); err != nil {
				return fmt.Errorf("%w en %s: %w", ErrJSONInvalido, ruta, err)
			}
		}
	}
	return nil
}

func validarThemeLiquid(contenido []byte) error {
	s := string(contenido)
	if !reContentForHeader.MatchString(s) || !reContentForLayout.MatchString(s) {
		return ErrFaltaHeaderLayout
	}
	return nil
}

func validarContenidosArchivos(archivos map[string][]byte, clavesLocales map[string]bool) error {
	seccionesExistentes := make(map[string]bool)
	for ruta := range archivos {
		if strings.HasPrefix(ruta, "sections/") {
			base := strings.TrimPrefix(ruta, "sections/")
			if strings.HasSuffix(base, ".liquid") {
				seccionesExistentes[strings.TrimSuffix(base, ".liquid")] = true
			} else if strings.HasSuffix(base, ".json") {
				seccionesExistentes[strings.TrimSuffix(base, ".json")] = true
			}
		}
	}

	for ruta, contenido := range archivos {
		if err := validarArchivoIndividual(ruta, contenido, seccionesExistentes, clavesLocales); err != nil {
			return err
		}
	}
	return nil
}

func validarArchivoIndividual(ruta string, contenido []byte, seccionesExistentes, clavesLocales map[string]bool) error {
	if strings.HasPrefix(ruta, "templates/") && strings.HasSuffix(ruta, ".json") {
		if err := validarPlantillaJSON(contenido, seccionesExistentes); err != nil {
			return fmt.Errorf("error en plantilla %s: %w", ruta, err)
		}
	}
	if strings.HasPrefix(ruta, "sections/") && strings.HasSuffix(ruta, ".liquid") {
		if err := validarSeccionLiquid(contenido); err != nil {
			return fmt.Errorf("error en sección %s: %w", ruta, err)
		}
	}
	if strings.HasSuffix(ruta, ".liquid") || strings.HasSuffix(ruta, ".json") {
		if err := validarTraducciones(contenido, clavesLocales); err != nil {
			return fmt.Errorf("error de traducción en %s: %w", ruta, err)
		}
	}
	if strings.HasSuffix(ruta, ".liquid") || strings.HasSuffix(ruta, ".html") {
		if reExternalScript.Match(contenido) || reExternalCSS.Match(contenido) {
			return fmt.Errorf("%w en %s", ErrScriptExterno, ruta)
		}
	}
	return nil
}

func validarRutaSegura(ruta string) error {
	if strings.HasPrefix(ruta, "/") || strings.Contains(ruta, "\\") {
		return fmt.Errorf("la ruta debe ser relativa: %s", ruta)
	}
	limpia := filepath.Clean(ruta)
	if strings.HasPrefix(limpia, "../") || limpia == ".." {
		return fmt.Errorf("ruta con escape de directorio: %s", ruta)
	}
	if len(ruta) > 255 {
		return fmt.Errorf("ruta demasiado larga: %s", ruta)
	}
	return nil
}

func validarThemeInfo(data []byte, versionEsperada string) error {
	var schema []map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		return err
	}

	var encontrado bool
	for _, secc := range schema {
		name, _ := secc["name"].(string)
		if strings.EqualFold(name, "theme_info") || strings.EqualFold(name, "theme info") {
			encontrado = true
			themeVersion, _ := secc["theme_version"].(string)
			if themeVersion == "" {
				return fmt.Errorf("%w: theme_info debe incluir theme_version", ErrThemeInfoInvalido)
			}
			if versionEsperada != "" && themeVersion != versionEsperada {
				return fmt.Errorf("%w: versión del tema %q no coincide con %q",
					ErrThemeInfoInvalido, themeVersion, versionEsperada)
			}
			break
		}
	}
	if !encontrado {
		return fmt.Errorf("%w: no se encontró bloque theme_info", ErrThemeInfoInvalido)
	}
	return nil
}

func validarPlantillaJSON(data []byte, seccionesExistentes map[string]bool) error {
	var plantilla struct {
		Sections map[string]struct {
			Type string `json:"type"`
		} `json:"sections"`
		Order []string `json:"order"`
	}
	if err := json.Unmarshal(data, &plantilla); err != nil {
		return err
	}

	for id, secc := range plantilla.Sections {
		tipo := secc.Type
		if tipo == "" {
			continue
		}
		// Secciones estándar permitidas o existentes
		if !seccionesExistentes[tipo] && !esSeccionEspecialShopify(tipo) {
			return fmt.Errorf("%w: sección %q (id %s)", ErrSeccionInexistente, tipo, id)
		}
	}
	return nil
}

func esSeccionEspecialShopify(tipo string) bool {
	// Secciones integradas o grupos de Shopify
	especiales := map[string]bool{
		"header-group":    true,
		"footer-group":    true,
		"main-product":    true,
		"main-collection": true,
		"main-cart":       true,
		"main-404":        true,
	}
	return especiales[tipo]
}

type schemaAjuste struct {
	Type string `json:"type"`
	Min  *int   `json:"min,omitempty"`
	Max  *int   `json:"max,omitempty"`
}

type schemaBloque struct {
	Type     string         `json:"type"`
	Name     string         `json:"name"`
	Settings []schemaAjuste `json:"settings"`
}

func validarSeccionLiquid(data []byte) error {
	matches := reSchemaLiquid.FindSubmatch(data)
	if len(matches) < 2 {
		return nil // Sección sin schema estricto
	}

	var schema struct {
		Name     string         `json:"name"`
		Blocks   []schemaBloque `json:"blocks"`
		Settings []schemaAjuste `json:"settings"`
	}

	if err := json.Unmarshal(matches[1], &schema); err != nil {
		return fmt.Errorf("JSON de schema inválido: %w", err)
	}

	if len(schema.Name) > 50 {
		return fmt.Errorf("%w: nombre de sección '%s' excede 50 caracteres", ErrLimiteSuperado, schema.Name)
	}
	if len(schema.Blocks) > 50 {
		return fmt.Errorf("%w: sección con más de 50 bloques", ErrLimiteSuperado)
	}

	for _, b := range schema.Blocks {
		if err := validarBloqueSchema(b); err != nil {
			return err
		}
	}
	for _, s := range schema.Settings {
		if err := validarAjusteRange(s); err != nil {
			return err
		}
	}
	return nil
}

func validarBloqueSchema(b schemaBloque) error {
	if len(b.Type) > 25 {
		return fmt.Errorf("%w: tipo de bloque '%s' excede 25 caracteres", ErrLimiteSuperado, b.Type)
	}
	if len(b.Name) > 50 {
		return fmt.Errorf("%w: nombre de bloque '%s' excede 50 caracteres", ErrLimiteSuperado, b.Name)
	}
	for _, s := range b.Settings {
		if err := validarAjusteRange(s); err != nil {
			return err
		}
	}
	return nil
}

func validarAjusteRange(s schemaAjuste) error {
	if s.Type != "range" {
		return nil
	}
	if s.Min != nil && (*s.Min < 1 || *s.Min > 250) {
		return fmt.Errorf("%w: ajuste range min %d fuera de rango 1-250", ErrLimiteSuperado, *s.Min)
	}
	if s.Max != nil && (*s.Max < 1 || *s.Max > 250) {
		return fmt.Errorf("%w: ajuste range max %d fuera de rango 1-250", ErrLimiteSuperado, *s.Max)
	}
	return nil
}

func extraerClavesLocales(data []byte) (map[string]bool, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	claves := make(map[string]bool)
	aplanarClaves("", m, claves)
	return claves, nil
}

func aplanarClaves(prefijo string, val any, res map[string]bool) {
	switch v := val.(type) {
	case map[string]any:
		for k, hijo := range v {
			nueva := k
			if prefijo != "" {
				nueva = prefijo + "." + k
			}
			aplanarClaves(nueva, hijo, res)
		}
	default:
		if prefijo != "" {
			res[prefijo] = true
		}
	}
}

func validarTraducciones(contenido []byte, clavesLocales map[string]bool) error {
	// Comprobar {{ 'clave' | t }}
	matches := reLiquidTrad.FindAllSubmatch(contenido, -1)
	for _, m := range matches {
		clave := string(m[1])
		if !clavesLocales[clave] {
			return fmt.Errorf("%w: %q", ErrTraduccionFaltante, clave)
		}
	}

	// Comprobar "t:clave"
	matchesSchema := reLiquidSchemaTrad.FindAllSubmatch(contenido, -1)
	for _, m := range matchesSchema {
		clave := string(m[1])
		if !clavesLocales[clave] {
			return fmt.Errorf("%w: %q", ErrTraduccionFaltante, clave)
		}
	}

	return nil
}
