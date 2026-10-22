// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package catalogo

import (
	"encoding/json"
	"io/fs"
	"testing"
)

func TestBinarioIncluyeRecursosDelCatalogo(t *testing.T) {
	c, err := CargarPorDefecto()
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range c.Familias {
		for _, script := range fam.Scripts {
			b, err := fs.ReadFile(Embebido, "fuentes/"+fam.ID+"/"+script+".woff2")
			if err != nil || len(b) < 4 || string(b[:4]) != "wOF2" {
				t.Errorf("fuente %s/%s no está materializada en el binario: %v", fam.ID, script, err)
			}
		}
	}
	var mb manifestoBiblioteca
	raw, _ := fs.ReadFile(Embebido, "biblioteca.json")
	if err := json.Unmarshal(raw, &mb); err != nil {
		t.Fatal(err)
	}
	for _, foto := range mb.Fotos {
		if _, err := fs.ReadFile(Embebido, "biblioteca/"+foto.Fichero); err != nil {
			t.Errorf("foto %s no está en el binario: %v", foto.ID, err)
		}
	}
}
