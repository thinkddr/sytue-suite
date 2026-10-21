// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de responder a un comentario.
//
// Lo que hay que comprobar no es que se escriba el /IRT —eso es poner una clave—
// sino que la respuesta VUELVA a leerse como respuesta DE ESE comentario. Un
// /IRT que apunte a otro objeto da un hilo colgando de quien no dijo eso, y no
// hay ningún error por el camino.
package domain

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// docConAnotaciones arma un PDF de dos páginas con las anotaciones que se pasen,
// todas en la primera. Los objetos 1-4 son el catálogo, el árbol de páginas y
// las dos páginas; las anotaciones empiezan en el 5.
func docConAnotaciones(t *testing.T, anots ...string) *PDFDoc {
	t.Helper()
	refs := make([]string, 0, len(anots))
	for i := range anots {
		refs = append(refs, strconv.Itoa(5+i)+" 0 R")
	}
	cuerpos := make([]string, 0, 4+len(anots))
	cuerpos = append(cuerpos,
		`<< /Type /Catalog /Pages 2 0 R >>`,
		`<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 200 200] >>`,
		`<< /Type /Page /Parent 2 0 R /Annots [`+strings.Join(refs, " ")+`] >>`,
		`<< /Type /Page /Parent 2 0 R >>`,
	)
	cuerpos = append(cuerpos, anots...)

	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
	offsets := make([]int, 1, 1+len(cuerpos))
	for _, cuerpo := range cuerpos {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, cuerpo)
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

func TestUnaRespuestaSeLeeComoRespuestaDeSuComentario(t *testing.T) {
	d := docConAnotaciones(t,
		`<< /Type /Annot /Subtype /Text /Rect [10 10 30 30] /Contents (Revisar esta cl\341usula) /T (Ana) >>`,
		`<< /Type /Annot /Subtype /Text /Rect [50 50 70 70] /Contents (Otro comentario) /T (Luis) >>`,
	)
	// Se responde al SEGUNDO: si el índice se resolviera mal, el hilo colgaría
	// del primero y el test no lo notaría respondiendo siempre al de arriba.
	salida, err := ResponderAAnotacionPDF(d, 1, "De acuerdo, lo cambio", "Ana")
	if err != nil {
		t.Fatalf("ResponderAAnotacionPDF: %v", err)
	}
	if len(salida) <= len(d.buf) || !bytes.Equal(salida[:len(d.buf)], d.buf) {
		t.Fatal("la respuesta no se escribió por anexo: se ha reescrito el original")
	}

	releido, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	got := releido.AnotacionesDelDocumento()
	if len(got) != 3 {
		t.Fatalf("anotaciones = %d, quería 3: %+v", len(got), got)
	}
	r := got[2]
	if r.Texto != "De acuerdo, lo cambio" || r.Autor != "Ana" {
		t.Errorf("la respuesta llegó como %+v", r)
	}
	if r.RespuestaA == nil {
		t.Fatal("la respuesta no dice a qué comentario contesta")
	}
	if *r.RespuestaA != 1 {
		t.Errorf("contesta al comentario %d, quería al 1 («Otro comentario»)", *r.RespuestaA)
	}
	// Los dos originales siguen sin ser respuesta de nadie.
	for i := 0; i < 2; i++ {
		if got[i].RespuestaA != nil {
			t.Errorf("el comentario %d dice responder a %d, y no responde a nadie", i, *got[i].RespuestaA)
		}
	}
}

// TestNoSePuedeResponderAUnComentarioQueNoExiste: el índice viene de fuera y un
// número cualquiera no puede colgar una respuesta de la nada.
func TestNoSePuedeResponderAUnComentarioQueNoExiste(t *testing.T) {
	d := docConAnotaciones(t,
		`<< /Type /Annot /Subtype /Text /Rect [10 10 30 30] /Contents (Uno) >>`,
	)
	for _, i := range []int{-1, 1, 99} {
		if _, err := ResponderAAnotacionPDF(d, i, "hola", "Ana"); err == nil {
			t.Errorf("responder al comentario %d debería fallar", i)
		}
	}
	if _, err := ResponderAAnotacionPDF(d, 0, "   ", "Ana"); err == nil {
		t.Error("una respuesta en blanco no dice nada y debería rechazarse")
	}
}

// TestLosEnlacesNoCuentanComoComentarios: un /Link está en el mismo /Annots y no
// es de nadie. Si contara, el índice 0 del panel sería el enlace y la respuesta
// colgaría de él.
//
// El acento va escapado en OCTAL (`\372` = «ú») y no en UTF-8: una cadena
// literal `(...)` de un PDF sin BOM UTF-16 es WinAnsi, así que escribir «ú» tal
// cual mete los dos bytes de UTF-8 en el documento y el lector —haciendo lo
// correcto— devuelve «Ãº». El fallo parecía del decodificador y estaba en el PDF
// que fabrica el test.
func TestLosEnlacesNoCuentanComoComentarios(t *testing.T) {
	d := docConAnotaciones(t,
		`<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] >>`,
		`<< /Type /Annot /Subtype /Text /Rect [10 10 30 30] /Contents (El \372nico comentario) >>`,
	)
	got := d.AnotacionesDelDocumento()
	if len(got) != 1 || got[0].Texto != "El único comentario" {
		t.Fatalf("anotaciones enumeradas = %+v", got)
	}
	salida, err := ResponderAAnotacionPDF(d, 0, "vale", "Ana")
	if err != nil {
		t.Fatalf("ResponderAAnotacionPDF: %v", err)
	}
	releido, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	got = releido.AnotacionesDelDocumento()
	if len(got) != 2 || got[1].RespuestaA == nil || *got[1].RespuestaA != 0 {
		t.Errorf("el hilo no cuelga del comentario: %+v", got)
	}
}

// TestMarcarUnComentarioNoCorreLosIndices es EL check de la marca de estado.
//
// La marca es una anotación más dentro del /Annots. Si contara como comentario,
// aceptar el primero correría el índice de todos los de abajo y la siguiente
// respuesta colgaría del que no es —sin ningún error por el camino—.
func TestMarcarUnComentarioNoCorreLosIndices(t *testing.T) {
	d := docConAnotaciones(t,
		`<< /Type /Annot /Subtype /Text /Rect [10 10 30 30] /Contents (Primero) >>`,
		`<< /Type /Annot /Subtype /Text /Rect [50 50 70 70] /Contents (Segundo) >>`,
	)
	salida, err := MarcarComentarioPDF(d, 0, "Accepted", "Ana")
	if err != nil {
		t.Fatalf("MarcarComentarioPDF: %v", err)
	}
	releido, err := LeerPDF(salida)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	got := releido.AnotacionesDelDocumento()
	if len(got) != 2 {
		t.Fatalf("la marca se coló en la lista: %d comentarios, %+v", len(got), got)
	}
	if got[0].Estado != "Accepted" {
		t.Errorf("el primero está %q, quería «Accepted»", got[0].Estado)
	}
	if got[1].Texto != "Segundo" || got[1].Estado != "" {
		t.Errorf("el segundo salió mal: %+v", got[1])
	}

	// Y el índice 1 sigue siendo «Segundo»: aquí es donde se vería el
	// desplazamiento si la marca contara.
	salida2, err := ResponderAAnotacionPDF(releido, 1, "sobre el segundo", "Luis")
	if err != nil {
		t.Fatalf("ResponderAAnotacionPDF: %v", err)
	}
	rel2, err := LeerPDF(salida2)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	got = rel2.AnotacionesDelDocumento()
	if len(got) != 3 || got[2].RespuestaA == nil || *got[2].RespuestaA != 1 {
		t.Errorf("la respuesta no cuelga del segundo comentario: %+v", got)
	}
}

// TestUnEstadoInventadoNoSeEscribe: el valor llega de fuera y solo cinco son
// legales; escribir otro daría un documento que Acrobat no sabe leer.
func TestUnEstadoInventadoNoSeEscribe(t *testing.T) {
	d := docConAnotaciones(t, `<< /Type /Annot /Subtype /Text /Contents (Uno) >>`)
	for _, e := range []string{"", "Resuelto", "accepted", "<script>"} {
		if _, err := MarcarComentarioPDF(d, 0, e, "Ana"); err == nil {
			t.Errorf("el estado %q debería rechazarse", e)
		}
	}
}
