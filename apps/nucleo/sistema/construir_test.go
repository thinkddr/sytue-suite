// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sistema

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNadieUsaJniNewObject: `jni.NewObject` de git.wow.st/gmp/jni pasa mal los argumentos
// al constructor (llama a la variante de puntos suspensivos con un `jvalue*`) y tumba la
// app en el teléfono: así se caía Drive al guardar la sesión en el Keystore. Se construye
// con Construir. Esto recorre el núcleo y las apps de Fyne.
//
// VISTO EN ROJO con el `jni.NewObject` que tenía auth/store_android.go.
func TestNadieUsaJniNewObject(t *testing.T) {
	for _, raiz := range []string{"..", "../../fyne"} {
		err := filepath.WalkDir(raiz, func(ruta string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(ruta, ".go") || strings.HasSuffix(ruta, "_test.go") {
				return err
			}
			b, err := os.ReadFile(ruta)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), "jni.NewObject(") {
				t.Errorf("%s usa jni.NewObject: construye con sistema.Construir", ruta)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
