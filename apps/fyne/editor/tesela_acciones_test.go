// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/office"
)

// elementoDe es la entrada del menú con ese texto; sin ella, el test cae.
func elementoDe(t *testing.T, m *ui.Menu, texto string) *ui.ElementoMenu {
	t.Helper()
	for _, e := range m.Elementos {
		if it, ok := e.(*ui.ElementoMenu); ok && it.Texto == texto {
			return it
		}
	}
	t.Fatalf("el menú no tiene %q: %v", texto, textos(m))
	return nil
}

func rotulosAcciones(s *TeselaDoc) []string {
	out := make([]string, len(s.acciones))
	for i, b := range s.acciones {
		out[i] = b.Rotulo
	}
	return out
}

// Las acciones de la tesela van en el orden de `DocTile`: renombrar · más acciones ·
// compartir · eliminar; y «Más acciones» lleva la copia, un separador y las descargas en
// el orden de `office.Exports()`, cada una con «Descargar» en su nombre accesible.
//
// VISTO EN ROJO (2026-11-13): la tesela era renombrar · copiar · eliminar, sin
// descargas ni «Compartir», y este test no compilaba sin `MenuMas`.
func TestLaTeselaLlevaLasAccionesDeLaWeb(t *testing.T) {
	d := api.Document{ID: "d1", Kind: api.DocSheet, Title: "Cuentas"}
	var bajado, compartido string
	var desde fyne.CanvasObject
	s := NuevaTeselaDoc(d, nil, "es", AccionesTesela{
		Descargar: func(x api.Document, f string) { bajado = x.ID + ":" + f },
		Compartir: func(x api.Document, o fyne.CanvasObject) { compartido, desde = x.ID, o },
	})
	quiero := []string{"tile.rename.label", "tile.more.label", "tile.share.label", "tile.delete.label"}
	if got := rotulosAcciones(s); !slices.Equal(got, quiero) {
		t.Fatalf("las acciones: %v; la web: %v", got, quiero)
	}
	if !s.acciones[3].Peligro {
		t.Error("«Eliminar» tiene que teñirse de peligro")
	}

	// «Compartir» abre el panel colgado de SU botón, como el de la cabecera.
	s.acciones[2].AlPulsar()
	if compartido != "d1" || desde != s.acciones[2] {
		t.Errorf("«Compartir» llama con %q desde %v", compartido, desde)
	}

	m := s.MenuMas()
	formatos := make([]string, 0, len(office.Exports()))
	for _, c := range office.Exports() {
		formatos = append(formatos, "export.format."+c.ID)
	}
	if got := textos(m); !slices.Equal(got, append([]string{"copy.action"}, formatos...)) {
		t.Fatalf("«Más acciones»: %v", got)
	}
	if _, es := m.Elementos[1].(*ui.SeparadorMenu); !es {
		t.Error("la copia y las descargas van separadas, como en la web")
	}
	if m.Rotulo != "tile.more.label" {
		t.Errorf("el menú se llama %q", m.Rotulo)
	}
	ooxml := elementoDe(t, m, "export.format.ooxml")
	if !strings.Contains(ooxml.AccessibilityLabel(), "tile.download") {
		t.Errorf("un formato suelto no dice que descarga: %q", ooxml.AccessibilityLabel())
	}
	ooxml.AlPulsar()
	if bajado != "d1:ooxml" {
		t.Errorf("descargar llama con %q", bajado)
	}

	// Sin gancho de compartir, el botón no sale; sin descargas, el menú solo copia.
	s = NuevaTeselaDoc(d, nil, "es", AccionesTesela{})
	if got := rotulosAcciones(s); !slices.Equal(got, []string{"tile.rename.label", "tile.more.label", "tile.delete.label"}) {
		t.Errorf("sin «Compartir»: %v", got)
	}
	if got := textos(s.MenuMas()); !slices.Equal(got, []string{"copy.action"}) {
		t.Errorf("sin descargas, el menú: %v", got)
	}
}

// Sin red no se muta ni se baja nada (`!offline` de la web): ninguna acción se ve, ni
// siquiera «Compartir» o «Más acciones».
func TestSinRedLaTeselaEscondeTodasSusAcciones(t *testing.T) {
	d := api.Document{ID: "d1", Kind: api.DocText, Title: "Acta", Starred: true}
	s := NuevaTeselaDoc(d, nil, "es", AccionesTesela{
		Descargar: func(api.Document, string) {}, Compartir: func(api.Document, fyne.CanvasObject) {},
	})
	s.SinRed = true
	w := test.NewTempWindow(t, s)
	w.Resize(fyne.NewSize(240, MedidaTesela()))
	s.MouseIn(nil)
	s.Refresh()
	for _, b := range append([]*BotonTesela{s.estrella}, s.acciones...) {
		if b.Visible() {
			t.Errorf("sin red se ve %q", b.Rotulo)
		}
	}
	s.SinRed = false
	s.Refresh()
	for _, b := range s.acciones {
		if !b.Visible() {
			t.Errorf("con red y el puntero encima no se ve %q", b.Rotulo)
		}
	}
}

// Descargar desde la portada deja el fichero en la carpeta de descargas sin pisar el
// que ya hay, y sin servicio o sin red no pide nada.
func TestDescargarDesdeLaPortadaNoPisaOtroFichero(t *testing.T) {
	var pedidas []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidas = append(pedidas, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/exports") && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"export":{"id":"e1"}}`))
		case strings.HasSuffix(r.URL.Path, "/exports/e1"):
			_, _ = w.Write([]byte("PK-hoja"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	p := NuevaPortada(Hoja, nil, "es", api.NewDocs(srv.URL, nil, srv.Client()))
	p.Lanzar = func(f func()) { f() }
	p.Carpeta = func() (string, error) { return dir, nil }
	p.Avisos = ui.NuevaPilaAvisos()
	d := api.Document{ID: "d1", Kind: api.DocSheet, Title: "Cuentas"}
	p.Descargar(d, "ooxml")
	p.Descargar(d, "ooxml")
	fyne.DoAndWait(func() {})
	ficheros, _ := os.ReadDir(dir)
	if len(ficheros) != 2 {
		t.Fatalf("dos descargas dejan %d ficheros: %v", len(ficheros), ficheros)
	}
	if ficheros[0].Name() == ficheros[1].Name() || !strings.HasSuffix(ficheros[0].Name(), ".xlsx") {
		t.Errorf("los nombres: %s, %s", ficheros[0].Name(), ficheros[1].Name())
	}
	if datos, _ := os.ReadFile(dir + "/" + ficheros[0].Name()); string(datos) != "PK-hoja" {
		t.Errorf("el fichero lleva %q", datos)
	}
	if !slices.Contains(pedidas, "POST /api/v1/documents/d1/exports") {
		t.Errorf("no se pidió la exportación: %v", pedidas)
	}

	antes := len(pedidas)
	(&Portada{Tipo: Hoja}).Descargar(d, "ooxml") // sin servicio no hace nada ni cae
	if len(pedidas) != antes {
		t.Error("sin servicio no se pide nada")
	}
}

// La portada pasa a sus teselas descargar y, si la app lo ha enganchado, compartir.
func TestLaPortadaPasaDescargarYCompartirALaTesela(t *testing.T) {
	p := NuevaPortada(Documento, nil, "es", nil)
	p.Estado, p.Docs = Listo, []api.Document{{ID: "d1", Kind: api.DocText, Title: "Acta"}}
	p.vista.rehacerCuerpo()
	s := p.vista.cuerpo.(*fyne.Container).Objects[0].(*TeselaDoc)
	if s.acc.Descargar == nil || s.acc.Compartir != nil {
		t.Fatal("sin gancho de la app, la tesela descarga y no comparte")
	}
	var compartido string
	p.Compartir = func(d api.Document, _ fyne.CanvasObject) { compartido = d.ID }
	p.vista.rehacerCuerpo()
	s = p.vista.cuerpo.(*fyne.Container).Objects[0].(*TeselaDoc)
	if got := rotulosAcciones(s); len(got) != 4 || got[2] != "tile.share.label" {
		t.Fatalf("con gancho: %v", got)
	}
	s.acciones[2].AlPulsar()
	if compartido != "d1" {
		t.Error("«Compartir» de la tesela no llega a la app")
	}
}
