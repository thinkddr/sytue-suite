// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package catalogo

import (
	"encoding/json"
	"testing"
)

// El catálogo traducido es la PRIMERA petición de cada carga de página y la que
// bloquea el primer pintado: hasta que llega, la interfaz no sabe ni qué poner
// en la barra lateral. Así que su coste —el de construirlo y el de mandarlo por
// el cable— es el suelo de lo rápido que puede sentirse el ERP.
func BenchmarkVista(b *testing.B) {
	c, err := Cargar()
	if err != nil {
		b.Fatalf("cargar: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Vista("es")
	}
}

// BenchmarkVistaYSerializar mide lo que de verdad hace el handler: traducir y
// codificar a JSON.
func BenchmarkVistaYSerializar(b *testing.B) {
	c, err := Cargar()
	if err != nil {
		b.Fatalf("cargar: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(c.Vista("es")); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVistaJSON es el camino de AHORA: la vista ya está traducida y
// serializada desde el arranque, así que el manejador solo copia bytes.
// Comparar con BenchmarkVistaYSerializar, que es lo que costaba antes en CADA
// petición.
func BenchmarkVistaJSON(b *testing.B) {
	c, err := Cargar()
	if err != nil {
		b.Fatalf("cargar: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(c.VistaJSON("es")) == 0 {
			b.Fatal("vista vacía")
		}
	}
}

// TestVistaJSONCoincideConVista: los bytes precalculados tienen que decir
// EXACTAMENTE lo que diría construirlo al vuelo. Si se separan, el catálogo que
// ve la interfaz deja de ser el que valida el servicio.
func TestVistaJSONCoincideConVista(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar: %v", err)
	}
	for _, idioma := range Idiomas {
		alVuelo, err := json.Marshal(c.Vista(idioma))
		if err != nil {
			t.Fatal(err)
		}
		if string(c.VistaJSON(idioma)) != string(alVuelo) {
			t.Errorf("[%s] la vista precalculada no coincide con la construida", idioma)
		}
		if c.Sello(idioma) == "" {
			t.Errorf("[%s] sin sello para el ETag", idioma)
		}
	}
	// Un idioma que no existe cae al inglés en vez de devolver nada.
	if string(c.VistaJSON("xx")) != string(c.VistaJSON("en")) {
		t.Error("un idioma desconocido debe caer al inglés")
	}
}

// TestTamanoDelCatalogo no es una prueba: es una MEDIDA que sale en el log. El
// número importa porque viaja entero en cada carga.
func TestTamanoDelCatalogo(t *testing.T) {
	c, err := Cargar()
	if err != nil {
		t.Fatalf("cargar: %v", err)
	}
	for _, idioma := range Idiomas {
		crudo, err := json.Marshal(c.Vista(idioma))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("catálogo [%s]: %d bytes", idioma, len(crudo))
	}
}
