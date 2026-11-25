// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Las variantes descomprimen al original y salen iguales byte a byte en dos pasadas:
// el núcleo de cliente se genera en el CI y en el Dockerfile, y Rooms las incrusta.
func TestPrecomprimirEsDeterministaYReversible(t *testing.T) {
	dir := t.TempDir()
	fichero := filepath.Join(dir, "core.wasm")
	crudo := bytes.Repeat([]byte("\x00asm\x01\x00\x00\x00 sección de código "), 2048)
	if err := os.WriteFile(fichero, crudo, 0o644); err != nil {
		t.Fatal(err)
	}
	primera := map[string][]byte{}
	for pasada := range 2 {
		tamanos, err := precomprimir(fichero)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range variantes {
			datos, err := os.ReadFile(fichero + v.extension)
			if err != nil {
				t.Fatal(err)
			}
			if len(datos) != tamanos[v.extension] || len(datos) >= len(crudo) {
				t.Fatalf("%s: %d bytes, informó %d", v.extension, len(datos), tamanos[v.extension])
			}
			if pasada == 0 {
				primera[v.extension] = datos
			} else if !bytes.Equal(primera[v.extension], datos) {
				t.Fatalf("%s no es determinista", v.extension)
			}
		}
	}
	if sobrantes, _ := filepath.Glob(filepath.Join(dir, ".*")); len(sobrantes) != 0 {
		t.Fatal("quedaron temporales", sobrantes)
	}
}
