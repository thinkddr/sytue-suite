// SPDX-License-Identifier: Apache-2.0
package api

import (
	"testing"
	"time"
)

// En el almacenamiento privado el servicio no ve los nombres: la página se ordena en el
// dispositivo con la misma regla que aplicaría él.
func TestOrdenListadoOrdenaLaPaginaPrivada(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ch := Children{
		Folders: []Folder{{Name: "b"}, {Name: "A"}},
		Files: []File{
			{Name: "x", SizeBytes: 5, UpdatedAt: t0.Add(time.Hour)},
			{Name: "y", SizeBytes: 9, UpdatedAt: t0},
			{Name: "z", SizeBytes: 1, UpdatedAt: t0.Add(2 * time.Hour)},
		},
	}
	OrdenListado{Campo: "tamano", Sentido: "desc"}.ordenar(&ch)
	if ch.Folders[0].Name != "b" || ch.Files[0].Name != "y" || ch.Files[2].Name != "z" {
		t.Fatalf("tamaño desc: %+v %+v", ch.Folders, ch.Files)
	}
	OrdenListado{Campo: "modificado"}.ordenar(&ch)
	if ch.Files[0].Name != "y" || ch.Files[2].Name != "z" {
		t.Fatalf("modificado asc: %+v", ch.Files)
	}
	OrdenListado{Campo: "nombre"}.ordenar(&ch)
	if ch.Folders[0].Name != "A" || ch.Files[0].Name != "x" {
		t.Fatalf("nombre asc: %+v %+v", ch.Folders, ch.Files)
	}
	antes := ch.Files[0].Name
	OrdenListado{}.ordenar(&ch)
	if ch.Files[0].Name != antes {
		t.Fatal("el orden cero no debe tocar la página")
	}
}
