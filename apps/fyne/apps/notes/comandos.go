// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Los comandos de «/» (client/comandos.ts de la web, con la misma lista en el mismo
// orden): al escribir una barra al principio de una línea o tras un blanco sale un menú
// con los bloques que se pueden poner ahí, y lo que se sigue escribiendo lo filtra. El
// texto se queda en la nota mientras tanto; si no se elige nada, «/» es una barra.
//
// El menú NO es una capa: una capa se queda con el teclado, y aquí hay que seguir
// escribiendo en el cuerpo para filtrar (ver `flotante`).

// comando es una entrada del menú: su orden, su rótulo y los alias que valen en
// cualquier idioma («/h1», «/todo», «/---»).
type comando struct {
	id, clave string
	alias     []string
	glifo     string
	icono     iconos.Icono
}

var comandos = []comando{
	{id: "p", clave: "editor.paragraph", alias: []string{"p", "text"}, icono: iconos.Texto},
	{id: "h1", clave: "editor.h1", alias: []string{"h1", "#"}, glifo: TituloD},
	{id: "h2", clave: "editor.h2", alias: []string{"h2", "##"}, glifo: TituloD},
	{id: "h3", clave: "editor.h3", alias: []string{"h3", "###"}, glifo: TituloD},
	{id: "tarea", clave: "note.newTodo", alias: []string{"todo", "task", "[]"}, glifo: TareaD},
	{id: "vinetas", clave: "editor.bulletList", alias: []string{"ul", "list", "-"}, glifo: VinetasD},
	{id: "numeros", clave: "editor.numberedList", alias: []string{"ol", "1."}, glifo: NumerosD},
	{id: "cita", clave: "editor.quote", alias: []string{"quote", ">"}, glifo: CitaD},
	{id: "codigo", clave: "editor.codeBlock", alias: []string{"code", "```"}, icono: iconos.Codigo},
	{id: "regla", clave: "editor.hr", alias: []string{"hr", "divider", "---"}, icono: iconos.Menos},
	{id: "tabla", clave: "editor.table", alias: []string{"table"}, glifo: TablaD},
}

// filasMenuCmd son las filas que se ven del menú a la vez.
const filasMenuCmd = 8

// maxConsultaBarra es el tope de lo que se escribe tras la barra: más ya no es un comando.
const maxConsultaBarra = 20

var reBarra = regexp.MustCompile(`(?:^|\s)/([^\s/]*)$`)

// consultaDeBarra es `consultaDeBarra` de la web: si el texto antes del cursor acaba en
// una barra que abre el menú, lo escrito tras ella (quizá nada).
func consultaDeBarra(antes string) (string, bool) {
	m := reBarra.FindStringSubmatch(strings.ReplaceAll(antes, "\u00a0", " "))
	if len(m) < 2 || len([]rune(m[1])) > maxConsultaBarra {
		return "", false
	}
	return m[1], true
}

// filtrarComandos son los que deja la consulta: por el rótulo en el idioma de la app o
// por el principio de un alias, con la regla de búsqueda de la suite.
func filtrarComandos(q string, rotulo func(comando) string) []comando {
	q = strings.TrimSpace(q)
	if q == "" {
		return comandos
	}
	pq := buscar.Plegar(q)
	var out []comando
	for _, c := range comandos {
		ok := buscar.Contiene(rotulo(c), q)
		for _, a := range c.alias {
			ok = ok || strings.HasPrefix(buscar.Plegar(a), pq)
		}
		if ok {
			out = append(out, c)
		}
	}
	return out
}

// menuComandos es el menú abierto: dónde está su barra, qué se ha escrito y cuál de las
// filas está activa.
type menuComandos struct {
	inicio Pos
	q      string
	idx    int
	panel  *flotante
}

func (e *EditorNota) rotuloComando(c comando) string { return e.tr.T(c.clave, nil) }

// barraEnCursor es la barra de «/» que hay justo antes del cursor, con su consulta.
func (e *EditorNota) barraEnCursor() (Pos, string, bool) {
	if e.haySeleccion() || len(e.doc.Bloques) == 0 {
		return Pos{}, "", false
	}
	b := e.bloqueActual()
	if b.Tipo == Codigo || !editable(b) {
		return Pos{}, "", false
	}
	r := []rune(b.Texto())
	o := min(e.cur.O, len(r))
	q, ok := consultaDeBarra(string(r[:o]))
	inicio := o - len([]rune(q)) - 1
	if !ok || inicio < 0 || r[inicio] != '/' {
		return Pos{}, "", false
	}
	return Pos{e.cur.B, inicio}, q, true
}

// mirarMenu abre, filtra o cierra el menú tras cada cambio o movimiento del cursor.
func (e *EditorNota) mirarMenu() {
	inicio, q, ok := e.barraEnCursor()
	if !ok {
		e.descartada = nil
	}
	if !ok || (e.descartada != nil && *e.descartada == inicio) ||
		len(filtrarComandos(q, e.rotuloComando)) == 0 {
		if e.menu != nil {
			e.menu = nil
			e.Refresh()
		}
		return
	}
	idx := 0
	if e.menu != nil && e.menu.q == q && e.menu.inicio == inicio {
		idx = e.menu.idx
	}
	e.menu = &menuComandos{inicio: inicio, q: q, idx: idx}
	e.armarMenu()
	e.Refresh()
}

// armarMenu rehace las filas del menú con la activa marcada: `w-60 p-1`, filas `px-2.5
// py-1.5 text-sm gap-2` con el dibujo de 15 en su caja de 20 y el alias en `sytue-meta`.
func (e *EditorNota) armarMenu() {
	m := e.menu
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	opciones := filtrarComandos(m.q, e.rotuloComando)
	m.idx = min(m.idx, len(opciones)-1)
	// Como mucho ocho filas a la vista (el `max-h-72` de la web), con la activa dentro.
	desde := max(0, min(m.idx-filasMenuCmd/2, len(opciones)-filasMenuCmd))
	hasta := min(len(opciones), desde+filasMenuCmd)
	filas := make([]fyne.CanvasObject, 0, hasta-desde)
	for i := desde; i < hasta; i++ {
		filas = append(filas, nuevaFilaComando(e, opciones[i], i, i == m.idx, v))
	}
	m.panel = nuevoFlotante(container.New(ui.Columna{}, filas...), tema.Espacio(1))
}

// teclaMenu atiende las flechas, Intro, Tab y Escape mientras el menú está abierto.
func (e *EditorNota) teclaMenu(k fyne.KeyName) bool {
	if e.menu == nil {
		return false
	}
	opciones := filtrarComandos(e.menu.q, e.rotuloComando)
	n := len(opciones)
	switch k {
	case fyne.KeyDown, fyne.KeyUp:
		paso := 1
		if k == fyne.KeyUp {
			paso = -1
		}
		e.menu.idx = (e.menu.idx + paso + n) % n
		e.armarMenu()
		e.Refresh()
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyTab:
		e.ejecutarComando(opciones[min(e.menu.idx, n-1)])
	case fyne.KeyEscape:
		inicio := e.menu.inicio
		e.descartada = &inicio
		e.menu = nil
		e.Refresh()
	default:
		return false
	}
	return true
}

// ejecutarComando borra «/consulta» y aplica la orden de su botón de la barra.
func (e *EditorNota) ejecutarComando(c comando) {
	inicio, _, ok := e.barraEnCursor()
	e.menu = nil
	if !ok {
		e.Refresh()
		return
	}
	e.hacer(func() {
		e.cur = e.doc.Borrar(inicio, e.cur)
		e.ancla = e.cur
	})
	switch c.id {
	case "p", "h1", "h2", "h3":
		e.Orden("bloque", c.id)
	case "cita":
		e.Orden("bloque", "blockquote")
	case "codigo":
		e.Orden("bloque", "pre")
	default: // tarea, vinetas, numeros, regla y tabla se llaman igual que su orden
		e.Orden(c.id, "")
	}
	e.Refresh()
}

// filaComando es una fila del menú. Se pulsa y se resalta al pasar por encima, pero NO
// toma el foco: el teclado sigue en el cuerpo, que es donde se filtra.
type filaComando struct {
	widget.BaseWidget
	e      *EditorNota
	c      comando
	i      int
	activa bool
	v      tema.Variante
}

var (
	_ fyne.Tappable     = (*filaComando)(nil)
	_ desktop.Hoverable = (*filaComando)(nil)
)

func nuevaFilaComando(e *EditorNota, c comando, i int, activa bool, v tema.Variante) *filaComando {
	f := &filaComando{e: e, c: c, i: i, activa: activa, v: v}
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaComando) AccessibilityLabel() string { return f.e.rotuloComando(f.c) }
func (f *filaComando) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

func (f *filaComando) Tapped(*fyne.PointEvent) { f.e.ejecutarComando(f.c) }

func (f *filaComando) MouseIn(*desktop.MouseEvent) {
	if f.e.menu != nil && f.e.menu.idx != f.i {
		f.e.menu.idx = f.i
		f.e.armarMenu()
		f.e.Refresh()
	}
}
func (f *filaComando) MouseMoved(*desktop.MouseEvent) {}
func (f *filaComando) MouseOut()                      {}

// Medidas de la fila (`px-2.5 py-1.5 gap-2`, caja del dibujo `h-5 w-5`, `w-60`).
var (
	pxFilaCmd    = tema.Espacio(2.5)
	pyFilaCmd    = tema.Espacio(1.5)
	huecoFilaCmd = tema.Espacio(2)
	cajaDibCmd   = tema.Espacio(5)
	anchoMenuCmd = tema.Espacio(60) - 2*(tema.Espacio(1)+tema.Borde)
)

func (f *filaComando) CreateRenderer() fyne.WidgetRenderer {
	fondo := canvas.NewRectangle(color.NRGBA{})
	fondo.CornerRadius = tema.RadioMD
	if f.activa {
		fondo.FillColor = tema.Color("menu-selected", f.v)
	}
	var dib fyne.CanvasObject
	if f.c.glifo != "" {
		dib = Glifo(f.c.glifo, 15, "text-muted", f.v)
	} else {
		dib = Icono(f.c.icono, 15, "text-muted", f.v)
	}
	texto := ui.Texto(f.e.rotuloComando(f.c), tema.TextoSM, ui.Normal, tema.Color("text", f.v))
	alias := ui.Texto("/"+f.c.alias[0], tema.TextoXS, ui.Normal, tema.Color("text-subtle", f.v))
	return &filaComandoRenderer{f: f, fondo: fondo, dib: dib, texto: texto, alias: alias}
}

type filaComandoRenderer struct {
	f            *filaComando
	fondo        *canvas.Rectangle
	dib          fyne.CanvasObject
	texto, alias *canvas.Text
}

func (r *filaComandoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(anchoMenuCmd, max(tema.TextoSM.Linea, cajaDibCmd)+2*pyFilaCmd)
}

func (r *filaComandoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.dib, r.texto, r.alias}
}
func (r *filaComandoRenderer) Destroy() {}
func (r *filaComandoRenderer) Refresh() {}

func (r *filaComandoRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	r.dib.Move(fyne.NewPos(pxFilaCmd+(cajaDibCmd-15)/2, (t.Height-15)/2))
	r.dib.Resize(fyne.NewSize(15, 15))
	anchoAlias := ui.Medir(r.alias.Text, tema.TextoXS, ui.Normal).Width
	ui.ColocarTexto(r.alias, tema.TextoXS, t.Width-pxFilaCmd-anchoAlias, (t.Height-tema.TextoXS.Linea)/2)
	x := pxFilaCmd + cajaDibCmd + huecoFilaCmd
	ui.Truncar(r.texto, r.f.e.rotuloComando(r.f.c), tema.TextoSM, ui.Normal, t.Width-x-anchoAlias-huecoFilaCmd-pxFilaCmd)
	ui.ColocarTexto(r.texto, tema.TextoSM, x, (t.Height-tema.TextoSM.Linea)/2)
}
