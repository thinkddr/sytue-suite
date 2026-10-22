// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package bases

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type m = map[string]any

// falso es un servicio de Bases de pega CON ESTADO: una base (`b1`) con sus tablas,
// campos, registros y vistas, que recuerda lo que se le escribe. Pagina por cursor como
// el de verdad (el cursor es la posición) y apunta cada petición.
type falso struct {
	mu        sync.Mutex
	rol       string
	tablas    []m
	registros map[string][]m
	vistas    map[string][]m
	vistas2   []string // peticiones, en orden
	n         int
	// conflicto hace que la siguiente operación de celda diga que ha pisado un valor.
	conflicto bool
	// referenciado hace que borrar sin `force` conteste 409 `referenced`.
	referenciado bool
	// Con quién se comparte la base y los enlaces públicos de sus formularios.
	permisos []m
	enlaces  []m
}

func campoDe(id, tabla, nombre, tipo string, mas m) m {
	c := m{"id": id, "table_id": tabla, "name": nombre, "kind": tipo, "position": 0}
	for k, v := range mas {
		c[k] = v
	}
	return c
}

// nuevoFalso es una base con «Personas» (un campo de cada tipo y `n` registros) y
// «Empresas» (dos registros a los que enlazar).
func nuevoFalso(n int) *falso {
	f := &falso{rol: "owner", registros: map[string][]m{}, vistas: map[string][]m{}}
	f.tablas = []m{
		{"id": "t1", "base_id": "b1", "name": "Personas", "position": 0, "fields": []m{
			campoDe("nom", "t1", "Nombre", "text", nil),
			campoDe("eda", "t1", "Edad", "number", nil),
			campoDe("alt", "t1", "Alta", "date", nil),
			campoDe("act", "t1", "Activa", "checkbox", nil),
			campoDe("niv", "t1", "Nivel", "select", m{"choices": []string{"Oro", "Plata"}}),
			campoDe("eti", "t1", "Etiquetas", "multiselect", m{"choices": []string{"vip", "moroso", "nuevo"}}),
			campoDe("emp", "t1", "Empresa", "link", m{"link_table_id": "t2"}),
			campoDe("dob", "t1", "Doble", "formula", m{"formula": "{eda} * 2", "result": "number"}),
		}},
		{"id": "t2", "base_id": "b1", "name": "Empresas", "position": 1, "fields": []m{campoDe("raz", "t2", "Razón social", "text", nil)}},
	}
	for i := 1; i <= n; i++ {
		f.registros["t1"] = append(f.registros["t1"], m{
			"id": fmt.Sprintf("r%03d", i), "table_id": "t1", "rev": 1,
			"data": m{"nom": fmt.Sprintf("Persona %d", i), "eda": float64(20 + i%40), "dob": float64(2 * (20 + i%40))}, "links": m{},
		})
	}
	f.registros["t2"] = []m{
		{"id": "e1", "table_id": "t2", "rev": 1, "data": m{"raz": "Acme"}, "links": m{}},
		{"id": "e2", "table_id": "t2", "rev": 1, "data": m{"raz": "Globex"}, "links": m{}},
	}
	return f
}

func (f *falso) tabla(id string) m {
	for _, t := range f.tablas {
		if t["id"] == id {
			return t
		}
	}
	return nil
}

func (f *falso) registro(id string) (m, string) {
	for t, rs := range f.registros {
		for _, r := range rs {
			if r["id"] == id {
				return r, t
			}
		}
	}
	return nil, ""
}

func (f *falso) vio(linea string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.vistas2, linea)
}

func (f *falso) peticiones(prefijo string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, l := range f.vistas2 {
		if strings.HasPrefix(l, prefijo) {
			out = append(out, l)
		}
	}
	return out
}

func (f *falso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cuerpo, _ := io.ReadAll(r.Body)
	ruta := strings.TrimPrefix(r.URL.Path, "/api/v1")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vistas2 = append(f.vistas2, strings.TrimSpace(r.Method+" "+strings.TrimPrefix(r.URL.RequestURI(), "/api/v1")+" "+string(cuerpo)))
	var pedido m
	_ = json.Unmarshal(cuerpo, &pedido)
	x := strings.Split(strings.Trim(ruta, "/"), "/")
	di := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	nada := func() { w.WriteHeader(http.StatusNoContent) }
	falla := func(estado int, codigo string) {
		w.WriteHeader(estado)
		di(m{"error": m{"code": codigo}})
	}
	f.n++
	switch clave := r.Method + " " + x[0] + "/" + fmt.Sprint(len(x)); {
	case ruta == "/bases/b1" && r.Method == http.MethodGet:
		di(m{
			"id": "b1", "name": "Clientes", "description": "", "owner_sub": "u1", "role": f.rol,
			"created_at": "2026-10-01T09:00:00Z", "updated_at": "2026-10-02T09:00:00Z",
		})
	case ruta == "/bases/b1/tables" && r.Method == http.MethodGet:
		di(f.tablas)
	case ruta == "/bases/b1/tables" && r.Method == http.MethodPost:
		t := m{"id": fmt.Sprintf("t%d", f.n), "base_id": "b1", "name": pedido["name"], "position": len(f.tablas), "fields": []m{}}
		f.tablas = append(f.tablas, t)
		di(t)
	case clave == "PATCH tables/2":
		f.tabla(x[1])["name"] = pedido["name"]
		nada()
	case clave == "DELETE tables/2":
		f.tablas = slices.DeleteFunc(f.tablas, func(t m) bool { return t["id"] == x[1] })
		nada()
	case clave == "POST tables/3" && x[2] == "fields":
		pedido["id"], pedido["table_id"] = fmt.Sprintf("c%d", f.n), x[1]
		t := f.tabla(x[1])
		t["fields"] = append(t["fields"].([]m), pedido)
		di(pedido)
	case clave == "GET tables/3" && x[2] == "records":
		q := r.URL.Query()
		todos := slices.Clone(f.registros[x[1]])
		// Una vista con orden descendente da la vuelta a los registros: basta para ver
		// que el filtro y el orden los ejecuta el servicio.
		for _, v := range f.vistas[x[1]] {
			if cfg, _ := v["config"].(m); v["id"] == q.Get("view") && fmt.Sprint(cfg["sort"]) != "<nil>" {
				slices.Reverse(todos)
			}
		}
		desde, _ := strconv.Atoi(q.Get("cursor"))
		limite, _ := strconv.Atoi(q.Get("limit"))
		if limite == 0 {
			falla(http.StatusBadRequest, "sin_limite") // la interfaz NUNCA pide la tabla entera
			return
		}
		hasta := min(desde+limite, len(todos))
		pag := m{"fields": f.tabla(x[1])["fields"], "records": todos[min(desde, len(todos)):hasta], "role": f.rol}
		if hasta < len(todos) {
			pag["next_cursor"] = strconv.Itoa(hasta)
		}
		di(pag)
	case clave == "POST tables/3" && x[2] == "records":
		reg := m{"id": fmt.Sprintf("n%d", f.n), "table_id": x[1], "rev": 1, "data": m{}, "links": m{}}
		f.registros[x[1]] = append(f.registros[x[1]], reg)
		di(reg)
	case clave == "GET tables/3" && x[2] == "views":
		di(append([]m{}, f.vistas[x[1]]...))
	case clave == "POST tables/3" && x[2] == "views":
		pedido["id"], pedido["table_id"] = fmt.Sprintf("v%d", f.n), x[1]
		f.vistas[x[1]] = append(f.vistas[x[1]], pedido)
		di(pedido)
	case clave == "DELETE views/2":
		for t := range f.vistas {
			f.vistas[t] = slices.DeleteFunc(f.vistas[t], func(v m) bool { return v["id"] == x[1] })
		}
		nada()
	case clave == "PUT records/4":
		reg, _ := f.registro(x[1])
		if enlaces, hay := pedido["links"]; hay {
			reg["links"].(m)[x[3]] = enlaces
		} else {
			reg["data"].(m)[x[3]] = pedido["value"]
		}
		reg["rev"] = reg["rev"].(int) + 1
		di(m{"record": reg, "conflict": f.conflicto, "replayed": false})
		f.conflicto = false
	case clave == "DELETE records/2":
		if f.referenciado && r.URL.Query().Get("force") != "true" {
			falla(http.StatusConflict, "referenced")
			return
		}
		_, t := f.registro(x[1])
		f.registros[t] = slices.DeleteFunc(f.registros[t], func(r m) bool { return r["id"] == x[1] })
		nada()
	case clave == "GET bases/3" && x[2] == "permissions":
		di(append([]m{}, f.permisos...))
	case clave == "POST bases/3" && x[2] == "permissions":
		if pedido["email"] == "nadie@acme.example" {
			falla(http.StatusUnprocessableEntity, "not_a_member")
			return
		}
		pedido["id"] = fmt.Sprintf("p%d", f.n)
		for _, campo := range []string{"email", "query"} { // el servicio resuelve a la persona
			if v, hay := pedido[campo]; hay {
				pedido["principal_id"], pedido["principal_label"] = "sub-"+fmt.Sprint(v), v
			}
		}
		f.permisos = append(f.permisos, pedido)
		di(pedido)
	case clave == "DELETE bases/4" && x[2] == "permissions":
		f.permisos = slices.DeleteFunc(f.permisos, func(p m) bool { return p["id"] == x[3] })
		nada()
	case clave == "POST bases/3" && x[2] == "transfer":
		f.rol = "editor"
		nada()
	case clave == "GET groups/1":
		di(m{"groups": []string{"ventas", "direccion"}})
	case clave == "GET views/3" && x[2] == "form-links":
		di(append([]m{}, f.enlaces...))
	case clave == "POST views/3" && x[2] == "form-links":
		e := m{"id": fmt.Sprintf("e%d", f.n), "view_id": x[1], "max_responses": pedido["max_responses"], "responses": 0, "created_at": "2026-10-05T09:00:00Z"}
		f.enlaces = append(f.enlaces, e)
		con := m{"token": "tok-secreto"}
		for k, v := range e {
			con[k] = v
		}
		di(con)
	case clave == "DELETE form-links/2":
		for _, e := range f.enlaces {
			if e["id"] == x[1] {
				e["revoked_at"] = "2026-10-05T10:00:00Z"
			}
		}
		nada()
	case clave == "GET tables/3" && x[2] == "export":
		_, _ = io.WriteString(w, "Nombre,Edad\nPersona 1,21\n")
	default:
		falla(http.StatusNotFound, "not_found")
	}
}

// abrir monta la app contra el servicio de pega y abre la base.
func abrir(t *testing.T, f *falso) (*Pantalla, fyne.Window) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	tr, err := Traductor("es")
	if err != nil {
		t.Fatal(err)
	}
	w := test.NewWindow(nil)
	t.Cleanup(w.Close)
	w.SetPadded(false)
	p := NuevaPantalla(tr, api.NewBases(srv.URL, nil, nil), w)
	p.fondo = func(g func()) { g() }
	w.SetContent(p.Objeto())
	w.Resize(fyne.NewSize(1280, 800))
	p.Abrir("b1")
	return p, w
}

// enCapas recorre lo que hay en los diálogos y menús abiertos.
func enCapas(w fyne.Window, mirar func(fyne.CanvasObject)) {
	for _, capa := range w.Canvas().Overlays().List() {
		recorrer(capa, mirar)
	}
}

// campoDeDialogo es el campo de texto de un diálogo abierto con ese nombre accesible.
func campoDeDialogo(t *testing.T, w fyne.Window, rotulo string) *ui.Entrada {
	t.Helper()
	var hallado *ui.Entrada
	enCapas(w, func(o fyne.CanvasObject) {
		switch e := o.(type) {
		case *ui.Entrada:
			if e.Rotulo == rotulo {
				hallado = e
			}
		case *ui.AreaTexto:
			if e.Rotulo == rotulo {
				hallado = &e.Entrada
			}
		}
	})
	if hallado == nil {
		t.Fatalf("no hay un campo «%s» en el diálogo", rotulo)
	}
	return hallado
}

// elegir elige una opción de un selector, por su texto: el de un diálogo abierto o, si
// no hay ninguno con ese nombre, el de la pantalla.
func elegir(t *testing.T, w fyne.Window, rotulo, opcion string) {
	t.Helper()
	var s *ui.Selector
	mirar := func(o fyne.CanvasObject) {
		if x, ok := o.(*ui.Selector); ok && x.Rotulo == rotulo {
			s = x
		}
	}
	enCapas(w, mirar)
	if s == nil {
		recorrer(w.Canvas().Content(), mirar)
	}
	if s == nil {
		t.Fatalf("no hay un selector «%s»", rotulo)
	}
	i := slices.Index(s.Opciones, opcion)
	if i < 0 {
		t.Fatalf("«%s» no ofrece «%s»: %v", rotulo, opcion, s.Opciones)
	}
	s.Elegida = i
	if s.AlElegir != nil {
		s.AlElegir(i)
	}
}

func pulsar(t *testing.T, w fyne.Window, texto string) {
	t.Helper()
	b := boton(w, texto)
	if b == nil {
		t.Fatalf("no hay un botón «%s»:\n%s", texto, textos(w))
	}
	if b.Inerte {
		t.Fatalf("el botón «%s» está apagado", texto)
	}
	test.Tap(b)
}

func tecla(g *rejilla, k fyne.KeyName) { g.TypedKey(&fyne.KeyEvent{Name: k}) }

// escribir teclea sobre la celda con el foco y confirma con Intro, como en una ventana.
func escribir(t *testing.T, g *rejilla, texto string) {
	t.Helper()
	for i, r := range texto {
		if i == 0 {
			g.TypedRune(r)
			if g.editor == nil {
				t.Fatalf("escribir %q no abre el editor de la celda %v", texto, g.foco)
			}
			continue
		}
		g.editor.TypedRune(r)
	}
	g.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
}

// TestLaRejillaCargaPorVentanas: se pide de cien en cien por cursor, nunca la tabla
// entera; en memoria quedan como mucho tres páginas y se puede volver a las anteriores.
func TestLaRejillaCargaPorVentanas(t *testing.T) {
	f := nuevoFalso(450)
	p, w := abrir(t, f)
	b := p.abierta
	capturar(t, w, "rejilla")
	if n := len(b.rejilla.Registros); n != 100 {
		t.Fatalf("la primera ventana trae %d registros, no 100", n)
	}
	if boton(w, "Cargar anteriores") != nil {
		t.Error("en la primera ventana no hay anteriores que cargar")
	}
	pulsar(t, w, "Cargar más")
	pulsar(t, w, "Cargar más")
	if n := len(b.rejilla.Registros); n != 300 || b.rejilla.Desde != 0 {
		t.Fatalf("tres ventanas: %d registros desde %d", n, b.rejilla.Desde)
	}
	pulsar(t, w, "Cargar más")
	if got := textos(w); !strings.Contains(got, "Registros 101 a 400") {
		t.Errorf("tras cargar otra ventana no se dice qué registros se ven:\n%s", got)
	}
	if n := len(b.rejilla.Registros); n != 300 || b.rejilla.Desde != 100 || b.rejilla.Registros[0].ID != "r101" {
		t.Fatalf("la cuarta suelta la primera: %d registros desde %d, el primero %s", n, b.rejilla.Desde, b.rejilla.Registros[0].ID)
	}
	pulsar(t, w, "Cargar más") // la última: 400 a 450
	if boton(w, "Cargar más") != nil {
		t.Error("sin más páginas no se ofrece «Cargar más»")
	}
	capturar(t, w, "rejilla-ultima-ventana")
	pulsar(t, w, "Cargar anteriores")
	if b.rejilla.Desde != 100 || b.rejilla.Registros[0].ID != "r101" || len(b.rejilla.Registros) != 300 {
		t.Fatalf("al cargar anteriores: desde %d, el primero %s, %d registros", b.rejilla.Desde, b.rejilla.Registros[0].ID, len(b.rejilla.Registros))
	}
	quiero := []string{
		"GET /tables/t1/records?limit=100", "GET /tables/t1/records?cursor=100&limit=100", "GET /tables/t1/records?cursor=200&limit=100",
		"GET /tables/t1/records?cursor=300&limit=100", "GET /tables/t1/records?cursor=400&limit=100", "GET /tables/t1/records?cursor=100&limit=100",
	}
	if got := f.peticiones("GET /tables/t1/records"); !slices.Equal(got, quiero) {
		t.Errorf("peticiones de registros:\n%q\nquiero:\n%q", got, quiero)
	}
}

// TestEditarCeldasSegunSuTipo: cada tipo de campo se edita a su manera y lo que viaja es
// una operación de celda con el valor de ese tipo y lo que se veía.
func TestEditarCeldasSegunSuTipo(t *testing.T) {
	f := nuevoFalso(3)
	p, w := abrir(t, f)
	g := p.abierta.rejilla
	w.Canvas().Focus(g)
	celdas := func() []string {
		vistas := f.peticiones("PUT /records/")
		out := make([]string, 0, len(vistas))
		for _, l := range vistas {
			out = append(out, strings.TrimPrefix(l, "PUT /records/"))
		}
		return out
	}
	ultima := func() string {
		c := celdas()
		if len(c) == 0 {
			return ""
		}
		return c[len(c)-1]
	}

	// Texto: escribir abre el editor con esa letra; Intro guarda y baja a la fila siguiente.
	escribir(t, g, "Ana")
	if got := ultima(); got != `r001/cells/nom?v=1 {"expected":"Persona 1","value":"Ana"}` {
		t.Fatalf("texto: %s", got)
	}
	if g.foco != (celda{1, 0}) || g.editando {
		t.Errorf("tras Intro el foco baja y el editor se cierra: %v, editando %v", g.foco, g.editando)
	}
	// Número: admite la coma decimal; lo que no es un número no se envía y el campo sigue abierto.
	tecla(g, fyne.KeyUp)
	tecla(g, fyne.KeyRight)
	escribir(t, g, "3,5")
	if got := ultima(); got != `r001/cells/eda?v=1 {"expected":21,"value":3.5}` {
		t.Fatalf("número: %s", got)
	}
	tecla(g, fyne.KeyUp)
	antes := len(celdas())
	escribir(t, g, "x")
	if len(celdas()) != antes || !g.editando || !g.editor.Invalida || g.Aviso != "Ese valor no encaja en este campo." {
		t.Fatalf("un número que no lo es no se envía y se marca: %d peticiones, editando %v, aviso %q", len(celdas())-antes, g.editando, g.Aviso)
	}
	g.editor.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if g.editando {
		t.Fatal("Escape no cancela la edición")
	}
	// Fecha: se teclea como se escribe y viaja en ISO.
	tecla(g, fyne.KeyRight)
	escribir(t, g, "05/10/2026")
	if got := ultima(); got != `r001/cells/alt?v=1 {"expected":null,"value":"2026-10-05"}` {
		t.Fatalf("fecha: %s", got)
	}
	// Casilla: Espacio la marca y la vuelve a desmarcar (null, no false).
	tecla(g, fyne.KeyUp)
	tecla(g, fyne.KeyRight)
	g.TypedRune(' ')
	if got := ultima(); got != `r001/cells/act?v=1 {"expected":null,"value":true}` {
		t.Fatalf("casilla: %s", got)
	}
	g.TypedRune(' ')
	if got := ultima(); got != `r001/cells/act?v=1 {"expected":true,"value":null}` {
		t.Fatalf("casilla al desmarcar: %s", got)
	}
	// Elección única: Intro despliega sus opciones y elegir una la guarda.
	tecla(g, fyne.KeyRight)
	tecla(g, fyne.KeyReturn)
	capturar(t, w, "rejilla-opciones")
	var plata *ui.ElementoMenu
	enCapas(w, func(o fyne.CanvasObject) {
		if e, ok := o.(*ui.ElementoMenu); ok && e.Texto == "Plata" {
			plata = e
		}
	})
	if plata == nil {
		t.Fatal("Intro sobre una elección única no despliega sus opciones")
	}
	test.Tap(plata)
	if got := ultima(); got != `r001/cells/niv?v=1 {"expected":null,"value":"Plata"}` {
		t.Fatalf("elección única: %s", got)
	}
	// Elección múltiple: un diálogo con una casilla por opción; se guarda en el orden de las opciones.
	tecla(g, fyne.KeyRight)
	tecla(g, fyne.KeyF2)
	capturar(t, w, "dialogo-varios")
	enCapas(w, func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Eleccion); ok && (c.Etiqueta == "nuevo" || c.Etiqueta == "vip") {
			test.Tap(c)
		}
	})
	pulsar(t, w, "Guardar")
	if got := ultima(); got != `r001/cells/eti?v=1 {"expected":null,"value":["vip","nuevo"]}` {
		t.Fatalf("elección múltiple: %s", got)
	}
	// Enlace: el mismo diálogo, con los registros de la otra tabla por su nombre.
	tecla(g, fyne.KeyRight)
	tecla(g, fyne.KeyReturn)
	capturar(t, w, "dialogo-enlace")
	enCapas(w, func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Eleccion); ok && c.Etiqueta == "Globex" {
			test.Tap(c)
		}
	})
	pulsar(t, w, "Guardar")
	if got := ultima(); got != `r001/cells/emp?v=1 {"expected":[],"links":["e2"]}` {
		t.Fatalf("enlace: %s", got)
	}
	if got := g.textoCelda(g.Registros[0], g.Campos[6]); got != "Globex" {
		t.Errorf("la celda de enlace enseña %q y no el nombre del registro enlazado", got)
	}
	// Fórmula: no se edita, y se dice.
	tecla(g, fyne.KeyRight)
	antes = len(celdas())
	tecla(g, fyne.KeyReturn)
	g.TypedRune('7')
	tecla(g, fyne.KeyDelete)
	if len(celdas()) != antes || g.editando || g.Aviso != "Este campo se calcula solo." {
		t.Fatalf("una fórmula no se edita: %d peticiones, editando %v, aviso %q", len(celdas())-antes, g.editando, g.Aviso)
	}
	// Ctrl+Fin y Ctrl+Inicio llegan como atajo, no como tecla: última y primera celda.
	g.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEnd, Modifier: fyne.KeyModifierControl})
	if g.foco != (celda{2, 7}) {
		t.Errorf("Ctrl+Fin no lleva a la última celda: %v", g.foco)
	}
	g.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyHome, Modifier: fyne.KeyModifierControl})
	if g.foco != (celda{0, 0}) {
		t.Errorf("Ctrl+Inicio no lleva a la primera celda: %v", g.foco)
	}
	// Supr vacía la celda (no un enlace).
	tecla(g, fyne.KeyHome)
	tecla(g, fyne.KeyDelete)
	if got := ultima(); got != `r001/cells/nom?v=1 {"expected":"Ana","value":null}` {
		t.Fatalf("Supr: %s", got)
	}
	capturar(t, w, "rejilla-editada")

	// Si otra persona había cambiado la celda, se guarda y se avisa.
	f.conflicto = true
	escribir(t, g, "Eva")
	if got := textos(w); !strings.Contains(got, "Otra persona había cambiado esta celda. Se ha guardado tu valor.") {
		t.Errorf("pisar el valor de otra persona no avisa:\n%s", got)
	}
}

// TestAnadirYBorrarRegistros: «Añadir registro» lo pone al final; borrar pregunta, y si
// otros registros enlazan con él, el propietario puede forzarlo con una segunda pregunta.
func TestAnadirYBorrarRegistros(t *testing.T) {
	f := nuevoFalso(2)
	p, w := abrir(t, f)
	g := p.abierta.rejilla
	pulsar(t, w, "Añadir registro")
	if !strings.Contains(textos(w), "Registro añadido") {
		t.Error("añadir un registro no lo dice")
	}
	if len(g.Registros) != 3 || !f.vio(`POST /tables/t1/records {"data":{}}`) {
		t.Fatalf("no se ha añadido el registro: %d", len(g.Registros))
	}
	pulsar(t, w, "Borrar la fila 2")
	if got := textos(w); !strings.Contains(got, "¿Borrar este registro?") || f.vio("DELETE /records/r002") {
		t.Fatalf("borrar un registro pregunta antes:\n%s", got)
	}
	f.referenciado = true
	pulsar(t, w, "Borrar")
	if got := textos(w); !strings.Contains(got, "Otros registros enlazan con este. ¿Borrarlo y quitar esos enlaces?") {
		t.Fatalf("un registro enlazado pide una segunda confirmación:\n%s", got)
	}
	pulsar(t, w, "Borrar")
	if !f.vio("DELETE /records/r002?force=true") || len(g.Registros) != 2 || g.Registros[1].ID == "r002" {
		t.Fatalf("no se ha forzado el borrado: %q", f.peticiones("DELETE"))
	}
}

// TestQuienSoloLeeNoEdita: sin permiso de escritura no hay botones de añadir ni de
// borrar, la rejilla lo dice y ninguna tecla escribe.
func TestQuienSoloLeeNoEdita(t *testing.T) {
	f := nuevoFalso(2)
	f.rol = "viewer"
	p, w := abrir(t, f)
	capturar(t, w, "rejilla-lector")
	g := p.abierta.rejilla
	w.Canvas().Focus(g)
	g.TypedRune('x')
	tecla(g, fyne.KeyReturn)
	tecla(g, fyne.KeyDelete)
	if g.editando || len(f.peticiones("PUT")) != 0 {
		t.Fatal("un lector ha podido editar")
	}
	got := textos(w)
	if !strings.Contains(got, "Puedes leer esta tabla pero no cambiarla.") {
		t.Errorf("la rejilla no dice que es de solo lectura:\n%s", got)
	}
	for _, no := range []string{"[Añadir registro]", "[Añadir campo]", "[Añadir vista]", "[Añadir tabla]", "[Renombrar tabla]", "[Borrar la fila 1]"} {
		if strings.Contains(got, no) {
			t.Errorf("un lector ve %s", no)
		}
	}
}

// TestLasVistasGuardadas: crear una vista manda su configuración, la elige y vuelve a
// pedir los registros CON ella (el filtro y el orden los ejecuta el servicio); borrarla
// vuelve a «todos los registros».
func TestLasVistasGuardadas(t *testing.T) {
	f := nuevoFalso(3)
	p, w := abrir(t, f)
	b := p.abierta
	pulsar(t, w, "Añadir vista")
	if c := boton(w, "Crear vista"); c == nil || !c.Inerte {
		t.Fatal("sin nombre, «Crear vista» tiene que estar apagado")
	}
	campoDeDialogo(t, w, "Nombre de la vista").PonerTexto("Mayores")
	elegir(t, w, "Campo por el que filtrar", "Edad")
	elegir(t, w, "Condición", "mayor que")
	if c := boton(w, "Crear vista"); !c.Inerte {
		t.Error("con un filtro sin valor, «Crear vista» tiene que seguir apagado")
	}
	campoDeDialogo(t, w, "Valor").PonerTexto("30")
	elegir(t, w, "Campo por el que ordenar", "Nombre")
	elegir(t, w, "Sentido", "Descendente")
	capturar(t, w, "dialogo-vista")
	pulsar(t, w, "Crear vista")
	if !f.vio(`POST /tables/t1/views {"config":{"filters":[{"field":"eda","op":"gt","value":30}],"sort":[{"field":"nom","dir":"desc"}]},"kind":"grid","name":"Mayores"}`) {
		t.Fatalf("la vista no se ha creado como se pidió: %q", f.peticiones("POST /tables/t1/views"))
	}
	vista := b.Vistas[0].ID
	if b.VistaID != vista || !f.vio("GET /tables/t1/records?limit=100&view="+vista) {
		t.Fatalf("la vista nueva no se ha elegido o no se han pedido sus registros: %q", f.peticiones("GET /tables/t1/records"))
	}
	if b.rejilla.Registros[0].ID != "r003" {
		t.Errorf("los registros no vienen en el orden de la vista: el primero es %s", b.rejilla.Registros[0].ID)
	}
	capturar(t, w, "rejilla-con-vista")
	if got := textos(w); !strings.Contains(got, "[Mayores · Rejilla]") || !strings.Contains(got, "[Borrar vista]") {
		t.Errorf("la vista elegida no sale en la barra:\n%s", got)
	}
	pulsar(t, w, "Borrar vista")
	pulsar(t, w, "Borrar")
	if !f.vio("DELETE /views/"+vista) || b.VistaID != "" || len(b.Vistas) != 0 || b.rejilla.Registros[0].ID != "r001" {
		t.Fatalf("al borrar la vista no se vuelve a todos los registros: vista %q, %d vistas", b.VistaID, len(b.Vistas))
	}

	// Una vista de formulario enseña los campos que pide, no la rejilla.
	pulsar(t, w, "Añadir vista")
	campoDeDialogo(t, w, "Nombre de la vista").PonerTexto("Alta de personas")
	elegir(t, w, "Tipo", "Formulario")
	enCapas(w, func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Eleccion); ok && (c.Etiqueta == "Nombre" || c.Etiqueta == "Edad") {
			test.Tap(c)
		}
	})
	pulsar(t, w, "Crear vista")
	if !f.vio(`POST /tables/t1/views {"config":{"fields":["nom","eda"]},"kind":"form","name":"Alta de personas"}`) {
		t.Fatalf("el formulario no se ha creado con sus campos: %q", f.peticiones("POST /tables/t1/views"))
	}
	capturar(t, w, "vista-formulario")
	if got := textos(w); !strings.Contains(got, "Esto es lo que rellenará quien tenga el enlace.") || !strings.Contains(got, "•  Edad (Número)") {
		t.Errorf("la vista de formulario no enseña sus campos:\n%s", got)
	}
}

// TestElPropietarioCambiaElEsquema: añadir, renombrar y borrar tablas, y crear campos de
// cada clase (con sus opciones, su tabla enlazada o su fórmula por ids).
func TestElPropietarioCambiaElEsquema(t *testing.T) {
	f := nuevoFalso(1)
	p, w := abrir(t, f)
	b := p.abierta

	pulsar(t, w, "Añadir campo")
	if c := boton(w, "Crear campo"); c == nil || !c.Inerte {
		t.Fatal("sin nombre, «Crear campo» tiene que estar apagado")
	}
	campoDeDialogo(t, w, "Nombre del campo").PonerTexto("Estado")
	elegir(t, w, "Tipo", "Elección única")
	if c := boton(w, "Crear campo"); !c.Inerte {
		t.Error("una elección sin opciones no se puede crear")
	}
	campoDeDialogo(t, w, "Opciones").PonerTexto("Abierto\n\n Cerrado ")
	capturar(t, w, "dialogo-campo")
	pulsar(t, w, "Crear campo")
	if !f.vio(`POST /tables/t1/fields {"name":"Estado","kind":"select","choices":["Abierto","Cerrado"]}`) {
		t.Fatalf("el campo de elección no se ha creado: %q", f.peticiones("POST /tables/t1/fields"))
	}
	if n := len(b.Campos); n != 9 {
		t.Fatalf("la rejilla no trae el campo nuevo: %d campos", n)
	}

	pulsar(t, w, "Añadir campo")
	campoDeDialogo(t, w, "Nombre del campo").PonerTexto("Triple")
	elegir(t, w, "Tipo", "Fórmula")
	campoDeDialogo(t, w, "Fórmula").PonerTexto("{Edad} * {Nada}")
	if got := textos(w); !strings.Contains(got, "Hay un campo entre llaves que no existe en esta tabla.") {
		t.Errorf("una fórmula con un campo que no existe no avisa:\n%s", got)
	}
	campoDeDialogo(t, w, "Fórmula").PonerTexto("{Edad} * 3")
	capturar(t, w, "dialogo-campo-formula")
	pulsar(t, w, "Crear campo")
	if !f.vio(`POST /tables/t1/fields {"name":"Triple","kind":"formula","formula":"{eda} * 3","result":"number"}`) {
		t.Fatalf("la fórmula no viaja por ids: %q", f.peticiones("POST /tables/t1/fields"))
	}

	pulsar(t, w, "Añadir campo")
	campoDeDialogo(t, w, "Nombre del campo").PonerTexto("Sede")
	elegir(t, w, "Tipo", "Enlace a otra tabla")
	pulsar(t, w, "Crear campo")
	if !f.vio(`POST /tables/t1/fields {"name":"Sede","kind":"link","link_table_id":"t2"}`) {
		t.Fatalf("el enlace no apunta a la otra tabla: %q", f.peticiones("POST /tables/t1/fields"))
	}

	pulsar(t, w, "Renombrar tabla")
	campoDeDialogo(t, w, "").PonerTexto("Gente")
	pulsar(t, w, "Guardar")
	if !f.vio(`PATCH /tables/t1 {"name":"Gente"}`) || !strings.Contains(textos(w), "<Gente>") {
		t.Fatalf("la tabla no se ha renombrado:\n%s", textos(w))
	}
	pulsar(t, w, "Añadir tabla")
	campoDeDialogo(t, w, "").PonerTexto("Pedidos")
	pulsar(t, w, "Crear tabla")
	if !f.vio(`POST /bases/b1/tables {"name":"Pedidos"}`) || b.tabla() == nil || b.tabla().Name != "Pedidos" {
		t.Fatalf("la tabla nueva no se ha creado o no se ha abierto: activa %q", b.Activa)
	}
	capturar(t, w, "tabla-nueva-sin-campos")
	if got := textos(w); !strings.Contains(got, "Esta tabla todavía no tiene campos.") {
		t.Errorf("una tabla sin campos lo dice:\n%s", got)
	}
	pulsar(t, w, "Borrar tabla")
	pulsar(t, w, "Borrar")
	if len(b.Tablas) != 2 || b.Activa != "t1" {
		t.Fatalf("al borrar la tabla no se vuelve a la primera: %d tablas, activa %q", len(b.Tablas), b.Activa)
	}
}

// TestElTableroMueveTarjetasConSuSelector: una vista de tablero reparte los registros en
// una columna por opción (y «Sin valor» para lo demás, también para un valor que ya no es
// una opción), y «Mover a» escribe ese campo en el registro.
func TestElTableroMueveTarjetasConSuSelector(t *testing.T) {
	f := nuevoFalso(4)
	for i, nivel := range []string{"Oro", "Plata", "Bronce"} {
		f.registros["t1"][i]["data"].(m)["niv"] = nivel
	}
	f.vistas["t1"] = []m{{"id": "vk", "table_id": "t1", "name": "Por nivel", "kind": "kanban", "config": m{"group_by": "niv"}}}
	p, w := abrir(t, f)
	pulsar(t, w, "Por nivel · Tablero")
	capturar(t, w, "tablero")
	if !f.vio("GET /tables/t1/records?limit=100&view=vk") {
		t.Fatalf("el tablero no pide los registros de su vista: %q", f.peticiones("GET /tables/t1/records"))
	}
	cuentas := func() []int {
		campo, hay := p.abierta.campoDelTablero()
		if !hay {
			t.Fatal("la vista no se pinta como tablero")
		}
		cols := columnasDeTablero(campo, p.abierta.vent.registros())
		out := make([]int, len(cols))
		for i, c := range cols {
			out[i] = len(c.registros)
		}
		return out
	}
	if got := cuentas(); !slices.Equal(got, []int{1, 1, 2}) {
		t.Fatalf("Oro, Plata y sin valor (con el que ya no es una opción): %v", got)
	}
	got := textos(w)
	for _, quiero := range []string{"Oro", "Plata", "Sin valor", "(2)", "Persona 3", "Edad: 22"} {
		if !strings.Contains(got, quiero) {
			t.Errorf("falta %q en el tablero:\n%s", quiero, got)
		}
	}
	if strings.Contains(got, "Intro o F2 edita una celda") {
		t.Error("con un tablero no se pinta la rejilla")
	}

	elegir(t, w, "Mover Persona 2 a", "Oro")
	if !f.vio(`PUT /records/r002/cells/niv?v=1 {"expected":"Plata","value":"Oro"}`) {
		t.Fatalf("mover una tarjeta no escribe el campo: %q", f.peticiones("PUT"))
	}
	if got := cuentas(); !slices.Equal(got, []int{2, 0, 2}) {
		t.Errorf("la tarjeta no ha cambiado de columna: %v", got)
	}
	capturar(t, w, "tablero-tarjeta-movida")
	elegir(t, w, "Mover Persona 1 a", "Sin valor")
	if !f.vio(`PUT /records/r001/cells/niv?v=1 {"expected":"Oro","value":null}`) {
		t.Fatalf("mover a «Sin valor» vacía el campo: %q", f.peticiones("PUT"))
	}
	antes := len(f.peticiones("PUT"))
	elegir(t, w, "Mover Persona 2 a", "Oro") // ya está ahí
	if len(f.peticiones("PUT")) != antes {
		t.Error("«mover» una tarjeta a su propia columna no escribe nada")
	}

	// Quien solo lee ve el tablero sin selectores.
	f.rol = "viewer"
	p.Abrir("b1")
	pulsar(t, w, "Por nivel · Tablero")
	recorrer(w.Canvas().Content(), func(o fyne.CanvasObject) {
		if _, ok := o.(*ui.Selector); ok {
			t.Error("un lector puede mover tarjetas")
		}
	})
	if !strings.Contains(textos(w), "Puedes leer esta tabla pero no cambiarla.") {
		t.Error("el tablero de un lector no dice que es de solo lectura")
	}
}
