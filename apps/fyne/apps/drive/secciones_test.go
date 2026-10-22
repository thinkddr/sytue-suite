// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// servicioSecciones contesta las cuatro secciones planas y la raíz.
func servicioSecciones(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/folders/root":
			_, _ = io.WriteString(w, `{"id":"raiz","is_root":true}`)
		case "GET /api/v1/folders/raiz/children":
			_, _ = io.WriteString(w, `{"folders":[],"files":[]}`)
		case "GET /api/v1/favorites":
			_, _ = io.WriteString(w, `{"items":[{"id":"f2","name":"b.pdf","path":"/b.pdf"},{"id":"c1","name":"Actas","is_directory":true,"path":"/Actas"}]}`)
		case "GET /api/v1/recent":
			_, _ = io.WriteString(w, `{"items":[{"id":"f1","name":"acta.md","path":"/Actas/acta.md","size_bytes":10}],"next_cursor":"mas"}`)
		case "GET /api/v1/shared-with-me":
			_, _ = io.WriteString(w, `{"items":[{"id":"s1","name":"Proyecto","is_directory":true,"path":"/Proyecto","permission":"edit","shared_by":"Ana"}]}`)
		case "GET /api/v1/shared/s1/list":
			w.WriteHeader(http.StatusNotFound) // el share se revocó mientras se miraba
		case "GET /api/v1/documents":
			_, _ = io.WriteString(w, `{"documents":[{"id":"d1","kind":"spreadsheet","title":"Cuentas","updated_at":"2026-09-22T10:00:00Z"}]}`)
		default:
			t.Errorf("petición inesperada: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestLasSeccionesYaNoSeQuedanCargando: «Recientes», «Destacados», «Compartido conmigo» y
// «Documentos» se quedaban en «Cargando» para siempre porque nadie las pedía. Ahora cada
// sección tiene su carga y cada una llega a su lista, en el orden del servicio.
//
// Se llama a la carga en ESTA rutina y no con `irA`, que la lanza aparte: el driver de
// pruebas de Fyne ejecuta `fyne.Do` en el sitio, y leer el estado mientras otra rutina lo
// escribe es una carrera de verdad.
//
// VISTO EN ROJO (2026-09-22, en el frontal): quitando el `case Recientes` de `cargadorDe`,
// el test dice que «Recientes» no tiene carga.
func TestLasSeccionesYaNoSeQuedanCargando(t *testing.T) {
	srv := servicioSecciones(t)
	p := NuevaPantalla(traductor(t), api.NewDrive(srv.URL, sesionDoble{}, nil), nil, nil)
	p.Docs = api.NewDocs(srv.URL, sesionDoble{}, nil)
	for _, s := range []Seccion{MiUnidad, Documentos, CompartidoConmigo, Recientes, Favoritos, Papelera} {
		if p.cargadorDe(s) == nil {
			t.Fatalf("la sección %d no tiene carga y se quedaría en «Cargando»", s)
		}
	}

	p.cargadorDe(Recientes)()
	if r := p.sec.recientes; r.Fase != Exito || len(r.Elementos) != 1 || !r.HayMas || r.Elementos[0].Ruta != "/Actas/acta.md" {
		t.Errorf("Recientes: %d %+v", r.Fase, r.Elementos)
	}

	p.cargadorDe(Favoritos)()
	if d := p.sec.destacados; d.Fase != Exito || len(d.Elementos) != 2 || d.Elementos[0].ID != "f2" || !d.Elementos[1].Carpeta {
		t.Errorf("Destacados no conserva el orden del servicio: %+v", d.Elementos)
	}

	p.cargadorDe(CompartidoConmigo)()
	c := p.sec.compartido
	if c.Fase != Exito || len(c.Raices) != 1 {
		t.Fatalf("Compartido conmigo: %d %+v", c.Fase, c.Raices)
	}
	// Entrar en una carpeta cuyo share ya no vale es «ya no tienes acceso», no un fallo.
	raiz := c.Raices[0]
	c.Raiz = &raiz
	p.cargarCarpetaCompartida()
	if !c.Denegada {
		t.Error("un 404 dentro de un share no se lee como acceso retirado")
	}

	p.cargadorDe(Documentos)()
	if d := p.sec.documentos; d.Fase != Exito || len(d.Documentos) != 1 || d.Documentos[0].Tipo != api.DocSheet {
		t.Errorf("Documentos: %d %+v", d.Fase, d.Documentos)
	}
}

// TestLaFilaPlanaMideLoDeLaWeb: `py-2` y la tesela de 36 con su filete son 51, igual
// que la del explorador; en «Documentos» el glifo es de 20 y el tipo va en `text-sm`, y
// la fila se queda en 50. Y solo el NOMBRE abre: pulsar la meta no hace nada.
func TestLaFilaPlanaMideLoDeLaWeb(t *testing.T) {
	f := NuevaFilaPlana("acta.md")
	f.Meta = "4 KB · 6 sept 2026 · Actas"
	casi(t, "la fila plana", f.AltoFilaPlana(), 57)

	doc := NuevaFilaPlana("Cuentas")
	doc.Glifo = glifoCon(glifoMas, LadoGlifoDoc, tema.Color("text-muted", tema.Claro))
	doc.Meta, doc.MetaDocs = "Hoja de cálculo", true
	casi(t, "la fila de un documento", doc.AltoFilaPlana(), 57)

	abierto, estrella := false, false
	f.AlAbrir = func() { abierto = true }
	f.Botones = []BotonFila{{Cuerpo: glifoMas, Tinta: "text-subtle", AlPulsar: func(fyne.CanvasObject) { estrella = true }}}
	w := test.NewWindow(f)
	defer w.Close()
	w.Resize(fyne.NewSize(900, 51))
	f.Resize(fyne.NewSize(900, 51))

	// La meta está debajo del nombre, a la misma x: pulsarla no abre.
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(ui.MargenFila+ui.TeselaDrive+HuecoFilaPlana+2, 40)})
	if abierto {
		t.Error("pulsar la línea de meta abre el fichero, y en la web solo abre el nombre")
	}
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(ui.MargenFila+ui.TeselaDrive+HuecoFilaPlana+2, 16)})
	if !abierto {
		t.Error("pulsar el nombre no abre")
	}
	f.Tapped(&fyne.PointEvent{Position: fyne.NewPos(900-ui.MargenFila-LadoBotonFila/2, 25)})
	if !estrella {
		t.Error("el botón del final no responde")
	}
}

// TestUbicacionDe es `locationOf`: la carpeta que contiene la ruta, o «Mi unidad».
func TestUbicacionDe(t *testing.T) {
	tr := traductor(t)
	for ruta, quiero := range map[string]string{
		"/acta.md":            tr.T("nav.myDrive", nil),
		"/Actas/acta.md":      "Actas",
		"/A/Dise%C3%B1o/x.md": "Diseño",
	} {
		if got := UbicacionDe(tr, ruta); got != quiero {
			t.Errorf("%s: %q", ruta, got)
		}
	}
}

// TestLasSeccionesSePintan pinta cada vista en los dos temas y en sus fases: un token mal
// escrito entra en pánico AL PINTAR, no al compilar.
func TestLasSeccionesSePintan(t *testing.T) {
	tr := traductor(t)
	for _, v := range []tema.Variante{tema.Claro, tema.Oscuro} {
		for _, fase := range []Fase{Cargando, Exito, Vacia, Fallo, SinSesion, SinConexion} {
			favs := map[string]bool{"f1": true}
			vp := NuevaVistaPlana(tr, Recientes, favs)
			vp.Fase, vp.Ventana, vp.HayMas = fase, 1280, true
			vp.Elementos = []Elemento{{ID: "f1", Nombre: "acta.md", Tamano: 10, Ruta: "/Actas/acta.md"}}
			c := NuevaVistaCompartido(tr)
			c.Fase, c.Ventana = fase, 1280
			c.Raices = []api.Compartido{{File: api.File{ID: "s1", Name: "Proyecto"}, IsDir: true, Permission: "edit", SharedBy: "Ana"}}
			dentro := NuevaVistaCompartido(tr)
			raiz := c.Raices[0]
			dentro.Raiz, dentro.Fase, dentro.FaseCarpeta, dentro.Ventana = &raiz, Exito, fase, 1280
			dentro.Carpeta = []api.Compartido{{File: api.File{ID: "x", Name: "plano.pdf", SizeBytes: 9}}}
			d := NuevaVistaDocumentos(tr)
			d.Fase, d.Ventana = fase, 1280
			d.Documentos = []Documento{{ID: "d1", Titulo: "Cuentas", Tipo: api.DocSheet, Cambiado: time.Now()}}
			for _, o := range []fyne.CanvasObject{vp.Objeto(v), c.Objeto(v), dentro.Objeto(v), d.Objeto(v)} {
				w := test.NewWindow(o)
				w.SetPadded(false)
				w.Resize(fyne.NewSize(1020, 744))
				w.Canvas().Capture()
				w.Close()
			}
		}
	}
}
