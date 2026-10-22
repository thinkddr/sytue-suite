// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"context"
	"errors"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	fcanvas "fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// sesionDoble es una sesión con token fijo: lo que el cliente necesita para hablar con el
// servicio de mentira.
type sesionDoble struct{}

func (sesionDoble) AccessToken(context.Context) (string, error) { return "tok", nil }
func (sesionDoble) Invalidate(string)                           {}

// esperar deja correr a las rutinas que la pantalla lanza por su cuenta, y ejecuta lo que
// dejaron en la cola de `fyne.Do` (el driver de pruebas del fork lo deja para esta rutina).
func esperar() {
	fyne.DoAndWait(func() {})
	time.Sleep(5 * time.Millisecond)
}

// servicioDoble contesta lo mínimo para que la pantalla recargue, y apunta las
// peticiones que no son de carga.
type servicioDoble struct {
	mu      sync.Mutex
	pedidas []string
}

func (s *servicioDoble) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
			return
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
			return
		}
		s.mu.Lock()
		s.pedidas = append(s.pedidas, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *servicioDoble) lista() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.pedidas)
}

func textos(acciones []accionMenu) []string {
	out := make([]string, 0, len(acciones))
	for _, a := range acciones {
		out = append(out, a.texto)
	}
	return out
}

// TestElMenuDeUnElementoEsElDeLaWeb compara el «⋯» con `ActionsMenu`, en su orden y con
// sus condiciones. Sin red solo queda descargar un FICHERO: el ZIP de una carpeta lo arma
// el servicio, y sin servicio no hay ZIP.
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando el `!sinRed` de «Renombrar», el
// test dice que sin red el menú lo ofrece.
func TestElMenuDeUnElementoEsElDeLaWeb(t *testing.T) {
	tr := traductor(t)
	p := NuevaPantalla(tr, nil, nil, nil)
	p.IA = api.NewIA("http://ia.invalid", nil, nil)
	k := func(claves ...string) []string {
		out := make([]string, 0, len(claves))
		for _, c := range claves {
			out = append(out, tr.T(c, nil))
		}
		return out
	}
	fichero := Elemento{ID: "f1", Nombre: "acta.odt"}
	pdf := Elemento{ID: "f2", Nombre: "informe.pdf"}
	carpeta := Elemento{ID: "c1", Nombre: "Contratos", Carpeta: true}
	notas := Elemento{ID: "f3", Nombre: "notas.md"}
	video := Elemento{ID: "f5", Nombre: "demo.mp4", Tipo: "video/mp4"}
	etiquetado := Elemento{ID: "f4", Nombre: "acta.odt", Etiqueta: &api.DriveLabel{Name: "Confidencial"}}
	clasificado := tr.T("action.classified", map[string]any{"name": "Confidencial"})

	casos := []struct {
		que      string
		el       Elemento
		sinRed   bool
		favorito bool
		quiero   []string
	}{
		{"un fichero", fichero, false, false, k("action.download", "action.rename", "action.move",
			"action.copy", "action.shareLink", "action.versions", "action.classify", "action.attachToMail",
			"action.shareToRooms", "action.addFavorite", "action.delete")},
		{"un PDF", pdf, false, true, k("action.download", "action.rename", "action.move",
			"action.copy", "action.shareLink", "action.versions", "pdf.openInPdf", "action.classify",
			"action.attachToMail", "action.shareToRooms", "action.removeFavorite", "action.delete")},
		// Resumir solo donde hay texto que leer, y detrás de Clasificar.
		{"un texto", notas, false, false, k("action.download", "action.rename", "action.move",
			"action.copy", "action.shareLink", "action.versions", "action.classify", "ai.summarize",
			"action.attachToMail", "action.shareToRooms", "action.addFavorite", "action.delete")},
		// Clasificado, el menú dice con qué.
		{"un fichero clasificado", etiquetado, false, false, append(append(k("action.download", "action.rename", "action.move",
			"action.copy", "action.shareLink", "action.versions"), clasificado),
			k("action.attachToMail", "action.shareToRooms", "action.addFavorite", "action.delete")...)},
		// Un vídeo se puede llevar a Sytue Cast, detrás de Rooms como en la web.
		{"un vídeo", video, false, false, k("action.download", "action.rename", "action.move",
			"action.copy", "action.shareLink", "action.versions", "action.classify", "action.attachToMail",
			"action.shareToRooms", "action.uploadToCast", "action.addFavorite", "action.delete")},
		// Una carpeta no se adjunta a un correo, pero sí se comparte en una sala.
		{"una carpeta", carpeta, false, false, k("action.downloadZip", "action.rename", "action.move",
			"action.copy", "action.shareLink", "action.shareToRooms", "action.addFavorite", "action.delete")},
		// Sin red quedan las dos de lectura de un fichero: bajarlo y su historial.
		{"un fichero sin red", fichero, true, false, k("action.download", "action.versions")},
		{"una carpeta sin red", carpeta, true, false, nil},
	}
	for _, c := range casos {
		acciones := p.AccionesDe(c.el, c.sinRed, c.favorito)
		if got := textos(acciones); !slices.Equal(got, c.quiero) {
			t.Errorf("%s: el menú es %q y en la web %q", c.que, got, c.quiero)
		}
		// Eliminar es la única acción de peligro, y va la última.
		for i, a := range acciones {
			if a.peligro != (a.texto == tr.T("action.delete", nil)) || (a.peligro && i != len(acciones)-1) {
				t.Errorf("%s: «%s» está mal marcada como peligro", c.que, a.texto)
			}
		}
	}
}

// TestNuevoEsUnDesplegableYMideLoDeProduccion: «Nuevo» lleva su `chevronDown` y el
// `gap-1.5` de su clase escrita a mano, 76,25 medidos, y el buscador acaba cayendo donde
// en producción (x=833,25) porque el grupo se alinea a la derecha.
//
// Remedición Chromium 2026-10-04, FileBrowser real a1280, raíz16.
// VISTO EN ROJO (2026-09-22, en el frontal): sin el `Hueco`, «Nuevo» mide 78 y el buscador
// cae en 831,5.
func TestNuevoEsUnDesplegableYMideLoDeProduccion(t *testing.T) {
	e := NuevoExplorador(traductor(t))
	e.Ventana = 1280
	var disparado fyne.CanvasObject
	e.AlNuevo = func(d fyne.CanvasObject) { disparado = d }
	cab := e.cabecera(tema.Claro)
	cab.Resize(fyne.NewSize(1280-tema.AnchoLateral, AltoCabeceraVista))
	lay, _ := cab.(*fyne.Container).Layout.(*CabeceraVista)
	nuevo, ok := lay.Acciones[1].(*ui.Boton)
	if !ok {
		t.Fatal("«Nuevo» ya no es un botón")
	}
	cercaDe(t, "el ancho de «Nuevo»", nuevo.Size().Width, 84.734375, 0.75)
	cercaDe(t, "«Nuevo» empieza en x=1036,25", tema.AnchoLateral+nuevo.Position().X, 1006.328125, 0.75)
	cercaDe(t, "el buscador empieza en x=833,25", tema.AnchoLateral+lay.Acciones[0].Position().X, 774.328125, 0.75)
	nuevo.Tapped(nil)
	if disparado != nuevo {
		t.Error("«Nuevo» no abre su menú colgado de sí mismo")
	}
}

// TestRutaHastaSubeHastaLaRaiz: una carpeta abierta desde una búsqueda entra con su ruta
// verdadera en las migas, no colgada de la carpeta que se estaba mirando.
func TestRutaHastaSubeHastaLaRaiz(t *testing.T) {
	padre := func(id string) *string { return &id }
	arbol := map[string]api.Folder{
		"raiz": {ID: "raiz", IsRoot: true},
		"c1":   {ID: "c1", Name: "Contabilidad", ParentID: padre("raiz")},
		"c2":   {ID: "c2", Name: "2026", ParentID: padre("c1")},
		"c3":   {ID: "c3", Name: "IVA", ParentID: padre("c2")},
	}
	pedir := func(_ context.Context, id string) (api.Folder, error) {
		f, ok := arbol[id]
		if !ok {
			return api.Folder{}, errors.New("no existe")
		}
		return f, nil
	}
	ruta, err := RutaHasta(context.Background(), pedir, Elemento{ID: "c3", Nombre: "IVA", Carpeta: true})
	if err != nil {
		t.Fatal(err)
	}
	nombres := make([]string, 0, len(ruta))
	for _, c := range ruta {
		nombres = append(nombres, c.Nombre)
	}
	if !slices.Equal(nombres, []string{"Contabilidad", "2026", "IVA"}) {
		t.Errorf("la ruta es %q", nombres)
	}
	if _, err := RutaHasta(context.Background(), pedir, Elemento{ID: "perdida"}); err == nil {
		t.Error("una carpeta que el servicio no da no puede dar ruta")
	}
}

// TestMoverALaMismaCarpetaNoHaceNada es la regla que apaga «Mover aquí».
func TestMoverALaMismaCarpetaNoHaceNada(t *testing.T) {
	for _, c := range []struct {
		origen, destino string
		mismo           bool
	}{
		{"/acta.odt", "/", true},
		{"/Contratos/acta.odt", "/Contratos", true},
		{"/Contratos/acta.odt", "/", false},
		{"/Contratos", "/Contratos", false}, // una carpeta dentro de sí misma no es «donde ya está»
	} {
		if got := EsMismaCarpeta(c.origen, c.destino); got != c.mismo {
			t.Errorf("%s → %s: %v", c.origen, c.destino, got)
		}
	}
}

// TestTirarALaPapeleraSeDeshace: el aviso dura los 10 s de la web y trae su «Deshacer»,
// que devuelve el elemento a su carpeta. No pregunta antes: es reversible.
//
// VISTO EN ROJO (2026-09-22, en el frontal): sin `a.Duracion = DuracionDeshacer`, el aviso
// dura lo de serie y el test lo dice.
func TestTirarALaPapeleraSeDeshace(t *testing.T) {
	s := &servicioDoble{}
	srv := s.servidor(t)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	el := Elemento{ID: "f1", Nombre: "acta.odt"}

	p.aLaPapelera(el)
	avisos := p.avisos.Avisos()
	if len(avisos) == 0 {
		t.Fatal("tirar a la papelera no avisa")
	}
	a := avisos[len(avisos)-1]
	if a.Duracion != DuracionDeshacer || a.RotuloAccion != p.tr.T("files.delete.undo", nil) || a.AlPulsarAccion == nil {
		t.Fatalf("el aviso no trae su «Deshacer» de 10 s: %+v", a)
	}
	a.AlPulsarAccion()
	quiero := []string{"DELETE /api/v1/files/f1", "POST /api/v1/files/f1/restore"}
	for i := 0; i < 200 && len(s.lista()) < 2; i++ {
		esperar()
	}
	if got := s.lista(); !slices.Equal(got, quiero) {
		t.Errorf("al servicio ha llegado %q, y se esperaba %q", got, quiero)
	}
}

// TestLaDescargaNoDiceSubiendo: la franja de una descarga lleva el nombre, no «Subiendo
// 1/1», y «Subir» no gira mientras se baja algo.
func TestLaDescargaNoDiceSubiendo(t *testing.T) {
	tr := traductor(t)
	e := NuevoExplorador(tr)
	e.Fase, e.Ventana = Exito, 1280
	e.Subida = NuevoProgreso(1)
	e.Subida.Bajada = true
	e.Subida.Empezar(0, "informe.pdf")
	obj := e.Objeto(tema.Claro)
	obj.Resize(fyne.NewSize(1020, 744))
	if !contieneTexto(obj, "informe.pdf") {
		t.Error("la franja de la descarga no dice qué se baja")
	}
	subiendo := false
	recorrer(obj, func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == tr.T("files.upload", nil) && b.Cargando {
			subiendo = true
		}
	})
	if subiendo || e.Subiendo() {
		t.Error("«Subir» gira durante una descarga")
	}
}

// TestLaPapeleraSeDesplaza: con más filas de las que caben, las últimas se alcanzan.
func TestLaPapeleraSeDesplaza(t *testing.T) {
	p := NuevaVistaPapelera(traductor(t))
	p.Fase, p.Ventana = Exito, 1280
	for range 40 {
		p.Elementos = append(p.Elementos, Elemento{ID: "x", Nombre: "x.txt", Tamano: 1})
	}
	if _, ok := p.listado(tema.Claro).(*container.Scroll); !ok {
		t.Error("la lista de la papelera no se desplaza")
	}
}

// TestConectarDispositivosAbreElAsistenteDelHub: el pie del carril no era un botón
// muerto; abre `/dispositivos` del portal, que es otra app (§13.6).
func TestConectarDispositivosAbreElAsistenteDelHub(t *testing.T) {
	a := &abridorDoble{}
	p := NuevaPantalla(traductor(t), nil, nil, nil)
	p.App, p.Apex = a, "sytue.eu"
	if p.lateral.AlAbrirDispositivos == nil {
		t.Fatal("«Conectar tus dispositivos» no tiene qué hacer")
	}
	p.lateral.AlAbrirDispositivos()
	if len(a.urls) != 1 || a.urls[0] != "sytue-one:/dispositivos" {
		t.Errorf("abre %v", a.urls)
	}
}

// TestOrdenarPorNombreSigueElIdioma: la «ñ» va tras la «n» en castellano, no al final
// como en un orden por bytes.
func TestOrdenarPorNombreSigueElIdioma(t *testing.T) {
	els := []Elemento{{Nombre: "Ñu"}, {Nombre: "Oca"}, {Nombre: "Nube"}}
	OrdenarPorNombre(els, traductor(t).Locale())
	got := make([]string, 0, len(els))
	for _, e := range els {
		got = append(got, e.Nombre)
	}
	if strings.Join(got, ",") != "Nube,Ñu,Oca" {
		t.Errorf("el orden es %q", got)
	}
}

// TestElGlifoDeEliminarVaEnRojo: ElementoMenu no tiñe un glifo propio, así que el de
// «Eliminar» tiene que llegar ya en `danger-text`; sin tinta saldría transparente.
func TestElGlifoDeEliminarVaEnRojo(t *testing.T) {
	objs := elementosMenu([]accionMenu{{texto: "Eliminar", cuerpo: glifoPapelera, peligro: true}})
	e := objs[0].(*ui.ElementoMenu)
	quiero := tema.Color("danger-text", tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	r, _ := e.Glifo.Objeto().(*fcanvas.Raster)
	if r == nil {
		t.Fatal("el glifo no es un ráster")
	}
	img := r.Generator(32, 32)
	var pintado bool
	for y := range 32 {
		for x := range 32 {
			if c, _ := img.At(x, y).(color.NRGBA); c.A == 255 {
				pintado = true
				if c.R != quiero.R || c.G != quiero.G || c.B != quiero.B {
					t.Fatalf("el glifo de «Eliminar» pinta %v y tiene que ir en %v", c, quiero)
				}
			}
		}
	}
	if !pintado {
		t.Fatal("el glifo de «Eliminar» no pinta ni un píxel opaco")
	}
}
