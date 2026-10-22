// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package textos

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestTextos(t *testing.T) {
	fsys := fstest.MapFS{
		"t/en.json": {Data: []byte(`{"hola": "Hello {nombre}, order {pedido}", "adios": "Bye"}`)},
		"t/es.json": {Data: []byte(`{"hola": "Hola {nombre}, pedido {pedido}", "adios": "Adiós"}`)},
	}
	c, err := Cargar(fsys, "t")
	if err != nil {
		t.Fatal(err)
	}
	datos := map[string]string{"nombre": "Ana {pedido}", "pedido": "P-000123"}
	for _, caso := range []struct{ idioma, clave, quiero string }{
		{"es", "hola", "Hola Ana {pedido}, pedido P-000123"}, // un dato no se vuelve a sustituir
		{"fr", "adios", "Bye"},                               // sin catálogo, la base
		{"es", "no.existe", "no.existe"},
	} {
		if got := c.Texto(caso.idioma, caso.clave, datos); got != caso.quiero {
			t.Errorf("Texto(%s, %s) = %q, quiero %q", caso.idioma, caso.clave, got, caso.quiero)
		}
	}
	if got := strings.Join(c.Idiomas(), ","); got != "en,es" {
		t.Errorf("Idiomas = %s", got)
	}

	// Una clave que falta en una lengua, o un área sin base, no carga.
	fsys["t/es.json"] = &fstest.MapFile{Data: []byte(`{"hola": "Hola"}`)}
	if _, err := Cargar(fsys, "t"); err == nil {
		t.Error("es.json sin «adios» ha cargado")
	}
	if _, err := Cargar(fstest.MapFS{"u/es.json": {Data: []byte(`{}`)}}, "u"); err == nil {
		t.Error("un área sin en.json ha cargado")
	}
}
