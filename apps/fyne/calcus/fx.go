// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calcus

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/formula"
)

// # Escribir una fórmula
//
// Es la otra mitad de una hoja de cálculo, y en la web son unas cuantas piezas que van
// juntas mientras se escribe algo que empieza por «=», en la celda o en la barra:
//
//   - cada referencia se TIÑE de un color, en el texto y con un marco del mismo color
//     sobre las celdas a las que apunta;
//   - al teclear el nombre de una función sale la lista de las que empiezan así, con su
//     descripción; con las flechas se elige, con Intro o Tab se acepta;
//   - dentro de los paréntesis de una función, su firma con el argumento actual marcado;
//   - con el cursor donde va un operando, pulsar una celda (o arrastrar un rango, o una
//     cabecera) escribe su referencia en vez de seleccionarla;
//   - F4 cicla el anclaje de la referencia bajo el cursor.
//
// Todo sale de `apps/nucleo/formula/edicion.go`, que es `formula-edit.ts`; aquí solo se
// decide qué se pinta y dónde. El estado es `fxEdit` de SheetCanvas.tsx.

// firma es `FnSignature`: la firma de una función para la pista de argumentos y la
// descripción que acompaña a cada sugerencia del autocompletado.
type firma struct{ Args, Resumen string }

// buscarFirma es `lookupSignature`: sin distinguir mayúsculas.
func buscarFirma(nombre string) (firma, bool) {
	f, ok := catalogoFirmas[strings.ToUpper(nombre)]
	return f, ok
}

// partirFirma es `splitSignature`: «SUM(número1; [número2]; …)» → «SUM» y sus tres
// parámetros, para resaltar el argumento en el que está el cursor.
func partirFirma(args string) (nombre string, params []string) {
	abre := strings.IndexByte(args, '(')
	if abre < 0 {
		return args, nil
	}
	cierra := strings.LastIndexByte(args, ')')
	if cierra < abre {
		cierra = len(args)
	}
	for _, p := range strings.Split(args[abre+1:cierra], ";") {
		if p = strings.TrimSpace(p); p != "" {
			params = append(params, p)
		}
	}
	return args[:abre], params
}

// Los colores de las referencias (`--editor-ref-1..6` de editor.css). Son de la
// INTERFAZ, no del contenido: sobre el papel (la celda y los marcos) valen los mismos en
// los dos temas; en la barra, que es cromo, el tema oscuro lleva su propio juego.
var (
	coloresRef = [...]color.NRGBA{
		{R: 0x31, G: 0x71, B: 0xb9, A: 0xff},
		{R: 0x8c, G: 0x36, B: 0xc9, A: 0xff},
		{R: 0xc5, G: 0x34, B: 0x7c, A: 0xff},
		{R: 0x9e, G: 0x60, B: 0x2a, A: 0xff},
		{R: 0x22, G: 0x7f, B: 0x22, A: 0xff},
		{R: 0x21, G: 0x7a, B: 0x77, A: 0xff},
	}
	coloresRefFxOscuro = [...]color.NRGBA{
		{R: 0x66, G: 0x93, B: 0xc5, A: 0xff},
		{R: 0xab, G: 0x7b, B: 0xcd, A: 0xff},
		{R: 0xca, G: 0x74, B: 0x9f, A: 0xff},
		{R: 0xbd, G: 0x83, B: 0x50, A: 0xff},
		{R: 0x46, G: 0xb9, B: 0x46, A: 0xff},
		{R: 0x46, G: 0xb9, B: 0xb5, A: 0xff},
	}
)

// ColorRef es `refColor(i)`: el de la referencia i sobre el papel.
func ColorRef(i int) color.NRGBA { return coloresRef[i%len(coloresRef)] }

// ColorRefFx es `refFxColor(i)`: el de la referencia i en la barra de fórmulas.
func ColorRefFx(i int, v tema.Variante) color.NRGBA {
	if v == tema.Oscuro {
		return coloresRefFxOscuro[i%len(coloresRefFxOscuro)]
	}
	return ColorRef(i)
}

// edicionFx es `FxEditState` más lo que la web guarda en refs sueltas: la superficie
// (la barra o la celda), el texto y el cursor (en BYTES del texto), la referencia recién
// escrita por señalar, el ancla del señalar, y el estado de la lista.
type edicionFx struct {
	barra    bool
	texto    string
	cursor   int
	refIns   *[2]int
	ancla    *Celda
	arrastre int // qué se está arrastrando al señalar: celdas, columnas o filas
	indice   int
	cerrada  bool // `acDismissed`: Escape cierra la lista hasta la próxima tecla
}

// acFx es `AcState`: lo que casa con lo tecleado —funciones, nombres de tabla o
// columnas de una tabla—, con su clase en paralelo, y el tramo a sustituir.
type acFx struct {
	items       []string
	clases      []string // "fn", "table" o "column"
	inicio, fin int
}

// Las clases de lo que ofrece el autocompletado (`AcItem.kind`).
const (
	acFuncion = "fn"
	acTabla   = "table"
	acColumna = "column"
)

// autocompletar es `acAt`: las columnas si el cursor teclea dentro de `Tabla[…`; si
// no, hasta ocho funciones del motor y los nombres de tabla que empiecen por el
// identificador bajo el cursor (doce en total). Nada si la única función que casa ya
// está escrita entera. `l` da las tablas; sin libro, solo funciones.
//
//nolint:gocognit,gocyclo // tres modos (dentro de corchetes, tabla o función) con sus filtros, como `acAt` de la web
func autocompletar(texto string, cursor int, l *Libro, ancla Celda) (acFx, bool) {
	if brk, ok := formula.TableBracketAt(texto, cursor); ok {
		if l == nil {
			return acFx{}, false
		}
		pre := strings.ToLower(brk.Prefijo)
		var out acFx
		for _, n := range l.ColumnasDeTabla(brk.Tabla, ancla) {
			if strings.HasPrefix(strings.ToLower(n), pre) {
				out.items = append(out.items, n)
			}
		}
		if len(out.items) == 0 || (len(out.items) == 1 && strings.EqualFold(out.items[0], pre)) {
			return acFx{}, false
		}
		out.items = out.items[:min(len(out.items), 8)]
		for range out.items {
			out.clases = append(out.clases, acColumna)
		}
		out.inicio, out.fin = brk.Inicio, brk.Fin
		return out, true
	}
	tok, ok := formula.FuncTokenAt(texto, cursor)
	if !ok {
		return acFx{}, false
	}
	pref := strings.ToUpper(tok.Prefijo)
	var out acFx
	for _, n := range formula.FunctionNames() {
		if strings.HasPrefix(n, pref) {
			out.items, out.clases = append(out.items, n), append(out.clases, acFuncion)
			if len(out.items) >= 8 {
				break
			}
		}
	}
	if l != nil {
		vistas := map[string]bool{}
		for _, n := range l.NombresDeTabla() {
			if len(out.items) >= 12 {
				break
			}
			if strings.HasPrefix(strings.ToUpper(n), pref) && !vistas[n] {
				vistas[n] = true
				out.items, out.clases = append(out.items, n), append(out.clases, acTabla)
			}
		}
	}
	if len(out.items) == 0 || (len(out.items) == 1 && out.clases[0] == acFuncion && out.items[0] == pref) {
		return acFx{}, false
	}
	out.inicio, out.fin = tok.Inicio, tok.Fin
	return out, true
}

// ac es la lista abierta de la edición en curso, si la hay.
func (c *Lienzo) ac() (acFx, bool) {
	fe := c.fx
	if fe == nil || fe.cerrada {
		return acFx{}, false
	}
	return autocompletar(fe.texto, fe.cursor, c.Libro, c.Rejilla.sel)
}

// pista es la firma de la función que envuelve al cursor, con el argumento actual. Solo
// cuando no hay lista abierta, como en la web.
func (c *Lienzo) pista() (f firma, arg int, ok bool) {
	fe := c.fx
	if fe == nil {
		return firma{}, 0, false
	}
	if _, abierta := c.ac(); abierta {
		return firma{}, 0, false
	}
	h, ok := formula.ArgHintAt(fe.texto, fe.cursor)
	if !ok {
		return firma{}, 0, false
	}
	f, ok = buscarFirma(h.Nombre)
	return f, h.Arg, ok
}

// editorFx es el editor de esa superficie.
func (c *Lienzo) editorFx(barra bool) *ui.EditorPlano {
	if barra {
		return c.Barra.ed
	}
	return c.Rejilla.ed
}

// cursorEnBytes pasa la columna del cursor del editor (en runas) a bytes del texto.
func cursorEnBytes(s string, runas int) int {
	for i := range s {
		if runas == 0 {
			return i
		}
		runas--
	}
	return len(s)
}

// sincronizarFx lee el texto y el cursor del editor de una superficie y pone al día la
// edición de fórmula: `syncCellFx` en la celda y `onFxInput`/`onFxCaret` en la barra.
// Escribir o mover el cursor olvidan la referencia recién señalada; escribir, además,
// vuelve a abrir la lista.
//
// No hace nada si ni el texto ni el cursor han cambiado, y eso no es una optimización:
// repintar el editor recoloca su cursor y avisa de ello (`OnCursorChanged` desde el
// `Refresh` de `widget.Entry`), así que sin este corte cada aviso repinta y cada
// repintado avisa, sin fin.
func (c *Lienzo) sincronizarFx(barra bool) {
	if c.fxAplicando {
		return
	}
	ed := c.editorFx(barra)
	// La barra solo edita cuando tiene el foco: su texto también cambia al moverse la
	// selección, y eso no es escribir una fórmula.
	if (barra && !c.Barra.enfocado) || (!barra && !c.Rejilla.editando) {
		return
	}
	texto := ed.Texto()
	if !strings.HasPrefix(texto, "=") {
		if c.fx != nil && c.fx.barra == barra {
			c.terminarFx()
		}
		if !barra {
			c.espejarBarra(texto)
		}
		return
	}
	cursor := cursorEnBytes(texto, ed.CursorColumn)
	nueva := c.fx == nil || c.fx.barra != barra
	if !nueva && c.fx.texto == texto && c.fx.cursor == cursor {
		return
	}
	if nueva {
		c.fx = &edicionFx{barra: barra}
	}
	fe := c.fx
	if fe.texto != texto {
		fe.cerrada, fe.indice = false, 0
	}
	fe.refIns = nil
	fe.texto, fe.cursor = texto, cursor
	if !barra {
		c.espejarBarra(texto)
	}
	c.refrescarFx()
}

// espejarBarra pone en la barra lo que se escribe en la celda (`setFx(text)`).
func (c *Lienzo) espejarBarra(texto string) {
	if c.Barra.ed.Texto() == texto {
		return
	}
	c.fxAplicando = true
	c.Barra.ed.PonerTexto(texto)
	c.fxAplicando = false
	c.Barra.Refresh()
}

// terminarFx es `endFx`: se acaba la edición de fórmula (se confirmó, se canceló o se
// dejó de escribir un «=»).
func (c *Lienzo) terminarFx() {
	if c.fx == nil {
		return
	}
	c.fx = nil
	c.refrescarFx()
}

// refrescarFx repinta lo que depende de la edición de fórmula: los marcos de la rejilla,
// el tinte de la celda y de la barra, y la lista o la pista.
func (c *Lienzo) refrescarFx() {
	c.Rejilla.refsFx = nil
	if c.fx != nil {
		for _, r := range formula.ExtractRefsCon(c.fx.texto, c.Libro.ResolverParaResaltar(c.Rejilla.sel)) {
			if r.Hoja == "" || r.Hoja == c.Libro.Hoja() {
				c.Rejilla.refsFx = append(c.Rejilla.refsFx, r)
			}
		}
	}
	c.Rejilla.celdaFx = c.fx != nil && !c.fx.barra
	c.Rejilla.Refresh()
	c.Barra.Refresh()
	c.Refresh()
}

// aplicarFx escribe un texto y un cursor nuevos en el editor de la superficie activa sin
// que eso cuente como teclear (`applyFxText`).
func (c *Lienzo) aplicarFx(texto string, cursor int) {
	fe := c.fx
	ed := c.editorFx(fe.barra)
	c.fxAplicando = true
	ed.SetText(texto)
	ed.CursorColumn = utf8.RuneCountInString(texto[:cursor])
	ed.Refresh()
	c.fxAplicando = false
	fe.texto, fe.cursor = texto, cursor
	if !fe.barra {
		c.espejarBarra(texto)
	}
	c.refrescarFx()
}

// teclaFx son las teclas que la edición de fórmula se queda: las flechas, Intro, Tab y
// Escape con la lista abierta, y F4. Devuelve true si se la queda.
func (c *Lienzo) teclaFx(barra bool, k *fyne.KeyEvent) bool {
	fe := c.fx
	if fe == nil || fe.barra != barra {
		return false
	}
	ac, abierta := c.ac()
	switch {
	case abierta && (k.Name == fyne.KeyDown || k.Name == fyne.KeyUp):
		n := len(ac.items)
		if k.Name == fyne.KeyDown {
			fe.indice = (fe.indice + 1) % n
		} else {
			fe.indice = (fe.indice - 1 + n) % n
		}
		c.refrescarFx()
		return true
	case abierta && (k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter || k.Name == fyne.KeyTab):
		c.aceptarAC(min(fe.indice, len(ac.items)-1))
		return true
	case abierta && k.Name == fyne.KeyEscape:
		fe.cerrada = true
		c.refrescarFx()
		return true
	case k.Name == fyne.KeyF4:
		if texto, cursor, ok := formula.CycleRefAt(fe.texto, fe.cursor); ok {
			fe.refIns = nil
			c.aplicarFx(texto, cursor)
		}
		return true
	}
	return false
}

// aceptarAC es `acceptAc` con el i-ésimo de la lista: una función se escribe «NOMBRE(»
// con el cursor dentro; una tabla, «Tabla[» (sin repetir el corchete si ya está) con
// el cursor tras él; y una columna, «Columna]» con el cursor tras el cierre.
func (c *Lienzo) aceptarAC(i int) {
	fe := c.fx
	ac, ok := autocompletar(fe.texto, fe.cursor, c.Libro, c.Rejilla.sel)
	if !ok || i < 0 || i >= len(ac.items) {
		return
	}
	nombre := ac.items[i]
	var cuerpo string
	cursor := ac.inicio + len(nombre) + 1
	siguiente := byte(0)
	if ac.fin < len(fe.texto) {
		siguiente = fe.texto[ac.fin]
	}
	switch ac.clases[i] {
	case acTabla:
		cuerpo = nombre + "["
		if siguiente == '[' {
			cuerpo = nombre
		}
	case acColumna:
		cuerpo = nombre + "]"
		if siguiente == ']' {
			cuerpo = nombre
		}
	default:
		cuerpo = nombre + "("
		cursor = ac.inicio + len(cuerpo)
	}
	fe.refIns, fe.cerrada, fe.indice = nil, false, 0
	c.aplicarFx(fe.texto[:ac.inicio]+cuerpo+fe.texto[ac.fin:], cursor)
}

// Los tres gestos de señalar.
const (
	señalarPulsar    = iota // un clic (con o sin Mayús)
	señalarExtender         // Mayús+clic: extiende la recién señalada desde su ancla
	señalarArrastrar        // arrastrar: extiende hasta donde va el puntero
)

// listoParaSeñalar es `pickReady`: el cursor espera un operando, o acaba de escribirse
// una referencia señalando, o toca una ya escrita (que un clic sustituye).
func (fe *edicionFx) listoParaSeñalar() bool {
	if formula.ExpectsRef(fe.texto, fe.cursor) || fe.refIns != nil {
		return true
	}
	_, ok := formula.RefAtCaret(fe.texto, fe.cursor)
	return ok
}

// señalar es el point-and-click de `onGridMouseDown`: con una fórmula en edición lista
// para una referencia, pulsar una celda (o una cabecera) escribe su referencia en vez de
// seleccionarla, y la fórmula SIGUE en edición. Devuelve true si se lo quedó.
func (c *Lienzo) señalar(zona int, celda Celda, gesto int) bool {
	fe := c.fx
	if fe == nil {
		return false
	}
	if gesto == señalarArrastrar {
		if fe.ancla == nil {
			return false
		}
		switch fe.arrastre {
		case zonaCabCol:
			c.insertarRef(textoCols(fe.ancla.C, celda.C))
		case zonaCabFila:
			c.insertarRef(textoFilas(fe.ancla.F, celda.F))
		default:
			c.insertarRef(textoRango(*fe.ancla, celda))
		}
		return true
	}
	if !fe.listoParaSeñalar() {
		return false
	}
	switch zona {
	case zonaCelda:
		if !fe.barra && celda == c.Rejilla.sel {
			return false // la propia celda en edición: el clic coloca el cursor
		}
		ancla := celda
		if gesto == señalarExtender && fe.refIns != nil && fe.ancla != nil {
			ancla = *fe.ancla
		}
		fe.ancla, fe.arrastre = &ancla, zonaCelda
		c.insertarRef(textoRango(ancla, celda))
	case zonaCabCol:
		fe.ancla, fe.arrastre = &Celda{C: celda.C, F: c.Rejilla.sel.F}, zonaCabCol
		c.insertarRef(textoCols(celda.C, celda.C))
	case zonaCabFila:
		fe.ancla, fe.arrastre = &Celda{C: c.Rejilla.sel.C, F: celda.F}, zonaCabFila
		c.insertarRef(textoFilas(celda.F, celda.F))
	default:
		return false
	}
	return true
}

// insertarRef es `insertRefText`: escribe la referencia en el cursor, o sustituye la
// recién señalada, o la que toca el cursor.
func (c *Lienzo) insertarRef(ref string) {
	fe := c.fx
	ini, fin := fe.cursor, fe.cursor
	if fe.refIns != nil {
		ini, fin = fe.refIns[0], fe.refIns[1]
	} else if r, ok := formula.RefAtCaret(fe.texto, fe.cursor); ok {
		ini, fin = r.Inicio, r.Fin
	}
	texto := fe.texto[:ini] + ref + fe.texto[fin:]
	fe.refIns = &[2]int{ini, ini + len(ref)}
	c.aplicarFx(texto, ini+len(ref))
}

// textoRango es `rangeRefStr`: «A1», o «A1:B3» normalizado.
func textoRango(a, b Celda) string {
	c1, f1, c2, f2 := min(a.C, b.C), min(a.F, b.F), max(a.C, b.C), max(a.F, b.F)
	if c1 == c2 && f1 == f2 {
		return Ref(c1, f1)
	}
	return Ref(c1, f1) + ":" + Ref(c2, f2)
}

// textoCols y textoFilas son `insertHeaderRef`: «A:C» y «1:3», normalizados.
func textoCols(a, b int) string {
	return formula.NumToCol(min(a, b)) + ":" + formula.NumToCol(max(a, b))
}

func textoFilas(a, b int) string {
	return Ref(0, min(a, b))[1:] + ":" + Ref(0, max(a, b))[1:]
}

// devolverFoco pone el foco otra vez en el editor de la fórmula tras señalar: pulsar la
// rejilla se lo quita, y en la web el `preventDefault` del `mousedown` no lo deja irse.
func (c *Lienzo) devolverFoco() {
	if c.fx == nil {
		return
	}
	if lz := fyne.CurrentApp().Driver().CanvasForObject(c); lz != nil {
		lz.Focus(c.editorFx(c.fx.barra))
	}
}

// tinte es el texto de una fórmula pintado por trozos: cada referencia de su color y lo
// demás de la tinta normal (`tintedFormulaHtml`). Va DEBAJO del editor, que deja su
// texto transparente y conserva el cursor: es el truco de la web (un `<input>`
// transparente sobre un fondo tintado), porque un campo de texto no sabe pintar trozos
// de colores.
//
// Su letra es Figtree regular, la de la barra; la celda en edición pone la suya (`cara`
// y `estilo`, los de `caraCelda`).
type tinte struct {
	trozos []*canvas.Text
	cara   fyne.Resource
	estilo fyne.TextStyle
}

// pintar coloca los trozos con la caja de línea en (x, y). Devuelve si se pintó —no se
// pinta si el texto no cabe en `cabe`: la celda en edición no corre su texto, y los
// trozos se saldrían de ella; la barra, que sí lo corre, pasa su desplazamiento en `x` y
// no pone tope— y si el charco creció (hay que rehacer los objetos).
func (tn *tinte) pintar(texto string, refs []formula.Referencia, colorDe func(int) color.NRGBA,
	plano color.NRGBA, esc tema.Escalon, x, y, cabe float32,
) (pintado, crecio bool) {
	cara, estilo := tn.cara, tn.estilo
	if cara == nil && estilo == (fyne.TextStyle{}) {
		cara = tema.Regular
	}
	medir := func(s string) float32 {
		tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, esc.Tam, estilo, cara)
		return tam.Width
	}
	n := 0
	if texto != "" && medir(texto) <= cabe {
		pos := 0
		poner := func(ini, fin int, c color.NRGBA) {
			if ini >= fin {
				return
			}
			for len(tn.trozos) <= n {
				tn.trozos = append(tn.trozos, ui.Texto("", esc, ui.Normal, c))
				crecio = true
			}
			t := tn.trozos[n]
			n++
			t.Text, t.Color, t.TextSize, t.FontSource, t.TextStyle = texto[ini:fin], c, esc.Tam, cara, estilo
			ui.ColocarTexto(t, esc, x+medir(texto[:ini]), y)
			t.Show()
		}
		for _, r := range refs {
			if r.Inicio < pos {
				continue // se solaparía
			}
			poner(pos, r.Inicio, plano)
			poner(r.Inicio, r.Fin, colorDe(r.Color))
			pos = r.Fin
		}
		poner(pos, len(texto), plano)
		pintado = true
	}
	for i := n; i < len(tn.trozos); i++ {
		tn.trozos[i].Hide()
	}
	return pintado, crecio
}

func (tn *tinte) objetos() []fyne.CanvasObject {
	o := make([]fyne.CanvasObject, len(tn.trozos))
	for i, t := range tn.trozos {
		o[i] = t
	}
	return o
}
