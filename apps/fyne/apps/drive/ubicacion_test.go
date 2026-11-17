// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioUbicacion es un árbol de tres niveles: Mi unidad › Proyectos › Actas, con
// `acta.md` dentro de Actas. «Destacados» trae la carpeta Actas y «Recientes» el fichero,
// cada uno con su ruta y SIN su carpeta, como los manda el servicio.
func servicioUbicacion(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root", "GET /api/v1/folders/raiz":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[{"id":"p1","name":"Proyectos","parent_id":"raiz"}],"files":[]}`)
		case "GET /api/v1/folders/p1":
			_, _ = io.WriteString(w, `{"id":"p1","name":"Proyectos","parent_id":"raiz"}`)
		case "GET /api/v1/folders/c1":
			_, _ = io.WriteString(w, `{"id":"c1","name":"Actas","parent_id":"p1"}`)
		case "GET /api/v1/folders/c1/children":
			_, _ = io.WriteString(w, `{"folders":[{"id":"c2","name":"Anexos","parent_id":"c1"}],"files":[`+
				`{"id":"f0","folder_id":"c1","name":"aa.md"},{"id":"f1","folder_id":"c1","name":"acta.md"}]}`)
		case "GET /api/v1/files/f1":
			_, _ = io.WriteString(w, `{"id":"f1","folder_id":"c1","name":"acta.md"}`)
		case "GET /api/v1/favorites":
			_, _ = io.WriteString(w, `{"items":[{"id":"c1","name":"Actas","is_directory":true,"path":"/Proyectos/Actas"}]}`)
		case "GET /api/v1/recent":
			_, _ = io.WriteString(w, `{"items":[{"id":"f1","name":"acta.md","path":"/Proyectos/Actas/acta.md","size_bytes":10}]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func pantallaUbicacion(t *testing.T) *Pantalla {
	t.Helper()
	p := NuevaPantalla(traductor(t), api.NewDrive(servicioUbicacion(t).URL, sesionDoble{}, nil), nil, nil)
	p.aparte = func(f func()) { f() }
	p.dentro = true
	w := test.NewWindow(container.NewStack())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	p.win = w
	w.SetContent(p.Objeto())
	return p
}

func ids(els []Elemento) []string {
	out := make([]string, 0, len(els))
	for _, e := range els {
		out = append(out, e.ID)
	}
	return out
}

// TestLaCarpetaPlanaAbreMiUnidad: pulsar una carpeta de «Destacados» —antes texto
// muerto— abre «Mi unidad» en ella, con las migas VERDADERAS (Proyectos › Actas) y no
// colgada de la carpeta que se miraba.
//
// VISTO EN ROJO (2026-11-09, en el frontal): con el `!el.Carpeta &&` de vuelta en
// `VistaPlana.fila`, la fila de la carpeta no tiene `AlAbrir` y el test lo dice.
func TestLaCarpetaPlanaAbreMiUnidad(t *testing.T) {
	p := pantallaUbicacion(t)
	p.explor.Ruta = []Elemento{{ID: "otra", Nombre: "Otra", Carpeta: true}}
	p.seccion = Favoritos
	p.cargarDestacados()
	d := p.sec.destacados
	if d.Fase != Exito || len(d.Elementos) != 1 || !d.Elementos[0].Carpeta {
		t.Fatalf("Destacados: %d %+v", d.Fase, d.Elementos)
	}

	f := d.fila(d.Elementos[0])
	if f.AlAbrir == nil {
		t.Fatal("la carpeta de Destacados sigue sin abrir nada")
	}
	w := test.NewWindow(f)
	defer w.Close()
	w.Resize(fyne.NewSize(900, 57))
	f.Resize(fyne.NewSize(900, 57))
	// El NOMBRE, a la altura de la primera línea.
	test.TapAt(f, fyne.NewPos(ui.MargenFila+ui.TeselaDrive+HuecoFilaPlana+4, 18))
	esperar()

	if p.seccion != MiUnidad || p.lateral.Activa != MiUnidad {
		t.Fatalf("pulsar la carpeta deja la sección en %d", p.seccion)
	}
	if got := ids(p.explor.Ruta); !slices.Equal(got, []string{"p1", "c1"}) {
		t.Errorf("las migas son %q y la carpeta vive en Proyectos › Actas", got)
	}
	if p.explor.Fase != Exito || len(p.explor.Elementos) != 3 {
		t.Errorf("no se cargó la carpeta: %d %+v", p.explor.Fase, p.explor.Elementos)
	}
}

// TestUbicacionMarcaElFicheroDeRecientes: «Mostrar en su carpeta» desde «Recientes» abre
// Actas con sus migas, con `acta.md` marcado (selección de uno, ancla en él) y pedido a la
// vista; y lo marcado no se queda pegado: otra carpeta llega sin nada marcado.
//
// VISTO EN ROJO (2026-11-09, en el frontal): sin `p.aplicarMarca` en `cargarCarpeta`,
// la carpeta se abre pero no hay nada marcado.
func TestUbicacionMarcaElFicheroDeRecientes(t *testing.T) {
	p := pantallaUbicacion(t)
	p.seccion = Recientes
	p.cargarRecientes()
	r := p.sec.recientes
	if len(r.Elementos) != 1 {
		t.Fatalf("Recientes: %+v", r.Elementos)
	}
	var mostrar *accionMenu
	for _, a := range p.accionesPlano(r.Elementos[0], false) {
		if a.texto == p.tr.T("action.showInFolder", nil) {
			mostrar = &a
		}
	}
	if mostrar == nil {
		t.Fatal("el «⋯» de Recientes no ofrece «Mostrar en su carpeta»")
	}
	mostrar.hacer()
	esperar()

	e := p.explor
	if p.seccion != MiUnidad {
		t.Fatalf("la sección es %d", p.seccion)
	}
	if got := ids(e.Ruta); !slices.Equal(got, []string{"p1", "c1"}) {
		t.Errorf("las migas son %q", got)
	}
	if e.Marcados() != 1 || !e.Seleccion["f1"] {
		t.Errorf("lo marcado es %v y debía ser solo acta.md", e.Seleccion)
	}
	if orden := e.EnOrden(); p.ancla < 0 || orden[p.ancla].ID != "f1" {
		t.Errorf("el ancla (%d) no está en acta.md: Mayús + clic seguiría desde otro", p.ancla)
	}
	if e.AVista != "" {
		t.Errorf("el pintado no consumió la petición de llevar %q a la vista", e.AVista)
	}

	// La marca era de un solo uso.
	p.cargarCarpeta("c1")
	if e.Marcados() != 0 {
		t.Errorf("al recargar sigue marcado %v", e.Seleccion)
	}
}

// TestUbicacionSinRedSoloConLaCarpetaGuardada: sin red, la opción solo sale si su
// carpeta se cargó en esta sesión (y la sirve la caché del transporte); y en el «⋯» de
// «Mi unidad» solo sale en una búsqueda, que es lo que cruza el árbol.
func TestUbicacionSinRedSoloConLaCarpetaGuardada(t *testing.T) {
	p := pantallaUbicacion(t)
	fichero := Elemento{ID: "f1", Nombre: "acta.md", Ruta: "/Proyectos/Actas/acta.md"}
	mostrar := p.tr.T("action.showInFolder", nil)
	tiene := func(a []accionMenu) bool { return slices.Contains(textos(a), mostrar) }

	if !tiene(p.accionesPlano(fichero, false)) {
		t.Error("con red no se ofrece")
	}
	if tiene(p.accionesPlano(fichero, true)) {
		t.Error("sin red se ofrece una carpeta que no se ha visto nunca")
	}
	p.cargarCarpeta("c1")
	if !tiene(p.accionesPlano(fichero, true)) {
		t.Error("sin red no se ofrece aunque su carpeta esté cargada")
	}
	if tiene(p.accionesPlano(Elemento{Nombre: "suelto.txt"}, false)) {
		t.Error("un elemento sin id no tiene carpeta que buscar")
	}

	if tiene(p.AccionesDe(fichero, false, false)) {
		t.Error("en una carpeta ya se está en la suya")
	}
	p.explor.EnBusqueda = true
	got := textos(p.AccionesDe(fichero, false, false))
	if len(got) < 2 || got[1] != mostrar {
		t.Errorf("en una búsqueda va detrás de descargar, como en la web: %q", got)
	}
}

// TestUbicacionPadreYMigas: la carpeta de un fichero es su `folder_id` y la de una
// carpeta su `parent_id`; la raíz no tiene carpeta. Las migas de la raíz están vacías, y
// sin poder subir se entra con la carpeta sola.
func TestUbicacionPadreYMigas(t *testing.T) {
	ctx := context.Background()
	raiz, p1 := "raiz", "p1"
	carpetas := map[string]api.Folder{
		"raiz": {ID: "raiz", IsRoot: true},
		"p1":   {ID: "p1", Name: "Proyectos", ParentID: &raiz},
		"c1":   {ID: "c1", Name: "Actas", ParentID: &p1},
	}
	carpeta := func(_ context.Context, id string) (api.Folder, error) {
		if c, ok := carpetas[id]; ok {
			return c, nil
		}
		return api.Folder{}, errors.New("sin red")
	}
	fichero := func(_ context.Context, id string) (api.File, error) {
		if id == "f1" {
			return api.File{ID: "f1", FolderID: "c1"}, nil
		}
		return api.File{}, errors.New("sin red")
	}

	if got, err := PadreDe(ctx, fichero, carpeta, Elemento{ID: "f1"}); err != nil || got != "c1" {
		t.Errorf("fichero: %q %v", got, err)
	}
	if got, err := PadreDe(ctx, fichero, carpeta, Elemento{ID: "c1", Carpeta: true}); err != nil || got != "p1" {
		t.Errorf("carpeta: %q %v", got, err)
	}
	if _, err := PadreDe(ctx, fichero, carpeta, Elemento{ID: "raiz", Carpeta: true}); err == nil {
		t.Error("la raíz no está en ninguna carpeta")
	}

	if got := MigasDe(ctx, carpeta, "raiz", ""); len(got) != 0 {
		t.Errorf("la raíz no lleva migas: %+v", got)
	}
	if got := MigasDe(ctx, carpeta, "raiz", "raiz"); len(got) != 0 {
		t.Errorf("la raíz conocida no lleva migas: %+v", got)
	}
	if got := ids(MigasDe(ctx, carpeta, "c1", "raiz")); !slices.Equal(got, []string{"p1", "c1"}) {
		t.Errorf("migas de Actas: %q", got)
	}
	if got := ids(MigasDe(ctx, carpeta, "perdida", "raiz")); !slices.Equal(got, []string{"perdida"}) {
		t.Errorf("sin poder subir, la carpeta sola: %q", got)
	}

	// Lo que trae el listado ya dice dónde vive, sin preguntar.
	if e := DeFichero(api.File{ID: "f1", FolderID: "c1"}); e.Padre != "c1" {
		t.Errorf("DeFichero no guarda la carpeta: %+v", e)
	}
	if e := DeCarpeta(carpetas["c1"]); e.Padre != "p1" {
		t.Errorf("DeCarpeta no guarda su padre: %+v", e)
	}
}
