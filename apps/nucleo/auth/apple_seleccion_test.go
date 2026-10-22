// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package auth

import (
	"go/build"
	"slices"
	"testing"
)

// iOS también activa la etiqueta darwin. No debe compilar los adaptadores de
// desarrollo del Mac ni enlazar SecTask, que sólo existe en macOS.
func TestSeleccionDeAlmacenApple(t *testing.T) {
	for _, caso := range []struct {
		nombre, os string
		tags       []string
		privado    bool
	}{
		{nombre: "ios", os: "ios"},
		{nombre: "mac-tienda", os: "darwin", tags: []string{"tienda"}},
		{nombre: "mac-developer-id", os: "darwin", privado: true},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			ctx := build.Default
			ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled, ctx.BuildTags = caso.os, "arm64", true, caso.tags
			paquete, err := ctx.ImportDir(".", 0)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(paquete.GoFiles, "store.go") != caso.privado ||
				slices.Contains(paquete.GoFiles, "store_apple.go") == caso.privado ||
				slices.Contains(paquete.CgoFiles, "compartido_darwin.go") != caso.privado ||
				!slices.Contains(paquete.CgoFiles, "llavero_apple.go") ||
				!slices.Contains(paquete.CFiles, "store_apple.c") {
				t.Fatalf("adaptadores de Keychain incompatibles: go=%v cgo=%v c=%v", paquete.GoFiles, paquete.CgoFiles, paquete.CFiles)
			}
			base, err := ctx.ImportDir("../ajustes", 0)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(base.GoFiles, "base_desarrollo_darwin.go") != caso.privado ||
				slices.Contains(base.GoFiles, "base_firmada_apple.go") == caso.privado ||
				slices.Contains(base.CgoFiles, "proteccion_ios.go") != (caso.os == "ios") ||
				slices.Contains(base.GoFiles, "proteccion_otros.go") == (caso.os == "ios") ||
				!slices.Contains(base.CgoFiles, "base_apple.go") {
				t.Fatalf("contenedores Apple incompatibles: go=%v cgo=%v", base.GoFiles, base.CgoFiles)
			}
		})
	}
}
