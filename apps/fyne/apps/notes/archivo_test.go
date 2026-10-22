// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func textosInternos(o fyne.CanvasObject) []string {
	var out []string
	recorrerInterno(o, func(x fyne.CanvasObject) {
		if c, ok := x.(*canvas.Text); ok && c.Text != "" {
			out = append(out, c.Text)
		}
	})
	return out
}

// pintado son los textos de la vista una vez colocada en una ventana de 1280×800: fuera
// de una, las líneas miden 0 de ancho y `ui.Truncar` las deja vacías.
func pintado(t *testing.T, p *Pantalla) []string {
	t.Helper()
	w := test.NewWindow(p.contenido(p.variante()))
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	return textosInternos(w.Content())
}

func idsDe(cs []Cuaderno) string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return strings.Join(out, ",")
}

// Archivar manda POST a /archive, saca la rama del árbol y la deja en «Archivo» por su
// cabeza; desarchivar es el DELETE de la misma ruta. Es la regla de client/archivo.ts.
func TestArchivarSacaLaRamaDelArbolYLaDejaEnElArchivo(t *testing.T) {
	const ayer = `"archived_at":"2026-09-30T10:00:00Z"`
	p, falso := pantallaConServicio(t, map[string]string{
		"GET /notebooks": `{"notebooks":[{"id":"c1","title":"Mudanza",` + ayer + `},` +
			`{"id":"c2","title":"Trabajo"},{"id":"c3","parent_id":"c1","title":"Cajas",` + ayer + `}]}`,
	})
	p.activo = p.cuadernos[1] // se archiva uno que no es el abierto
	p.archivar(p.cuadernos[0], true)
	if r := falso.rutas(); len(r) < 2 || r[0] != "POST /notebooks/c1/archive" || r[1] != "GET /notebooks" {
		t.Fatalf("peticiones = %v", r)
	}
	p.sincronizar()
	if got := idsDe(p.lateral.Cuadernos); got != "c2" {
		t.Errorf("el árbol enseña %q; lo archivado tiene que salir de él", got)
	}
	if got := idsDe(raicesArchivadas(p.cuadernos)); got != "c1" {
		t.Errorf("el Archivo ofrece %q; solo la cabeza de la rama", got)
	}
	p.vista = VistaArchivo
	textos := pintado(t, p)
	if !slices.Contains(textos, "Mudanza") || slices.Contains(textos, "Cajas") {
		t.Errorf("la vista Archivo pinta %q", textos)
	}

	p.archivar(p.cuadernos[0], false)
	if r := falso.rutas(); !slices.Contains(r, "DELETE /notebooks/c1/archive") {
		t.Fatalf("desarchivar no manda el DELETE: %v", r)
	}
}

// «Fijadas» pide /notes/pinned y pinta sus notas con el título de la vista.
func TestFijadasPintaLasDeTodosLosCuadernos(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"GET /notes/pinned": `{"notes":[{"id":"n1","notebook_id":"c2","notebook":"Trabajo","title":"Presupuesto"}]}`,
	})
	p.cargarFijadas()
	if r := falso.rutas(); len(r) != 1 || r[0] != "GET /notes/pinned" {
		t.Fatalf("peticiones = %v", r)
	}
	p.vista = VistaFijadas
	textos := pintado(t, p)
	if !slices.Contains(textos, "Presupuesto") || !slices.Contains(textos, p.tr.T("focus.pinned", nil)) {
		t.Errorf("Fijadas pinta %q", textos)
	}
	// Sin ninguna, el vacío y no una lista en blanco.
	p.fijadas.Notas, p.fijadas.Fase = nil, Vacia
	if textos := pintado(t, p); !slices.Contains(textos, p.tr.T("pinned.empty", nil)) {
		t.Errorf("Fijadas vacía pinta %q", textos)
	}
}

// «Buscar notas» pide /hashtags, los pinta con su recuento y pulsar uno lo busca.
func TestBusquedaOfreceLosHashtags(t *testing.T) {
	p, falso := pantallaConServicio(t, map[string]string{
		"GET /hashtags": `{"hashtags":[{"tag":"plan","count":3}]}`,
	})
	p.cargarHashtags()
	if r := falso.rutas(); len(r) != 1 || r[0] != "GET /hashtags" {
		t.Fatalf("peticiones = %v", r)
	}
	p.vista = VistaBusqueda
	if textos := pintado(t, p); !slices.Contains(textos, "#plan") || !slices.Contains(textos, "3") {
		t.Errorf("la búsqueda pinta %q; falta el hashtag con su recuento", textos)
	}
	var buscada string
	p.busqueda.AlBuscar = func(q string) { buscada = q }
	recorrerInterno(p.busqueda.hashtags(p.variante()), func(x fyne.CanvasObject) {
		if o, ok := x.(*Opcion); ok {
			test.Tap(o)
		}
	})
	if buscada != "#plan" || p.busqueda.Consulta != "#plan" {
		t.Errorf("pulsar el hashtag busca %q y deja escrito %q", buscada, p.busqueda.Consulta)
	}
}

// Ajustes ofrece archivar solo a su dueño y nunca en un diario.
func TestAjustesOfreceArchivarSoloAlDuenoYNoEnUnDiario(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Yo = func() string { return "ada" }
	archivar := p.tr.T("archive.archive", nil)
	for _, caso := range []struct {
		c      Cuaderno
		ofrece bool
	}{
		{DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", OwnerSub: "ada"}), true},
		{DeCuaderno(api.Notebook{ID: "c2", Title: "Trabajo", OwnerSub: "bob"}), false},
		{DeCuaderno(api.Notebook{ID: "c3", Title: "Hoy", OwnerSub: "ada", DailyDate: "2026-10-01"}), false},
	} {
		p.dialogoAjustes(&caso.c, "")
		if got := slices.Contains(textosInternos(p.ajustes), archivar); got != caso.ofrece {
			t.Errorf("%s: ofrece archivar = %v", caso.c.Titulo, got)
		}
		p.cerrarAjustes()
	}
}

// «Desarchivar» en la vista Archivo solo en los cuadernos propios: a los demás el
// servicio les responde 404 y el botón fallaría siempre.
func TestArchivoSoloOfreceDesarchivarLoPropio(t *testing.T) {
	const ayer = `"archived_at":"2026-09-30T10:00:00Z"`
	p, _ := pantallaConServicio(t, map[string]string{
		"GET /notebooks": `{"notebooks":[{"id":"c1","title":"Mudanza","owner_sub":"ada",` + ayer + `},` +
			`{"id":"c2","title":"Ajeno","owner_sub":"bob",` + ayer + `}]}`,
	})
	p.Yo = func() string { return "ada" }
	p.cargarCuadernos()
	p.vista = VistaArchivo
	w := test.NewWindow(p.contenido(p.variante()))
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	capturar(t, w, "archivo")
	textos := textosInternos(w.Content())
	volver := 0
	for _, x := range textos {
		if x == p.tr.T("archive.unarchive", nil) {
			volver++
		}
	}
	if !slices.Contains(textos, "Ajeno") || volver != 1 {
		t.Errorf("el Archivo ofrece desarchivar %d veces (pinta %q); solo el propio", volver, textos)
	}
}

// En ajustes, desarchivar solo la cabeza de la rama: un hijo con el padre archivado
// volvería colgando de uno que no se ve.
func TestAjustesNoDesarchivaUnHijoConElPadreArchivado(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Yo = func() string { return "ada" }
	ayer, padre := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC), "c1"
	p.cuadernos = []Cuaderno{
		DeCuaderno(api.Notebook{ID: "c1", Title: "Mudanza", OwnerSub: "ada", ArchivedAt: &ayer}),
		DeCuaderno(api.Notebook{ID: "c2", ParentID: &padre, Title: "Cajas", OwnerSub: "ada", ArchivedAt: &ayer}),
	}
	volver := p.tr.T("archive.unarchive", nil)
	for i, ofrece := range []bool{true, false} {
		c := p.cuadernos[i]
		p.dialogoAjustes(&c, "")
		if got := slices.Contains(textosInternos(p.ajustes), volver); got != ofrece {
			t.Errorf("%s: ofrece desarchivar = %v", c.Titulo, got)
		}
		p.cerrarAjustes()
	}
}

// «Mover a» no ofrece cuadernos archivados, como MoverNotaModal (cuadernosEnArbol).
func TestMoverNoOfreceLoArchivado(t *testing.T) {
	w := test.NewWindow(nil)
	defer w.Close()
	p := NuevaPantalla(trInterno(t), "es", nil, nil, w)
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.cuadernos = []Cuaderno{
		{ID: "c1", Titulo: "Mudanza"},
		{ID: "c2", Titulo: "Trabajo"},
		{ID: "c3", Titulo: "Viejo", Archivado: true},
	}
	p.activo = p.cuadernos[0]
	p.dialogoMover(DeNota(api.Note{ID: "n1", NotebookID: "c1", Kind: "text", Title: "Presupuesto"}))
	textos := textosInternos(w.Canvas().Overlays().Top())
	if !slices.Contains(textos, "Trabajo") || slices.Contains(textos, "Viejo") {
		t.Errorf("mover ofrece %q; lo archivado no es destino", textos)
	}
}
