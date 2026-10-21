// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de los adjuntos. Lo que se comprueba de verdad es que el NOMBRE que
// sale no sirva para escribir donde no se debe: lo escribe el documento, y de
// ahí salen tanto el fichero que se guarda como la cabecera que lo sirve.
package domain

import (
	"strconv"
	"strings"
	"testing"
)

// docConAdjuntos arma un PDF con el árbol /Names /EmbeddedFiles indicado.
func docConAdjuntos(t *testing.T, extraCatalogo string, objs ...string) *PDFDoc {
	t.Helper()
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	obj := func(cuerpo string) {
		offsets = append(offsets, b.Len())
		b.WriteString(strconv.Itoa(len(offsets)-1) + " 0 obj\n" + cuerpo + "\nendobj\n")
	}
	obj(`<< /Type /Catalog /Pages 2 0 R ` + extraCatalogo + ` >>`)
	obj(`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`)
	obj(`<< /Type /Page /Parent 2 0 R >>`)
	for _, o := range objs {
		obj(o)
	}

	inicio := b.Len()
	b.WriteString("xref\n0 " + strconv.Itoa(len(offsets)) + "\n0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		b.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size " + strconv.Itoa(len(offsets)) + " /Root 1 0 R >>\nstartxref\n" +
		strconv.Itoa(inicio) + "\n%%EOF")

	d, err := LeerPDF([]byte(b.String()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	return d
}

// flujoAdjunto arma el objeto de un fichero incrustado con ese contenido.
func flujoAdjunto(contenido string) string {
	return `<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length ` +
		strconv.Itoa(len(contenido)) + ` /Params << /Size ` + strconv.Itoa(len(contenido)) +
		` >> >>` + "\nstream\n" + contenido + "\nendstream"
}

// TestAdjuntoSeListaYSeExtrae: una factura electrónica ES un PDF con el XML
// dentro (Factur-X, ZUGFeRD). Sin esto el XML está en el documento, ocupa su
// tamaño y no hay forma de sacarlo.
func TestAdjuntoSeListaYSeExtrae(t *testing.T) {
	const xml = `<factura><total>119.00</total></factura>`
	d := docConAdjuntos(t,
		`/Names << /EmbeddedFiles 4 0 R >>`,
		`<< /Names [(factura.xml) 5 0 R] >>`,
		`<< /Type /Filespec /F (factura.xml) /UF (factura.xml) /Desc (Factura electronica) /EF << /F 6 0 R >> >>`,
		flujoAdjunto(xml))

	lista := d.AdjuntosDelDocumento()
	if len(lista) != 1 {
		t.Fatalf("adjuntos = %+v, quería 1", lista)
	}
	if lista[0].Nombre != "factura.xml" {
		t.Errorf("nombre = %q", lista[0].Nombre)
	}
	if lista[0].Descripcion != "Factura electronica" {
		t.Errorf("descripción = %q", lista[0].Descripcion)
	}
	if lista[0].Tipo != "text/xml" {
		t.Errorf("tipo = %q, quería el MIME con la barra decodificada de #2F", lista[0].Tipo)
	}
	if lista[0].Pagina != -1 {
		t.Errorf("página = %d: este cuelga del documento, no de una página", lista[0].Pagina)
	}

	datos, err := d.ExtraerAdjunto("factura.xml")
	if err != nil {
		t.Fatalf("ExtraerAdjunto: %v", err)
	}
	if string(datos) != xml {
		t.Errorf("contenido = %q, quería %q", datos, xml)
	}
}

// TestNombreDeAdjuntoNoPuedeSalirDeSuSitio es EL check de esta pieza. El nombre
// lo escribe el documento y de él salen el fichero que se guarda y la cabecera
// que lo sirve: un «../» o un salto de línea ahí valen para escribir fuera del
// sitio previsto o para partir la respuesta.
func TestNombreDeAdjuntoNoPuedeSalirDeSuSitio(t *testing.T) {
	casos := map[string]string{
		"../../.ssh/id_rsa":              "id_rsa",
		`..\..\Windows\System32\dll.dll`: "dll.dll",
		"/etc/passwd":                    "passwd",
		"informe\r\nContent-Type: x":     "informeContent-Type_ x",
		"con:dos*":                       "con_dos_",
	}
	for crudo, quiero := range casos {
		if got := nombreDeAdjuntoSeguro(crudo); got != quiero {
			t.Errorf("%q → %q, quería %q", crudo, got, quiero)
		}
	}
	// Y el que no deja nada utilizable no se entrega: un fichero que no se puede
	// nombrar tampoco se puede guardar.
	for _, vacio := range []string{"", "   ", "...", "/", `\`} {
		if got := nombreDeAdjuntoSeguro(vacio); got != "" {
			t.Errorf("%q → %q, quería vacío", vacio, got)
		}
	}
}

// Y el nombre saneado es el que sirve para pedirlo: aceptar el crudo permitiría
// pedir uno con «../» que la lista nunca enseñó.
func TestExtraerAdjuntoUsaElNombreQueSeEnsena(t *testing.T) {
	d := docConAdjuntos(t,
		`/Names << /EmbeddedFiles 4 0 R >>`,
		`<< /Names [(x) 5 0 R] >>`,
		`<< /Type /Filespec /F (../../secreto.txt) /EF << /F 6 0 R >> >>`,
		flujoAdjunto("contenido"))

	lista := d.AdjuntosDelDocumento()
	if len(lista) != 1 || lista[0].Nombre != "secreto.txt" {
		t.Fatalf("adjuntos = %+v, quería el nombre ya saneado", lista)
	}
	if _, err := d.ExtraerAdjunto("secreto.txt"); err != nil {
		t.Errorf("no se pudo extraer por el nombre que se enseña: %v", err)
	}
	if _, err := d.ExtraerAdjunto("no-existe.txt"); err == nil {
		t.Error("pedir un adjunto que no está debe fallar")
	}
}

// TestAdjuntoPegadoAUnaPagina: Acrobat pega así los ficheros que se arrastran
// sobre una página, y son invisibles si solo se mira el árbol de nombres.
func TestAdjuntoPegadoAUnaPagina(t *testing.T) {
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	obj := func(cuerpo string) {
		offsets = append(offsets, b.Len())
		b.WriteString(strconv.Itoa(len(offsets)-1) + " 0 obj\n" + cuerpo + "\nendobj\n")
	}
	obj(`<< /Type /Catalog /Pages 2 0 R >>`)
	obj(`<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 100 100] >>`)
	obj(`<< /Type /Page /Parent 2 0 R /Annots [4 0 R] >>`)
	obj(`<< /Type /Annot /Subtype /FileAttachment /Rect [10 10 30 30] /FS 5 0 R >>`)
	obj(`<< /Type /Filespec /F (notas.txt) /EF << /F 6 0 R >> >>`)
	obj(flujoAdjunto("apuntes"))
	inicio := b.Len()
	b.WriteString("xref\n0 " + strconv.Itoa(len(offsets)) + "\n0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		b.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	b.WriteString("trailer\n<< /Size " + strconv.Itoa(len(offsets)) + " /Root 1 0 R >>\nstartxref\n" +
		strconv.Itoa(inicio) + "\n%%EOF")

	d, err := LeerPDF([]byte(b.String()))
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	lista := d.AdjuntosDelDocumento()
	if len(lista) != 1 || lista[0].Nombre != "notas.txt" {
		t.Fatalf("adjuntos = %+v, quería el de la anotación", lista)
	}
	if lista[0].Pagina != 0 {
		t.Errorf("página = %d, quería 0: está pegado a la primera", lista[0].Pagina)
	}
}

// TestSinAdjuntosNoHayLista: la mayoría de los PDF no llevan ninguno, y eso no
// es un error.
func TestSinAdjuntosNoHayLista(t *testing.T) {
	if a := docDePrueba(t, "sin adjuntos").AdjuntosDelDocumento(); len(a) != 0 {
		t.Errorf("adjuntos = %+v, quería vacío", a)
	}
}
