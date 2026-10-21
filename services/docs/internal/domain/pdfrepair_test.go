// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la recuperación de un PDF roto. Cada caso rompe el fichero de una
// manera que pasa DE VERDAD —una descarga cortada, unos desplazamientos
// desfasados, un tráiler perdido— y comprueba que el documento se sigue
// abriendo, que es lo único que le importa a quien lo tiene delante.
package domain

import (
	"bytes"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// TestReparaOffsetsDesfasados es el caso más común de todos: alguien pegó bytes
// al principio del fichero (una cabecera de correo, un BOM) y TODOS los
// desplazamientos de la tabla se quedaron cortos. El índice existe y es
// sintácticamente válido; simplemente apunta al sitio equivocado.
func TestReparaOffsetsDesfasados(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("documento recuperable", "F1", 12)
	original := w.Bytes()

	// Basura DENTRO del primer kilobyte: el lector la tolera y recorta la
	// cabecera, con lo que cada offset de la xref queda desplazado.
	roto := append([]byte(strings.Repeat("% relleno\n", 20)), original...)

	d, err := LeerPDF(roto)
	if err != nil {
		t.Fatalf("un PDF con offsets desfasados debería recuperarse: %v", err)
	}
	if d.NumPaginas() != 1 {
		t.Fatalf("NumPaginas = %d, quería 1", d.NumPaginas())
	}
	texto, err := d.TextoPlano(0)
	if err != nil {
		t.Fatalf("TextoPlano: %v", err)
	}
	if !strings.Contains(texto, "documento recuperable") {
		t.Errorf("el texto no sobrevivió a la reparación: %q", texto)
	}
}

// TestReparaSinStartxref cubre la descarga cortada: el final del fichero —donde
// vive el índice— no llegó.
func TestReparaSinStartxref(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("hola", "F1", 12)
	original := w.Bytes()

	i := bytes.LastIndex(original, []byte("xref"))
	if i < 0 {
		t.Fatal("el escritor de la casa no emitió xref")
	}
	roto := append(append([]byte{}, original[:i]...), []byte("\n%%EOF")...)

	d, err := LeerPDF(roto)
	if err != nil {
		t.Fatalf("un PDF sin tabla de referencias debería recuperarse: %v", err)
	}
	if !d.EstabaDanado() {
		t.Error("EstabaDanado debería avisar de que hubo que reconstruir")
	}
	if d.NumPaginas() != 1 {
		t.Errorf("NumPaginas = %d, quería 1", d.NumPaginas())
	}
}

// TestReparaSinTrailer va un paso más allá: sin índice Y sin tráiler, así que
// hay que deducir el catálogo mirando qué objeto se declara /Type /Catalog.
func TestReparaSinTrailer(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("sin trailer", "F1", 12)
	original := w.Bytes()

	i := bytes.LastIndex(original, []byte("xref"))
	if i < 0 {
		t.Fatal("el escritor de la casa no emitió xref")
	}
	roto := original[:i] // se va la tabla, el trailer y el startxref de golpe

	d, err := LeerPDF(roto)
	if err != nil {
		t.Fatalf("debería deducirse el catálogo: %v", err)
	}
	if d.NumPaginas() != 1 {
		t.Errorf("NumPaginas = %d, quería 1", d.NumPaginas())
	}
}

// TestNoSeReparaLoQueEstaBien comprueba que la recuperación NO se dispara sin
// motivo: un documento sano no debe marcarse como reparado, o el aviso de la
// interfaz se volvería ruido que nadie lee.
func TestNoSeReparaLoQueEstaBien(t *testing.T) {
	w := pdfsimple.Nuevo()
	w.Texto("documento sano", "F1", 12)
	d, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	if d.EstabaDanado() {
		t.Error("un PDF íntegro no debe marcarse como reparado")
	}
}

// TestNoSeInventaUnPDF es la otra cara: reparar no puede convertirse en «abrir
// cualquier cosa». Un fichero que no es un PDF tiene que seguir fallando.
func TestNoSeInventaUnPDF(t *testing.T) {
	for _, basura := range [][]byte{
		[]byte("%PDF-1.4\nesto no tiene ni un objeto\n"),
		[]byte("%PDF-1.4\n1 0 obj\n<< /Type /Nada >>\nendobj\n"),
		append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("obj "), 500)...),
	} {
		if _, err := LeerPDF(basura); err == nil {
			t.Errorf("se abrió algo que no es un documento: %.40q", basura)
		}
	}
}

// TestCabeceraDeObjetoNoSeCreeCualquierCosa protege el barrido de los falsos
// positivos: la palabra «obj» aparece dentro de cadenas y comentarios, y tomarla
// por una cabecera mete objetos inventados en el índice.
func TestCabeceraDeObjetoNoSeCreeCualquierCosa(t *testing.T) {
	casos := []struct {
		buf    string
		quiero bool
	}{
		{"12 0 obj", true},
		{"7 65535 obj", true},
		{"objeto", false},            // no hay números delante
		{"(texto) obj", false},       // lo de delante no es un entero
		{"0 0 obj", false},           // el objeto 0 no existe (§7.5.4)
		{"99999999999 0 obj", false}, // número absurdo
	}
	for _, c := range casos {
		pos := strings.LastIndex(c.buf, "obj")
		_, _, ok := cabeceraDeObjeto([]byte(c.buf), pos)
		if ok != c.quiero {
			t.Errorf("cabeceraDeObjeto(%q) = %v, quería %v", c.buf, ok, c.quiero)
		}
	}
}

// TestReparaGanaLaUltimaDefinicion comprueba la regla del barrido: sin índice
// fiable, la actualización buena es la que está más ABAJO en el fichero, porque
// las actualizaciones incrementales se anexan al final.
func TestReparaGanaLaUltimaDefinicion(t *testing.T) {
	objs := []string{
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R] /Count 1 >>`,
		`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>`,
	}
	base := pdfMinimo(t, `<< /Size 4 /Root 1 0 R >>`, objs)
	// Se anexa una redefinición de la página con OTRO tamaño y se rompe el
	// índice, que es exactamente lo que deja una actualización incremental mal
	// cerrada.
	roto := append(append([]byte{}, base[:bytes.LastIndex(base, []byte("xref"))]...),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 400] >>\nendobj\n")...)

	d, err := LeerPDF(roto)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	p, _ := d.Pagina(0)
	if p.Ancho != 200 || p.Alto != 400 {
		t.Errorf("tamaño = %.0f×%.0f, quería 200×400 (la definición de más abajo)", p.Ancho, p.Alto)
	}
}

// TestReparaNoSeAtasca: el barrido recorre el fichero entero, así que una
// entrada adversa no puede dejarlo girando.
func TestReparaNoSeAtasca(t *testing.T) {
	hecho := make(chan struct{})
	go func() {
		defer close(hecho)
		for _, basura := range [][]byte{
			append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("1 0 obj "), 20000)...),
			append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("obj"), 50000)...),
			append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("9 9 obj\nendobj\n"), 10000)...),
		} {
			_, _ = LeerPDF(basura)
		}
	}()
	select {
	case <-hecho:
	case <-timeoutCorto():
		t.Fatal("la reconstrucción se colgó")
	}
}
