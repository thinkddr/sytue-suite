// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrPrestaShopArchivoFaltante   = errors.New("prestashop validador: falta archivo obligatorio")
	ErrPrestaShopYAMLInvalido      = errors.New("prestashop validador: theme.yml malformado")
	ErrPrestaShopPadreHummingbird  = errors.New("prestashop validador: theme.yml debe declarar obligatoriamente 'parent: hummingbird'")
	ErrPrestaShopPNGInvalido       = errors.New("prestashop validador: preview.png no es una imagen PNG válida")
	ErrPrestaShopVersionNoCoincide = errors.New("prestashop validador: la versión del tema no coincide con la esperada")
)

var archivosObligatoriosPrestaShop = []string{
	"config/theme.yml",
	"preview.png",
	"templates/index.tpl",
	"README.md",
}

// PrestaShop valida la capa 1 (estructural, sin red) de un tema PrestaShop exportado (§9.4).
func PrestaShop(archivos map[string][]byte, versionEsperada ...string) error {
	// 1. Validar rutas seguras
	for ruta := range archivos {
		if err := validarRutaSegura(ruta); err != nil {
			return fmt.Errorf("%w: %s", ErrRutaPeligrosa, ruta)
		}
	}

	// 2. Comprobar archivos obligatorios
	for _, req := range archivosObligatoriosPrestaShop {
		if _, ok := archivos[req]; !ok {
			return fmt.Errorf("%w: %s", ErrPrestaShopArchivoFaltante, req)
		}
	}

	// 3. Validar config/theme.yml con padre Hummingbird (§9.4)
	var vEsperada string
	if len(versionEsperada) > 0 {
		vEsperada = versionEsperada[0]
	}
	if err := validarThemeYML(archivos["config/theme.yml"], vEsperada); err != nil {
		return err
	}

	// 4. Validar preview.png
	if !bytes.HasPrefix(archivos["preview.png"], pngSignature) {
		return ErrPrestaShopPNGInvalido
	}

	return nil
}

func validarThemeYML(data []byte, versionEsperada string) error {
	var cfg struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		Parent  string `yaml:"parent"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("%w: %w", ErrPrestaShopYAMLInvalido, err)
	}

	if strings.ToLower(strings.TrimSpace(cfg.Parent)) != "hummingbird" {
		return fmt.Errorf("%w (encontrado: %q)", ErrPrestaShopPadreHummingbird, cfg.Parent)
	}

	if versionEsperada != "" && cfg.Version != versionEsperada {
		return fmt.Errorf("%w: tema tiene %q pero se esperaba %q",
			ErrPrestaShopVersionNoCoincide, cfg.Version, versionEsperada)
	}

	return nil
}
