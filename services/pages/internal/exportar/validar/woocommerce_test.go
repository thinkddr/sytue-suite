// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package validar

import (
	"errors"
	"testing"
)

func archivosWooCommerceBase() map[string][]byte {
	return map[string][]byte{
		"style.css": []byte(`/*
Theme Name: Sytue Store
Version: 1.0.0
*/`),
		"theme.json": []byte(`{
  "version": 3,
  "settings": {}
}`),
		"functions.php": []byte(`<?php add_theme_support('woocommerce');`),
		"templates/index.html": []byte(`<!-- wp:group {"layout":{"type":"constrained"}} -->
<div>
  <!-- wp:site-title /-->
  <!-- wp:paragraph -->
  <p>Hola mundo</p>
  <!-- /wp:paragraph -->
</div>
<!-- /wp:group -->`),
		"screenshot.png": {0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00},
		"README.md":      []byte(`# WooCommerce`),
	}
}

func TestWooCommerceValido(t *testing.T) {
	archivos := archivosWooCommerceBase()
	if err := WooCommerce(archivos, "1.0.0"); err != nil {
		t.Fatalf("WooCommerce devolvió error inesperado: %v", err)
	}
}

func TestWooCommerceArchivoFaltante(t *testing.T) {
	archivos := archivosWooCommerceBase()
	delete(archivos, "screenshot.png")

	err := WooCommerce(archivos, "1.0.0")
	if !errors.Is(err, ErrWooCommerceArchivoFaltante) {
		t.Fatalf("esperaba ErrWooCommerceArchivoFaltante, obtuvo: %v", err)
	}
}

func TestWooCommerceThemeJSONInvalido(t *testing.T) {
	archivos := archivosWooCommerceBase()
	archivos["theme.json"] = []byte(`{"version": 2}`)

	err := WooCommerce(archivos, "1.0.0")
	if !errors.Is(err, ErrWooCommerceThemeJSONInvalido) {
		t.Fatalf("esperaba ErrWooCommerceThemeJSONInvalido, obtuvo: %v", err)
	}
}

func TestWooCommerceBloquesDesbalanceados(t *testing.T) {
	archivos := archivosWooCommerceBase()
	archivos["templates/index.html"] = []byte(`<!-- wp:group -->
<div>
  <!-- wp:paragraph -->
  <p>Falta cierre</p>
</div>`)

	err := WooCommerce(archivos, "1.0.0")
	if !errors.Is(err, ErrWooCommerceBloquesDesbalance) {
		t.Fatalf("esperaba ErrWooCommerceBloquesDesbalance, obtuvo: %v", err)
	}
}

func TestWooCommercePNGInvalido(t *testing.T) {
	archivos := archivosWooCommerceBase()
	archivos["screenshot.png"] = []byte(`no-es-png`)

	err := WooCommerce(archivos, "1.0.0")
	if !errors.Is(err, ErrWooCommercePNGInvalido) {
		t.Fatalf("esperaba ErrWooCommercePNGInvalido, obtuvo: %v", err)
	}
}

func TestWooCommerceStyleCSSInvalido(t *testing.T) {
	archivos := archivosWooCommerceBase()
	archivos["style.css"] = []byte(`/* solo css */`)

	err := WooCommerce(archivos, "1.0.0")
	if !errors.Is(err, ErrWooCommerceStyleCSSInvalido) {
		t.Fatalf("esperaba ErrWooCommerceStyleCSSInvalido, obtuvo: %v", err)
	}
}
