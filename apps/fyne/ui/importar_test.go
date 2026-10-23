// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

func TestImportarCancelaLecturasYNoPublicaOtraCuenta(t *testing.T) {
	tr := traductorPrueba(t)
	var d *ImportarDatos
	leidas, cambiar := 0, true
	cliente := &http.Client{Transport: transporteFunc(func(r *http.Request) (*http.Response, error) {
		leidas++
		body := `{"instances":[{"id":"anterior","template_key":"google","status":"active"}]}`
		if strings.Contains(r.URL.Path, "migrations") {
			body = `{"migrations":[]}`
			if cambiar {
				cambiar = false
				d.CargarCon(context.Background())
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	d = NuevoImportarDatos(tr, api.NewIntegraciones("https://fixture.example.invalid", nil, cliente), "drive")
	d.Sondeo = 0
	var trabajo []func()
	d.aparte = func(f func()) { trabajo = append(trabajo, f) }
	d.CargarCon(context.Background())
	trabajo[0]() // El transporte antiguo devuelve datos incluso tras la cancelación.
	if d.cargada || len(d.origenes) != 0 || len(trabajo) != 2 {
		t.Fatal("la respuesta anterior se publicó en la siguiente vida")
	}
	d.Parar()
	antes := leidas
	trabajo[1]()
	if leidas != antes || d.Objeto(231, tema.Claro) != nil {
		t.Fatal("una lectura encolada sobrevivió al cierre de sesión")
	}
}

func TestImportarNoReenviaUnaOrdenTrasCambiarSesion(t *testing.T) {
	g := &pasarelaDoble{instancias: `{"instances":[{"id":"g","template_key":"google","status":"active"}]}`, migraciones: `{"migrations":[]}`}
	srv := g.servidor(t)
	d := NuevoImportarDatos(traductorPrueba(t), api.NewIntegraciones(srv.URL, nil, nil), "drive")
	d.Sondeo = 0
	d.aparte = func(f func()) { f() }
	d.CargarCon(context.Background())
	origen := d.origenes[0]
	var pendiente func()
	d.aparte = func(f func()) { pendiente = f }
	d.traer(origen)
	d.Parar()
	pendiente()
	if g.cuantas() != 0 || d.enviando || len(d.trabajos) != 0 {
		t.Fatal("una orden encolada llegó a la nueva sesión")
	}
	d.aparte = func(f func()) { f() }
	d.CargarCon(context.Background())
	if g.cuantas() != 0 || len(d.trabajos) != 0 {
		t.Fatal("volver a abrir reenvió la importación")
	}
}

func TestImportarMantieneUnSoloSondeoYLoDetiene(t *testing.T) {
	g := &pasarelaDoble{instancias: `{"instances":[]}`, migraciones: `{"migrations":[{"id":"n","domain":"drive","status":"running"}]}`}
	srv := g.servidor(t)
	d := NuevoImportarDatos(traductorPrueba(t), api.NewIntegraciones(srv.URL, nil, nil), "drive")
	d.aparte = func(f func()) { f() }
	var relojes []func()
	paradas := 0
	d.esperar = func(_ time.Duration, f func()) func() {
		relojes = append(relojes, f)
		return func() { paradas++ }
	}
	d.CargarCon(context.Background())
	d.Cargar()
	if len(relojes) != 2 || paradas != 1 {
		t.Fatal("el refresco no sustituyó el temporizador anterior")
	}
	relojes[0]() // La parada no puede retirar un callback que el SO ya encoló.
	if len(relojes) != 2 {
		t.Fatal("el temporizador viejo abrió otra cadena de sondeo")
	}
	d.Parar()
	relojes[1]()
	if len(relojes) != 2 || paradas != 2 || d.Objeto(231, tema.Claro) != nil {
		t.Fatal("cerrar la vida dejó sondeos o datos antiguos")
	}
}

// pasarelaDoble hace de pasarela de integraciones con los orígenes y trabajos que se le
// pongan, y cuenta cuántas veces se le ha preguntado.
type pasarelaDoble struct {
	mu                      sync.Mutex
	instancias, migraciones string
	lanzadas                int
}

// poner cambia lo que contesta, con el cerrojo del servidor.
func (g *pasarelaDoble) poner(instancias, migraciones string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if instancias != "" {
		g.instancias = instancias
	}
	if migraciones != "" {
		g.migraciones = migraciones
	}
}

func (g *pasarelaDoble) cuantas() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.lanzadas
}

func (g *pasarelaDoble) servidor(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/instances":
			_, _ = io.WriteString(w, g.instancias)
		case "GET /api/v1/migrations/self":
			_, _ = io.WriteString(w, g.migraciones)
		case "POST /api/v1/migrations/self":
			g.lanzadas++
			_, _ = io.WriteString(w, `{"migrations":[{"id":"n","domain":"drive","status":"queued"}]}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// textosDeArbol son los rótulos de un árbol ya colocado.
func textosDeArbol(o fyne.CanvasObject) string {
	var b strings.Builder
	var ir func(fyne.CanvasObject)
	ir = func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *canvas.Text:
			b.WriteString(x.Text + " ")
		case *fyne.Container:
			for _, h := range x.Objects {
				ir(h)
			}
		case fyne.Widget:
			for _, h := range test.WidgetRenderer(x).Objects() {
				ir(h)
			}
		}
	}
	ir(o)
	return b.String()
}

// TestTraerMisDatosSoloConOrigenes: sin ningún origen conectado el bloque NO se pinta
// (un botón que siempre falla es peor que ninguno), tampoco mientras se averigua; con
// uno, se ofrece «Traer de Google»; al lanzarla dice que está trayendo, y al terminar
// avisa para que la vista se recargue.
//
// VISTO EN ROJO (2026-09-23, en el frontal): sin el `len(d.origenes) == 0` de `Objeto`,
// una organización sin orígenes enseña el bloque vacío.
func TestTraerMisDatosSoloConOrigenes(t *testing.T) {
	tr, err := i18n.New("es", i18n.Common())
	if err != nil {
		t.Fatal(err)
	}
	g := &pasarelaDoble{instancias: `{"instances":[]}`, migraciones: `{"migrations":[]}`}
	srv := g.servidor(t)
	d := NuevoImportarDatos(tr, api.NewIntegraciones(srv.URL, nil, nil), "drive")
	d.aparte = func(f func()) { f() }
	d.Sondeo = 0
	if d.Objeto(231, tema.Claro) != nil {
		t.Error("se pinta antes de saber si hay orígenes")
	}
	d.Cargar()
	if d.Objeto(231, tema.Claro) != nil {
		t.Error("sin orígenes se pinta el bloque")
	}

	g.poner(`{"instances":[{"id":"g","name":"G","template_key":"google","status":"active"}]}`, "")
	d.Cargar()
	o := d.Objeto(231, tema.Claro)
	if o == nil || !strings.Contains(textosDeArbol(o), strings.Fields(tr.T("import.fromGoogle", nil))[0]) {
		t.Fatalf("con un origen no se ofrece traer: %q", textosDeArbol(o))
	}
	d.traer(d.origenes[0])
	if g.cuantas() != 1 || !enCurso(d.trabajos) {
		t.Fatalf("lanzar no llega a la pasarela o no queda en curso: %d %+v", g.cuantas(), d.trabajos)
	}
	terminada := false
	d.AlTerminar = func() { terminada = true }
	g.poner("", `{"migrations":[{"id":"n","domain":"drive","status":"done","counts":{"migrated":12,"skipped":1}}]}`)
	d.Cargar()
	if !terminada {
		t.Error("al acabar no se avisa para recargar")
	}
	if !strings.Contains(textosDeArbol(d.Objeto(231, tema.Claro)), "12") {
		t.Error("el recuento de lo traído no se dice")
	}
}

// TestElFlujoDeDatosDeduceLaDireccion: la dirección sale de las listas, y la política
// va en el idioma (la raíz es la inglesa).
func TestElFlujoDeDatosDeduceLaDireccion(t *testing.T) {
	for _, c := range []struct {
		entra, sale []string
		modo        string
	}{{[]string{"files"}, nil, "in"}, {nil, []string{"files"}, "out"}, {[]string{"files"}, []string{"files"}, "both"}, {nil, nil, "none"}} {
		if got := ModoFlujo(c.entra, c.sale); got != c.modo {
			t.Errorf("%v/%v = %s", c.entra, c.sale, got)
		}
	}
	if PoliticaDePrivacidad("sytue.eu", "en") != "https://sytue.eu/privacy" || PoliticaDePrivacidad("sytue.eu", "es") != "https://sytue.eu/es/privacy" {
		t.Error("la política no va en el idioma")
	}
	tr, _ := i18n.New("es", i18n.Common())
	t.Cleanup(func() { tema.Elegir("") })
	for _, nombre := range []string{"claro", "oscuro"} {
		tema.Elegir(nombre)
		completo := FlujoDatos(tr, "S3", []string{"files"}, []string{"files"}, false, func() {}, 480)
		compacto := FlujoDatos(tr, "", []string{"files"}, nil, true, func() {}, 200)
		if completo.MinSize().Height <= LadoCirculoFlujo || compacto.MinSize().Height < LadoCirculoFlujoCompacto {
			t.Errorf("altos: completo %v, compacto %v", completo.MinSize(), compacto.MinSize())
		}
		w := test.NewWindow(completo)
		w.Resize(fyne.NewSize(480, 400))
		w.Canvas().Capture()
		w.Close()
	}
}
