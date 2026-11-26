// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package archivos

import (
	"path/filepath"
	"testing"
)

// Buscar por nombre y por etiqueta sigue la regla de apps/nucleo/buscar: «cancion»
// encuentra «Canción.txt» y «educacion», lo que lleva la etiqueta «Educación».
func TestLaBusquedaDeFicherosNoDistingueAcentos(t *testing.T) {
	a, _ := abrirPrueba(t)
	ctx := t.Context()
	cancion := filepath.Join(a.Raiz(), "Canción.txt")
	escribirPrueba(t, cancion, "la")
	otro := filepath.Join(a.Raiz(), "notas.txt")
	escribirPrueba(t, otro, "x")
	if err := a.PonerEtiquetas(ctx, idDe(otro), []string{"Educación"}); err != nil {
		t.Fatal(err)
	}
	if r, err := a.Search(ctx, "CANCION"); err != nil || len(r.Files) != 1 || r.Files[0].Path != cancion {
		t.Fatalf("por nombre: %+v %v", r, err)
	}
	if r, err := a.Search(ctx, "educacion"); err != nil || len(r.Files) != 1 || r.Files[0].Path != otro {
		t.Fatalf("por etiqueta: %+v %v", r, err)
	}
}
