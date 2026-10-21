// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"encoding/json"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func TestMain(m *testing.M) {
	test.NewApp().Settings().SetTheme(tema.Sytue{})
	tema.FormaDeLosGuardianes()
	os.Exit(m.Run())
}

// doble es un servicio de Bases de pega: contesta con lo que se le deja en `rutas`
// (método y ruta → cuerpo) y apunta lo que recibe.
type doble struct {
	mu     sync.Mutex
	rutas  map[string]string
	vistas []string
}

func (d *doble) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cuerpo, _ := io.ReadAll(r.Body)
	d.mu.Lock()
	d.vistas = append(d.vistas, strings.TrimSpace(r.Method+" "+strings.TrimPrefix(r.URL.RequestURI(), "/api/v1")+" "+string(cuerpo)))
	resp, ok := d.rutas[r.Method+" "+strings.TrimPrefix(r.URL.Path, "/api/v1")]
	d.mu.Unlock()
	switch {
	case !ok:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"not_found"}}`)
	case resp == "":
		w.WriteHeader(http.StatusNoContent)
	default:
		_, _ = io.WriteString(w, resp)
	}
}

func (d *doble) pon(ruta, cuerpo string) {
	d.mu.Lock()
	d.rutas[ruta] = cuerpo
	d.mu.Unlock()
}

func (d *doble) vio(linea string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Contains(d.vistas, linea)
}

const (
	baseClientes = `{"id":"b1","name":"Clientes","description":"","owner_sub":"u1","role":"owner","created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-02T09:00:00Z"}`
	basePedidos  = `{"id":"b2","name":"Pedidos","description":"","owner_sub":"u2","role":"viewer","host_org":"acme","created_at":"2026-10-01T09:00:00Z","updated_at":"2026-10-03T09:00:00Z"}`
	tablasB1     = `[{"id":"t1","base_id":"b1","name":"Personas","position":0,"fields":[]},{"id":"t2","base_id":"b1","name":"Empresas","position":1,"fields":[]}]`
)

// montar abre la app contra el doble, con la carga en la rutina del test y en orden.
func montar(t *testing.T, rutas map[string]string) (*Pantalla, *doble, fyne.Window) {
	t.Helper()
	d := &doble{rutas: rutas}
	srv := httptest.NewServer(d)
	t.Cleanup(srv.Close)
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	p := NuevaPantalla(tr, api.NewBases(srv.URL, nil, nil), w)
	p.fondo = func(f func()) { f() }
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Arrancar()
	return p, d, w
}

func recorrer(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	if o == nil || !o.Visible() {
		return
	}
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			recorrer(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			recorrer(h, f)
		}
	}
}

// boton busca un botón VISIBLE por su rótulo en la ventana y sus capas (diálogos).
func boton(w fyne.Window, texto string) *ui.Boton {
	var hallado *ui.Boton
	mirar := func(o fyne.CanvasObject) {
		if b, ok := o.(*ui.Boton); ok && b.Texto == texto {
			hallado = b
		}
	}
	recorrer(w.Canvas().Content(), mirar)
	for _, capa := range w.Canvas().Overlays().List() {
		recorrer(capa, mirar)
	}
	return hallado
}

// textos son los rótulos y textos de lienzo que hay a la vista, capas incluidas.
func textos(w fyne.Window) string {
	var b strings.Builder
	mirar := func(o fyne.CanvasObject) {
		switch x := o.(type) {
		case *ui.Rotulo:
			b.WriteString(x.Texto + "\n")
		case *ui.Parrafo:
			b.WriteString(x.Texto + "\n")
		case *tarjetaBase:
			b.WriteString(x.AccessibilityLabel() + "\n")
		case *ui.Boton:
			b.WriteString("[" + x.Texto + "]\n")
		case *ui.Enlace:
			b.WriteString(x.Texto + "\n")
		case *ui.Pestana:
			b.WriteString("<" + x.Rotulo + ">\n")
		}
	}
	recorrer(w.Canvas().Content(), mirar)
	for _, capa := range w.Canvas().Overlays().List() {
		recorrer(capa, mirar)
	}
	return b.String()
}

// capturar deja la pantalla en disco con SYTUE_CAPTURA=/ruta, para MIRARLA.
func capturar(t *testing.T, w fyne.Window, nombre string) {
	t.Helper()
	destino := os.Getenv("SYTUE_CAPTURA")
	if destino == "" {
		return
	}
	f, err := os.Create(destino + "/" + nombre + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}

// TestMisBasesEnsenaLaListaYElRol: la lista trae el nombre, cuándo se actualizó, de qué
// organización viene una base de fuera y el rol de quien mira.
func TestMisBasesEnsenaLaListaYElRol(t *testing.T) {
	_, _, w := montar(t, map[string]string{"GET /bases": "[" + baseClientes + "," + basePedidos + "]"})
	capturar(t, w, "lista")
	got := textos(w)
	for _, quiero := range []string{
		"Mis bases", "Nombre de la nueva base", "[Crear base]",
		"Clientes, Actualizada el 2 oct 2026, Propietario",
		"Pedidos, Actualizada el 3 oct 2026 · Compartida desde acme, Lector",
	} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, got)
		}
	}
	if b := boton(w, "Crear base"); b == nil || !b.Inerte {
		t.Error("«Crear base» tiene que estar apagado sin nombre")
	}
}

// TestMisBasesVaciaYConFallo: sin bases sale el estado vacío; si la lista falla, el
// mensaje traducido y NO el estado vacío (que diría que no hay ninguna).
func TestMisBasesVaciaYConFallo(t *testing.T) {
	_, _, w := montar(t, map[string]string{"GET /bases": "[]"})
	capturar(t, w, "lista-vacia")
	var vacio *ui.Vacio
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if v, ok := o.(*ui.Vacio); ok {
			vacio = v
		}
	})
	if vacio == nil || vacio.Titulo != "Todavía no tienes ninguna base" {
		t.Fatalf("no sale el estado vacío: %+v", vacio)
	}

	_, _, w = montar(t, map[string]string{})
	if got := textos(w); !strings.Contains(got, "Ese elemento ya no existe.") {
		t.Errorf("el fallo de la lista no se enseña:\n%s", got)
	}
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if _, ok := o.(*ui.Vacio); ok {
			t.Error("con la lista fallida no puede salir «todavía no tienes ninguna base»")
		}
	})
}

// TestCrearUnaBaseLaAbre: escribir el nombre enciende el botón, crear manda el nombre
// recortado y se entra en la base nueva.
func TestCrearUnaBaseLaAbre(t *testing.T) {
	p, d, w := montar(t, map[string]string{
		"GET /bases": "[]", "POST /bases": baseClientes,
		"GET /bases/b1": baseClientes, "GET /bases/b1/tables": "[]",
	})
	p.nombre.PonerTexto("  Clientes ")
	p.ajustarCrear()
	b := boton(w, "Crear base")
	if b == nil || b.Inerte {
		t.Fatal("con nombre, «Crear base» tiene que estar encendido")
	}
	test.Tap(b)
	if !d.vio(`POST /bases {"name":"Clientes","description":""}`) {
		t.Fatalf("no se ha creado la base: %q", d.vistas)
	}
	capturar(t, w, "base-sin-tablas")
	got := textos(w)
	for _, quiero := range []string{"Clientes", "Esta base todavía no tiene tablas.", "[Renombrar base]", "[Borrar base]"} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, got)
		}
	}
	if p.nombre.Texto() != "" {
		t.Error("el campo del nombre se queda escrito tras crear")
	}
}

// TestLaBaseAbiertaSegunElRol: el propietario renombra y borra; quien solo lee no ve
// esos botones, y quien solo accede a una tabla (la base contesta 404) ve sus tablas.
func TestLaBaseAbiertaSegunElRol(t *testing.T) {
	rutas := map[string]string{
		"GET /bases":    "[" + baseClientes + "," + basePedidos + "]",
		"GET /bases/b1": baseClientes, "GET /bases/b1/tables": tablasB1,
		"GET /bases/b2": basePedidos, "GET /bases/b2/tables": `[]`,
		"GET /bases/b3/tables": `[{"id":"t9","base_id":"b3","name":"Sueltas","position":0,"role":"viewer","fields":[]}]`,
		"PATCH /bases/b1":      "", "DELETE /bases/b1": "",
		"GET /tables/t1/views": "[]", "GET /tables/t1/records": `{"fields":[],"records":[],"role":"owner"}`,
		"GET /tables/t9/views": "[]", "GET /tables/t9/records": `{"fields":[],"records":[],"role":"viewer"}`,
	}
	p, d, w := montar(t, rutas)

	var tarjeta *tarjetaBase
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if x, ok := o.(*tarjetaBase); ok && x.Nombre == "Clientes" {
			tarjeta = x
		}
	})
	if tarjeta == nil {
		t.Fatal("no está la tarjeta de Clientes")
	}
	test.Tap(tarjeta)
	capturar(t, w, "base-propietario")
	got := textos(w)
	for _, quiero := range []string{"Todas las bases", "<Personas>", "<Empresas>", "[Renombrar base]", "[Borrar base]"} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q en:\n%s", quiero, got)
		}
	}

	// Renombrar: el diálogo trae el nombre y manda el nuevo con la descripción de siempre.
	test.Tap(boton(w, "Renombrar base"))
	capturar(t, w, "base-renombrar")
	var campo *ui.Entrada
	for _, capa := range w.Canvas().Overlays().List() {
		recorrer(capa, func(o fyne.CanvasObject) {
			if e, ok := o.(*ui.Entrada); ok {
				campo = e
			}
		})
	}
	if campo == nil || campo.Texto() != "Clientes" {
		t.Fatalf("el diálogo de renombrar no trae el nombre: %+v", campo)
	}
	campo.PonerTexto("Clientes 2026")
	d.pon("GET /bases/b1", strings.Replace(baseClientes, `"Clientes"`, `"Clientes 2026"`, 1))
	test.Tap(boton(w, "Guardar"))
	if !d.vio(`PATCH /bases/b1 {"name":"Clientes 2026","description":""}`) {
		t.Fatalf("no se ha renombrado: %q", d.vistas)
	}
	if p.abierta.Datos.Name != "Clientes 2026" {
		t.Errorf("la cabecera sigue con el nombre viejo: %q", p.abierta.Datos.Name)
	}

	// Borrar pregunta antes, y al confirmar vuelve a la lista.
	test.Tap(boton(w, "Borrar base"))
	capturar(t, w, "base-borrar")
	if d.vio("DELETE /bases/b1") {
		t.Fatal("se ha borrado sin confirmar")
	}
	if got := textos(w); !strings.Contains(got, "¿Borrar la base Clientes 2026 con todas sus tablas y registros?") {
		t.Errorf("la confirmación no dice qué se borra:\n%s", got)
	}
	test.Tap(boton(w, "Borrar"))
	if !d.vio("DELETE /bases/b1") || p.abierta != nil {
		t.Fatalf("no se ha borrado o no se ha vuelto a la lista: %q", d.vistas)
	}

	// Quien solo lee no ve lo del propietario.
	p.Abrir("b2")
	if got := textos(w); strings.Contains(got, "[Renombrar base]") || strings.Contains(got, "[Borrar base]") {
		t.Errorf("un lector ve los botones del propietario:\n%s", got)
	}
	// Y quien solo accede a una tabla (404 en la base) se queda con ella.
	p.Abrir("b3")
	capturar(t, w, "base-parcial")
	if got := textos(w); !strings.Contains(got, "<Sueltas>") || strings.Contains(got, "ya no existe") {
		t.Errorf("con acceso a una sola tabla no se ven sus pestañas:\n%s", got)
	}
}

// TestLosFallosSeTraducen: el mensaje sale del estado y del código, nunca del texto del
// servidor (espejo de `mensajeDeError` de lib/api.ts).
func TestLosFallosSeTraducen(t *testing.T) {
	p, _, _ := montar(t, map[string]string{"GET /bases": "[]"})
	cuerpo := func(codigo string) string {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": codigo, "message": "texto crudo"}})
		return string(b)
	}
	casos := []struct {
		err    error
		quiero string
	}{
		{&api.Error{Status: 401}, "Tu sesión ha caducado. Vuelve a entrar."},
		{&api.Error{Status: 403}, "No tienes permiso para hacer esto."},
		{&api.Error{Status: 404}, "Ese elemento ya no existe."},
		{&api.Error{Status: 409, Body: cuerpo("rev_conflict")}, "Otra persona ha cambiado este registro. Se ha vuelto a cargar."},
		{&api.Error{Status: 409, Body: cuerpo("referenced")}, "Otros registros enlazan con este, así que no se puede quitar."},
		{&api.Error{Status: 422, Body: cuerpo("not_a_member")}, "Nadie de tu organización tiene esa dirección."},
		{&api.Error{Status: 422, Body: cuerpo("limit_reached")}, "Has llegado a un límite de esta base."},
		{&api.Error{Status: 422, Body: cuerpo("otro")}, "Revisa los datos que has escrito."},
		{&api.Error{Status: 500}, "Algo ha fallado. Inténtalo de nuevo."},
		{io.ErrUnexpectedEOF, "Algo ha fallado. Inténtalo de nuevo."},
	}
	for _, c := range casos {
		if got := p.mensajeDeError(c.err); got != c.quiero {
			t.Errorf("%v: %q, quiero %q", c.err, got, c.quiero)
		}
	}
}
