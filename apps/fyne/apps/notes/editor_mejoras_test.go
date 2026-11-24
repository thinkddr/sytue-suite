// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// LOS GUARDIANES DE LO QUE EL EDITOR GANÓ EL 2026-11-24: «/» y «+», el formato que se
// escribe, los símbolos, buscar y reemplazar, el pie, la burbuja con copiar/cortar/pegar,
// la barra de tabla y el asa de cada bloque.

func editorNotas(t *testing.T, html string) *EditorNota {
	t.Helper()
	ed := NuevoEditorNota(trInterno(t), Leer(html), "")
	ed.AutoformatoEnLinea = true
	ed.conPie()
	return ed
}

func tecla(z *zonaEdicion, k fyne.KeyName) { z.TypedKey(&fyne.KeyEvent{Name: k}) }

// TestElMenuDeLaBarraFiltraSeRecorreEInserta: «/» abre el catálogo, lo escrito lo filtra
// (por el rótulo o por un alias), las flechas mueven la activa, Intro la aplica borrando
// «/consulta», Escape lo cierra dejando lo escrito y, sin nada que case, dice «Sin
// resultados» hasta que lo escrito ya no puede ser un comando.
//
// VISTO EN ROJO (2026-11-24): sin «Sin resultados», «/zz» cerraba el menú en silencio.
func TestElMenuDeLaBarraFiltraSeRecorreEInserta(t *testing.T) {
	ed := editorNotas(t, "")
	teclear(ed.zona, "/")
	if got := rotulos(ed); len(got) < 15 || ed.menu.panel.rotulo != ed.tr.T("editor.slashMenu", nil) {
		t.Fatalf("«/» ofrece %d: %v", len(got), got)
	}
	teclear(ed.zona, "tít")
	if got := rotulos(ed); len(got) != 3 || !strings.HasPrefix(got[0], ed.tr.T("editor.heading1", nil)) {
		t.Fatalf("«/tít» ofrece %v", got)
	}
	tecla(ed.zona, fyne.KeyDown)
	tecla(ed.zona, fyne.KeyReturn)
	if got := ed.HTML(); got != "<h2><br/></h2>" || ed.menu != nil {
		t.Fatalf("elegir «Título 2» deja %q (menú abierto: %v)", got, ed.menu != nil)
	}
	teclear(ed.zona, "Plan /zz")
	if !contieneTexto(ed.menu.panel, ed.tr.T("editor.slashNoResults", nil)) {
		t.Error("«/zz» no dice que no hay resultados")
	}
	teclear(ed.zona, "zzzzz")
	if ed.menu != nil {
		t.Error("una palabra entera tras «/» sigue con el menú abierto")
	}
	teclear(ed.zona, " /deci")
	tecla(ed.zona, fyne.KeyEscape)
	if ed.menu != nil || !strings.Contains(ed.HTML(), "/deci") {
		t.Fatalf("Escape deja %q y el menú abierto: %v", ed.HTML(), ed.menu != nil)
	}
	// Fecha de hoy: un `<time>` con la fecha en el idioma de la app.
	antes := ahoraEditor
	ahoraEditor = func() time.Time { return time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { ahoraEditor = antes })
	hoy := editorNotas(t, "")
	teclear(hoy.zona, "/today")
	tecla(hoy.zona, fyne.KeyReturn)
	if got := hoy.HTML(); !strings.Contains(got, `<time datetime="2026-11-24">24 nov 2026</time>`) || strings.Contains(got, "/today") {
		t.Fatalf("«/today» deja %q", got)
	}
}

// TestElCatalogoDeInsertarEsElMismo: «+» de la barra ofrece lo mismo que «/» en el mismo
// orden, filtra con su caja y Intro inserta donde está el cursor.
func TestElCatalogoDeInsertarEsElMismo(t *testing.T) {
	ed := editorNotas(t, "<p>Ojo</p>")
	w := test.NewWindow(ed)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 600))
	w.Canvas().Focus(ed.Zona())
	test.Tap(ed.botones["insertar"])
	var campo *ui.Entrada
	recorrerInterno(w.Canvas().Overlays().Top(), func(o fyne.CanvasObject) {
		if c, ok := o.(*ui.Entrada); ok {
			campo = c
		}
	})
	if campo == nil {
		t.Fatal("«+» no abre su panel con la caja de buscar")
	}
	campo.PonerTexto("aviso")
	campo.AlTeclear(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := ed.HTML(); got != `<div data-panel="aviso"><p>Ojo</p></div>` {
		t.Fatalf("«+» → «Panel de aviso» deja %q", got)
	}
}

// TestElFormatoQueSeEscribeSeDeshaceAlTexto: `**x**`, `*x*`, `_x_`, “ `x` “ y `~~x~~`
// se convierten al cerrarse; Ctrl/⌘+Z justo después devuelve el texto tal cual. Lo de
// bloque («## ») también. Nada de eso en un bloque de código ni en «snake_case_».
//
// VISTO EN ROJO (2026-11-24): con la conversión en el MISMO paso que la letra, deshacer
// borraba «## » entero en vez de devolverlo.
func TestElFormatoQueSeEscribeSeDeshaceAlTexto(t *testing.T) {
	for _, c := range []struct{ escrito, queda string }{
		{"**hola**", "<b>hola</b>"},
		{"*hola*", "<i>hola</i>"},
		{"_hola_", "<i>hola</i>"},
		{"`x := 1`", "<code>x := 1</code>"},
		{"~~viejo~~", "<s>viejo</s>"},
	} {
		ed := editorNotas(t, "")
		teclear(ed.zona, "di "+c.escrito)
		if got := ed.HTML(); got != "di "+c.queda {
			t.Errorf("%q deja %q", c.escrito, got)
		}
		teclear(ed.zona, "y")
		if got := ed.HTML(); !strings.HasSuffix(got, c.queda+"y") {
			t.Errorf("tras %q lo que sigue va con la marca: %q", c.escrito, got)
		}
		ed.zona.TypedShortcut(&fyne.ShortcutUndo{})
		ed.zona.TypedShortcut(&fyne.ShortcutUndo{})
		if got := ed.HTML(); got != "di "+c.escrito {
			t.Errorf("deshacer tras %q deja %q", c.escrito, got)
		}
	}
	for _, escrito := range []string{"2*3*", "snake_case_", "** a**", "a ~~ b~~"} {
		ed := editorNotas(t, "")
		teclear(ed.zona, escrito)
		if got := ed.HTML(); got != escrito {
			t.Errorf("%q no es una marca y deja %q", escrito, got)
		}
	}
	codigo := editorNotas(t, "<pre>x</pre>")
	teclear(codigo.zona, " **no**")
	if got := codigo.HTML(); got != "<pre>x **no**</pre>" {
		t.Errorf("en un bloque de código deja %q", got)
	}
	titulo := editorNotas(t, "")
	teclear(titulo.zona, "## ")
	if got := titulo.HTML(); got != "<h2><br/></h2>" {
		t.Fatalf("«## » deja %q", got)
	}
	titulo.zona.TypedShortcut(&fyne.ShortcutUndo{})
	if got := titulo.HTML(); got != "## " {
		t.Errorf("deshacer «## » deja %q", got)
	}
	// Sin AutoformatoEnLinea (el redactor de Mail) las marcas en línea se quedan.
	mail := NuevoEditorNota(trInterno(t), Leer(""), "")
	teclear(mail.zona, "**hola**")
	if got := mail.HTML(); got != "**hola**" {
		t.Errorf("sin autoformato en línea deja %q", got)
	}
}

// TestLosSimbolosSeEscribenYSeRecuerdan: el selector se recorre con las flechas, Intro
// escribe el símbolo, y los recientes guardan los últimos dieciséis sin repetir.
func TestLosSimbolosSeEscribenYSeRecuerdan(t *testing.T) {
	fyne.CurrentApp().Preferences().SetStringList(clavePrefSimbolos, nil)
	t.Cleanup(func() { fyne.CurrentApp().Preferences().SetStringList(clavePrefSimbolos, nil) })
	ed := editorNotas(t, "<p>a</p>")
	w := test.NewWindow(ed)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 700))
	w.Canvas().Focus(ed.Zona())
	test.Tap(ed.botones["simbolos"])
	s := ed.simbolos
	if s == nil || w.Canvas().Focused() != fyne.Focusable(s) {
		t.Fatal("el selector no se abre con el foco")
	}
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	s.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got := ed.HTML(); got != "<p>a→</p>" || w.Canvas().Focused() != ed.Zona() {
		t.Fatalf("elegir → deja %q (foco en %T)", got, w.Canvas().Focused())
	}
	if r := recientesSimbolos(); !slices.Equal(r, []string{"→"}) {
		t.Fatalf("los recientes son %v", r)
	}
	todos := slices.Concat(CategoriasSimbolos[1].Simbolos, CategoriasSimbolos[3].Simbolos)
	for _, x := range todos[:17] {
		ed.insertarSimbolo(x)
	}
	ed.insertarSimbolo(todos[5])
	r := recientesSimbolos()
	if len(r) != MaxRecientesSimbolos || r[0] != todos[5] || slices.Index(r[1:], todos[5]) >= 0 {
		t.Fatalf("los recientes no son los dieciséis últimos sin repetir: %v", r)
	}
	// Y salen arriba del selector.
	if secs := ed.seccionesSimbolos(); secs[0].titulo != ed.tr.T("editor.symbolsRecent", nil) {
		t.Errorf("el primer apartado es %q", secs[0].titulo)
	}
}

// TestBuscarYReemplazar: «Aa» distingue mayúsculas; «Reemplazar» cambia la actual y salta a
// la siguiente; «Reemplazar todo» cambia todas en UN paso de deshacer.
func TestBuscarYReemplazar(t *testing.T) {
	ed := editorNotas(t, "<p>Caja, caja y CAJA.</p>")
	ed.zona.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyH, Modifier: fyne.KeyModifierShortcutDefault})
	b := ed.buscador
	if b == nil || !b.conRepl {
		t.Fatal("Ctrl/⌘+H no abre buscar y reemplazar")
	}
	b.campo.PonerTexto("caja")
	if got := b.cuenta.Text; got != "1 de 3" || ed.coincidencias() != "3 coincidencias" {
		t.Fatalf("la cuenta es %q / %q", got, ed.coincidencias())
	}
	test.Tap(b.btMayus)
	if got := b.cuenta.Text; got != "1 de 1" || ed.Seleccionado() != "caja" {
		t.Fatalf("con «Aa» la cuenta es %q y la selección %q", got, ed.Seleccionado())
	}
	test.Tap(b.btMayus)
	b.reemplazo.PonerTexto("bolsa")
	ed.reemplazarActual()
	if got := ed.HTML(); got != "<p>bolsa, caja y CAJA.</p>" || ed.Seleccionado() != "caja" {
		t.Fatalf("reemplazar deja %q con %q seleccionado", got, ed.Seleccionado())
	}
	ed.reemplazarTodo()
	if got := ed.HTML(); got != "<p>bolsa, bolsa y bolsa.</p>" {
		t.Fatalf("reemplazar todo deja %q", got)
	}
	ed.zona.TypedShortcut(&fyne.ShortcutUndo{})
	if got := ed.HTML(); got != "<p>bolsa, caja y CAJA.</p>" {
		t.Errorf("reemplazar todo no es un solo paso de deshacer: %q", got)
	}
}

// TestElPieCuenta: palabras, caracteres y minutos de lectura de la nota, y de lo
// seleccionado si lo hay. Una nota vacía, cero.
func TestElPieCuenta(t *testing.T) {
	if p, c := cuentaDocumento(Leer(`<p>Hola, mundo — adiós.</p><div class="task-row"><input type="checkbox"/> <span>pan</span></div>`)); p != 4 || c != 23 {
		t.Errorf("cuenta %d palabras y %d caracteres", p, c)
	}
	if minutosLectura(0) != 0 || minutosLectura(1) != 1 || minutosLectura(200) != 1 || minutosLectura(201) != 2 {
		t.Error("los minutos de lectura no redondean hacia arriba")
	}
	ed := editorNotas(t, "<p>uno dos tres</p>")
	if got := ed.estado.texto; got != "3 palabras · 12 caracteres · 1 min de lectura" {
		t.Errorf("el pie dice %q", got)
	}
	ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 3}
	ed.repintar()
	if got := ed.estado.texto; got != "1 palabra · 3 caracteres · 1 min de lectura" {
		t.Errorf("con «uno» seleccionado el pie dice %q", got)
	}
	if NuevoEditorNota(trInterno(t), Leer(""), "").estado != nil {
		t.Error("el editor de Mail lleva pie")
	}
}

// TestLaBurbujaCopiaCortaYPega: con algo seleccionado la burbuja lleva Copiar, Cortar y
// Pegar; copiar y cortar dejan el texto en el portapapeles y pegar entra por el camino
// de Ctrl/⌘+V (el Markdown se convierte).
func TestLaBurbujaCopiaCortaYPega(t *testing.T) {
	ed := editorNotas(t, "<p>hola mundo</p>")
	ed.zona.FocusGained()
	ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 4}
	if !ed.conBurbuja() {
		t.Fatal("con selección no hay burbuja")
	}
	cb := fyne.CurrentApp().Clipboard()
	cb.SetContent("")
	botonDeBurbuja(t, ed, ed.tr.T("editor.copy", nil)).Tapped(&fyne.PointEvent{})
	if cb.Content() != "hola" || ed.HTML() != "<p>hola mundo</p>" {
		t.Fatalf("copiar deja %q en el portapapeles y %q en la nota", cb.Content(), ed.HTML())
	}
	botonDeBurbuja(t, ed, ed.tr.T("editor.cut", nil)).Tapped(&fyne.PointEvent{})
	if cb.Content() != "hola" || ed.HTML() != "<p> mundo</p>" {
		t.Fatalf("cortar deja %q en el portapapeles y %q en la nota", cb.Content(), ed.HTML())
	}
	cb.SetContent("- uno\n- dos")
	ed.ancla, ed.cur = Pos{0, 0}, Pos{0, 6}
	botonDeBurbuja(t, ed, ed.tr.T("editor.paste", nil)).Tapped(&fyne.PointEvent{})
	if got := ed.HTML(); got != "<ul><li>uno</li><li>dos</li></ul>" {
		t.Fatalf("pegar Markdown desde la burbuja deja %q", got)
	}
}

// TestLaBarraDeTablaAgrupadaYConTab: sale con el cursor en una celda; sus botones añaden y quitan filas y
// columnas, ponen la fila y la columna de cabecera y el fondo de la celda; Tab va de celda
// en celda y en la última añade una fila.
func TestLaBarraDeTablaAgrupadaYConTab(t *testing.T) {
	ed := editorNotas(t, `<table><tbody><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></tbody></table><p>fin</p>`)
	w := test.NewWindow(ed)
	t.Cleanup(w.Close)
	w.Resize(fyne.NewSize(1000, 600))
	ed.cur, ed.ancla = Pos{0, 1}, Pos{0, 1}
	ed.repintar()
	if !slices.Contains(ed.barras(), fyne.CanvasObject(ed.barraTabla)) {
		t.Fatal("en una celda no sale la barra de tabla")
	}
	test.Tap(ed.botones["cabeceraFila"])
	if got := ed.HTML(); !strings.HasPrefix(got, "<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td>c</td>") {
		t.Fatalf("la fila de cabecera deja %q", got)
	}
	test.Tap(ed.botones["cabeceraCol"])
	if got := ed.HTML(); !strings.Contains(got, `<tr><th scope="row">c</th><td>d</td></tr>`) {
		t.Fatalf("la columna de cabecera deja %q", got)
	}
	ed.Orden("fondoCelda", "verde")
	if got := ed.HTML(); !strings.Contains(got, `<th data-fondo="verde">a</th>`) {
		t.Fatalf("el fondo de la celda deja %q", got)
	}
	if d := Leer(ed.HTML()); !d.Fiel() {
		t.Fatalf("la tabla editada no es fiel: %s", d.HTML())
	}
	// Tab: a → b → c → d → (fila nueva).
	quiero := []string{"b", "c", "d"}
	for _, q := range quiero {
		tecla(ed.zona, fyne.KeyTab)
		if got := strings.TrimSpace(strings.ReplaceAll(ed.bloqueActual().Texto(), " ", " ")); got != q {
			t.Fatalf("Tab lleva a %q y tocaba %q", got, q)
		}
	}
	tecla(ed.zona, fyne.KeyTab)
	if c := ed.bloqueActual().Celda; c.Fila != 2 || c.Col != 0 || len(c.Tabla.Filas) != 3 {
		t.Fatalf("Tab en la última celda deja el cursor en %+v", c)
	}
	ed.zona.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	tecla(ed.zona, fyne.KeyTab)
	ed.zona.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	if got := ed.bloqueActual().Texto(); got != "d" {
		t.Fatalf("Mayús+Tab lleva a %q", got)
	}
	test.Tap(ed.botones["borrarFila"])
	test.Tap(ed.botones["borrarCol"])
	if got := ed.HTML(); strings.Count(got, "<tr>") != 2 || strings.Count(got, "</td>")+strings.Count(got, "</th>") != 2 {
		t.Fatalf("borrar fila y columna deja %q", got)
	}
	if l := ladoBotonEd(); ed.botones["filaDebajo"].MinSize().Width != l {
		t.Errorf("los botones de la tabla miden %v", ed.botones["filaDebajo"].MinSize())
	}
}

// TestLasAccionesDelBloque: subir, bajar, duplicar, eliminar y convertir, cada una un paso
// de deshacer; y lo rico se inserta con el catálogo.
func TestLasAccionesDelBloque(t *testing.T) {
	ed := editorNotas(t, "<p>uno</p><p>dos</p><p>tres</p>")
	ed.cur, ed.ancla = Pos{1, 0}, Pos{1, 0}
	hacer := func(rotulo string) {
		t.Helper()
		for _, o := range ed.opcionesDeBloque() {
			if o != nil && strings.HasPrefix(o.rotulo, ed.tr.T(rotulo, nil)) {
				o.hacer()
				return
			}
		}
		t.Fatalf("el menú del bloque no tiene %q", rotulo)
	}
	hacer("editor.moveUp")
	if got := ed.HTML(); got != "<p>dos</p><p>uno</p><p>tres</p>" {
		t.Fatalf("subir deja %q", got)
	}
	hacer("editor.moveDown")
	hacer("editor.moveDown")
	if got := ed.HTML(); got != "<p>uno</p><p>tres</p><p>dos</p>" {
		t.Fatalf("bajar dos veces deja %q", got)
	}
	hacer("editor.duplicate")
	hacer("editor.delete")
	if got := ed.HTML(); got != "<p>uno</p><p>tres</p><p>dos</p>" {
		t.Fatalf("duplicar y eliminar deja %q", got)
	}
	ed.zona.TypedShortcut(&fyne.ShortcutUndo{})
	if got := ed.HTML(); got != "<p>uno</p><p>tres</p><p>dos</p><p>dos</p>" {
		t.Fatalf("deshacer eliminar deja %q", got)
	}
	for id, quiero := range map[string]string{
		"decision":    `<ul data-tipo="decisiones"><li>uno</li></ul>`,
		"panel_exito": `<div data-panel="exito"><p>uno</p></div>`,
		"expandir":    `<details data-expandir=""><summary></summary><p>uno</p></details>`,
	} {
		ed := editorNotas(t, "<p>uno</p>")
		for _, c := range comandos {
			if c.id == id {
				ed.aplicarComando(c)
			}
		}
		if got := ed.HTML(); got != quiero {
			t.Errorf("%s deja %q", id, got)
		}
	}
	estado := editorNotas(t, "<p>Va </p>")
	estado.insertarEspecial("HECHO", Tramo{Estado: "verde"})
	if got := estado.HTML(); got != "<p>Va <span data-estado=\"verde\">HECHO</span> </p>" {
		t.Errorf("la etiqueta de estado deja %q", got)
	}
}
