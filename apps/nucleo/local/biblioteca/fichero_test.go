// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office/model"
)

// escribirTexto cambia el contenido de un documento de Docus como lo haría el editor.
func escribirTexto(t *testing.T, b *Biblioteca, id, html string) {
	t.Helper()
	base, err := b.DocumentContent(t.Context(), id, "doc:html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentChecked(t.Context(), id, "doc:html", html, base, api.DocDot{}, true); err != nil {
		t.Fatal(err)
	}
}

func contenido(t *testing.T, b *Biblioteca, id, clave string) string {
	t.Helper()
	c, err := b.DocumentContent(t.Context(), id, clave)
	if err != nil {
		t.Fatal(err)
	}
	return c.Value
}

// TestFicheroNativoAncladoIdaYVuelta: un `.docus` se abre como documento anclado; abrirlo
// otra vez da el MISMO; Ctrl+S lo escribe de vuelta sin preguntar y sin pérdida (contenido
// byte a byte y comentarios), y una biblioteca nueva lo vuelve a leer igual.
func TestFicheroNativoAncladoIdaYVuelta(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	ruta := filepath.Join(dir, "Carta.docus")
	html := "<p>Hola  <mark data-comment-id=\"c1\">mundo</mark></p>"
	datos, err := model.EscribirNativo(model.Nativo{Kind: model.KindDocument, Titulo: "Carta", Contenido: html, Comentarios: []model.Comment{
		{ID: "c1", AuthorSub: "otra", AuthorName: "Bea", Body: "ojo", Anchor: []byte(`{"mark":"c1","text":"mundo"}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, datos, 0o640); err != nil {
		t.Fatal(err)
	}
	b := abrir(t, t.TempDir(), "docs")
	a, err := b.AbrirFichero(ctx, ruta, true)
	if err != nil || !a.Nuevo || !a.Fichero.Nativo || a.Documento.Title != "Carta" {
		t.Fatalf("abrir: %+v %v", a, err)
	}
	if got := contenido(t, b, a.Documento.ID, "doc:html"); got != html {
		t.Fatalf("contenido %q", got)
	}
	if cs, _ := b.DocumentComments(ctx, a.Documento.ID, true); len(cs) != 1 || cs[0].AuthorName != "Bea" || cs[0].ID != "c1" {
		t.Fatalf("comentarios %+v", cs)
	}
	otra, err := b.AbrirFichero(ctx, ruta, true)
	if err != nil || otra.Documento.ID != a.Documento.ID || otra.Nuevo {
		t.Fatalf("reabrir dio otro documento: %+v %v", otra, err)
	}

	nuevo := "<p>Hola  <mark data-comment-id=\"c1\">mundo</mark></p><p>  y más </p>"
	escribirTexto(t, b, a.Documento.ID, nuevo)
	f, err := b.GuardarEnFichero(ctx, a.Documento.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(ruta); st.Mode().Perm() != 0o640 {
		t.Errorf("los permisos del fichero cambiaron a %v", st.Mode().Perm())
	}
	escrito, _ := os.ReadFile(ruta)
	n, err := model.LeerNativo(escrito)
	if err != nil || n.Contenido != nuevo || len(n.Comentarios) != 1 || n.Comentarios[0].AuthorName != "Bea" {
		t.Fatalf("lo escrito: %+v %v", n, err)
	}
	if f.Huella != huella(escrito) || f.Copiado {
		t.Errorf("ancla tras guardar: %+v", f)
	}
	if hay, _ := filepath.Glob(filepath.Join(dir, "*(original)*")); len(hay) != 0 {
		t.Errorf("un nativo no lleva copia del original: %v", hay)
	}
	// Otra biblioteca (otro equipo, el mismo fichero): abre lo guardado, sin pérdida.
	c := abrir(t, t.TempDir(), "docs")
	a2, err := c.AbrirFichero(ctx, ruta, true)
	if err != nil || contenido(t, c, a2.Documento.ID, "doc:html") != nuevo {
		t.Fatalf("otra biblioteca: %v", err)
	}
}

// TestFicheroAjenoPreguntaYCopiaElOriginal: un `.docx` no se escribe sin aceptar la
// conversión; aceptada, el original queda al lado como `nombre (original).docx` (solo la
// primera vez) y el fichero pasa a tener lo escrito.
func TestFicheroAjenoPreguntaYCopiaElOriginal(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	b := abrir(t, t.TempDir(), "docs")
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "Informe", "")
	escribirTexto(t, b, d.ID, "<p>original</p>")
	docx, err := b.ExportDocument(ctx, d.ID, "ooxml", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(dir, "Informe.docx")
	if err := os.WriteFile(ruta, docx, 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := b.AbrirFichero(ctx, ruta, true)
	if err != nil || a.Fichero.Nativo || a.Fichero.Bloqueo != "" || !strings.Contains(contenido(t, b, a.Documento.ID, "doc:html"), "original") {
		t.Fatalf("abrir docx: %+v %v", a, err)
	}
	escribirTexto(t, b, a.Documento.ID, "<p>cambiado</p>")
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, false); !errors.Is(err, ErrConfirmarConversion) {
		t.Fatalf("sin aceptar: %v", err)
	}
	if escrito, _ := os.ReadFile(ruta); !bytes.Equal(escrito, docx) {
		t.Fatal("se escribió sin aceptar")
	}
	if err := b.AceptarConversion(a.Documento.ID); err != nil {
		t.Fatal(err)
	}
	f, err := b.GuardarEnFichero(ctx, a.Documento.ID, false)
	if err != nil || f.Copia != filepath.Join(dir, "Informe (original).docx") {
		t.Fatalf("guardar: %+v %v", f, err)
	}
	if copia, _ := os.ReadFile(f.Copia); !bytes.Equal(copia, docx) {
		t.Error("la copia no es el original")
	}
	imp, err := Importar("x.docx", mustLeer(t, ruta), false)
	if err != nil || !strings.Contains(imp.Content, "cambiado") {
		t.Fatalf("el fichero no tiene lo escrito: %v", err)
	}
	escribirTexto(t, b, a.Documento.ID, "<p>otra vez</p>")
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, false); err != nil {
		t.Fatal(err)
	}
	if hay, _ := filepath.Glob(filepath.Join(dir, "*(original*")); len(hay) != 1 {
		t.Errorf("copias del original: %v", hay)
	}
}

func mustLeer(t *testing.T, ruta string) []byte {
	t.Helper()
	d, err := os.ReadFile(ruta) //nolint:gosec // G304: fichero del propio test
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestFicheroCambiadoFueraNoSePisa: si el fichero cambió por fuera desde que se leyó, guardar
// no lo pisa (ErrCambioFuera) salvo que se fuerce; y reabrirlo sin cambios propios lo pone
// al día con una versión de lo de antes.
func TestFicheroCambiadoFueraNoSePisa(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	ruta := filepath.Join(dir, "a.docus")
	escribir := func(html string) {
		datos, _ := model.EscribirNativo(model.Nativo{Kind: model.KindDocument, Titulo: "a", Contenido: html})
		if err := os.WriteFile(ruta, datos, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribir("<p>uno</p>")
	b := abrir(t, t.TempDir(), "docs")
	a, err := b.AbrirFichero(ctx, ruta, false)
	if err != nil {
		t.Fatal(err)
	}
	escribir("<p>de fuera</p>")
	re, err := b.AbrirFichero(ctx, ruta, false)
	if err != nil || !re.Recargado || re.Documento.ID != a.Documento.ID || contenido(t, b, a.Documento.ID, "doc:html") != "<p>de fuera</p>" {
		t.Fatalf("recargar: %+v %v", re, err)
	}
	if vs, _ := b.DocumentVersions(ctx, a.Documento.ID); len(vs) != 1 {
		t.Errorf("recargar no dejó versión: %v", vs)
	}

	escribirTexto(t, b, a.Documento.ID, "<p>mío</p>")
	escribir("<p>otra vez de fuera</p>")
	if re, _ := b.AbrirFichero(ctx, ruta, false); !re.CambioFuera || contenido(t, b, a.Documento.ID, "doc:html") != "<p>mío</p>" {
		t.Fatalf("con cambios de los dos se pisó lo mío: %+v", re)
	}
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, false); !errors.Is(err, ErrCambioFuera) {
		t.Fatalf("guardar encima de un cambio de fuera: %v", err)
	}
	if n, _ := model.LeerNativo(mustLeer(t, ruta)); n.Contenido != "<p>otra vez de fuera</p>" {
		t.Fatal("se pisó el fichero")
	}
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, true); err != nil {
		t.Fatal(err)
	}
	if n, _ := model.LeerNativo(mustLeer(t, ruta)); n.Contenido != "<p>mío</p>" {
		t.Fatal("forzar no escribió")
	}
}

// libroGrande es un .xlsx que no cabe entero y se importa recortado.
func libroGrande(t *testing.T, filas int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	poner := func(nombre, datos string) {
		w, err := zw.Create(nombre)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(datos))
	}
	poner("[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/></Types>`)
	poner("xl/workbook.xml", `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Datos" r:id="rId1"/></sheets></workbook>`)
	poner("xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`)
	var s strings.Builder
	s.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i := range filas {
		f := strconv.Itoa(i + 1)
		s.WriteString(`<row r="` + f + `"><c r="A` + f + `" t="inlineStr"><is><t>` + strings.Repeat("relleno ", 12) + f + `</t></is></c></row>`)
	}
	s.WriteString(`</sheetData></worksheet>`)
	poner("xl/worksheets/sheet1.xml", s.String())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestImportacionRecortadaNoSeGuardaDeVuelta: un libro importado recortado se ancla (abrirlo
// otra vez da el mismo) pero guardar en el fichero está bloqueado, y el fichero no cambia;
// «Guardar como» sí funciona y ancla el documento al fichero nuevo.
func TestImportacionRecortadaNoSeGuardaDeVuelta(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	ruta := filepath.Join(dir, "grande.xlsx")
	datos := libroGrande(t, 40_000)
	if err := os.WriteFile(ruta, datos, 0o644); err != nil {
		t.Fatal(err)
	}
	b := abrir(t, t.TempDir(), "sheets")
	a, err := b.AbrirFichero(ctx, ruta, false)
	if err != nil {
		t.Fatal(err)
	}
	if a.Fichero.Bloqueo != BloqueoRecortado {
		t.Fatalf("bloqueo %q", a.Fichero.Bloqueo)
	}
	_ = b.AceptarConversion(a.Documento.ID)
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, true); !errors.Is(err, ErrGuardadoBloqueado) {
		t.Fatalf("guardar un recortado: %v", err)
	}
	if !bytes.Equal(mustLeer(t, ruta), datos) {
		t.Fatal("el fichero recortado se tocó")
	}
	otra := filepath.Join(dir, "grande.calcus")
	f, err := b.GuardarComo(ctx, a.Documento.ID, otra)
	if err != nil || !f.Nativo || f.Bloqueo != "" {
		t.Fatalf("guardar como: %+v %v", f, err)
	}
	if n, err := model.LeerNativo(mustLeer(t, otra)); err != nil || n.Contenido != contenido(t, b, a.Documento.ID, "doc:sheet") {
		t.Fatalf("lo guardado como: %v", err)
	}
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, false); err != nil {
		t.Fatalf("tras guardar como, Ctrl+S: %v", err)
	}
	if re, _ := b.AbrirFichero(ctx, otra, false); re.Documento.ID != a.Documento.ID {
		t.Fatal("el fichero nuevo no abre el mismo documento")
	}
}

// TestCorteAMitadNoDejaFicheroAMedias: si el guardado se corta con el temporal escrito y
// antes de ponerlo en su sitio, el fichero de la persona queda como estaba y no quedan
// temporales.
func TestCorteAMitadNoDejaFicheroAMedias(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	ruta := filepath.Join(dir, "a.docus")
	datos, _ := model.EscribirNativo(model.Nativo{Kind: model.KindDocument, Contenido: "<p>antes</p>"})
	if err := os.WriteFile(ruta, datos, 0o644); err != nil {
		t.Fatal(err)
	}
	b := abrir(t, t.TempDir(), "docs")
	a, err := b.AbrirFichero(ctx, ruta, false)
	if err != nil {
		t.Fatal(err)
	}
	escribirTexto(t, b, a.Documento.ID, "<p>después</p>")
	corte := errors.New("corte de luz")
	antesDeRenombrar = func(tmp string) error {
		if st, err := os.Stat(tmp); err != nil || st.Size() == 0 {
			t.Errorf("el temporal no estaba escrito: %v", err)
		}
		return corte
	}
	defer func() { antesDeRenombrar = nil }()
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, false); !errors.Is(err, corte) {
		t.Fatalf("guardar: %v", err)
	}
	if !bytes.Equal(mustLeer(t, ruta), datos) {
		t.Fatal("el fichero quedó a medias")
	}
	if hay, _ := filepath.Glob(filepath.Join(dir, ".*")); len(hay) != 0 {
		t.Fatalf("quedaron temporales: %v", hay)
	}
	// Y la huella sigue siendo la del fichero: el siguiente guardado no ve un cambio de fuera.
	antesDeRenombrar = nil
	if _, err := b.GuardarEnFichero(ctx, a.Documento.ID, false); err != nil {
		t.Fatal(err)
	}
}

// TestPlantillaNoSeAncla: abrir una plantilla da un documento nuevo cada vez y bloqueado.
func TestPlantillaNoSeAncla(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	b := abrir(t, t.TempDir(), "docs")
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "Plantilla", "")
	escribirTexto(t, b, d.ID, "<p>modelo</p>")
	dotx, err := b.ExportDocument(ctx, d.ID, "ooxml", true, nil)
	if err != nil {
		t.Fatal(err)
	}
	ruta := filepath.Join(dir, "modelo.dotx")
	if err := os.WriteFile(ruta, dotx, 0o644); err != nil {
		t.Fatal(err)
	}
	uno, err := b.AbrirFichero(ctx, ruta, false)
	if err != nil {
		t.Fatal(err)
	}
	dos, _ := b.AbrirFichero(ctx, ruta, false)
	if uno.Documento.ID == dos.Documento.ID || uno.Fichero.Bloqueo != BloqueoPlantilla || uno.Documento.SourceRef != "" {
		t.Fatalf("plantilla: %+v / %+v", uno, dos)
	}
	if _, err := b.GuardarEnFichero(ctx, uno.Documento.ID, true); !errors.Is(err, ErrGuardadoBloqueado) {
		t.Fatalf("guardar plantilla: %v", err)
	}
}

// TestDiaposAbreElJSONAntiguoYElNativo (O-13): el JSON del modelo de siempre se abre tal
// cual (pero no se escribe: se guarda como `.diapos`), y el `.diapos` se abre y se guarda.
func TestDiaposAbreElJSONAntiguoYElNativo(t *testing.T) {
	dir := t.TempDir()
	ctx := t.Context()
	b := abrir(t, t.TempDir(), "slides")
	modelo := `{"slides":[{"id":"s1","boxes":[{"id":"b1","text":"Hola"}]}],"theme":"cielo"}`
	viejo := filepath.Join(dir, "charla.json")
	if err := os.WriteFile(viejo, []byte(modelo), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := b.AbrirFichero(ctx, viejo, false)
	if err != nil || a.Documento.Title != "charla" || a.Fichero.Bloqueo != BloqueoSinVuelta {
		t.Fatalf("json antiguo: %+v %v", a, err)
	}
	if got := contenido(t, b, a.Documento.ID, "doc:slides"); got != modelo {
		t.Fatalf("contenido %q", got)
	}
	nuevo := filepath.Join(dir, "charla.diapos")
	if _, err := b.GuardarComo(ctx, a.Documento.ID, nuevo); err != nil {
		t.Fatal(err)
	}
	c := abrir(t, t.TempDir(), "slides")
	a2, err := c.AbrirFichero(ctx, nuevo, false)
	if err != nil || !a2.Fichero.Nativo || contenido(t, c, a2.Documento.ID, "doc:slides") != modelo {
		t.Fatalf("nativo: %+v %v", a2, err)
	}
	if _, err := c.AbrirFichero(ctx, filepath.Join(dir, "no-existe.diapos"), false); err == nil {
		t.Error("un fichero que no existe")
	}
	d := abrir(t, t.TempDir(), "docs")
	if _, err := d.AbrirFichero(ctx, nuevo, false); !errors.Is(err, ErrOtroTipo) {
		t.Errorf("un .diapos en Docus: %v", err)
	}
}

// TestRutaYFuente: la fuente es una URI file:// y vuelve a ser la ruta.
func TestRutaYFuente(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "un informe #1.docx")
	f, err := FuenteDe(ruta)
	if err != nil || !strings.HasPrefix(f, "file:///") {
		t.Fatalf("fuente %q %v", f, err)
	}
	if r, ok := RutaDe(f); !ok || r != ruta {
		t.Fatalf("RutaDe(%q) = %q %v", f, r, ok)
	}
	if _, ok := RutaDe("content://com.android/x"); ok {
		t.Error("content:// no es una ruta")
	}
}
