// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de escribir el índice.
//
// El check de verdad es la IDA Y VUELTA: se escribe un árbol anidado y se relee
// con `IndiceDelDocumento`, que es código independiente y ya probado. Un árbol
// de marcadores tiene seis enlaces por nodo (/First, /Last, /Next, /Prev,
// /Parent, /Count) y equivocarse en uno solo da un índice que el lector recorre
// a medias sin dar ningún error: comprobar el diccionario clave a clave no lo
// vería, releerlo sí.
package domain

import (
	"bytes"
	"testing"
)

func TestElIndiceEscritoSeVuelveALeerIgual(t *testing.T) {
	d := docConIndice(t, "",
		// El documento YA trae un índice: reescribirlo es el caso real, no
		// ponerle uno a un documento pelado.
		`<< /Type /Outlines /First 7 0 R /Last 7 0 R /Count 1 >>`,
		`<< /Title (viejo) /Parent 6 0 R /Dest [3 0 R /Fit] >>`,
	)

	quiero := []Marcador{
		{Titulo: "Uno", Pagina: 0, Nivel: 0},
		{Titulo: "Uno.a", Pagina: 1, Nivel: 1},
		{Titulo: "Uno.b", Pagina: 1, Nivel: 1, Abierto: true},
		// Volver a nivel 0 después de dos hijos es donde se rompe un árbol mal
		// enlazado: «Dos» tiene que ser hermano de «Uno», no de «Uno.b».
		{Titulo: "Dos", Pagina: 2, Nivel: 0},
	}
	salida, err := EscribirIndicePDF(d, quiero)
	if err != nil {
		t.Fatalf("EscribirIndicePDF: %v", err)
	}
	// Va por anexo: el fichero CRECE y lo anterior sigue byte a byte donde estaba.
	if len(salida) <= len(d.buf) || !bytes.Equal(salida[:len(d.buf)], d.buf) {
		t.Fatal("el índice no se escribió por anexo: se ha reescrito el original")
	}

	releido, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF de la salida: %v", err)
	}
	got := releido.IndiceDelDocumento()
	if len(got) != len(quiero) {
		t.Fatalf("releído %d marcadores, quería %d: %+v", len(got), len(quiero), got)
	}
	for i := range quiero {
		if got[i].Titulo != quiero[i].Titulo || got[i].Pagina != quiero[i].Pagina ||
			got[i].Nivel != quiero[i].Nivel {
			t.Errorf("marcador %d = %+v, quería %+v", i, got[i], quiero[i])
		}
	}
	// El viejo tiene que haber DESAPARECIDO: si el catálogo siguiera apuntando al
	// árbol anterior, el documento tendría dos índices y el lector vería el malo.
	for _, m := range got {
		if m.Titulo == "viejo" {
			t.Error("el índice anterior sigue ahí")
		}
	}
}

// TestUnaListaVaciaQuitaElIndice: es la única forma de deshacerse del índice de
// un documento mal generado, así que tiene que quitar la clave del catálogo y no
// dejar un árbol sin entradas —que un lector enseña como panel vacío—.
func TestUnaListaVaciaQuitaElIndice(t *testing.T) {
	d := docConIndice(t, "",
		`<< /Type /Outlines /First 7 0 R /Last 7 0 R /Count 1 >>`,
		`<< /Title (algo) /Parent 6 0 R /Dest [3 0 R /Fit] >>`,
	)
	salida, err := EscribirIndicePDF(d, nil)
	if err != nil {
		t.Fatalf("EscribirIndicePDF: %v", err)
	}
	releido, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if got := releido.IndiceDelDocumento(); len(got) != 0 {
		t.Errorf("quedaron %d marcadores: %+v", len(got), got)
	}
	if raiz, ok := releido.resolver(releido.trailer["Root"]).(pdfDict); ok {
		if _, hay := raiz["Outlines"]; hay {
			t.Error("el catálogo conserva /Outlines: debe quitarse la clave, no vaciar el árbol")
		}
	}
}

// TestUnMarcadorNoPuedeApuntarFueraDelDocumento: el nivel se corrige en silencio
// —una sangría rara no es motivo para perder el trabajo— pero una página que no
// existe sí se rechaza: escribirla daría un marcador que no navega a ningún
// sitio, que es exactamente el defecto que esta función viene a arreglar.
func TestUnMarcadorNoPuedeApuntarFueraDelDocumento(t *testing.T) {
	d := docConIndice(t, "", `<< /Type /Outlines >>`)
	if _, err := EscribirIndicePDF(d, []Marcador{{Titulo: "x", Pagina: 9}}); err == nil {
		t.Error("una página inexistente debería rechazarse")
	}

	// El primero sangrado y un salto de nivel se ARREGLAN: el resultado es un
	// árbol legal, no un error.
	salida, err := EscribirIndicePDF(d, []Marcador{
		{Titulo: "sangrado sin padre", Pagina: 0, Nivel: 3},
		{Titulo: "salto", Pagina: 0, Nivel: 7},
		{Titulo: "", Pagina: 0}, // sin título: se cae de la lista, no rompe
	})
	if err != nil {
		t.Fatalf("EscribirIndicePDF: %v", err)
	}
	releido, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	got := releido.IndiceDelDocumento()
	if len(got) != 2 || got[0].Nivel != 0 || got[1].Nivel != 1 {
		t.Errorf("niveles sin arreglar: %+v", got)
	}
}
