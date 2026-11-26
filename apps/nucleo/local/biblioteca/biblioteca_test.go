// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var yo = Persona{ID: "instalacion-1", Nombre: "Ana"}

func abrir(t *testing.T, raiz, producto string) *Biblioteca {
	t.Helper()
	b, err := Abrir(raiz, producto, yo)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUnDocumentoSobreviveAReabrirYQuedaEnClaro(t *testing.T) {
	raiz := t.TempDir()
	b := abrir(t, raiz, "docs")
	ctx := t.Context()
	d, nuevo, err := b.CreateDocumentFromSource(ctx, "", "Informe", "")
	if err != nil || !nuevo || d.Kind != api.DocText || d.OwnerSub != yo.ID {
		t.Fatal("crear", d, nuevo, err)
	}
	base, err := b.DocumentContent(ctx, d.ID, "doc:html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", "<p>hola mundo</p>", base, api.DocDot{}, true); err != nil {
		t.Fatal(err)
	}

	otra := abrir(t, raiz, "docs")
	c, err := otra.DocumentContent(ctx, d.ID, "doc:html")
	if err != nil || c.Value != "<p>hola mundo</p>" || c.Seq == 0 {
		t.Fatal("tras reabrir", c, err)
	}
	raw, err := os.ReadFile(filepath.Join(raiz, "docs", d.ID+".json"))
	if err != nil || !strings.Contains(string(raw), "hola mundo") || !strings.Contains(string(raw), `"Informe"`) {
		t.Fatal("el fichero no es legible en claro", err)
	}
	if fs, _ := filepath.Glob(filepath.Join(raiz, "docs", ".*.tmp")); len(fs) != 0 {
		t.Fatal("quedaron temporales", fs)
	}
}

func TestGuardarSobreUnaBaseViejaEsConflicto(t *testing.T) {
	b := abrir(t, t.TempDir(), "docs")
	ctx := t.Context()
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "x", "")
	base, _ := b.DocumentContent(ctx, d.ID, "doc:html")
	if _, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", "<p>1</p>", base, api.DocDot{}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", "<p>2</p>", base, api.DocDot{}, true); !errors.Is(err, api.ErrDocConflicto) {
		t.Fatal("se pisó una escritura", err)
	}
	if c, _ := b.DocumentContent(ctx, d.ID, "doc:html"); c.Value != "<p>1</p>" {
		t.Fatal("el conflicto cambió el documento", c.Value)
	}
	ops, err := b.DocumentOpsSince(ctx, d.ID, 0)
	if err != nil || len(ops) == 0 {
		t.Fatal("sin diario de operaciones", err)
	}
}

func TestVersionesSeRestauran(t *testing.T) {
	raiz := t.TempDir()
	b := abrir(t, raiz, "docs")
	ctx := t.Context()
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "x", "")
	guardar := func(v string) {
		t.Helper()
		base, _ := b.DocumentContent(ctx, d.ID, "doc:html")
		if _, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", v, base, api.DocDot{}, true); err != nil {
			t.Fatal(err)
		}
	}
	guardar("<p>primera</p>")
	v, err := b.CreateDocumentVersion(ctx, d.ID, " Borrador ")
	if err != nil || v.Label != "Borrador" {
		t.Fatal(v, err)
	}
	guardar("<p>segunda</p>")
	b2 := abrir(t, raiz, "docs")
	vs, err := b2.DocumentVersions(ctx, d.ID)
	if err != nil || len(vs) != 1 || vs[0].ID != v.ID {
		t.Fatal("versiones tras reabrir", vs, err)
	}
	if err := b2.RestoreDocumentVersion(ctx, d.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	if c, _ := b2.DocumentContent(ctx, d.ID, "doc:html"); c.Value != "<p>primera</p>" {
		t.Fatal("no restauró", c.Value)
	}
}

func TestListarFiltraOrdenaYPagina(t *testing.T) {
	b := abrir(t, t.TempDir(), "sheets")
	ctx := t.Context()
	for _, n := range []string{"Beta", "alfa", "Gamma"} {
		if _, _, err := b.CreateDocumentFromSource(ctx, api.DocSheet, n, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := b.CreateDocumentFromSource(ctx, api.DocText, "otro tipo", ""); err == nil {
		t.Fatal("una hoja aceptó un documento de texto")
	}
	todos, _, _ := b.ListDocuments(ctx, api.DocsFilter{ByTitle: true})
	if len(todos) != 3 || todos[0].Title != "alfa" || todos[2].Title != "Gamma" {
		t.Fatal("orden por título", todos)
	}
	pag, next, _ := b.ListDocuments(ctx, api.DocsFilter{ByTitle: true, Limit: 2})
	if len(pag) != 2 || next == "" {
		t.Fatal("paginación", pag, next)
	}
	resto, _, _ := b.ListDocuments(ctx, api.DocsFilter{ByTitle: true, Cursor: next})
	if len(resto) != 1 || resto[0].Title != "Gamma" {
		t.Fatal("página siguiente", resto)
	}
	if err := b.DeleteDocument(ctx, todos[0].ID); err != nil {
		t.Fatal(err)
	}
	if l, _, _ := b.ListDocuments(ctx, api.DocsFilter{Query: "ALFA"}); len(l) != 0 {
		t.Fatal("la papelera sigue en la lista", l)
	}
	if _, _, err := b.GetDocument(ctx, todos[0].ID); err == nil {
		t.Fatal("un documento borrado se abre")
	}
}

func TestAncladoAUnFicheroSeReutiliza(t *testing.T) {
	b := abrir(t, t.TempDir(), "docs")
	ctx := t.Context()
	a, nuevo, _ := b.CreateDocumentFromSource(ctx, "", "a", "file:///tmp/a.docx")
	otra, nuevo2, _ := b.CreateDocumentFromSource(ctx, "", "a", "file:///tmp/a.docx")
	if !nuevo || nuevo2 || a.ID != otra.ID {
		t.Fatal("el mismo fichero abrió dos documentos")
	}
}

func TestExportarEImportarDocxSinRed(t *testing.T) {
	b := abrir(t, t.TempDir(), "docs")
	ctx := t.Context()
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "Carta", "")
	base, _ := b.DocumentContent(ctx, d.ID, "doc:html")
	if _, err := b.SaveDocumentChecked(ctx, d.ID, "doc:html", "<p>Querida Marta</p>", base, api.DocDot{}, true); err != nil {
		t.Fatal(err)
	}
	docx, err := b.ExportDocument(ctx, d.ID, "ooxml", false, nil)
	if err != nil || len(docx) < 100 {
		t.Fatal("exportar", err)
	}
	imp, err := b.ImportDocument(ctx, "Carta.docx", docx, true)
	if err != nil || imp.Kind != api.DocText || !strings.Contains(imp.Content, "Querida Marta") {
		t.Fatal("importar", imp, err)
	}
	pdf, err := b.ExportDocument(ctx, d.ID, "pdf", false, nil)
	if err != nil || !strings.HasPrefix(string(pdf), "%PDF") {
		t.Fatal("pdf", err)
	}
}

func TestComentariosYRespuestas(t *testing.T) {
	raiz := t.TempDir()
	b := abrir(t, raiz, "docs")
	ctx := t.Context()
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "x", "")
	if _, err := b.CreateDocumentComment(ctx, d.ID, "", "  "); !errors.Is(err, api.ErrComentarioVacio) {
		t.Fatal("comentario vacío", err)
	}
	c, err := b.CreateDocumentComment(ctx, d.ID, "", "revisar")
	if err != nil || c.AuthorName != "Ana" {
		t.Fatal(c, err)
	}
	if _, err := b.CreateDocumentComment(ctx, d.ID, c.ID, "hecho"); err != nil {
		t.Fatal(err)
	}
	if err := b.ResolveDocumentComment(ctx, d.ID, c.ID, true); err != nil {
		t.Fatal(err)
	}
	b2 := abrir(t, raiz, "docs")
	if l, _ := b2.DocumentComments(ctx, d.ID, false); len(l) != 1 {
		t.Fatal("sin resueltos", l)
	}
	if err := b2.DeleteDocumentComment(ctx, d.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if l, _ := b2.DocumentComments(ctx, d.ID, true); len(l) != 0 {
		t.Fatal("quedaron respuestas", l)
	}
	if _, err := b2.CreateDocumentShare(ctx, d.ID, "viewer"); !errors.Is(err, ErrSinCompartir) {
		t.Fatal("compartir sin servidor", err)
	}
}

func TestPresentaciones(t *testing.T) {
	b := abrir(t, t.TempDir(), "slides")
	ctx := t.Context()
	p, err := b.CreateSlidePresentation(ctx, "Demo")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := b.SlideContent(ctx, p.ID)
	guardado, err := b.SaveSlideContent(ctx, p.ID, base, "", 0, `{"slides":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveSlideContent(ctx, p.ID, base, "otro", 0, `{"slides":[1]}`); !errors.Is(err, api.ErrSlidesConflict) {
		t.Fatal("conflicto", err)
	}
	if c, _ := b.SlideContent(ctx, p.ID); c.Modelo != `{"slides":[]}` || c.Dot != guardado.Dot {
		t.Fatal(c)
	}
	if l, _, _ := b.ListSlides(ctx, "dem", ""); len(l) != 1 {
		t.Fatal(l)
	}
}

func TestProductoDesconocido(t *testing.T) {
	if _, err := Abrir(t.TempDir(), "mail", yo); !errors.Is(err, ErrProducto) {
		t.Fatal(err)
	}
}
