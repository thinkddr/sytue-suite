// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Vectores de fidelidad de comentarios, notas e hipervínculos de .xlsx y .ods (sheet_notas.go).

const nsRelsT = "http://schemas.openxmlformats.org/package/2006/relationships"

// libroXlsxComentado es un .xlsx como lo escribe Excel 365: una nota clásica en A1, un hilo con
// respuesta (resuelto) en B2 con su nota de compatibilidad, y tres hipervínculos (web, interno
// y uno `javascript:` que no se conserva). C1 lleva una fórmula con una cadena.
func libroXlsxComentado(t *testing.T) []byte {
	t.Helper()
	return zipStore(t, map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"xl/workbook.xml": `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="Ventas" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="` + nsRelsT + `">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId9" Type="http://schemas.microsoft.com/office/2017/10/relationships/person" Target="persons/person.xml"/></Relationships>`,
		"xl/persons/person.xml": `<?xml version="1.0"?><personList xmlns="http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments">` +
			`<person displayName="Ana Ruiz" id="{P1}" userId="ana" providerId="None"/><person displayName="Bea Gil" id="{P2}" userId="bea" providerId="None"/></personList>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheetData>` +
			`<row r="1"><c r="A1" t="inlineStr"><is><t>Sytue</t></is></c><c r="B1" t="inlineStr"><is><t>Resumen</t></is></c><c r="C1"><f>IF(A1="Sytue","sí","no")</f></c><c r="D1" t="inlineStr"><is><t>Peligro</t></is></c></row>` +
			`<row r="2"><c r="B2"><v>42</v></c></row></sheetData>` +
			`<hyperlinks><hyperlink ref="A1" r:id="rIdL1"/><hyperlink ref="B1" location="'Ventas'!B2" display="Resumen"/><hyperlink ref="D1" r:id="rIdL2"/></hyperlinks>` +
			`<legacyDrawing r:id="rIdV"/></worksheet>`,
		"xl/worksheets/_rels/sheet1.xml.rels": `<?xml version="1.0"?><Relationships xmlns="` + nsRelsT + `">` +
			`<Relationship Id="rIdL1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://sytue.eu/" TargetMode="External"/>` +
			`<Relationship Id="rIdL2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="javascript:alert(1)" TargetMode="External"/>` +
			`<Relationship Id="rIdC" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments" Target="../comments1.xml"/>` +
			`<Relationship Id="rIdT" Type="http://schemas.microsoft.com/office/2017/10/relationships/threadedComment" Target="../threadedComments/threadedComment1.xml"/></Relationships>`,
		"xl/comments1.xml": `<?xml version="1.0"?><comments xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><authors><author>Carla</author><author>tc={T1}</author></authors><commentList>` +
			`<comment ref="A1" authorId="0"><text><r><rPr><b/></rPr><t>Carla:</t></r><r><t xml:space="preserve">` + "\n" + `Revisar el enlace</t></r></text></comment>` +
			`<comment ref="B2" authorId="1"><text><t>[Threaded comment] Ana Ruiz: ¿Cifra buena?</t></text></comment></commentList></comments>`,
		"xl/threadedComments/threadedComment1.xml": `<?xml version="1.0"?><ThreadedComments xmlns="http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments">` +
			`<threadedComment ref="B2" dT="2026-03-04T10:20:30.00" personId="{P1}" id="{T1}" done="1"><text>¿Cifra buena?</text></threadedComment>` +
			`<threadedComment ref="B2" dT="2026-03-05T08:00:00.00" personId="{P2}" id="{T2}" parentId="{T1}"><text>Sí, cuadrada.</text></threadedComment></ThreadedComments>`,
	})
}

func celdasDe(t *testing.T, content string) map[string]string {
	t.Helper()
	var m sheetModel
	if err := json.Unmarshal([]byte(content), &m); err != nil || len(m.Sheets) == 0 {
		t.Fatalf("modelo de hoja: %v (%s)", err, content)
	}
	return m.Sheets[0].Cells
}

func TestImportarXlsxConNotasHilosYEnlaces(t *testing.T) {
	res, err := ImportWithComments("ventas.xlsx", libroXlsxComentado(t))
	if err != nil {
		t.Fatal(err)
	}
	celdas := celdasDe(t, res.Content)
	quiero := map[string]string{
		"A1": `=HYPERLINK("https://sytue.eu/","Sytue")`,
		"B1": `=HYPERLINK("#'Ventas'!B2","Resumen")`,
		"C1": `=IF(A1="Sytue","sí","no")`, // las comillas, como las lee la web (sin &#34;)
		"D1": "Peligro",                   // un javascript: no entra en el libro
	}
	for ref, w := range quiero {
		if got := cellRaw(celdas[ref]); got != w {
			t.Errorf("celda %s = %q (html %q), quiero %q", ref, got, celdas[ref], w)
		}
	}
	if strings.Contains(res.Content, "&#34;") {
		t.Errorf("el contenido lleva comillas escapadas que la web no deshace: %s", res.Content)
	}

	if len(res.Comments) != 3 {
		t.Fatalf("comentarios = %+v, quiero la nota de A1 y el hilo de B2 (sin su nota de compatibilidad)", res.Comments)
	}
	hilo, respuesta, nota := res.Comments[0], res.Comments[1], res.Comments[2]
	if hilo.Cell == nil || *hilo.Cell != (CeldaComentario{Hoja: "h:Ventas", Fila: "t1", Col: "t1", Text: "B2"}) {
		t.Errorf("ancla del hilo = %+v", hilo.Cell)
	}
	if hilo.Text != "Ana Ruiz, 2026-03-04: ¿Cifra buena?" || !hilo.Resolved {
		t.Errorf("hilo = %+v", hilo)
	}
	if respuesta.ParentID != hilo.ID || respuesta.Cell != nil || respuesta.Text != "Bea Gil, 2026-03-05: Sí, cuadrada." {
		t.Errorf("respuesta = %+v (padre %q)", respuesta, hilo.ID)
	}
	if nota.Cell == nil || nota.Cell.Text != "A1" || nota.Cell.Fila != "t0" || nota.Body != "Revisar el enlace" || nota.Text != "Carla: Revisar el enlace" {
		t.Errorf("nota = %+v (%+v)", nota, nota.Cell)
	}
	if !idMarca(hilo.ID) || hilo.Anchored {
		t.Errorf("id local %q (hex) y sin mark: %+v", hilo.ID, hilo)
	}

	// Sin quien guarde los comentarios, no salen (como en .docx).
	if res, err := Import("ventas.xlsx", libroXlsxComentado(t)); err != nil || len(res.Comments) != 0 {
		t.Errorf("Import a secas: %v %+v", err, res)
	}
}

func idMarca(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// libroOdsComentado es un .ods de Calc: A1 con una nota y un enlace, B1 con una nota en una
// celda numérica.
func libroOdsComentado(t *testing.T) []byte {
	t.Helper()
	return zipStore(t, map[string]string{
		"mimetype": "application/vnd.oasis.opendocument.spreadsheet",
		"content.xml": `<?xml version="1.0"?><office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:xlink="http://www.w3.org/1999/xlink">` +
			`<office:body><office:spreadsheet><table:table table:name="Hoja1"><table:table-row>` +
			`<table:table-cell office:value-type="string"><office:annotation><dc:creator>Dani</dc:creator><dc:date>2026-02-01T09:00:00</dc:date><text:p>Primera línea</text:p><text:p>Segunda <text:span>línea</text:span></text:p></office:annotation>` +
			`<text:p><text:a xlink:href="https://sytue.eu/docs" xlink:type="simple">Documentación</text:a></text:p></table:table-cell>` +
			`<table:table-cell office:value-type="float" office:value="7"><office:annotation><text:p>Sin autor</text:p></office:annotation><text:p>7</text:p></table:table-cell>` +
			`</table:table-row></table:table></office:spreadsheet></office:body></office:document-content>`,
	})
}

func TestImportarOdsConAnotacionesYEnlaces(t *testing.T) {
	res, err := ImportWithComments("libro.ods", libroOdsComentado(t))
	if err != nil {
		t.Fatal(err)
	}
	celdas := celdasDe(t, res.Content)
	// La nota no se mezcla con el texto de la celda.
	if got := cellRaw(celdas["A1"]); got != `=HYPERLINK("https://sytue.eu/docs","Documentación")` {
		t.Errorf("A1 = %q", got)
	}
	if got := cellRaw(celdas["B1"]); got != "7" {
		t.Errorf("B1 = %q", got)
	}
	if len(res.Comments) != 2 {
		t.Fatalf("comentarios = %+v", res.Comments)
	}
	a, b := res.Comments[0], res.Comments[1]
	if a.Text != "Dani, 2026-02-01: Primera línea\nSegunda línea" || a.Cell == nil || *a.Cell != (CeldaComentario{Hoja: "h:Hoja1", Fila: "t0", Col: "t0", Text: "A1"}) {
		t.Errorf("nota de A1 = %+v (%+v)", a, a.Cell)
	}
	if b.Text != "Sin autor" || b.Cell == nil || b.Cell.Col != "t1" {
		t.Errorf("nota de B1 = %+v (%+v)", b, b.Cell)
	}
}

// comentariosDeHoja son los comentarios guardados de un libro: un hilo con respuesta en B3 de
// «Datos» (anclado tras insertar una fila encima: su fila tiene id propio), uno resuelto, otro
// de una fila borrada y uno de un documento de texto.
func comentariosDeHoja() []Comment {
	at := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	padre := "c1"
	return []Comment{
		{ID: "c1", AuthorName: "Ana", Anchor: json.RawMessage(`{"hoja":"h1","fila":"t1","col":"t1","text":"B2"}`), Body: "¿Seguro?", CreatedAt: at},
		{ID: "c2", ParentID: &padre, AuthorName: "Bea", Body: "Sí & <bien>", CreatedAt: at},
		{ID: "c3", AuthorSub: "sub-x", Anchor: json.RawMessage(`{"hoja":"h1","fila":"t0","col":"t0","text":"A1"}`), Body: "Hecho", Resolved: true, CreatedAt: at},
		{ID: "c4", AuthorName: "Ana", Anchor: json.RawMessage(`{"hoja":"h1","fila":"zzz","col":"t0","text":"A9"}`), Body: "Fila borrada", CreatedAt: at},
		{ID: "c5", AuthorName: "Ana", Anchor: json.RawMessage(`{"mark":"abc","text":"x"}`), Body: "De texto", CreatedAt: at},
	}
}

// La hoja «Datos» tiene una fila insertada arriba (id «n1»): la fila «t1» es ahora la 3.
const libroComentado = `{"active":0,"sheets":[{"id":"h1","name":"Datos","rowIds":[{"id":"n1"},{"id":"t0"},{"id":"t1"}],"rowBase":2,"cells":{"A1":"Título","B3":"10"}}]}`

func TestExportarXlsxConNotasYVolver(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroComentado)
	data, _, _, err := ExportWithComments(KindSpreadsheet, "Libro", s, FormatOOXML, comentariosDeHoja())
	if err != nil {
		t.Fatal(err)
	}
	parts := zipParts(t, data)
	notas := parts["xl/comments1.xml"]
	if !strings.Contains(notas, `<comment ref="B3" authorId="0">`) || !strings.Contains(notas, "Bea: Sí &amp; &lt;bien&gt;") {
		t.Errorf("notas = %s", notas)
	}
	for _, no := range []string{"Hecho", "Fila borrada", "De texto"} {
		if strings.Contains(notas, no) {
			t.Errorf("la nota %q no tenía que salir: %s", no, notas)
		}
	}
	if !strings.Contains(parts["xl/drawings/vmlDrawing1.vml"], `<x:Row>2</x:Row><x:Column>1</x:Column>`) {
		t.Errorf("vml = %s", parts["xl/drawings/vmlDrawing1.vml"])
	}
	hoja := parts["xl/worksheets/sheet1.xml"]
	if !strings.Contains(hoja, `<legacyDrawing r:id="`) {
		t.Errorf("la hoja no apunta a sus notas: %s", hoja)
	}
	if !strings.Contains(parts["[Content_Types].xml"], ctComentariosXlsx) || !strings.Contains(parts["xl/worksheets/_rels/sheet1.xml.rels"], relComentariosXlsx) {
		t.Errorf("faltan el tipo o la relación de las notas")
	}

	// El hilo sale además como comentario en hilo de Excel, con sus autores en persons, y la
	// nota es la de compatibilidad («tc={id}»).
	hilos := parts["xl/threadedComments/threadedComment1.xml"]
	raiz, bea := guidXlsx("comentario:c1"), guidXlsx("comentario:c2")
	if !strings.Contains(hilos, `<threadedComment ref="B3" dT="2026-04-01T12:00:00.00" personId="`+guidXlsx("persona:Ana")+`" id="`+raiz+`"><text>¿Seguro?</text>`) ||
		!strings.Contains(hilos, `id="`+bea+`" parentId="`+raiz+`"><text>Sí &amp; &lt;bien&gt;</text>`) {
		t.Errorf("threadedComments = %s", hilos)
	}
	if !strings.Contains(notas, `<author>tc=`+raiz+`</author>`) {
		t.Errorf("la nota no es la de compatibilidad del hilo: %s", notas)
	}
	personas := parts["xl/persons/person.xml"]
	if !strings.Contains(personas, `displayName="Ana"`) || !strings.Contains(personas, `displayName="Bea"`) {
		t.Errorf("persons = %s", personas)
	}
	if !strings.Contains(parts["xl/_rels/workbook.xml.rels"], relPersonasXlsx) || !strings.Contains(parts["xl/worksheets/_rels/sheet1.xml.rels"], relHilosXlsx) ||
		!strings.Contains(parts["[Content_Types].xml"], ctHilosXlsx) || !strings.Contains(parts["[Content_Types].xml"], ctPersonasXlsx) {
		t.Errorf("faltan los tipos o las relaciones de los hilos")
	}

	// Y de vuelta: el hilo vuelve a su celda, cada comentario con su autor.
	res, err := ImportWithComments("libro.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 2 {
		t.Fatalf("comentarios de vuelta = %+v", res.Comments)
	}
	c, r := res.Comments[0], res.Comments[1]
	if c.Cell == nil || c.Cell.Text != "B3" || c.Author != "Ana" || c.Body != "¿Seguro?" || c.Date != "2026-04-01T12:00:00.00" {
		t.Errorf("de vuelta = %+v (%+v)", c, c.Cell)
	}
	if r.ParentID != c.ID || r.Author != "Bea" || r.Body != "Sí & <bien>" {
		t.Errorf("respuesta de vuelta = %+v", r)
	}

	// Sin el hilo (un programa que no lo conoce lo descarta), queda la nota clásica.
	clasico := map[string]string{}
	for k, v := range parts {
		if !strings.HasPrefix(k, "xl/threadedComments/") {
			clasico[k] = v
		}
	}
	sinHilo, err := zipPackage(clasico)
	if err != nil {
		t.Fatal(err)
	}
	res, err = ImportWithComments("libro.xlsx", sinHilo)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Comments) != 1 || res.Comments[0].Author != "" || res.Comments[0].Body != "Ana: ¿Seguro?\n\nBea: Sí & <bien>" {
		t.Fatalf("nota clásica de vuelta = %+v", res.Comments)
	}
}

// Dos hilos en la misma celda: Excel admite uno, así que el segundo cuelga del primero.
func TestHilosXlsxDosEnLaMismaCelda(t *testing.T) {
	m := &sheetModel{Sheets: []sheetData{{ID: "h1", Name: "Datos", Cells: map[string]string{}}}}
	cs := []Comment{
		{ID: "a", AuthorName: "Ana", Anchor: json.RawMessage(`{"hoja":"h1","fila":"t0","col":"t0"}`), Body: "uno"},
		{ID: "b", AuthorName: "Bea", Anchor: json.RawMessage(`{"hoja":"h1","fila":"t0","col":"t0"}`), Body: "dos"},
	}
	notas := notasDeComentarios(m, cs)
	if len(notas[0]) != 1 || len(notas[0][0].hilo) != 2 || notas[0][0].hilo[1].padre != notas[0][0].hilo[0].clave {
		t.Fatalf("notas = %+v", notas)
	}
}

func TestExportarOdsConNotasYVolver(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", libroComentado)
	data, _, _, err := ExportWithComments(KindSpreadsheet, "Libro", s, FormatODF, comentariosDeHoja())
	if err != nil {
		t.Fatal(err)
	}
	content := zipParts(t, data)["content.xml"]
	if !strings.Contains(content, `<office:annotation><dc:creator>Ana</dc:creator><dc:date>2026-04-01T12:00:00</dc:date><text:p>¿Seguro?</text:p>`) {
		t.Errorf("content.xml = %s", content)
	}
	res, err := ImportWithComments("libro.ods", data)
	if err != nil {
		t.Fatal(err)
	}
	if got := cellRaw(celdasDe(t, res.Content)["B3"]); got != "10" {
		t.Errorf("B3 = %q: la nota se ha mezclado con el valor", got)
	}
	if len(res.Comments) != 1 || res.Comments[0].Cell == nil || res.Comments[0].Cell.Text != "B3" || res.Comments[0].Text != "Ana, 2026-04-01: ¿Seguro?\n\nBea: Sí & <bien>" {
		t.Fatalf("de vuelta = %+v", res.Comments)
	}
}

// Una nota en una celda vacía la hace existir en el .ods (si no, no hay dónde colgarla).
func TestNotaEnCeldaVaciaOds(t *testing.T) {
	s := stateWithContent(t, "doc:sheet", `{"active":0,"sheets":[{"name":"Hoja1","cells":{"A1":"x"}}]}`)
	cs := []Comment{{ID: "c", AuthorName: "Ana", Anchor: json.RawMessage(`{"hoja":"h:Hoja1","fila":"t1","col":"t2","text":"C2"}`), Body: "aquí"}}
	data, _, _, err := ExportWithComments(KindSpreadsheet, "x", s, FormatODF, cs)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ImportWithComments("x.ods", data)
	if err != nil || len(res.Comments) != 1 || res.Comments[0].Cell.Text != "C2" {
		t.Fatalf("%v %+v", err, res)
	}
}

func TestFormulaDeEnlace(t *testing.T) {
	casos := []struct{ destino, texto, quiero string }{
		{"https://a.eu/?q=\"x\"", "Ver", `=HYPERLINK("https://a.eu/?q=""x""","Ver")`},
		{"mailto:hola@sytue.eu", "", `=HYPERLINK("mailto:hola@sytue.eu")`},
		{"https://a.eu/", "12,5", `=HYPERLINK("https://a.eu/","12,5")`},
		{"https://a.eu/", "12.5", `=HYPERLINK("https://a.eu/",12.5)`},
		{"https://a.eu/", "=SUM(A1:A2)", "=SUM(A1:A2)"},
		{"https://a.eu/", "Inf", `=HYPERLINK("https://a.eu/","Inf")`},
		{"https://a.eu/", "NaN", `=HYPERLINK("https://a.eu/","NaN")`},
		{"file:///etc/passwd", "f", "f"},
		{"data:text/html,x", "d", "d"},
		{"https://a.eu/" + strings.Repeat("a", 300), "largo", "largo"},
	}
	for _, c := range casos {
		if got := formulaDeEnlace(c.destino, c.texto); got != c.quiero {
			t.Errorf("formulaDeEnlace(%q, %q) = %q, quiero %q", c.destino, c.texto, got, c.quiero)
		}
	}
}

// Un hipervínculo de rango enorme (el rango sale del fichero y ParseA1 no lo acota) se aplica
// a las celdas que hay sin recorrer cada posición.
func TestEnlaceDeRangoEnormeNoRecorreLaHoja(t *testing.T) {
	sd := sheetData{Cells: map[string]string{"B2": "Ir", "C3": "<b>Otra</b>"}}
	enlaces := []enlaceXlsx{
		{Ref: "A1:XFD1048576", Location: "Hoja2!A1"},
		{Ref: "A1:A999999999999", Location: "Hoja3!A1"},
	}
	hecho := make(chan struct{})
	go func() {
		aplicarEnlacesXlsx(&sd, enlaces, nil)
		close(hecho)
	}()
	select {
	case <-hecho:
	case <-time.After(5 * time.Second):
		t.Fatal("aplicarEnlacesXlsx recorre el rango entero")
	}
	if got := cellRaw(sd.Cells["B2"]); got != `=HYPERLINK("#Hoja2!A1","Ir")` {
		t.Errorf("B2 = %q", got)
	}
	if got := sd.Cells["C3"]; !strings.Contains(got, "<b>") || !strings.Contains(cellRaw(got), "#Hoja2!A1") {
		t.Errorf("C3 = %q", got)
	}
}

func TestIndiceDeAnclaComoLaWeb(t *testing.T) {
	ids := []anclaEje{{"n1"}, {"t0"}, {"t1"}}
	casos := map[string]int{"n1": 0, "t0": 1, "t1": 2, "t2": 3, "t9": 10, "t-1": -1, "x": -1}
	for id, w := range casos {
		if got := indiceDeAncla(ids, 2, id); got != w {
			t.Errorf("indiceDeAncla(%q) = %d, quiero %d", id, got, w)
		}
	}
	if got := idsDeHojas([]sheetData{{Name: "A"}, {Name: "A"}, {anclasHoja: anclasHoja{ID: "x"}, Name: "B"}}); strings.Join(got, "|") != "h:A|h:A#1|x" {
		t.Errorf("idsDeHojas = %v", got)
	}
}

// Vectores de LibreOffice (testdata/calcus/notas-*, generados con LibreOffice 24.2 en el
// contenedor de agentes): el libro de libroComentado exportado por Sytue a .xlsx y a .ods, con el
// hilo de B3 y el enlace de A1, y guardado por Calc en los dos formatos. La nota y el enlace
// tienen que volver a su celda. (Calc 24.2 no lee el autor de una nota de .xlsx: la deja como
// «Unknown Author» con el «Ana:» dentro del texto, y así vuelve.)
func TestNotasYEnlacesQueEscribeCalc(t *testing.T) {
	for _, f := range []string{"notas-lo-de-xlsx.xlsx", "notas-lo-de-xlsx.ods", "notas-lo-de-ods.xlsx", "notas-lo-de-ods.ods"} {
		datos, err := os.ReadFile("testdata/calcus/" + f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := ImportWithComments(f, datos)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		celdas := celdasDe(t, res.Content)
		// Calc escribe los argumentos de un .ods con «;»; el motor acepta los dos.
		if got := separadorArgumentos(cellRaw(celdas["A1"]), ','); got != `=HYPERLINK("https://sytue.eu/","Sytue")` {
			t.Errorf("%s: A1 = %q", f, got)
		}
		if got := cellRaw(celdas["B3"]); got != "10" {
			t.Errorf("%s: B3 = %q", f, got)
		}
		if len(res.Comments) != 1 {
			t.Fatalf("%s: comentarios = %+v", f, res.Comments)
		}
		c := res.Comments[0]
		if c.Cell == nil || *c.Cell != (CeldaComentario{Hoja: "h:Datos", Fila: "t2", Col: "t1", Text: "B3"}) ||
			!strings.HasSuffix(c.Body, "¿Seguro?\n\nBea: Sí & <bien>") || !strings.Contains(c.Text, "Ana") {
			t.Errorf("%s: comentario = %+v (%+v)", f, c, c.Cell)
		}
	}
}

func TestSeparadorArgumentos(t *testing.T) {
	casos := []struct{ en, ooxml, odf string }{
		{`IF(A1>0;1;2)`, `IF(A1>0,1,2)`, `IF(A1>0;1;2)`},
		{`IF(A1="a,b;c",1,2)`, `IF(A1="a,b;c",1,2)`, `IF(A1="a,b;c";1;2)`},
		{`SUM('Ventas, 2026'!A1,B1)`, `SUM('Ventas, 2026'!A1,B1)`, `SUM('Ventas, 2026'!A1;B1)`},
		{`SUM({1,2;3,4};5)`, `SUM({1,2;3,4},5)`, `SUM({1;2|3;4};5)`},
		{`INDEX({"a,b";"c;d"},2)`, `INDEX({"a,b";"c;d"},2)`, `INDEX({"a,b"|"c;d"};2)`},
		{`CONCAT("""",",")`, `CONCAT("""",",")`, `CONCAT("""";",")`},
		{`SUM(Tabla[[#Datos],[Ventas]];1)`, `SUM(Tabla[[#Datos],[Ventas]],1)`, `SUM(Tabla[[#Datos],[Ventas]];1)`},
		{`SUM(Tabla[[#Datos],[Col'],x]],1)`, `SUM(Tabla[[#Datos],[Col'],x]],1)`, `SUM(Tabla[[#Datos],[Col'],x]];1)`},
	}
	for _, c := range casos {
		if got := separadorArgumentos(c.en, ','); got != c.ooxml {
			t.Errorf("OOXML(%q) = %q, quiero %q", c.en, got, c.ooxml)
		}
		if got := separadorArgumentos(c.en, ';'); got != c.odf {
			t.Errorf("ODF(%q) = %q, quiero %q", c.en, got, c.odf)
		}
	}
}

// Una matriz constante sale en el .ods con la sintaxis de OpenFormula («;» entre columnas y «|»
// entre filas), que es la que Calc entiende, y vuelve al editor con la de Excel.
func TestMatricesConstantesOdsIdaYVuelta(t *testing.T) {
	const libro = `{"active":0,"sheets":[{"name":"Hoja1","cells":{"A1":"=SUM({1,2;3,4})","A2":"=INDEX({\"x;y\",\"z\";1,2},2,1)"}}]}`
	data, ext := exportarHoja(t, libro, FormatODF)
	content := zipParts(t, data)["content.xml"]
	for _, quiero := range []string{`of:=SUM({1;2|3;4})`, `of:=INDEX({&quot;x;y&quot;;&quot;z&quot;|1;2};2;1)`} {
		if !strings.Contains(content, quiero) {
			t.Errorf("content.xml sin %s:\n%s", quiero, content)
		}
	}
	sd := importarHoja(t, "m."+ext, data)
	if got := cellRaw(sd.Cells["A1"]); got != "=SUM({1,2;3,4})" {
		t.Errorf("A1 de vuelta = %q", got)
	}
	if got := separadorArgumentos(cellRaw(sd.Cells["A2"]), ','); got != `=INDEX({"x;y","z";1,2},2,1)` {
		t.Errorf("A2 de vuelta = %q", got)
	}
}

// Una hoja con más de 1024 notas toma más de un bloque de ids de formas VML, y la hoja siguiente
// empieza tras ellos: un id repetido entre hojas hace que Excel pida reparar el libro.
func TestNotasXlsxNoRepitenIdsDeForma(t *testing.T) {
	notas := func(n int) []notaCelda {
		out := make([]notaCelda, n)
		for i := range out {
			out[i] = notaCelda{ref: A1Ref(i+1, 1), cuerpo: "n"}
		}
		return out
	}
	cuenta := &contadorObjetos{}
	vistos := map[string]bool{}
	reID := regexp.MustCompile(`<v:shape id="(_x0000_s\d+)"`)
	reMapa := regexp.MustCompile(`data="([^"]*)"`)
	mapas := make([]string, 0, 2)
	for _, n := range []int{1500, 2} {
		p := objetosXlsx(sheetData{Cells: map[string]string{}, notas: notas(n)}, "H", cuenta)
		vml := p.partes["xl/drawings/vmlDrawing"+strconv.Itoa(cuenta.notas)+".vml"]
		mapas = append(mapas, reMapa.FindStringSubmatch(vml)[1])
		for _, m := range reID.FindAllStringSubmatch(vml, -1) {
			if vistos[m[1]] {
				t.Fatalf("id de forma repetido: %s", m[1])
			}
			vistos[m[1]] = true
		}
	}
	if len(vistos) != 1502 || mapas[0] != "1,2" || mapas[1] != "3" {
		t.Errorf("formas %d, idmap %v", len(vistos), mapas)
	}
}
