// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos

package drive

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

type ordenJSON struct {
	Campo   CampoOrden `json:"campo"`
	Sentido string     `json:"sentido"`
}

func (o ordenJSON) orden() Orden { return Orden{Campo: o.Campo, Desc: o.Sentido == "desc"} }

// Los MISMOS casos que `ordenarRecursos` de la web (`lib/orden.test.ts` los lee de
// aquí): la misma carpeta se ve en el mismo orden en la web y en el escritorio.
func TestOrdenComoLaWeb(t *testing.T) {
	b, err := os.ReadFile("testdata/orden.json")
	if err != nil {
		t.Fatal(err)
	}
	var casos struct {
		Elementos []struct {
			Name        string    `json:"name"`
			IsDirectory bool      `json:"isDirectory"` //nolint:tagliatelle // el JSON es el de la web
			Size        int64     `json:"size"`
			ModifiedAt  time.Time `json:"modifiedAt"` //nolint:tagliatelle // el JSON es el de la web
		} `json:"elementos"`
		Casos []struct {
			Nota     string      `json:"nota"`
			Orden    ordenJSON   `json:"orden"`
			Locale   i18n.Locale `json:"locale"`
			Esperado []string    `json:"esperado"`
		} `json:"casos"`
		Siguiente []struct {
			Actual   ordenJSON  `json:"actual"`
			Campo    CampoOrden `json:"campo"`
			Esperado ordenJSON  `json:"esperado"`
		} `json:"siguiente"`
	}
	if err := json.Unmarshal(b, &casos); err != nil {
		t.Fatal(err)
	}
	if len(casos.Casos) < 5 {
		t.Fatalf("solo %d casos", len(casos.Casos))
	}
	for _, c := range casos.Casos {
		els := make([]Elemento, 0, len(casos.Elementos))
		for _, e := range casos.Elementos {
			el := Elemento{ID: e.Name, Nombre: e.Name, Carpeta: e.IsDirectory, Tamano: e.Size, Modificado: e.ModifiedAt}
			if el.Carpeta {
				el.Tamano = -1 // lo que da DeCarpeta: el servicio no da tamaño de carpeta
			}
			els = append(els, el)
		}
		Ordenar(els, c.Orden.orden(), c.Locale)
		got := make([]string, len(els))
		for i, el := range els {
			got[i] = el.Nombre
		}
		if !slices.Equal(got, c.Esperado) {
			t.Errorf("%s (%s):\n tengo  %q\n quiero %q", c.Nota, c.Locale, got, c.Esperado)
		}
	}
	for _, s := range casos.Siguiente {
		if got := SiguienteOrden(s.Actual.orden(), s.Campo); got != s.Esperado.orden() {
			t.Errorf("SiguienteOrden(%+v, %s) = %+v, quiero %+v", s.Actual, s.Campo, got, s.Esperado)
		}
	}
}

// prefsFalsas es un almacén de preferencias en memoria.
type prefsFalsas map[string]string

func (p prefsFalsas) String(k string) string        { return p[k] }
func (p prefsFalsas) SetString(k, v string)         { p[k] = v }
func (p prefsFalsas) valor(k string) string         { return p[k] }
func (p prefsFalsas) poner(k, v string) prefsFalsas { p[k] = v; return p }

func TestOrdenYVistaSeRecuerdanComoEnLaWeb(t *testing.T) {
	p := prefsFalsas{}
	if LeerOrden(p) != OrdenDelServicio || LeerVista(p) {
		t.Fatal("sin nada guardado: nombre ascendente y lista")
	}
	GuardarOrden(p, Orden{Campo: OrdenModificado, Desc: true})
	GuardarVista(p, true)
	// Las claves y los valores de `localStorage` en la web.
	if p.valor("sytue-drive-orden") != "modified:desc" || p.valor("sytue-drive-vista") != "grid" {
		t.Fatalf("guardado = %v", p)
	}
	if LeerOrden(p) != (Orden{Campo: OrdenModificado, Desc: true}) || !LeerVista(p) {
		t.Fatal("no se lee lo guardado")
	}
	// Un valor que no se entiende vuelve al de siempre.
	if LeerOrden(p.poner("sytue-drive-orden", "peso:arriba")) != OrdenDelServicio {
		t.Fatal("un orden corrupto debería ser el de siempre")
	}
	if LeerOrden(nil) != OrdenDelServicio || LeerVista(nil) {
		t.Fatal("sin almacén: lo de siempre")
	}
}

func TestElOrdenDeSiempreNoCambiaLaURL(t *testing.T) {
	if OrdenDelServicio.Listado() != (api.OrdenListado{}) {
		t.Fatalf("el orden de siempre lleva parámetros: %+v", OrdenDelServicio.Listado())
	}
	casos := map[Orden]api.OrdenListado{
		{Campo: OrdenNombre, Desc: true}:     {Campo: "nombre", Sentido: "desc"},
		{Campo: OrdenModificado, Desc: true}: {Campo: "modificado", Sentido: "desc"},
		{Campo: OrdenTamano}:                 {Campo: "tamano", Sentido: "asc"},
	}
	for o, quiero := range casos {
		if got := o.Listado(); got != quiero {
			t.Errorf("%+v.Listado() = %+v, quiero %+v", o, got, quiero)
		}
	}
}

// conPrefs pone un almacén propio de preferencias durante la prueba.
func conPrefs(t *testing.T, p prefsFalsas) {
	t.Helper()
	antes := prefs
	prefs = func() Preferencias { return p }
	t.Cleanup(func() { prefs = antes })
}

// hasta espera a que se cumpla algo que llega por la rutina de la interfaz.
func hasta(t *testing.T, que string, f func() bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		var ok bool
		fyne.DoAndWait(func() { ok = f() })
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no llegó: %s", que)
}

// servicioOrden es «Mi unidad», con dos páginas si se pide, y apunta la consulta de
// cada listado.
type servicioOrden struct {
	dosPaginas bool
	mu         sync.Mutex
	consultas  []string
}

func (s *servicioOrden) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case "GET /api/v1/folders/raiz/children":
			s.mu.Lock()
			s.consultas = append(s.consultas, r.URL.RawQuery)
			s.mu.Unlock()
			if !s.dosPaginas {
				_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f1","folder_id":"raiz","name":"b.txt"}]}`)
				return
			}
			if r.URL.Query().Get("cursor") == "" {
				_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f1","folder_id":"raiz","name":"b.txt"}],"next_cursor":"pag2"}`)
				return
			}
			_, _ = io.WriteString(w, `{"folders":[],"files":[{"id":"f2","folder_id":"raiz","name":"a.txt"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *servicioOrden) pedidas() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.consultas)
}

// TestLaPantallaRecuerdaVistaYOrdenAlReabrir: lo que se eligió la última vez —la
// cuadrícula y «Modificado», de lo más reciente a lo más antiguo— es lo que se ve al
// abrir la app, y la carpeta se pide YA en ese orden, también la página siguiente (el
// cursor lleva su orden y seguirlo con otro es un 400).
//
// VISTO EN ROJO: sin restaurar en NuevaPantalla, se abría en lista y por nombre.
func TestLaPantallaRecuerdaVistaYOrdenAlReabrir(t *testing.T) {
	conPrefs(t, prefsFalsas{"sytue-drive-vista": "grid", "sytue-drive-orden": "modified:desc"})
	s := &servicioOrden{dosPaginas: true}
	p := NuevaPantalla(traductor(t), api.NewDrive(s.servidor(t).URL, sesionDoble{}, nil), nil, nil)
	p.aparte = func(f func()) { f() }
	p.dentro = true
	if !p.explor.Rejilla {
		t.Error("no se abre en la cuadrícula recordada")
	}
	if p.explor.Orden != (Orden{Campo: OrdenModificado, Desc: true}) {
		t.Errorf("se abre ordenada por %+v", p.explor.Orden)
	}
	p.cargarCarpeta("")
	hasta(t, "la primera página", func() bool { return p.cursor == "pag2" })
	fyne.DoAndWait(p.cargarMas)
	hasta(t, "la segunda página", func() bool { return len(p.explor.Elementos) == 2 })
	quiero := []string{"orden=modificado&sentido=desc", "cursor=pag2&orden=modificado&sentido=desc"}
	if got := s.pedidas(); !slices.Equal(got, quiero) {
		t.Errorf("consultas = %q, quiero %q", got, quiero)
	}
}

// TestPulsarUnaColumnaOrdenaYSeRecuerda: la cabecera de la lista ordena por la columna
// pulsada, la segunda pulsación invierte, y las dos cosas —orden y vista— quedan
// guardadas con las claves y los valores de la web. La carpeta se vuelve a pedir en el
// orden nuevo; el de siempre no lleva parámetros (la URL de la caché sin conexión).
func TestPulsarUnaColumnaOrdenaYSeRecuerda(t *testing.T) {
	almacen := prefsFalsas{}
	conPrefs(t, almacen)
	s := &servicioOrden{}
	p := NuevaPantalla(traductor(t), api.NewDrive(s.servidor(t).URL, sesionDoble{}, nil), nil, nil)
	p.aparte = func(f func()) { f() }
	p.dentro = true
	w := test.NewWindow(container.NewStack())
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1280, 800))
	p.win = w
	fyne.DoAndWait(func() { w.SetContent(p.Objeto()) })
	p.cargarCarpeta("")
	hasta(t, "la carpeta", func() bool { return p.explor.Fase == Exito })

	cabecera := func() *ui.CabeceraDrive {
		var cab *ui.CabeceraDrive
		fyne.DoAndWait(func() {
			for _, o := range test.LaidOutObjects(w.Content()) {
				if c, ok := o.(*ui.CabeceraDrive); ok && c.Visible() {
					cab = c
				}
			}
		})
		if cab == nil {
			t.Fatal("no hay cabecera de columnas")
		}
		return cab
	}
	if c := cabecera(); c.AlOrdenar == nil || c.OrdenCampo != "name" || c.OrdenDesc {
		t.Fatalf("la cabecera no ordena o no marca el nombre: %q desc=%v", c.OrdenCampo, c.OrdenDesc)
	}
	fyne.DoAndWait(func() { cabecera().AlOrdenar("size") })
	hasta(t, "la lista por tamaño", func() bool { return p.explor.Fase == Exito && len(s.pedidas()) == 2 })
	if p.explor.Orden != (Orden{Campo: OrdenTamano, Desc: true}) || almacen["sytue-drive-orden"] != "size:desc" {
		t.Errorf("tras pulsar «Tamaño»: orden %+v, guardado %q", p.explor.Orden, almacen["sytue-drive-orden"])
	}
	if c := cabecera(); c.OrdenCampo != "size" || !c.OrdenDesc {
		t.Errorf("la cabecera marca %q desc=%v", c.OrdenCampo, c.OrdenDesc)
	}
	fyne.DoAndWait(func() { cabecera().AlOrdenar("size") })
	hasta(t, "la lista invertida", func() bool { return len(s.pedidas()) == 3 })
	if almacen["sytue-drive-orden"] != "size:asc" {
		t.Errorf("la segunda pulsación guarda %q", almacen["sytue-drive-orden"])
	}
	quiero := []string{"", "orden=tamano&sentido=desc", "orden=tamano&sentido=asc"}
	if got := s.pedidas(); !slices.Equal(got, quiero) {
		t.Errorf("consultas = %q, quiero %q", got, quiero)
	}

	fyne.DoAndWait(func() { p.explor.AlCambiarVista(true) })
	if almacen["sytue-drive-vista"] != "grid" {
		t.Errorf("la vista guardada es %q", almacen["sytue-drive-vista"])
	}
}

// botonOrdenar es el «Ordenar» de la cabecera, o nil si no lo lleva.
func botonOrdenar(e *Explorador, ancho float32) (*CabeceraVista, *ui.Boton) {
	cab := e.cabecera(tema.Claro)
	cab.Resize(fyne.NewSize(ancho, cab.MinSize().Height))
	lay := cab.(*fyne.Container).Layout.(*CabeceraVista)
	b, _ := lay.Orden.(*ui.Boton)
	return lay, b
}

// TestOrdenarSaleDondeNoHayColumnas: el selector «Ordenar» de la web está en la
// cuadrícula, que no tiene cabeceras de columna, y en la lista por debajo de 768, donde
// «Tamaño» y «Modificado» no se pintan; no en la lista ancha ni en una búsqueda. En
// el teléfono sube a la primera fila, pegado a la derecha de la segunda columna, y las
// migas le ceden esa columna.
func TestOrdenarSaleDondeNoHayColumnas(t *testing.T) {
	web := webref.Read(t, "apps/frontend/apps/drive/src/components/FileBrowser.tsx")
	for _, trozo := range []string{
		`'max-md:col-start-2 max-md:row-start-1 max-md:justify-self-end',`,
		`view === 'list' && 'md:hidden',`,
	} {
		if !strings.Contains(web, trozo) {
			t.Errorf("FileBrowser.tsx ya no dice %q: el «Ordenar» de aquí ha dejado de ser el suyo", trozo)
		}
	}
	e := NuevoExplorador(traductor(t))
	if _, b := botonOrdenar(e, 1280); b != nil {
		t.Fatal("un explorador que no ordena ofrece «Ordenar»")
	}
	e.AlOrdenar = func(CampoOrden) {}
	e.Ventana = 1280
	if _, b := botonOrdenar(e, 1280); b != nil {
		t.Error("la lista ancha ordena desde sus columnas, no lleva «Ordenar»")
	}
	e.Rejilla = true
	lay, b := botonOrdenar(e, 1280)
	if b == nil || b.Texto != "Ordenar" {
		t.Fatalf("la cuadrícula no lleva «Ordenar»: %+v", b)
	}
	// En una línea, delante del conmutador: `gap-2` y el `ml-1` del conmutador.
	cercaDe(t, "«Ordenar», hasta el conmutador", b.Position().X+b.Size().Width+HuecoAcciones+MargenCromo,
		lay.Conmutador.Position().X, 0.01)
	e.EnBusqueda = true
	if _, b := botonOrdenar(e, 1280); b != nil {
		t.Error("en una búsqueda manda la relevancia: no hay «Ordenar»")
	}

	e.EnBusqueda, e.Rejilla, e.Ventana = false, false, 360
	lay, b = botonOrdenar(e, 360)
	if b == nil {
		t.Fatal("la lista del teléfono no lleva «Ordenar»: solo se podría ordenar por nombre")
	}
	cercaDe(t, "«Ordenar», en la fila del conmutador", b.Position().Y+b.Size().Height/2,
		lay.Conmutador.Position().Y+lay.Conmutador.Size().Height/2, 0.01)
	// La segunda columna mide lo que «Ordenar» (`auto`): ya no la ensancha «Nuevo», que
	// es el «+» flotante. Así que va pegado al conmutador, a un `gap-x-2`.
	cercaDe(t, "«Ordenar», pegado al final de la segunda columna", b.Position().X+b.Size().Width+HuecoAcciones,
		lay.Conmutador.Position().X, 0.01)
	if b.Size().Width < b.MinSize().Width {
		t.Errorf("«Ordenar» no cabe entero: %g de %g", b.Size().Width, b.MinSize().Width)
	}
	if fin := lay.Ruta.Position().X + lay.Ruta.Size().Width; fin > b.Position().X-HuecoAcciones+0.01 {
		t.Errorf("las migas acaban en %g y pisan la segunda columna, que empieza en %g", fin, b.Position().X)
	}
}

// TestUnaBusquedaSinNadaSeBorraDesdeElVacio es el «Borrar la búsqueda» del estado vacío
// de la web: vacía el buscador y le pide la carpeta otra vez a quien busca.
func TestUnaBusquedaSinNadaSeBorraDesdeElVacio(t *testing.T) {
	test.NewTempApp(t)
	tr := traductor(t)
	e := NuevoExplorador(tr)
	e.Fase, e.Ventana, e.EnBusqueda, e.Consulta = Vacia, 1280, true, "zzz"
	var pedido []string
	e.AlBuscar = func(s string) { pedido = append(pedido, s); e.Consulta = s }
	var borrar *ui.Boton
	recorrer(e.Objeto(testVariante()), func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == tr.T("files.search.clear", nil) {
			borrar = b
		}
	})
	if borrar == nil {
		t.Fatal("una búsqueda sin resultados no ofrece «Borrar la búsqueda»")
	}
	test.Tap(borrar)
	if len(pedido) != 1 || pedido[0] != "" || e.campoBusqueda().Texto() != "" {
		t.Errorf("«Borrar la búsqueda» pidió %q y dejó %q en el campo", pedido, e.campoBusqueda().Texto())
	}
}
