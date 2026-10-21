// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// TestFirmaIncrementalNoRompeLaAnterior es EL check de esta pieza. Si al firmar
// en segundo lugar se invalidara la firma del primero, la funcionalidad no vale
// para lo único que existe: que dos partes firmen el mismo contrato.
func TestFirmaIncrementalNoRompeLaAnterior(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)

	primera, err := FirmarConCertificadoPDF(docDePrueba(t, "importe: 1000 euros"), OpcionesFirma{
		CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Ana Pérez", Motivo: "Como arrendadora",
	})
	if err != nil {
		t.Fatalf("primera firma: %v", err)
	}
	if err := validarFirma(primera); err != nil {
		t.Fatalf("la primera firma ya no valida antes de empezar: %v", err)
	}

	doc, err := LeerPDF(primera)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	segunda, err := FirmarIncrementalPDF(doc, OpcionesFirma{
		CertificadoPEM: certPEM, ClavePEM: clavePEM, Nombre: "Luis Gil", Motivo: "Como arrendatario",
		Rect: [4]float64{80, 80, 300, 150},
	})
	if err != nil {
		t.Fatalf("FirmarIncrementalPDF: %v", err)
	}

	// 1. Lo anterior está intacto BYTE A BYTE: es la única forma de que el
	//    /ByteRange de la primera firma siga apuntando a lo mismo.
	if !bytes.HasPrefix(segunda, primera) {
		t.Fatal("el anexo tocó los bytes originales")
	}
	// 2. Y por tanto la primera firma sigue validando.
	if err := validarFirmaN(segunda, 0); err != nil {
		t.Errorf("la primera firma dejó de validar tras la segunda: %v", err)
	}
	// 3. La segunda también, y sobre el fichero ENTERO.
	if err := validarFirmaN(segunda, 1); err != nil {
		t.Errorf("la segunda firma no valida: %v", err)
	}

	d2, err := LeerPDF(segunda)
	if err != nil {
		t.Fatalf("el documento con dos firmas no abre: %v", err)
	}
	firmas := d2.FirmasDelDocumento()
	if len(firmas) != 2 {
		t.Fatalf("firmas = %+v, quería 2", firmas)
	}
	nombres := firmas[0].Nombre + "|" + firmas[1].Nombre
	if !strings.Contains(nombres, "Ana Pérez") || !strings.Contains(nombres, "Luis Gil") {
		t.Errorf("firmantes = %q", nombres)
	}
	// Dos campos con el mismo /T serían el MISMO campo para un lector.
	if firmas[0].Campo == firmas[1].Campo {
		t.Errorf("las dos firmas comparten el nombre de campo %q", firmas[0].Campo)
	}
	// La última firma cubre todo; la primera ya no, y eso es correcto y hay que
	// poder verlo: hay contenido posterior a ella.
	if !firmas[1].CubreTodo {
		t.Error("la última firma debería cubrir el fichero entero")
	}
	if d2.NumPaginas() != doc.NumPaginas() {
		t.Errorf("páginas = %d, antes %d", d2.NumPaginas(), doc.NumPaginas())
	}
}

// TestFirmaIncrementalDetectaLaManipulacion: si tocar el documento no invalidara
// la firma nueva, anexar no serviría de nada.
func TestFirmaIncrementalDetectaLaManipulacion(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	doc := docDePrueba(t, "importe: 1000 euros")
	firmado, err := FirmarIncrementalPDF(doc, OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM})
	if err != nil {
		t.Fatalf("FirmarIncrementalPDF: %v", err)
	}
	if err := validarFirma(firmado); err != nil {
		t.Fatalf("la firma de partida ya no valida: %v", err)
	}
	i := bytes.Index(firmado, []byte("1000"))
	if i < 0 {
		t.Skip("el texto no está en claro en este PDF; el check necesita verlo")
	}
	manipulado := append([]byte(nil), firmado...)
	copy(manipulado[i:], "9000")
	if err := validarFirma(manipulado); err == nil {
		t.Fatal("se cambió el importe y la firma sigue validando")
	}
}

// TestFirmaIncrementalSeNiegaAntesDeEstropearNada: un documento cifrado o con la
// xref reconstruida no se puede anexar bien, y decirlo ANTES es mejor que
// producir un fichero que unos lectores abren y otros no.
func TestFirmaIncrementalSeNiegaAntesDeEstropearNada(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	protegido, err := ProtegerPDF(docDePrueba(t, "secreto"), OpcionesProteccion{ContrasenaUsuario: "abc", Permitir: PermisosTodos()})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	doc, err := LeerPDFConClave(protegido, "abc")
	if err != nil {
		t.Fatalf("LeerPDFConClave: %v", err)
	}
	if _, err := FirmarIncrementalPDF(doc, OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM}); err == nil {
		t.Error("un documento cifrado no se puede firmar por anexo")
	}
}

// TestOffsetDelUltimoXrefLeeElDeAbajo: si cogiera el primer `startxref` en vez
// del último, la cadena de /Prev saltaría actualizaciones intermedias y el
// documento perdería objetos sin dar ningún error.
func TestOffsetDelUltimoXrefLeeElDeAbajo(t *testing.T) {
	buf := []byte("%PDF-1.7\nbasura startxref\n9\n%%EOF\nmas cosas\nstartxref\n17\n%%EOF\n")
	off, err := offsetDelUltimoXref(buf)
	if err != nil {
		t.Fatalf("offsetDelUltimoXref: %v", err)
	}
	if off != 17 {
		t.Errorf("offset = %d, quería el ÚLTIMO (17)", off)
	}
	if _, err := offsetDelUltimoXref([]byte("%PDF-1.7\nsin nada")); err == nil {
		t.Error("sin startxref hay que negarse")
	}
}

// Un fichero que ya se actualizó reutilizando números deja el catálogo y la
// página con generación 1 («1 1 R»). El anexo los reescribiría con la 0 y las
// referencias quedarían colgando —Poppler no abría el issue-149 de qpdf
// firmado—, así que se firma reescribiendo.
func TestFirmaIncrementalNoAnexaSobreGeneracionDistintaDeCero(t *testing.T) {
	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	objs := []string{
		"1 1 obj\n<< /Type /Catalog /Pages 2 1 R >>\nendobj\n",
		"2 1 obj\n<< /Type /Pages /Kids [3 1 R] /Count 1 >>\nendobj\n",
		"3 1 obj\n<< /Type /Page /Parent 2 1 R /MediaBox [0 0 100 100] >>\nendobj\n",
	}
	offsets := make([]int, 0, len(objs))
	for _, o := range objs {
		offsets = append(offsets, b.Len())
		b.WriteString(o)
	}
	xref := b.Len()
	b.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, o := range offsets {
		b.WriteString(pad10(o) + " 00001 n \n")
	}
	b.WriteString("trailer\n<< /Size 4 /Root 1 1 R >>\nstartxref\n" + strconv.Itoa(xref) + "\n%%EOF\n")

	doc, err := LeerPDF([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	certPEM, clavePEM := identidadDePrueba(t)
	_, err = FirmarIncrementalPDF(doc, OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM})
	if !errors.Is(err, ErrNoIncremental) {
		t.Fatalf("err = %v, quería ErrNoIncremental para caer a reescribir", err)
	}
	if _, err := FirmarConCertificadoPDF(doc, OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM}); err != nil {
		t.Fatalf("reescribiendo tenía que firmarse: %v", err)
	}
}
