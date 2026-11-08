// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaPrimeraLineaSaleDelCuerpo: los casos de `primeraLinea` en diario.test.ts. El
// resto conserva su formato, y lo que queda vacío se va con la línea.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin bajar a los hijos del nodo que contiene
// a `fin`, «<p><b>Hola<br>sigue</b></p>» se llevaba la negrita entera y «sigue» con ella.
func TestLaPrimeraLineaSaleDelCuerpo(t *testing.T) {
	for _, c := range []struct{ fuente, linea, resto string }{
		{"<p>Comprar pan</p>", "Comprar pan", ""},
		{"<p><b>Hola</b> mundo<br>segunda</p><p>Más</p>", "Hola mundo", "<p>segunda</p><p>Más</p>"},
		{"<h2>Plan</h2><ul><li><i>a</i></li></ul>", "Plan", "<ul><li><i>a</i></li></ul>"},
		{"<ul><li>uno</li><li>dos</li></ul>", "uno", "<ul><li>dos</li></ul>"},
		{"<p><b>Hola<br>sigue</b></p>", "Hola", "<p><b>sigue</b></p>"},
		{"<p><br></p>", "", "<p><br></p>"},
		{"suelto<br>otra", "suelto", "otra"},
	} {
		linea, resto := primeraLinea(c.fuente)
		if linea != c.linea || resto != c.resto {
			t.Errorf("primeraLinea(%q) = %q, %q; quiero %q, %q", c.fuente, linea, resto, c.linea, c.resto)
		}
	}
}

// TestConvertirNoPierdeNada: los casos de `convertirNota` en diario.test.ts.
//
// VISTO EN ROJO (2026-11-05, en el frontal): con `r[:MaxTituloTarea]` el título largo
// medía 401 runas con su «…», una más de lo que admite el servicio.
func TestConvertirNoPierdeNada(t *testing.T) {
	igual := func(que string, got, quiero conversion) {
		t.Helper()
		if got != quiero {
			t.Errorf("%s: %+v, quiero %+v", que, got, quiero)
		}
	}
	igual("texto a tarea", convertirNota("text", "", "<p>Llamar a Ana<br>al móvil</p>"),
		conversion{tipo: "todo", titulo: "Llamar a Ana", contenido: "<p>al móvil</p>"})
	igual("con título, el título manda", convertirNota("text", " Plan ", "<p>x</p>"),
		conversion{tipo: "todo", titulo: "Plan", contenido: "<p>x</p>"})
	larga := "<p>" + strings.Repeat("a", 450) + "</p>"
	c := convertirNota("text", "", larga)
	if r := []rune(c.titulo); len(r) != MaxTituloTarea || r[len(r)-1] != '…' || c.contenido != larga {
		t.Errorf("una línea que no cabe: título de %d runas, contenido %q", len(r), c.contenido)
	}
	igual("tarea sin cuerpo", convertirNota("todo", `Pan <y> "leche"`, "<p><br></p>"),
		conversion{tipo: "text", contenido: "<p>Pan &lt;y&gt; &quot;leche&quot;</p>", desmarcar: true})
	igual("tarea con cuerpo", convertirNota("todo", "Viaje", "<p>Lisboa</p>"),
		conversion{tipo: "text", titulo: "Viaje", contenido: "<p>Lisboa</p>", desmarcar: true})
	ida := convertirNota("text", "", "<p>Comprar pan</p>")
	if vuelta := convertirNota(ida.tipo, ida.titulo, ida.contenido); vuelta.tipo != "text" || vuelta.titulo != "" || vuelta.contenido != "<p>Comprar pan</p>" {
		t.Errorf("ida y vuelta de una línea suelta: %+v", vuelta)
	}
}

// diarioConServicio es un cuaderno en lista con sus notas en una ventana, contra el
// servicio falso: las filas del diario.
func diarioConServicio(t *testing.T, respuestas map[string]string, notas ...api.Note) (*Pantalla, *servicioFalso) {
	t.Helper()
	p, falso := pantallaConServicio(t, respuestas, notas...)
	p.activo.Vista, p.cuadernos[0].Vista = Lista, Lista
	w := test.NewWindow(p.Objeto())
	t.Cleanup(w.Close)
	p.win = w
	w.Resize(fyne.NewSize(1280, 800))
	p.Repintar()
	return p, falso
}

// rayaDe es la raya pulsable de la fila de esa nota, o nil.
func rayaDe(p *Pantalla, id string) *Opcion {
	var o *Opcion
	if t := p.lienzo.vivas[id]; t != nil {
		cadaObjeto(t, func(x fyne.CanvasObject) {
			if op, ok := x.(*Opcion); ok && strings.HasPrefix(op.Rotulo, p.tr.T("note.toTodo", nil)) {
				o = op
			}
		})
	}
	return o
}

// TestPulsarLaRayaLaHaceTarea: la raya de un texto es un botón de 24 dentro de la columna
// de 17,5, que no se mueve; al pasar por encima enseña la casilla vacía, y pulsarlo la
// hace tarea con su primera línea de título, se pinta antes de que conteste el servicio
// y se deshace entera.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `case t.convertible()` en
// glifosCabecera la fila seguía con su raya de dibujo y no había qué pulsar; y sin
// `vuelta.Tipo` en editarCon, deshacer devolvía el texto a una tarea sin título.
func TestPulsarLaRayaLaHaceTarea(t *testing.T) {
	contenido := "<p>Llamar a Ana<br>al móvil</p>"
	p, falso := diarioConServicio(t, map[string]string{
		"PUT /notes/n": `{"id":"n","notebook_id":"c1","kind":"todo","title":"Llamar a Ana","content":"<p>al móvil</p>"}`,
	}, api.Note{ID: "n", NotebookID: "c1", Kind: "text", Content: contenido})
	o := rayaDe(p, "n")
	if o == nil {
		t.Fatal("la raya de un texto no se puede pulsar")
	}
	if got := o.AccessibilityLabel(); got != "Convertir en tarea: Llamar a Ana al móvil" {
		t.Errorf("la raya se anuncia como %q", got)
	}
	if m := o.MinSize(); m.Width != LadoRayaPulsable || m.Height != LadoRayaPulsable {
		t.Errorf("el botón mide %v", m)
	}
	celda := (&celdaGlifo{lado: ColumnaGlifoFila}).MinSize(nil)
	if o.Position().X != (celda.Width-LadoRayaPulsable)/2 {
		t.Errorf("el botón no está centrado en la columna: x=%v", o.Position().X)
	}
	casilla := func() bool {
		visible := false
		cadaObjeto(o.Contenido, func(x fyne.CanvasObject) {
			if r, ok := x.(*canvas.Rectangle); ok && r.StrokeWidth == BordeCasilla && r.Visible() {
				visible = true
			}
		})
		return visible
	}
	if casilla() {
		t.Error("en reposo se ve la casilla, y tiene que verse la raya")
	}
	o.MouseIn(&desktop.MouseEvent{})
	if !casilla() || !p.lienzo.vivas["n"].sobreRaya {
		t.Error("al pasar por encima no sale la casilla en la que se va a convertir")
	}
	o.MouseOut()

	o.Tapped(&fyne.PointEvent{})
	esperar(t, func() bool { n, _ := p.nota("n"); return n.Tipo == "todo" && n.Crudo.Title == "Llamar a Ana" })
	c := falso.ultima(t, "PUT /notes/n")
	if c["kind"] != "todo" || c["title"] != "Llamar a Ana" || c["content"] != "<p>al móvil</p>" {
		t.Errorf("la conversión viaja como %v", c)
	}
	p.deshacer()
	c = falso.ultima(t, "PUT /notes/n")
	if c["kind"] != "text" || c["title"] != "" || c["content"] != contenido {
		t.Errorf("deshacer manda %v", c)
	}
}

// TestUnaConversionQueFallaVuelveEntera: si el servicio dice que no, la nota vuelve a
// ser lo que era, con su tipo.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `p.notas = antes` en el fallo de
// editarCon, la tarea se quedaba pintada como un texto que nadie había guardado.
func TestUnaConversionQueFallaVuelveEntera(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"no"}`, http.StatusConflict)
	}))
	defer srv.Close()
	p, _ := pantallaConServicio(t, nil, api.Note{ID: "n", NotebookID: "c1", Kind: "todo", Title: "Pan", Checked: true})
	p.svc = api.NewNotes(srv.URL, nil, nil)
	p.convertir("n")
	esperar(t, func() bool { return len(p.avisos.Avisos()) > 0 })
	if n, _ := p.nota("n"); n.Tipo != "todo" || n.Titulo != "Pan" || !n.Marcada {
		t.Errorf("tras el fallo la nota es %+v", n)
	}
}

// TestElMenuConvierteEnLosDosSentidos: «Convertir en tarea» en un texto y «Convertir en
// texto» en una tarea, que deja de estar hecha; la fila del árbol no los tiene.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `!enArbol` en menuNota, la fila del
// árbol ofrecía convertir, y en la web (NoteTree.tsx) no está.
func TestElMenuConvierteEnLosDosSentidos(t *testing.T) {
	p, falso := diarioConServicio(t, map[string]string{"PUT /notes/b": `{"id":"b","notebook_id":"c1","kind":"text"}`},
		api.Note{ID: "a", NotebookID: "c1", Kind: "text", Content: "<p>Comprar pan</p>"},
		api.Note{ID: "b", NotebookID: "c1", Kind: "todo", Title: "Comprar pan", Checked: true})
	entradas := func(id string, enArbol bool) map[string]*ui.ElementoMenu {
		n, _ := p.nota(id)
		out := map[string]*ui.ElementoMenu{}
		for _, e := range p.menuNota(n, enArbol).Elementos {
			if it, ok := e.(*ui.ElementoMenu); ok {
				out[it.Texto] = it
			}
		}
		return out
	}
	aTarea, aTexto := p.tr.T("note.toTodo", nil), p.tr.T("note.toText", nil)
	if m := entradas("a", false); m[aTarea] == nil || m[aTexto] != nil {
		t.Error("el «…» de un texto no ofrece convertirlo en tarea")
	}
	if m := entradas("a", true); m[aTarea] != nil {
		t.Error("la fila del árbol ofrece convertir")
	}
	m := entradas("b", false)
	if m[aTexto] == nil || m[aTarea] != nil {
		t.Fatal("el «…» de una tarea no ofrece convertirla en texto")
	}
	m[aTexto].AlPulsar()
	esperar(t, func() bool { n, _ := p.nota("b"); return n.Tipo == "text" })
	c := falso.ultima(t, "PUT /notes/b")
	if c["kind"] != "text" || c["title"] != "" || c["content"] != "<p>Comprar pan</p>" || c["checked"] != false || c["cancelled"] != false {
		t.Errorf("tarea a texto viaja como %v", c)
	}
}

// TestLaRayaNoConvierteLoQueNoSeEditaEntero: ni en solo lectura, ni en lo compartido
// conmigo (su tipo no es suyo), ni un separador, ni una tarea, que ya tiene su casilla.
//
// VISTO EN ROJO (2026-11-05, en el frontal): sin `!t.SinMenu` en `convertible`, la nota
// compartida conmigo tenía raya pulsable y el servicio ignoraba el cambio de tipo.
func TestLaRayaNoConvierteLoQueNoSeEditaEntero(t *testing.T) {
	notas := []api.Note{
		{ID: "a", NotebookID: "c1", Kind: "text", Content: "<p>Ajena</p>"},
		{ID: "hr", NotebookID: "c1", Kind: "text", Content: "<hr>"},
		{ID: "t", NotebookID: "c1", Kind: "todo", Title: "Hecha"},
	}
	p, _ := diarioConServicio(t, nil, notas...)
	if rayaDe(p, "a") == nil {
		t.Fatal("la prueba no parte de una raya pulsable")
	}
	for _, id := range []string{"hr", "t"} {
		if rayaDe(p, id) != nil {
			t.Errorf("la fila %q tiene raya pulsable", id)
		}
	}
	if n, _ := p.nota("hr"); p.puedeConvertir(n) {
		t.Error("el «…» de un separador ofrece convertirlo")
	}
	p.activo.Crudo.ReadOnly = true
	p.Repintar()
	if rayaDe(p, "a") != nil {
		t.Error("en solo lectura la raya se puede pulsar")
	}
	p.activo.Crudo.ReadOnly = false
	p.Repintar()
	p.lienzo.SinMenu = true
	tarjeta := p.lienzo.tarjeta(p.notas[0])
	tarjeta.AlConvertir = func() {}
	if tarjeta.convertible() {
		t.Error("lo compartido conmigo tiene raya pulsable")
	}
}

// cadaObjeto recorre `o` y todo lo que pinta.
func cadaObjeto(o fyne.CanvasObject, f func(fyne.CanvasObject)) {
	f(o)
	switch x := o.(type) {
	case *fyne.Container:
		for _, h := range x.Objects {
			cadaObjeto(h, f)
		}
	case fyne.Widget:
		for _, h := range test.WidgetRenderer(x).Objects() {
			cadaObjeto(h, f)
		}
	}
}
