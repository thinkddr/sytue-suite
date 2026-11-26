// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package biblioteca

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func fachada(t *testing.T, b *Biblioteca) *api.Docs {
	t.Helper()
	d, err := api.NewPrivateDocs().WithPrivateData(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestPapeleraPorLaFachada: lo borrado sale en la papelera, se restaura, se purga del
// disco y se vacía, todo por los métodos de siempre de `api.Docs`.
func TestPapeleraPorLaFachada(t *testing.T) {
	raiz := t.TempDir()
	b := abrir(t, raiz, "docs")
	docs := fachada(t, b)
	ctx := t.Context()
	uno, _ := docs.Create(ctx, api.DocText, "uno")
	dos, _ := docs.Create(ctx, api.DocText, "dos")
	for _, d := range []api.Document{uno, dos} {
		if err := docs.Delete(ctx, d.ID); err != nil {
			t.Fatal(err)
		}
	}
	tirados, err := docs.Papelera(ctx, api.DocText)
	if err != nil || len(tirados) != 2 {
		t.Fatalf("papelera: %v %v", tirados, err)
	}
	if err := docs.Restaurar(ctx, uno.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := docs.Get(ctx, uno.ID); err != nil {
		t.Fatal("no se restauró", err)
	}
	if err := docs.Purgar(ctx, uno.ID); err == nil {
		t.Fatal("se purgó un documento vivo")
	}
	if err := docs.Purgar(ctx, dos.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.ruta(dos.ID)); !os.IsNotExist(err) {
		t.Fatal("el fichero purgado sigue", err)
	}
	_ = docs.Delete(ctx, uno.ID)
	if f, sig, err := docs.Vaciar(ctx, "", ""); err != nil || f != 0 || sig != "" {
		t.Fatal(f, sig, err)
	}
	if tirados, _ := docs.Papelera(ctx, ""); len(tirados) != 0 {
		t.Fatal("la papelera no se vació", tirados)
	}
	if otra := abrir(t, raiz, "docs"); len(otra.docs) != 0 {
		t.Fatal("tras vaciar quedan documentos en disco")
	}
}

// TestRestaurarDosAncladosAlMismoFichero es el 409 del servicio.
func TestRestaurarDosAncladosAlMismoFichero(t *testing.T) {
	b := abrir(t, t.TempDir(), "docs")
	ctx := t.Context()
	d, _, _ := b.CreateDocumentFromSource(ctx, "", "a", "file:///x.docx")
	_ = b.DeleteDocument(ctx, d.ID)
	if _, _, err := b.CreateDocumentFromSource(ctx, "", "a", "file:///x.docx"); err != nil {
		t.Fatal(err)
	}
	var e *api.Error
	if err := b.RestoreDocument(ctx, d.ID); !errors.As(err, &e) || e.Status != http.StatusConflict {
		t.Fatalf("restaurar: %v", err)
	}
}

// TestCopiaPlantillaTemaYBloques: «Hacer una copia», la galería, un tema y los bloques,
// sin servidor.
func TestCopiaPlantillaTemaYBloques(t *testing.T) {
	b := abrir(t, t.TempDir(), "sheets")
	docs := fachada(t, b)
	ctx := t.Context()

	g, err := docs.Templates(ctx, "es")
	if err != nil || len(g.Templates) < 10 || g.CanManage {
		t.Fatalf("galería: %d %v", len(g.Templates), err)
	}
	d, err := docs.CreateFromTemplate(ctx, api.DocNuevo{Template: "presupuesto", Locale: "es", Theme: "cielo"})
	if err != nil || d.Kind != api.DocSheet || d.Title == "" {
		t.Fatalf("desde plantilla: %+v %v", d, err)
	}
	original := contenido(t, b, d.ID, "doc:sheet")
	if !strings.Contains(original, "sheets") {
		t.Fatalf("la plantilla no sembró: %q", original)
	}
	if _, err := docs.CreateFromTemplate(ctx, api.DocNuevo{Template: "carta"}); err == nil {
		t.Error("una plantilla de texto en Calcus")
	}

	copia, err := docs.CreateCopy(ctx, d, "Copia")
	if err != nil || copia.ID == d.ID || copia.Title != "Copia" || contenido(t, b, copia.ID, "doc:sheet") != original {
		t.Fatalf("copia: %+v %v", copia, err)
	}

	temas, err := docs.Themes(ctx)
	if err != nil || len(temas.Themes) < 2 || temas.Brand != nil {
		t.Fatalf("temas: %+v %v", temas, err)
	}
	previa, err := docs.PreviewTheme(ctx, d.ID, "carbon", 0)
	if err != nil || !previa.Changed {
		t.Fatalf("previa: %+v %v", previa, err)
	}
	if cambio, err := docs.ApplyTheme(ctx, d.ID, "carbon", 0); err != nil || !cambio {
		t.Fatalf("aplicar: %v %v", cambio, err)
	}
	if vs, _ := docs.Versions(ctx, d.ID); len(vs) != 1 || !strings.Contains(vs[0].Label, "carbon") {
		t.Errorf("sin versión antes del tema: %+v", vs)
	}
	if contenido(t, b, d.ID, "doc:sheet") == original {
		t.Error("el tema no cambió nada")
	}
	if _, err := docs.ApplyTheme(ctx, d.ID, api.IDTemaMarca, 0); err == nil {
		t.Error("la marca sin organización")
	}
	if _, err := docs.SaveBrand(ctx, api.DocBrand{Name: "x"}); !errors.Is(err, ErrSinOrganizacion) {
		t.Error("guardar marca:", err)
	}

	bl, err := docs.Blocks(ctx, api.DocSheet, "es")
	if err != nil || len(bl.Blocks) == 0 {
		t.Fatalf("bloques: %+v %v", bl, err)
	}
	if c, err := docs.Block(ctx, bl.Blocks[0].Key, "es"); err != nil || c == "" {
		t.Fatalf("bloque: %v", err)
	}
}
