// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// extraerVolcado saca el JSON embebido, deshaciendo el escape de `</`.
func extraerVolcado(t *testing.T, doc string) volcadoEmbebido {
	t.Helper()
	const abre = `<script type="application/json" id="sytue-notepads-volcado">`
	i := strings.Index(doc, abre)
	if i < 0 {
		t.Fatalf("no hay volcado embebido en el fichero:\n%s", doc)
	}
	resto := doc[i+len(abre):]
	j := strings.Index(resto, "</script>")
	if j < 0 {
		t.Fatal("el script del volcado no se cierra")
	}
	var v volcadoEmbebido
	if err := json.Unmarshal([]byte(strings.ReplaceAll(resto[:j], `<\/`, "</")), &v); err != nil {
		t.Fatalf("el volcado embebido no es JSON válido: %v", err)
	}
	return v
}

// parteVisible devuelve lo que el navegador PINTA, sin el volcado embebido.
//
// Con `Index` a pelo, un fichero sin volcado daría -1 y el corte entraría en
// pánico: el test moriría con una traza en vez de decir qué falta.
func parteVisible(t *testing.T, doc string) string {
	t.Helper()
	i := strings.Index(doc, `<script type="application/json"`)
	if i < 0 {
		t.Fatalf("el fichero no lleva volcado embebido:\n%s", doc)
	}
	return doc[:i]
}

// LO QUE HACE ÚTIL ESTE FORMATO: una nota cifrada no se puede PINTAR —el
// servidor no tiene la clave— pero sí viaja entera en el volcado. Es la
// diferencia con Markdown, donde una copia de un cuaderno cifrado no existe.
func TestHTMLLlevaElCriptogramaAunqueNoLoPinte(t *testing.T) {
	doc := NotebookAHTML(Notebook{Title: "C", Encrypted: true},
		[]Note{{Title: "secreta", ContentCipher: "encv1:AAAA"}})

	// En la parte visible no aparece: son bytes que nadie puede leer.
	visible := parteVisible(t, doc)
	if strings.Contains(visible, "encv1:AAAA") {
		t.Error("el criptograma se pintó en la página")
	}
	if !strings.Contains(visible, "cifrada") {
		t.Error("no avisa de que la nota está cifrada")
	}
	// Pero en el volcado sí, entero: sin esto no hay restauración posible.
	v := extraerVolcado(t, doc)
	if len(v.Notas) != 1 || v.Notas[0].ContentCipher != "encv1:AAAA" {
		t.Fatalf("el criptograma no viajó en el volcado: %+v", v.Notas)
	}
}

// `</script>` dentro de una nota cerraría el bloque antes de tiempo y el resto
// del volcado se pintaría como texto. Le pasa a cualquier nota que hable de
// HTML, así que no es un caso raro: es el caso.
func TestHTMLNoSeRompeConUnCierreDeScriptEnElContenido(t *testing.T) {
	doc := NotebookAHTML(Notebook{Title: "C"}, []Note{{
		Title:   "sobre HTML",
		Content: "<p>se cierra con &lt;/script&gt; y ya</p>",
	}})
	v := extraerVolcado(t, doc)
	if len(v.Notas) != 1 {
		t.Fatalf("el volcado se partió: %d notas", len(v.Notas))
	}
	// Y solo hay UN cierre de script en todo el fichero: el de verdad.
	if n := strings.Count(doc, "</script>"); n != 1 {
		t.Errorf("hay %d cierres de script; el contenido rompió el bloque", n)
	}
}

// El token del enlace público es una credencial: abre el cuaderno sin sesión.
func TestHTMLNoFiltraElTokenPublico(t *testing.T) {
	doc := NotebookAHTML(Notebook{Title: "C", PublicToken: "s3cr3t0-del-enlace"}, nil)
	if strings.Contains(doc, "s3cr3t0-del-enlace") {
		t.Fatal("el token del enlace público salió en el fichero exportado")
	}
}

// El color de un estado acaba dentro de un atributo `style`. Se valida en vez de
// confiar en que lo de la base de datos siempre esté bien.
func TestHTMLNoDejaColorArbitrarioEnElEstilo(t *testing.T) {
	doc := NotebookAHTML(Notebook{Title: "C"}, []Note{{
		Title:  "n",
		States: []NoteState{{TagName: "P", Name: "Alta", Color: `#fff;background:url(javascript:alert(1))`}},
	}})
	// Se mira SOLO la parte visible. En el volcado embebido el color original sí
	// viaja —es el dato tal cual, que es lo que permite restaurarlo— y ahí es
	// texto inerte dentro de un `script` que ningún navegador ejecuta. Afirmar
	// sobre el documento entero confundiría las dos cosas, que fue lo que hizo
	// fallar este test la primera vez.
	visible := parteVisible(t, doc)
	if strings.Contains(visible, "javascript:") {
		t.Fatalf("un color inválido se coló en el atributo style:\n%s", visible)
	}
	if !strings.Contains(visible, "background:#565b62") {
		t.Errorf("no cayó al color por defecto:\n%s", visible)
	}
	if !strings.Contains(doc, "Alta") {
		t.Error("se perdió el estado por tener un color malo; solo debía perderse el color")
	}
}

// Un enlace solo se convierte en `<a>` si es una URL web comprobada: un volcado
// no debería ser la vía por la que un `javascript:` guardado hace tiempo llegue
// a ejecutarse en el navegador de quien lo abre.
func TestHTMLSoloEnlazaURLsWeb(t *testing.T) {
	doc := NotebookAHTML(Notebook{Title: "C"}, []Note{{
		Title: "n",
		Links: []NoteLink{
			{TargetKind: "drive_file", TargetID: "f", Label: "malo", URL: "javascript:alert(1)"},
			{TargetKind: "drive_file", TargetID: "g", Label: "bueno", URL: "https://sytue.eu/x"},
		},
	}})
	if strings.Contains(doc, `href="javascript:`) {
		t.Error("se emitió un href con javascript:")
	}
	if !strings.Contains(doc, `href="https://sytue.eu/x"`) {
		t.Error("no se emitió el enlace bueno")
	}
	if !strings.Contains(doc, "malo") {
		t.Error("el enlace malo desapareció; debía quedarse como texto")
	}
}

// El contenido de una nota YA está saneado, así que se emite tal cual: escaparlo
// enseñaría las etiquetas en pantalla en vez del texto con formato.
func TestHTMLConservaElFormatoDeLaNota(t *testing.T) {
	doc := NotebookAHTML(Notebook{Title: "C"}, []Note{{Title: "n", Content: "<p><b>negrita</b></p>"}})
	if !strings.Contains(doc, "<b>negrita</b>") {
		t.Errorf("el formato de la nota se escapó:\n%s", doc)
	}
}

// …pero no se emite «porque ya está saneado»: lo guardado antes de sanear todos los
// tipos de nota puede no estarlo. El volcado HTML y el Markdown lo sanean al salir.
func TestExportarSaneaElContenidoAlSalir(t *testing.T) {
	nb := Notebook{Title: "C"}
	notas := []Note{{Kind: KindLink, Title: "n", Content: `<p onclick="x()">ok</p><script>alert(1)</script>`}}
	for nombre, doc := range map[string]string{
		"html":     parteVisible(t, NotebookAHTML(nb, notas)),
		"markdown": NotebookAMarkdown(nb, notas),
	} {
		if strings.Contains(doc, "onclick") || strings.Contains(doc, "alert(1)") {
			t.Errorf("%s: salió HTML sin sanear:\n%s", nombre, doc)
		}
		if !strings.Contains(doc, "<p>ok</p>") {
			t.Errorf("%s: se perdió el contenido:\n%s", nombre, doc)
		}
	}
}

func TestHTMLEsLaExtensionYElTipoCorrectos(t *testing.T) {
	f, ok := FormatoDe("html")
	if !ok || f != FormatoHTML {
		t.Fatalf("FormatoDe(\"html\") = (%q, %v)", f, ok)
	}
	if f.Extension() != ".html" || !strings.HasPrefix(f.TipoMIME(), "text/html") {
		t.Errorf("extensión %q, mime %q", f.Extension(), f.TipoMIME())
	}
	if otro, _ := FormatoDe(".HTM"); otro != FormatoHTML {
		t.Errorf("«.HTM» debería ser el mismo formato, y dio %q", otro)
	}
}

// El volcado HTML es un wiki que se lee solo: un enlace a otra nota del mismo
// cuaderno tiene que llevar a esa nota DENTRO del fichero, no a una app que
// exige sesión. Los enlaces a notas de fuera se quedan como están.
func TestHTMLConvierteLosEnlacesEntreNotasEnAnclajes(t *testing.T) {
	a := Note{ID: uuid.New(), Title: "A"}
	b := Note{ID: uuid.New(), Title: "B"}
	fuera := uuid.New()
	a.Content = `<p><a href="https://notes.sytue.eu/?notebook=x&amp;note=` + b.ID.String() + `&amp;h=presupuesto">B</a>` +
		` y <a href="https://notes.sytue.eu/?notebook=y&amp;note=` + fuera.String() + `">otra</a></p>`
	doc := parteVisible(t, NotebookAHTML(Notebook{Title: "Mudanza"}, []Note{a, b}))
	if !strings.Contains(doc, `href="#nota-`+b.ID.String()+`"`) {
		t.Errorf("el enlace a B no se volvió anclaje:\n%s", doc)
	}
	if !strings.Contains(doc, `id="nota-`+b.ID.String()+`"`) {
		t.Error("la nota B no lleva su anclaje")
	}
	if !strings.Contains(doc, `note=`+fuera.String()) {
		t.Error("el enlace a una nota de fuera del fichero se perdió; tenía que quedarse")
	}
}
