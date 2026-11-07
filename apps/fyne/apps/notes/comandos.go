// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"cmp"
	"image/color"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
)

// Los comandos de «/» (client/comandos.ts de la web, con la misma lista en el mismo
// orden): al escribir una barra al principio de una línea o tras un blanco sale un menú
// con los bloques que se pueden poner ahí, y lo que se sigue escribiendo lo filtra. El
// texto se queda en la nota mientras tanto; si no se elige nada, «/» es una barra.
//
// Y sus dos vecinos, que abren el MISMO menú con otra cosa (RichText.tsx): «[[», una nota
// que enlazar —con la búsqueda de la herramienta de enlaces—, y «@», alguien de la
// organización a quien mencionar (las reglas de `ui`, menciones.go). Las tres se llevan
// con el mismo teclado y se colocan igual.
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

// maxConsultaEnlace es el tope de lo que se escribe tras `[[`: el nombre de una nota
// cabe; más largo ya no es buscarla.
const maxConsultaEnlace = 60

var reEnlace = regexp.MustCompile(`\[\[([^\[\]\n]*)$`)

// consultaDeEnlace es `consultaDeEnlace` de la web: si el texto antes del cursor acaba en
// un `[[` sin cerrar, lo escrito tras los corchetes (quizá nada). Vale en cualquier sitio
// de la línea —«ver [[Presupuesto»— y la consulta admite espacios, que es como se llaman
// las notas. Un corchete la cierra: `[[x]]` ya no está buscando nada.
func consultaDeEnlace(antes string) (string, bool) {
	m := reEnlace.FindStringSubmatch(strings.ReplaceAll(antes, "\u00a0", " "))
	if len(m) < 2 || len([]rune(m[1])) > maxConsultaEnlace {
		return "", false
	}
	return m[1], true
}

// tipoMenu es cuál de los tres menús que se abren al teclear: un bloque, una nota que
// enlazar, alguien a quien mencionar.
type tipoMenu int

const (
	menuCmd tipoMenu = iota
	menuNota
	menuPersona
)

// disparos es lo que abre cada menú y cómo se lee lo escrito tras ello (`DISPAROS` de
// RichText.tsx). `[[` va primero: el nombre de una nota puede llevar « /», y mientras no
// se cierre se está buscando una nota.
var disparos = []struct {
	tipo     tipoMenu
	marca    string
	consulta func(string) (string, bool)
}{
	{menuNota, "[[", consultaDeEnlace},
	{menuPersona, "@", ui.ConsultaMencion},
	{menuCmd, "/", consultaDeBarra},
}

// rotulosMenu son los nombres de las tres listas (`ROTULO_MENU`).
var rotulosMenu = map[tipoMenu]string{menuCmd: "slash.title", menuNota: "editor.linkNote", menuPersona: "editor.mention"}

// menuComandos es el menú abierto: de qué tipo es, dónde está su disparador, qué se ha
// escrito tras él y cuál de las filas está activa.
type menuComandos struct {
	tipo   tipoMenu
	inicio Pos
	q      string
	idx    int
	panel  *flotante
}

// sugerencia es una fila de cualquiera de los tres menús (`Sugerencia` de RichText.tsx).
type sugerencia struct {
	glifo string
	icono iconos.Icono
	// rotulo es lo que se lee; dentro, el título dentro de la nota, tras «›»; detalle, a
	// la derecha y en gris: el atajo del comando, el cuaderno, el correo.
	rotulo, dentro, detalle string
	// grupo es al que pertenece (este cuaderno, otros cuadernos).
	grupo  string
	elegir func()
}

func (e *EditorNota) rotuloComando(c comando) string { return e.tr.T(c.clave, nil) }

// disparo es el disparador que hay justo antes del cursor: qué abre, dónde empieza y lo
// escrito tras él.
type disparo struct {
	tipo   tipoMenu
	inicio Pos
	q      string
}

// disparoEnCursor es el disparador («/», «[[» o «@») que hay justo antes del cursor.
func (e *EditorNota) disparoEnCursor() (disparo, bool) {
	if e.haySeleccion() || len(e.doc.Bloques) == 0 {
		return disparo{}, false
	}
	b := e.bloqueActual()
	if b.Tipo == Codigo || !editable(b) {
		return disparo{}, false
	}
	r := []rune(b.Texto())
	o := min(e.cur.O, len(r))
	texto := string(r[:o])
	for _, d := range disparos {
		// Sin cuaderno no hay notas que ofrecer, y sin gente (un cuaderno cifrado) a nadie.
		if d.tipo == menuNota && e.enlaces == nil || d.tipo == menuPersona && e.gente == nil {
			continue
		}
		q, ok := d.consulta(texto)
		if !ok {
			continue
		}
		// La consulta no lleva su marca dentro: la última es la que abre.
		largo := utf8.RuneCountInString(texto[max(strings.LastIndex(texto, d.marca), 0):])
		inicio := o - largo
		if inicio < 0 || !strings.HasPrefix(string(r[inicio:o]), d.marca) {
			continue
		}
		return disparo{tipo: d.tipo, inicio: Pos{e.cur.B, inicio}, q: q}, true
	}
	return disparo{}, false
}

// mirarMenu abre, filtra o cierra el menú tras cada cambio o movimiento del cursor.
func (e *EditorNota) mirarMenu() {
	d, ok := e.disparoEnCursor()
	if !ok {
		e.descartada = nil
	}
	ok = ok && (e.descartada == nil || *e.descartada != d.inicio)
	// `[[`: la búsqueda de la herramienta de enlaces, la misma, sin Drive.
	if e.enlaces != nil {
		e.enlaces.buscar(d.q, ok && d.tipo == menuNota)
	}
	if !ok {
		e.cerrarMenu()
		return
	}
	idx := 0
	if m := e.menu; m != nil && m.tipo == d.tipo && m.q == d.q && m.inicio == d.inicio {
		idx = m.idx
	}
	m := &menuComandos{tipo: d.tipo, inicio: d.inicio, q: d.q, idx: idx}
	// `[[` se ve aunque aún no haya nada: dice que busca, o que nada casa. Los otros dos,
	// solo con algo que ofrecer.
	if d.tipo != menuNota && len(e.sugerencias(m)) == 0 {
		e.cerrarMenu()
		return
	}
	e.menu = m
	e.armarMenu()
	e.Refresh()
}

func (e *EditorNota) cerrarMenu() {
	if e.menu != nil {
		e.menu = nil
		e.Refresh()
	}
}

// refrescarMenu vuelve a pintar el menú abierto: ha llegado lo que se buscaba fuera.
func (e *EditorNota) refrescarMenu() {
	if e.menu != nil {
		e.armarMenu()
		e.Refresh()
	}
}

// sugerencias son las filas del menú `m`.
func (e *EditorNota) sugerencias(m *menuComandos) []sugerencia {
	var out []sugerencia
	switch m.tipo {
	case menuCmd:
		for _, c := range filtrarComandos(m.q, e.rotuloComando) {
			out = append(out, sugerencia{
				glifo: c.glifo, icono: c.icono, rotulo: e.rotuloComando(c), detalle: "/" + c.alias[0],
				elegir: func() { e.ejecutarComando(c) },
			})
		}
	case menuNota:
		for _, o := range e.enlaces.opciones() {
			s := sugerencia{glifo: NotaD, rotulo: o.nota, dentro: o.titulo, detalle: o.cuaderno, elegir: func() { e.enlazarNota(o) }}
			if o.titulo != "" {
				s.glifo = TituloD
			}
			s.grupo = e.tr.T("editor.linkOtherNotebooks", nil)
			if o.tipo == enlaceLocal {
				s.grupo = e.tr.T("editor.linkThisNotebook", nil)
			}
			out = append(out, s)
		}
	case menuPersona:
		// La gente se pide a la primera arroba; cuando llega, se vuelve a mirar.
		gente := e.gente(e.mirarMenu)
		// Sin nada tras la arroba no se ofrece a nadie: una «@» suelta es una arroba.
		if m.q == "" {
			return nil
		}
		for _, p := range ui.CandidatosMencion(gente, m.q) {
			s := sugerencia{icono: iconos.Usuario, rotulo: cmp.Or(p.Nombre, p.Email), elegir: func() { e.mencionar(p) }}
			if p.Nombre != "" {
				s.detalle = p.Email
			}
			out = append(out, s)
		}
	}
	return out
}

// armarMenu rehace las filas del menú con la activa marcada: `p-1`, `w-60` el de los
// bloques y `w-72` los otros dos; filas `px-2.5 py-1.5 text-sm gap-2` con el dibujo de 15
// en su caja de 20 y el detalle en `sytue-meta`; el rótulo de cada grupo sobre su primera
// fila; y en `[[`, «buscando…» o «ninguna coincide» al pie.
func (e *EditorNota) armarMenu() {
	m := e.menu
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	sug := e.sugerencias(m)
	m.idx = min(m.idx, max(len(sug)-1, 0))
	ancho := anchoMenuCmd
	if m.tipo != menuCmd {
		ancho = anchoMenuSug
	}
	// Como mucho ocho filas a la vista (el `max-h-72` de la web), con la activa dentro. El
	// rótulo de un grupo sale sobre la primera fila que se vea de él.
	desde := max(0, min(m.idx-filasMenuCmd/2, len(sug)-filasMenuCmd))
	hasta := min(len(sug), desde+filasMenuCmd)
	filas := make([]fyne.CanvasObject, 0, hasta-desde+3)
	for i := desde; i < hasta; i++ {
		s := sug[i]
		if s.grupo != "" && (i == desde || s.grupo != sug[i-1].grupo) {
			filas = append(filas, container.New(ui.Margen{Izq: pxFilaCmd, Der: pxFilaCmd, Arr: tema.Espacio(1), Abj: tema.Espacio(0.5)},
				lineaTok(s.grupo, tema.TextoXS, ui.Normal, "text-subtle", v)))
		}
		filas = append(filas, nuevaFilaSugerencia(e, s, i, i == m.idx, ancho, v))
	}
	if m.tipo == menuNota && (len(sug) == 0 || e.enlaces.buscando) {
		estado := []fyne.CanvasObject{lineaTok(e.tr.T("editor.linkNoResults", nil), tema.TextoSM, ui.Normal, "text-muted", v)}
		if e.enlaces.buscando {
			estado = []fyne.CanvasObject{
				ui.NuevaRueda(14, "text-muted", v),
				lineaTok(e.tr.T("editor.linkSearching", nil), tema.TextoSM, ui.Normal, "text-muted", v),
			}
		}
		filas = append(filas, container.New(ui.Margen{Izq: pxFilaCmd, Der: pxFilaCmd, Arr: pyFilaCmd, Abj: pyFilaCmd},
			container.New(filaCentro{hueco: huecoFilaCmd}, estado...)))
	}
	// El ancho de la lista aunque no haya filas (`w-72`).
	tope := canvas.NewRectangle(color.NRGBA{})
	tope.SetMinSize(fyne.NewSize(ancho, 0))
	m.panel = nuevoFlotante(container.New(ui.Columna{}, append(filas, tope)...), tema.Espacio(1))
	m.panel.rotulo = e.tr.T(rotulosMenu[m.tipo], nil)
}

// teclaMenu atiende las flechas, Intro, Tab y Escape mientras el menú está abierto. Sin
// filas que elegir (`[[` buscando), solo Escape: lo demás sigue escribiendo.
func (e *EditorNota) teclaMenu(k fyne.KeyName) bool {
	if e.menu == nil {
		return false
	}
	sug := e.sugerencias(e.menu)
	n := len(sug)
	activa := min(e.menu.idx, max(n-1, 0))
	switch k {
	case fyne.KeyDown, fyne.KeyUp:
		if n == 0 {
			return false
		}
		e.menu.idx = ui.MoverActiva(activa, n, k)
		e.armarMenu()
		e.Refresh()
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeyTab:
		if n == 0 {
			return false
		}
		sug[activa].elegir()
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

// tomarDisparo borra lo escrito desde el disparador («/h», «[[pla», «@an») y, en el mismo
// paso de deshacer, hace `luego` donde estaba.
func (e *EditorNota) tomarDisparo(luego func()) bool {
	d, ok := e.disparoEnCursor()
	e.menu = nil
	if !ok {
		e.Refresh()
		return false
	}
	e.hacer(func() {
		e.cur = e.doc.Borrar(d.inicio, e.cur)
		e.ancla = e.cur
		if luego != nil {
			luego()
		}
	})
	return true
}

// ejecutarComando borra «/consulta» y aplica la orden de su botón de la barra.
func (e *EditorNota) ejecutarComando(c comando) {
	if !e.tomarDisparo(nil) {
		return
	}
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

// enlazarNota cambia `[[consulta` por el enlace a la nota: el mismo que pone la
// herramienta de enlaces sin selección.
func (e *EditorNota) enlazarNota(o opcionEnlace) {
	e.tomarDisparo(func() { e.escribirEnlace(o.url, o.rotulo) })
}

// mencionar cambia «@an» por `@ana@empresa.es `, la forma con que el servicio avisa: texto
// sin más, con el formato de donde se escribe.
func (e *EditorNota) mencionar(p api.Miembro) {
	e.tomarDisparo(func() {
		e.cur = e.doc.Insertar(e.cur, "@"+p.Email+" ", estiloEn(e.bloqueActual(), e.cur.O))
		e.ancla = e.cur
	})
}

// filaSugerencia es una fila del menú. Se pulsa y se resalta al pasar por encima, pero NO
// toma el foco: el teclado sigue en el cuerpo, que es donde se filtra.
type filaSugerencia struct {
	widget.BaseWidget
	e      *EditorNota
	s      sugerencia
	i      int
	activa bool
	ancho  float32
	v      tema.Variante
}

var (
	_ fyne.Tappable     = (*filaSugerencia)(nil)
	_ desktop.Hoverable = (*filaSugerencia)(nil)
)

func nuevaFilaSugerencia(e *EditorNota, s sugerencia, i int, activa bool, ancho float32, v tema.Variante) *filaSugerencia {
	f := &filaSugerencia{e: e, s: s, i: i, activa: activa, ancho: ancho, v: v}
	f.ExtendBaseWidget(f)
	return f
}

func (f *filaSugerencia) AccessibilityLabel() string {
	return strings.Join(slices.DeleteFunc([]string{f.s.rotulo, f.s.dentro, f.s.detalle}, func(x string) bool { return x == "" }), ", ")
}

func (f *filaSugerencia) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleButton
}

func (f *filaSugerencia) Tapped(*fyne.PointEvent) { f.s.elegir() }

func (f *filaSugerencia) MouseIn(*desktop.MouseEvent) {
	if f.e.menu != nil && f.e.menu.idx != f.i {
		f.e.menu.idx = f.i
		f.e.armarMenu()
		f.e.Refresh()
	}
}
func (f *filaSugerencia) MouseMoved(*desktop.MouseEvent) {}
func (f *filaSugerencia) MouseOut()                      {}

// Medidas de la fila (`px-2.5 py-1.5 gap-2`, caja del dibujo `h-5 w-5`), y el ancho de las
// listas: `w-60` la de los bloques y `w-72` las otras dos, sin su `p-1` ni su borde.
var (
	pxFilaCmd    = tema.Espacio(2.5)
	pyFilaCmd    = tema.Espacio(1.5)
	huecoFilaCmd = tema.Espacio(2)
	cajaDibCmd   = tema.Espacio(5)
	anchoMenuCmd = tema.Espacio(60) - 2*(tema.Espacio(1)+tema.Borde)
	anchoMenuSug = tema.Espacio(72) - 2*(tema.Espacio(1)+tema.Borde)
)

func (f *filaSugerencia) CreateRenderer() fyne.WidgetRenderer {
	fondo := canvas.NewRectangle(color.NRGBA{})
	fondo.CornerRadius = tema.RadioMD
	if f.activa {
		fondo.FillColor = tema.Color("menu-selected", f.v)
	}
	var dib fyne.CanvasObject
	if f.s.glifo != "" {
		dib = Glifo(f.s.glifo, 15, "text-muted", f.v)
	} else {
		dib = Icono(f.s.icono, 15, "text-muted", f.v)
	}
	return &filaSugerenciaRenderer{
		f: f, fondo: fondo, dib: dib,
		texto:   ui.Texto(f.s.rotulo, tema.TextoSM, ui.Normal, tema.Color("text", f.v)),
		dentro:  ui.Texto("", tema.TextoSM, ui.Normal, tema.Color("text-muted", f.v)),
		detalle: ui.Texto(f.s.detalle, tema.TextoXS, ui.Normal, tema.Color("text-subtle", f.v)),
	}
}

type filaSugerenciaRenderer struct {
	f                      *filaSugerencia
	fondo                  *canvas.Rectangle
	dib                    fyne.CanvasObject
	texto, dentro, detalle *canvas.Text
}

func (r *filaSugerenciaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(r.f.ancho, max(tema.TextoSM.Linea, cajaDibCmd)+2*pyFilaCmd)
}

func (r *filaSugerenciaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.fondo, r.dib, r.texto, r.dentro, r.detalle}
}
func (r *filaSugerenciaRenderer) Destroy() {}
func (r *filaSugerenciaRenderer) Refresh() {}

func (r *filaSugerenciaRenderer) Layout(t fyne.Size) {
	s := r.f.s
	r.fondo.Resize(t)
	r.dib.Move(fyne.NewPos(pxFilaCmd+(cajaDibCmd-15)/2, (t.Height-15)/2))
	r.dib.Resize(fyne.NewSize(15, 15))
	// El detalle a la derecha, `max-w-[45%] truncate`.
	anchoDetalle := float32(0)
	if s.detalle != "" {
		anchoDetalle = ui.Truncar(r.detalle, s.detalle, tema.TextoXS, ui.Normal, 0.45*t.Width)
		ui.ColocarTexto(r.detalle, tema.TextoXS, t.Width-pxFilaCmd-anchoDetalle, (t.Height-tema.TextoXS.Linea)/2)
		anchoDetalle += huecoFilaCmd
	}
	// El rótulo y, en gris, « › título», recortados juntos (`dosTonos`).
	x := pxFilaCmd + cajaDibCmd + huecoFilaCmd
	cabe := max(t.Width-x-anchoDetalle-pxFilaCmd, 0)
	y := (t.Height - tema.TextoSM.Linea) / 2
	ancho := ui.Truncar(r.texto, s.rotulo, tema.TextoSM, ui.Normal, cabe)
	ui.ColocarTexto(r.texto, tema.TextoSM, x, y)
	r.dentro.Text = ""
	if s.dentro != "" && r.texto.Text == s.rotulo {
		ui.Truncar(r.dentro, " › "+s.dentro, tema.TextoSM, ui.Normal, cabe-ancho)
	}
	ui.ColocarTexto(r.dentro, tema.TextoSM, x+ancho, y)
}
