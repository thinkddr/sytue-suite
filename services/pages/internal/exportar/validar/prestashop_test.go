// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"errors"
	"testing"
)

func archivosPrestaShopBase() map[string][]byte {
	return map[string][]byte{
		"config/theme.yml": []byte(`name: sytue_store
version: '1.0.0'
parent: hummingbird
`),
		"preview.png":         {0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00},
		"templates/index.tpl": []byte(`{extends file='page.tpl'}`),
		"README.md":           []byte(`# PrestaShop`),
	}
}

func TestPrestaShopValido(t *testing.T) {
	archivos := archivosPrestaShopBase()
	if err := PrestaShop(archivos, "1.0.0"); err != nil {
		t.Fatalf("PrestaShop devolvió error inesperado: %v", err)
	}
}

func TestPrestaShopArchivoFaltante(t *testing.T) {
	archivos := archivosPrestaShopBase()
	delete(archivos, "preview.png")

	err := PrestaShop(archivos, "1.0.0")
	if !errors.Is(err, ErrPrestaShopArchivoFaltante) {
		t.Fatalf("esperaba ErrPrestaShopArchivoFaltante, obtuvo: %v", err)
	}
}

func TestPrestaShopSinPadreHummingbird(t *testing.T) {
	archivos := archivosPrestaShopBase()
	archivos["config/theme.yml"] = []byte(`name: sytue_store
version: '1.0.0'
parent: classic
`)

	err := PrestaShop(archivos, "1.0.0")
	if !errors.Is(err, ErrPrestaShopPadreHummingbird) {
		t.Fatalf("esperaba ErrPrestaShopPadreHummingbird, obtuvo: %v", err)
	}
}

func TestPrestaShopPNGInvalido(t *testing.T) {
	archivos := archivosPrestaShopBase()
	archivos["preview.png"] = []byte(`corrupto`)

	err := PrestaShop(archivos, "1.0.0")
	if !errors.Is(err, ErrPrestaShopPNGInvalido) {
		t.Fatalf("esperaba ErrPrestaShopPNGInvalido, obtuvo: %v", err)
	}
}

func TestPrestaShopVersionNoCoincide(t *testing.T) {
	archivos := archivosPrestaShopBase()

	err := PrestaShop(archivos, "2.0.0")
	if !errors.Is(err, ErrPrestaShopVersionNoCoincide) {
		t.Fatalf("esperaba ErrPrestaShopVersionNoCoincide, obtuvo: %v", err)
	}
}
