// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del índice. Lo que hay que comprobar no es que se lean los títulos
// —eso es recorrer una lista— sino que cada entrada SEPA A QUÉ PÁGINA VA: un
// índice que no navega es una lista de texto, y es justo lo que devuelve un
// lector que solo entiende la forma directa del destino.
package domain

import (
	"fmt"
	"strings"
	"testing"
)

// docConIndice arma un PDF de tres páginas con el árbol de marcadores que se le
// pase. `extraCatalogo` entra en el catálogo (para /Dests y /Names).
func docConIndice(t *testing.T, extraCatalogo string, objs ...string) *PDFDoc {
	t.Helper()
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	obj := func(cuerpo string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, cuerpo)
	}
	obj(`<< /Type /Catalog /Pages 2 0 R /Outlines 6 0 R ` + extraCatalogo + ` >>`)
	obj(`<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 /MediaBox [0 0 100 100] >>`)
	for i := 0; i < 3; i++ {
		obj(`<< /Type /Page /Parent 2 0 R >>`)
	}
	for _, o := range objs {
		obj(o)
	}

	inicio := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets), inicio)

	d, err := LeerPDF([]byte(b.String()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	return d
}

// TestIndiceResuelveLasCuatroFormasDelDestino es EL check de esta pieza. La
// especificación permite escribir a dónde va un marcador de cuatro maneras y
// Word usa la de nombres, así que entender solo el array directo deja sin
// navegar la mitad de los documentos de oficina.
func TestIndiceResuelveLasCuatroFormasDelDestino(t *testing.T) {
	d := docConIndice(t,
		// /Dests del catálogo (PDF 1.1) y árbol de nombres (1.2+): las dos tablas.
		`/Dests << /porNombre [4 0 R /Fit] >> /Names << /Dests 11 0 R >>`,
		// 6: raíz del árbol
		`<< /Type /Outlines /First 7 0 R /Last 10 0 R /Count 4 >>`,
		// 7: destino DIRECTO a la página 1 (objeto 3)
		`<< /Title (Portada) /Parent 6 0 R /Next 8 0 R /Dest [3 0 R /Fit] >>`,
		// 8: destino por NOMBRE, en /Dests del catálogo → página 2 (objeto 4)
		`<< /Title (Capitulo I) /Parent 6 0 R /Prev 7 0 R /Next 9 0 R /Dest /porNombre >>`,
		// 9: destino por CADENA, en el árbol de nombres → página 3 (objeto 5)
		`<< /Title (Capitulo II) /Parent 6 0 R /Prev 8 0 R /Next 10 0 R /Dest (porCadena) >>`,
		// 10: destino dentro de una ACCIÓN GoTo → página 2
		`<< /Title (Anexo) /Parent 6 0 R /Prev 9 0 R /A << /S /GoTo /D [4 0 R /XYZ null null null] >> >>`,
		// 11: el árbol de nombres, con el destino envuelto en /D
		`<< /Names [(porCadena) << /D [5 0 R /Fit] >>] >>`)

	idx := d.IndiceDelDocumento()
	if len(idx) != 4 {
		t.Fatalf("marcadores = %+v, quería 4", idx)
	}
	quiero := []struct {
		titulo string
		pagina int
	}{
		{"Portada", 0},     // directo
		{"Capitulo I", 1},  // nombre en /Dests
		{"Capitulo II", 2}, // cadena en el árbol de nombres
		{"Anexo", 1},       // acción GoTo
	}
	for i, q := range quiero {
		if idx[i].Titulo != q.titulo {
			t.Errorf("marcador %d: título = %q, quería %q", i, idx[i].Titulo, q.titulo)
		}
		if idx[i].Pagina != q.pagina {
			t.Errorf("%s: página = %d, quería %d — ese destino no se resolvió",
				q.titulo, idx[i].Pagina, q.pagina)
		}
	}
}

// TestIndiceAnidaYConservaElOrdenDeLectura: la sangría es lo que convierte una
// lista en un índice. Y el orden tiene que ser el de lectura —padre, sus hijos,
// el siguiente hermano—, no el del recorrido interno.
func TestIndiceAnidaYConservaElOrdenDeLectura(t *testing.T) {
	d := docConIndice(t, "",
		`<< /Type /Outlines /First 7 0 R /Count 3 >>`,
		`<< /Title (Titulo I) /First 8 0 R /Next 9 0 R /Count 1 /Dest [3 0 R /Fit] >>`,
		`<< /Title (Articulo 1) /Parent 7 0 R /Dest [3 0 R /Fit] >>`,
		`<< /Title (Titulo II) /Prev 7 0 R /Dest [4 0 R /Fit] >>`)

	idx := d.IndiceDelDocumento()
	if len(idx) != 3 {
		t.Fatalf("marcadores = %+v, quería 3", idx)
	}
	orden := []string{idx[0].Titulo, idx[1].Titulo, idx[2].Titulo}
	if orden[0] != "Titulo I" || orden[1] != "Articulo 1" || orden[2] != "Titulo II" {
		t.Fatalf("orden = %v, quería el de lectura", orden)
	}
	if idx[0].Nivel != 0 || idx[1].Nivel != 1 || idx[2].Nivel != 0 {
		t.Errorf("niveles = %d/%d/%d, quería 0/1/0", idx[0].Nivel, idx[1].Nivel, idx[2].Nivel)
	}
	// /Count positivo = el documento pide que sus hijos se vean.
	if !idx[0].Abierto {
		t.Error("«Titulo I» declara /Count 1: debe salir abierto")
	}
}

// TestIndiceNoSeCuelgaConUnArbolCircular: un /Next que apunta hacia atrás es un
// documento roto que existe. Sin corte, el servicio se queda dando vueltas y la
// petición muere por tiempo, que desde fuera se lee como «el PDF es enorme».
func TestIndiceNoSeCuelgaConUnArbolCircular(t *testing.T) {
	d := docConIndice(t, "",
		`<< /Type /Outlines /First 7 0 R >>`,
		`<< /Title (Uno) /Next 8 0 R /Dest [3 0 R /Fit] >>`,
		`<< /Title (Dos) /Next 7 0 R /Dest [4 0 R /Fit] >>`) // ← vuelve al primero

	idx := d.IndiceDelDocumento()
	if len(idx) != 2 {
		t.Fatalf("marcadores = %+v, quería 2 y parar", idx)
	}
}

// TestIndiceEntregaElMarcadorQueNoNavega: un destino roto no puede esconder la
// entrada. El documento SÍ dice que ahí hay un capítulo; lo que no se sabe es
// dónde, y eso se comunica con página -1 en vez de con un índice incompleto.
func TestIndiceEntregaElMarcadorQueNoNavega(t *testing.T) {
	d := docConIndice(t, "",
		`<< /Type /Outlines /First 7 0 R >>`,
		`<< /Title (Sin destino) /Dest /noExiste >>`)

	idx := d.IndiceDelDocumento()
	if len(idx) != 1 {
		t.Fatalf("marcadores = %+v, quería 1", idx)
	}
	if idx[0].Pagina != -1 {
		t.Errorf("página = %d, quería -1 (destino sin resolver)", idx[0].Pagina)
	}
}

// TestSinIndiceNoHayIndice: un escaneo o un PDF de impresora virtual no trae
// /Outlines, y eso no es un error.
func TestSinIndiceNoHayIndice(t *testing.T) {
	if idx := docDePrueba(t, "sin marcadores").IndiceDelDocumento(); len(idx) != 0 {
		t.Errorf("índice = %+v, quería vacío", idx)
	}
}
