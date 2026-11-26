// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package indice_test

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/calendar"
	"git.sytue.eu/sytue/one/apps/nucleo/contacts"
	"git.sytue.eu/sytue/one/apps/nucleo/local/almacen"
	"git.sytue.eu/sytue/one/apps/nucleo/local/biblioteca"
	"git.sytue.eu/sytue/one/apps/nucleo/local/indice"
	"git.sytue.eu/sytue/one/apps/nucleo/local/libreta"
	"git.sytue.eu/sytue/one/apps/nucleo/notes"
)

// datosDePrueba llena una carpeta de datos de Sytue Office con lo que dejarían las
// apps de verdad: sus propios repositorios sobre sus propios almacenes, que se quedan
// ABIERTOS mientras el Hub lee (las apps están en marcha).
func datosDePrueba(t *testing.T) string {
	t.Helper()
	datos := t.TempDir()
	ctx := t.Context()
	abrir := func(producto string) *almacen.Recursos {
		rec, err := almacen.Abrir(ctx, filepath.Join(datos, "Datos", producto), producto, "instalacion")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rec.Cerrar)
		return rec
	}

	bib, err := biblioteca.Abrir(filepath.Join(datos, "Documentos"), "docs", biblioteca.Persona{ID: "yo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bib.CreateDocumentFromSource(ctx, api.DocText, "Presupuesto de la reforma", ""); err != nil {
		t.Fatal(err)
	}
	borrado, _, err := bib.CreateDocumentFromSource(ctx, api.DocText, "Presupuesto viejo", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := bib.DeleteDocument(ctx, borrado.ID); err != nil {
		t.Fatal(err)
	}

	n, err := notes.New(abrir("notes").Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	if err := almacen.Preparar(ctx, n); err != nil {
		t.Fatal(err)
	}
	libro, err := n.CreateNotebook(ctx, api.NotebookInput{Title: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.CreateNote(ctx, libro.ID, api.NoteInput{Title: "Lista", Content: "<p>Llamar al fontanero por la reforma</p>", Kind: "text"}); err != nil {
		t.Fatal(err)
	}

	// Contacts: su libreta SQLite (`local/libreta`), abierta escribiendo.
	c, err := libreta.Abrir(ctx, filepath.Join(datos, "Datos", "contacts"), "instalacion")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	lb, err := c.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Íñigo Fontanero", Organization: "Reformas Íñigo"}); err != nil {
		t.Fatal(err)
	}
	tirado, err := c.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Fontanero Tirado"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, tirado.ID); err != nil {
		t.Fatal(err)
	}

	k, err := calendar.New(abrir("calendar").Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := almacen.Preparar(ctx, k); err != nil {
		t.Fatal(err)
	}
	cal, err := k.CreateCalendar(ctx, api.CalendarInput{Name: "casa", DisplayName: "Casa"})
	if err != nil {
		t.Fatal(err)
	}
	inicio := time.Date(2026, 12, 1, 9, 0, 0, 0, time.UTC)
	if _, err := k.CreateEvent(ctx, cal.ID, api.EventInput{Summary: "Visita de la reforma", Location: "Calle Mayor 3", Start: inicio, End: inicio.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	drive := filepath.Join(datos, "Drive", "Obras")
	if err := os.MkdirAll(drive, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(drive, "planos reforma.pdf"), []byte("%PDF-1.7"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(datos, "Drive", ".sytue", "papelera"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(datos, "Drive", ".sytue", "papelera", "reforma tirada.pdf"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return datos
}

func titulos(es []indice.Elemento) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Titulo
	}
	return out
}

func TestBuscaEnTodasLasAppsSinAcentosNiMayusculas(t *testing.T) {
	s := indice.Leer(datosDePrueba(t))
	if len(s.Fallos) != 0 {
		t.Fatal("fuentes que fallan:", s.Fallos)
	}
	got := s.Buscar("REFORMA", 20)
	// Primero lo que lo lleva en el título, lo más reciente delante; luego el resto.
	quiero := map[string]string{
		"Presupuesto de la reforma": "docs",
		"Visita de la reforma":      "calendar",
		"planos reforma.pdf":        "pdf",
		"Lista":                     "notes",    // en el cuerpo de la nota
		"Íñigo Fontanero":           "contacts", // en la organización
	}
	if len(got) != len(quiero) {
		t.Fatalf("resultados = %q, quiero %d", titulos(got), len(quiero))
	}
	for _, e := range got {
		if quiero[e.Titulo] != e.Producto {
			t.Errorf("%q de %q, quiero de %q", e.Titulo, e.Producto, quiero[e.Titulo])
		}
	}
	if i := slices.IndexFunc(got, func(e indice.Elemento) bool { return e.Titulo == "Lista" }); i < 3 {
		t.Errorf("la nota que solo lo lleva en el cuerpo va en el puesto %d, delante de los que lo llevan en el título", i)
	}
	if got := s.Buscar("inigo fontanero", 5); len(got) != 1 || got[0].Clase != indice.Contacto || got[0].Detalle != "Reformas Íñigo" {
		t.Fatalf("contacto por su nombre sin tildes = %+v", got)
	}
	if got := s.Buscar("tirado", 5); len(got) != 0 {
		t.Fatal("encuentra un contacto de la papelera:", titulos(got))
	}
	if got := s.Buscar("planos", 5); len(got) != 1 || got[0].Detalle != filepath.Join("Drive", "Obras") || got[0].Ruta == "" {
		t.Fatalf("fichero de Drive = %+v", got)
	}
	if got := s.Buscar("viejo", 5); len(got) != 0 {
		t.Fatal("encuentra un documento borrado:", titulos(got))
	}
	if got := s.Buscar("tirada", 5); len(got) != 0 {
		t.Fatal("encuentra la papelera de Drive:", titulos(got))
	}
	if s.Buscar("  ", 5) != nil {
		t.Fatal("una consulta vacía no busca")
	}
}

func TestRecientesSonDocumentosNotasYFicheros(t *testing.T) {
	s := indice.Leer(datosDePrueba(t))
	got := s.Recientes(10)
	if len(got) != 3 {
		t.Fatalf("recientes = %q", titulos(got))
	}
	for i, e := range got {
		if e.Clase == indice.Evento || e.Clase == indice.Contacto {
			t.Errorf("%q no es un documento reciente", e.Titulo)
		}
		if i > 0 && e.Fecha.After(got[i-1].Fecha) {
			t.Errorf("%q va detrás de uno más viejo", e.Titulo)
		}
	}
	if len(s.Recientes(1)) != 1 {
		t.Fatal("no respeta el tope")
	}
}

// TestElHubNoEscribeEnLasCarpetasDeOtrasApps: leer no crea, no cambia ni borra nada.
func TestElHubNoEscribeEnLasCarpetasDeOtrasApps(t *testing.T) {
	datos := datosDePrueba(t)
	foto := func() map[string]string {
		out := map[string]string{}
		_ = filepath.WalkDir(datos, func(ruta string, d fs.DirEntry, err error) error {
			if err == nil {
				info, _ := d.Info()
				out[ruta] = info.ModTime().String() + "/" + info.Mode().String()
			}
			return err
		})
		return out
	}
	antes := foto()
	indice.Leer(datos)
	indice.Leer(filepath.Join(datos, "no-existe"))
	despues := foto()
	if len(antes) != len(despues) {
		t.Fatalf("leer ha cambiado la carpeta de datos: %d entradas antes, %d después", len(antes), len(despues))
	}
	for k, v := range antes {
		if despues[k] != v {
			t.Errorf("%s ha cambiado", k)
		}
	}
	if _, err := os.Stat(filepath.Join(datos, "no-existe")); !os.IsNotExist(err) {
		t.Fatal("ha creado una carpeta de datos que no existía")
	}
}

func TestElEnlaceLoAbreLaAppDuena(t *testing.T) {
	f := indice.Elemento{Producto: "pdf", Clase: indice.Fichero, Ruta: "/home/ana/Sytue Office/Drive/plano #2.pdf"}
	u, err := url.Parse(f.Enlace())
	if err != nil || u.Scheme != "sytueoffice-pdf" || u.Query().Get("local") != "file:///home/ana/Sytue Office/Drive/plano #2.pdf" {
		t.Fatalf("enlace de fichero = %q (%v)", f.Enlace(), err)
	}
	d := indice.Elemento{Producto: "docs", Clase: indice.Documento, ID: "abc"}
	if got := d.Enlace(); got != "sytueoffice-docs:/abrir?clase=documento&id=abc" {
		t.Fatalf("enlace de documento = %q", got)
	}
	if p, ok := indice.ProductoDeFichero("Informe.DOCX"); !ok || p != "docs" {
		t.Fatal("docx =", p, ok)
	}
}

// Mientras Contacts no haya estrenado su libreta SQLite, el Hub lee sus contactos del
// almacén de registros de antes.
func TestElHubLeeLosContactosDelAlmacenDeAntes(t *testing.T) {
	datos := t.TempDir()
	ctx := t.Context()
	rec, err := almacen.Abrir(ctx, filepath.Join(datos, "Datos", "contacts"), "contacts", "instalacion")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rec.Cerrar)
	c, err := contacts.New(rec.Config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if err := almacen.Preparar(ctx, c); err != nil {
		t.Fatal(err)
	}
	lb, err := c.CreateAddressBook(ctx, "Personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Create(ctx, lb.ID, api.ContactInput{FormattedName: "Íñigo Fontanero", Organization: "Reformas Íñigo"}); err != nil {
		t.Fatal(err)
	}
	s := indice.Leer(datos)
	if got := s.Buscar("inigo", 5); len(got) != 1 || got[0].Clase != indice.Contacto || s.Fallos["contacts"] != nil {
		t.Fatalf("contacto del almacén de antes = %+v (%v)", got, s.Fallos)
	}
}
