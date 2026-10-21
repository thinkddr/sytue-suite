// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package publicar

import (
	"strings"
	"testing"
)

func TestValidarRecursosLigadosATokenPreview(t *testing.T) {
	p, err := CargarPermitido()
	if err != nil {
		t.Fatal(err)
	}
	base := "/_vista/0123456789abcdef0123456789abcdef"
	archivos := []FicheroSalida{
		{Ruta: "_astro/0123456789abcdef.css", Contenido: []byte(`@font-face{font-family:Inter;src:url(` + base + `/_astro/inter.woff2)}`)},
		{Ruta: "es/index.html", Contenido: []byte(`<html><head><link rel="stylesheet" href="` + base + `/_astro/0123456789abcdef.css"></head><body><div class="sp-vista" role="status">Preview</div><img src="` + base + `/_astro/foto.png" alt="Foto"></body></html>`)},
	}
	opts := OpcionesValidacion{Kind: "preview", PreviewBase: base, RecursosValidos: map[string]bool{"/_astro/inter.woff2": true, "/_astro/foto.png": true}}
	if err := ValidarSalida(p, archivos, opts); err != nil {
		t.Fatalf("preview tokenizada rechazada: %v", err)
	}
	for _, tc := range []struct {
		name     string
		opts     OpcionesValidacion
		archivos []FicheroSalida
	}{
		{"font inexistente", OpcionesValidacion{Kind: "preview", PreviewBase: base, RecursosValidos: map[string]bool{"/_astro/foto.png": true}}, archivos},
		{"imagen inexistente", OpcionesValidacion{Kind: "preview", PreviewBase: base, RecursosValidos: map[string]bool{"/_astro/inter.woff2": true}}, archivos},
		{"base live", OpcionesValidacion{Kind: "live", PreviewBase: base}, archivos},
		{"base traversal", OpcionesValidacion{Kind: "preview", PreviewBase: "/_vista/../otro"}, archivos},
		{"CSS token ajeno", opts, []FicheroSalida{{Ruta: archivos[0].Ruta, Contenido: []byte(strings.ReplaceAll(string(archivos[0].Contenido), base, "/_vista/ffffffffffffffffffffffffffffffff"))}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidarSalida(p, tc.archivos, tc.opts); err == nil {
				t.Fatal("salida inválida aceptada")
			}
		})
	}
	// La validación trabaja sobre una copia; no cambia hashes ni bytes a publicar.
	if !strings.Contains(string(archivos[0].Contenido), base) {
		t.Fatal("validación mutó CSS")
	}
}
