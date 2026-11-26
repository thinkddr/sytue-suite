// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package exportar_test

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	privateBases "git.sytue.eu/sytue/one/apps/nucleo/bases"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/espacio"
	"git.sytue.eu/sytue/one/apps/nucleo/local/exportar"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
	"git.sytue.eu/sytue/one/apps/nucleo/local/traer"
	notesModel "git.sytue.eu/sytue/one/apps/nucleo/notes/model"
	privatePhotos "git.sytue.eu/sytue/one/apps/nucleo/photos"
)

const sujeto = "instalacion"

type secretos struct{}

func (secretos) Leer(string) (string, error)  { return "", nil }
func (secretos) Guardar(string, string) error { return nil }
func (secretos) Borrar(string) error          { return nil }

const ics = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Sytue//Sytue Calendar//ES\r\nBEGIN:VEVENT\r\nUID:visita-1@sytue\r\n" +
	"DTSTAMP:20261120T100000Z\r\nDTSTART:20261201T090000Z\r\nDTEND:20261201T100000Z\r\nSUMMARY:Visita del arquitecto\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

const vcf = "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Íñigo Fontanero\r\nORG:Reformas Íñigo\r\nEND:VCARD\r\n"

// docx es un .docx de verdad, de una biblioteca de usar y tirar.
func docx(t *testing.T) []byte {
	t.Helper()
	b, err := biblioteca.Abrir(t.TempDir(), "docs", biblioteca.Persona{ID: "otro"})
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := b.CreateDocumentFromSource(t.Context(), api.DocText, "Carta", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SaveDocumentContent(t.Context(), d.ID, "doc:html", "<p>La derrama del tejado</p>", api.DocDot{}, 1, false, 0); err != nil {
		t.Fatal(err)
	}
	raw, err := b.ExportDocument(t.Context(), d.ID, "ooxml", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// rellenar deja en datos lo de Calendar, Contacts, Notepads y Docus con «Traer desde
// Sytue» (el camino inverso), y una base y una foto con sus repositorios.
func rellenar(t *testing.T, datos string) {
	t.Helper()
	const r = "sytue-one-2026-11-20/"
	// El cuaderno, como lo entrega Sytue Notepads: su HTML con el volcado dentro.
	casa := notesModel.Notebook{ID: uuid.New(), Title: "Casa"}
	cuaderno := notesModel.NotebookAHTML(casa, []notesModel.Note{
		{ID: uuid.New(), NotebookID: casa.ID, Kind: notesModel.KindText, Title: "Llamar al fontanero", Content: "<p>Por la reforma</p>"},
	})
	ficheros := map[string][]byte{
		r + "calendar/calendars/Casa.ics": []byte(ics),
		r + "contacts/contacts.vcf":       []byte(vcf),
		r + "notes/notebooks/Casa.html":   []byte(cuaderno),
		r + "docs/documents/Carta.docx":   docx(t),
	}
	ruta := filepath.Join(t.TempDir(), "tus-datos.zip")
	f, err := os.Create(ruta)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for n, d := range ficheros {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(d)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := traer.Traer(t.Context(), ruta, traer.Destino{
		Datos: datos, Sujeto: sujeto,
		Persona: biblioteca.Persona{ID: sujeto, Nombre: "Yo"}, Secretos: secretos{},
	}); err != nil {
		t.Fatal(err)
	}

	rutas := espacio.Rutas{Datos: datos}
	rec, err := almacen.Abrir(t.Context(), rutas.Producto("bases"), "bases", sujeto)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := privateBases.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	fachada, err := api.NewBases("https://sin-red.invalid", nil, nil).WithPrivateData(repo)
	if err != nil {
		t.Fatal(err)
	}
	base, err := fachada.CreateBase(t.Context(), "Tienda")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fachada.CreateTable(t.Context(), base.ID, "Pedidos"); err != nil {
		t.Fatal(err)
	}
	repo.Close()
	rec.Cerrar()

	rec, err = almacen.Abrir(t.Context(), rutas.Producto("photos"), "photos", sujeto)
	if err != nil {
		t.Fatal(err)
	}
	fotos, err := privatePhotos.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(t.Context(), fotos); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	if _, err := fotos.ImportarFoto(t.Context(), uuid.NewString(), "playa.png", "image/png", bytes.NewReader(b.Bytes()), int64(b.Len())); err != nil {
		t.Fatal(err)
	}
	fotos.Close()
	rec.Cerrar()
}

// Exportar todo desde el Hub abre cada almacén con el repositorio de su app y deja lo
// mismo que su Ajustes → Copia, una carpeta por app; lo que nunca se usó no se abre ni
// deja carpeta.
func TestTodoExportaCadaAppEnSuCarpeta(t *testing.T) {
	datos := t.TempDir()
	rellenar(t, datos)
	destino := t.TempDir()
	partes, err := exportar.Todo(t.Context(), exportar.Origen{Datos: datos, Sujeto: sujeto, Persona: biblioteca.Persona{ID: sujeto}},
		destino, func(p string) string { return strings.ToUpper(p[:1]) + p[1:] })
	if err != nil {
		t.Fatal(err)
	}
	productos := make([]string, 0, len(partes))
	for _, p := range partes {
		productos = append(productos, p.Producto)
		if p.Err != nil || p.Ficheros == 0 || p.Carpeta == "" {
			t.Errorf("%s: %+v", p.Producto, p)
		}
	}
	if !slices.Equal(productos, []string{"calendar", "contacts", "docs", "notes", "photos", "bases"}) {
		t.Fatalf("partes: %v", productos)
	}
	for _, f := range []string{"Calendar/Casa.ics", "Contacts/Contactos.vcf", "Docs/Carta.docx", "Notes/Casa.md", "Bases/Tienda/Pedidos.csv"} {
		if _, err := os.Stat(filepath.Join(destino, filepath.FromSlash(f))); err != nil {
			hay, _ := filepath.Glob(filepath.Join(destino, "*", "*"))
			t.Errorf("falta %s: %v (hay %v)", f, err, hay)
		}
	}
	if fotos, _ := filepath.Glob(filepath.Join(destino, "Photos", "*", "*", "playa.png")); len(fotos) != 1 {
		t.Errorf("la foto no está en Photos/AAAA/MM: %v", fotos)
	}
	raw, _ := os.ReadFile(filepath.Join(destino, "Calendar", "Casa.ics"))
	if !strings.Contains(string(raw), "Visita del arquitecto") {
		t.Errorf("el .ics no lleva el evento: %s", raw)
	}
	// Ni Calcus ni Diapos tenían nada: ni se abren ni dejan carpeta.
	for _, p := range []string{"Sheets", "Slides"} {
		if _, err := os.Stat(filepath.Join(destino, p)); !os.IsNotExist(err) {
			t.Errorf("%s no tenía nada y hay carpeta: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(datos, "Documentos", "sheets")); !os.IsNotExist(err) {
		t.Errorf("exportar ha creado la biblioteca de Calcus: %v", err)
	}
}

// Una instalación recién estrenada no tiene nada que exportar, y no se crea nada.
func TestTodoSinDatosNoCreaNada(t *testing.T) {
	datos, destino := t.TempDir(), t.TempDir()
	partes, err := exportar.Todo(t.Context(), exportar.Origen{Datos: datos, Sujeto: sujeto}, destino, nil)
	if err != nil || len(partes) != 0 {
		t.Fatal(partes, err)
	}
	if es, _ := os.ReadDir(datos); len(es) != 0 {
		t.Fatalf("ha escrito en la carpeta de datos: %v", es)
	}
	if es, _ := os.ReadDir(destino); len(es) != 0 {
		t.Fatalf("ha dejado carpetas vacías: %v", es)
	}
}

// Los nombres no se salen de su carpeta ni pisan otro igual.
func TestNombres(t *testing.T) {
	vistos := map[string]int{}
	if n := exportar.NombreFichero("a/b:c", "id", ".md", vistos); n != "a_b_c.md" {
		t.Fatal(n)
	}
	if n := exportar.NombreFichero("A/B:C", "id", ".md", vistos); n != "A_B_C (2).md" {
		t.Fatal(n)
	}
	if n := exportar.NombreFichero("  ", "id", ".md", vistos); n != "id.md" {
		t.Fatal(n)
	}
	dir := t.TempDir()
	if err := exportar.Escribir(dir, "../fuera.txt", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fuera.txt")); err != nil {
		t.Fatal("Escribir se ha salido de su carpeta")
	}
}

// Exportar los contactos los lleva TODOS al .vcf, no solo la primera página de 500.
func TestContactosExportaMasDeUnaPagina(t *testing.T) {
	lib, err := libreta.Abrir(t.Context(), t.TempDir(), sujeto)
	if err != nil {
		t.Fatal(err)
	}
	defer lib.Close()
	lb, err := lib.CreateAddressBook(t.Context(), "Personal")
	if err != nil {
		t.Fatal(err)
	}
	const n = 620
	for i := range n {
		if _, err := lib.Create(t.Context(), lb.ID, api.ContactInput{FormattedName: "Persona " + strings.Repeat("x", i%7) + string(rune('a'+i%26))}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := exportar.Contactos(t.Context(), lib, dir); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "Contactos.vcf"))
	if err != nil || strings.Count(string(raw), "BEGIN:VCARD") != n {
		t.Fatalf("el .vcf lleva %d de %d (%v)", strings.Count(string(raw), "BEGIN:VCARD"), n, err)
	}
}
