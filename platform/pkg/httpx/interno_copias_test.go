// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El chequeo del secreto interno vive en SecretoInterno y en ningún otro sitio: contacts,
// rooms, hub e id llevaban cada uno su copia (con dos formas distintas de leer el Bearer).
// Una copia se reconoce por su mensaje de error, que es el de aquí.
func TestNadieCopiaElChequeoDelSecretoInterno(t *testing.T) {
	raiz := filepath.Join("..", "..", "..")
	var copias []string
	for _, dir := range []string{"services", "platform", "apps"} {
		_ = filepath.WalkDir(filepath.Join(raiz, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil //nolint:nilerr // un directorio ilegible no es una copia
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "vendor") {
				return filepath.SkipDir
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") || strings.HasSuffix(p, "interno.go") {
				return nil
			}
			if b, _ := os.ReadFile(p); strings.Contains(string(b), `"Invalid internal secret."`) {
				copias = append(copias, p)
			}
			return nil
		})
	}
	if len(copias) > 0 {
		t.Errorf("chequeo del secreto interno copiado; usad httpx.SecretoInterno: %v", copias)
	}
}
