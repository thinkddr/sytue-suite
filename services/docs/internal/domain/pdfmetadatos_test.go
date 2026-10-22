// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"strings"
	"testing"
)

// `ptr` a secas ya existe en el paquete (import_xlsx.go, para *int): dos
// declaraciones con el mismo nombre no compilan ni el test ni el paquete.
func strPtr(s string) *string { return &s }

// TestEditarMetadatosCambiaSoloLoQueSePide es el check de la pieza: corregir el
// título no puede borrar el autor ni la fecha de creación, porque entonces
// arreglar una cosa estropea otra sin avisar.
func TestEditarMetadatosCambiaSoloLoQueSePide(t *testing.T) {
	salida, _, err := EditarMetadatosPDF(docDePrueba(t, "informe"), Metadatos{
		Titulo: strPtr("Informe anual 2026"),
		Autor:  strPtr("Sytue"),
	})
	if err != nil {
		t.Fatalf("EditarMetadatosPDF: %v", err)
	}
	d, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("el PDF con metadatos no abre: %v", err)
	}
	h := d.Huella()
	if h.Titulo != "Informe anual 2026" {
		t.Errorf("título = %q", h.Titulo)
	}
	if h.Autor != "Sytue" {
		t.Errorf("autor = %q", h.Autor)
	}

	// Y ahora SOLO el asunto: el título y el autor tienen que sobrevivir.
	segunda, _, err := EditarMetadatosPDF(d, Metadatos{Asunto: strPtr("Cuentas")})
	if err != nil {
		t.Fatalf("segunda edición: %v", err)
	}
	d2, err := LeerPDF(segunda)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	h2 := d2.Huella()
	if h2.Titulo != "Informe anual 2026" || h2.Autor != "Sytue" {
		t.Errorf("se perdió lo anterior al tocar el asunto: %+v", h2)
	}
	if h2.Asunto != "Cuentas" {
		t.Errorf("asunto = %q", h2.Asunto)
	}
}

// TestMetadatoVacioBorra: es la única forma de quitar el nombre del autor sin
// borrar todo lo demás, que es lo que hace SanearMetadatosPDF.
func TestMetadatoVacioBorra(t *testing.T) {
	conAutor, _, err := EditarMetadatosPDF(docDePrueba(t, "x"), Metadatos{
		Titulo: strPtr("Contrato"), Autor: strPtr("Ana Pérez"),
	})
	if err != nil {
		t.Fatalf("EditarMetadatosPDF: %v", err)
	}
	d, err := LeerPDF(conAutor)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	sinAutor, _, err := EditarMetadatosPDF(d, Metadatos{Autor: strPtr("")})
	if err != nil {
		t.Fatalf("borrar el autor: %v", err)
	}
	d2, err := LeerPDF(sinAutor)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	h := d2.Huella()
	if h.Autor != "" {
		t.Errorf("autor = %q, quería vacío", h.Autor)
	}
	if h.Titulo != "Contrato" {
		t.Errorf("borrar el autor se llevó el título: %q", h.Titulo)
	}
}

// TestEditarMetadatosNoTocaLoAnterior: va por anexo, así que una firma previa
// sigue verificando su rango. Corregir un título no puede tirar una firma.
func TestEditarMetadatosNoTocaLoAnterior(t *testing.T) {
	base, err := ImagenesAPDF([]ImagenAPagina{{Datos: pngDePrueba(t, 40, 40, false), Nombre: "s.png"}})
	if err != nil {
		t.Fatalf("ImagenesAPDF: %v", err)
	}
	doc, err := LeerPDF(base)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	salida, _, err := EditarMetadatosPDF(doc, Metadatos{Titulo: strPtr("Escaneo")})
	if err != nil {
		t.Fatalf("EditarMetadatosPDF: %v", err)
	}
	if len(salida) <= len(base) || !bytes.Equal(salida[:len(base)], base) {
		t.Fatal("el anexo tocó los bytes originales")
	}
}

// TestMetadatosSeLimpian: un título con controles ensucia la barra de cualquier
// visor que lo pinte tal cual.
func TestMetadatosSeLimpian(t *testing.T) {
	if got := recortarMetadato("  Informe\x00\x07 anual \r\n "); got != "Informe anual" {
		t.Errorf("recortarMetadato = %q", got)
	}
	largo := strings.Repeat("a", maxLargoMetadato+50)
	if got := recortarMetadato(largo); len([]rune(got)) != maxLargoMetadato {
		t.Errorf("largo = %d, quería el tope de %d", len([]rune(got)), maxLargoMetadato)
	}
}

// TestEditarMetadatosSinNadaQueCambiar: pedirlo sin campos es un error de quien
// llama, y decirlo evita devolver una copia idéntica que parece un cambio.
func TestEditarMetadatosSinNadaQueCambiar(t *testing.T) {
	if _, _, err := EditarMetadatosPDF(docDePrueba(t, "x"), Metadatos{}); err == nil {
		t.Error("sin ninguna propiedad debería fallar")
	}
}
