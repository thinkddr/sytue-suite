// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"image/color"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// «SÍMBOLOS»: el selector de caracteres que no están en el teclado —flechas, signos
// matemáticos, monedas, la tipografía de las comillas y las rayas, y las marcas de una
// lista—, con los dieciséis últimos usados arriba («Recientes»). Se abre desde el botón
// Ω de la barra y desde «/» → «Símbolos…», y escribe el símbolo donde está el cursor,
// pisando lo seleccionado, en un paso de deshacer.
//
// Es un `Popover` (ui.MostrarPopover): se lleva el foco y lo devuelve al cuerpo al
// cerrarse. Se recorre con las flechas (arriba y abajo van a la misma columna de la fila
// de al lado, también entre apartados), Inicio y Fin, Intro o Espacio eligen y Escape
// cierra. Con el dedo, cada casilla mide 44.
//
// ponytail: sin emoji. El cuerpo de una nota se pinta con Figtree y, para lo que Figtree
// no trae, con la letra de reserva DEL SISTEMA (la de Noto Color Emoji en Linux y
// Android; en macOS y Windows sus emoji son sbix y COLR, que el pintor de Fyne no
// dibuja). La cara de color propia (`tema.Emoji`, 10 MB) solo la usa el selector de Sytue
// Rooms, no la prosa: un emoji puesto aquí se vería en blanco o como una caja según el
// equipo, así que no se ofrece.

// clavePrefSimbolos es donde la app recuerda los últimos usados, en sus preferencias
// locales: no sale del equipo.
const clavePrefSimbolos = "sytue-notes-simbolos-recientes"

// MaxRecientesSimbolos es cuántos recuerda.
const MaxRecientesSimbolos = 16

// CategoriasSimbolos son los apartados fijos, en su orden, con la clave de su rótulo.
var CategoriasSimbolos = []struct {
	Clave    string
	Simbolos []string
}{
	{"editor.symbolsArrows", []string{"←", "→", "↑", "↓", "⇒", "↔"}},
	{"editor.symbolsMath", []string{"±", "×", "÷", "≠", "≤", "≥", "∞", "√", "∑", "π", "°"}},
	{"editor.symbolsCurrency", []string{"€", "$", "£", "¥", "₿"}},
	{"editor.symbolsTypography", []string{"«", "»", "“", "”", "—", "–", "…", "•", "·", "§", "¶", "©", "®", "™"}},
	{"editor.symbolsMarks", []string{"✓", "✗", "★", "☆", "♥", "⚠", "☐", "☑"}},
}

// recientesSimbolos son los últimos usados, el más reciente primero.
func recientesSimbolos() []string {
	a := fyne.CurrentApp()
	if a == nil || a.Preferences() == nil {
		return nil
	}
	r := a.Preferences().StringList(clavePrefSimbolos)
	return r[:min(len(r), MaxRecientesSimbolos)]
}

// recordarSimbolo lo pone el primero de los recientes, sin repetirlo y sin pasar de
// dieciséis.
func recordarSimbolo(s string) {
	a := fyne.CurrentApp()
	if a == nil || a.Preferences() == nil || s == "" {
		return
	}
	r := slices.DeleteFunc(slices.Clone(recientesSimbolos()), func(x string) bool { return x == s })
	r = append([]string{s}, r...)
	a.Preferences().SetStringList(clavePrefSimbolos, r[:min(len(r), MaxRecientesSimbolos)])
}

// seccionSimbolos es un apartado del selector: su rótulo y sus símbolos.
type seccionSimbolos struct {
	titulo   string
	simbolos []string
}

// seccionesSimbolos son los apartados que se ven: los recientes, si los hay, y los fijos.
func (e *EditorNota) seccionesSimbolos() []seccionSimbolos {
	var out []seccionSimbolos
	if r := recientesSimbolos(); len(r) > 0 {
		out = append(out, seccionSimbolos{e.tr.T("editor.symbolsRecent", nil), r})
	}
	for _, c := range CategoriasSimbolos {
		out = append(out, seccionSimbolos{e.tr.T(c.Clave, nil), c.Simbolos})
	}
	return out
}

// abrirSimbolos abre el selector colgando de `disparador` (el botón, «Más» o el cursor).
func (e *EditorNota) abrirSimbolos(disparador fyne.CanvasObject) {
	var em *ui.Emergente
	s := nuevoSelectorSimbolos(e.seccionesSimbolos(), e.tr.T("editor.symbols", nil))
	s.cerrar = func() {
		if em != nil {
			em.Cerrar()
		}
		e.reenfocar()
	}
	s.elegir = func(x string) {
		s.cerrar()
		e.insertarSimbolo(x)
	}
	em = ui.MostrarPopover(disparador, s)
	em.Rotulo = s.rotulo
	e.simbolos = s
}

// insertarSimbolo escribe el símbolo donde está el cursor, pisando lo seleccionado, en
// su propio paso de deshacer, y lo apunta en los recientes.
func (e *EditorNota) insertarSimbolo(s string) {
	if s == "" {
		return
	}
	e.escribiendo = false
	e.escribir(s)
	e.escribiendo = false
	recordarSimbolo(s)
}

// Medidas del selector: casillas de 32 (44 con el dedo) con `gap-0.5`, ocho por fila
// (siete con el dedo, que en un teléfono de 390 no caben ocho de 44), y cada apartado con
// su rótulo en `text-xs text-subtle` y `mt-2` desde el de arriba.
var (
	ladoCeldaSimbolo  = tema.Espacio(8)
	huecoCeldaSimbolo = tema.Espacio(0.5)
	bajoRotuloSimbolo = tema.Espacio(1)
	entreSecciones    = tema.Espacio(2)
)

func columnasSimbolos() int {
	if ui.EsTactil() {
		return 7
	}
	return 8
}

func ladoCelda() float32 { return ui.SueloTactil(ladoCeldaSimbolo) }

// selectorSimbolos es la rejilla: un solo control enfocable que lleva la casilla activa,
// como un `listbox` con `aria-activedescendant`.
type selectorSimbolos struct {
	widget.BaseWidget
	rotulo    string
	secciones []seccionSimbolos
	todos     []string
	activo    int
	enfocado  bool
	elegir    func(string)
	cerrar    func()
	r         *selectorSimbolosRenderer
}

var (
	_ fyne.Focusable    = (*selectorSimbolos)(nil)
	_ fyne.Tappable     = (*selectorSimbolos)(nil)
	_ desktop.Hoverable = (*selectorSimbolos)(nil)
	_ fyne.Accessible   = (*selectorSimbolos)(nil)
)

func nuevoSelectorSimbolos(secciones []seccionSimbolos, rotulo string) *selectorSimbolos {
	s := &selectorSimbolos{secciones: secciones, rotulo: rotulo}
	for _, sec := range secciones {
		s.todos = append(s.todos, sec.simbolos...)
	}
	s.ExtendBaseWidget(s)
	return s
}

// AccessibilityLabel dice el nombre del selector y el símbolo activo.
func (s *selectorSimbolos) AccessibilityLabel() string {
	if s.activo < len(s.todos) {
		return s.rotulo + ": " + s.todos[s.activo]
	}
	return s.rotulo
}

// AccessibilityRole: un grupo de opciones (Fyne no tiene el rol `listbox`).
func (s *selectorSimbolos) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleContainer
}

func (s *selectorSimbolos) FocusGained() { s.enfocado = true; s.Refresh() }
func (s *selectorSimbolos) FocusLost()   { s.enfocado = false; s.Refresh() }

// TypedRune: el espacio elige, como en un botón.
func (s *selectorSimbolos) TypedRune(r rune) {
	if r == ' ' {
		s.elegirActivo()
	}
}

// TypedKey mueve la casilla activa, elige o cierra.
func (s *selectorSimbolos) TypedKey(k *fyne.KeyEvent) {
	n := len(s.todos)
	if n == 0 {
		if k.Name == fyne.KeyEscape {
			llamar0(s.cerrar)
		}
		return
	}
	switch k.Name {
	case fyne.KeyLeft:
		s.mover(max(s.activo-1, 0))
	case fyne.KeyRight:
		s.mover(min(s.activo+1, n-1))
	case fyne.KeyUp, fyne.KeyDown:
		s.mover(s.vecina(k.Name == fyne.KeyDown))
	case fyne.KeyHome:
		s.mover(0)
	case fyne.KeyEnd:
		s.mover(n - 1)
	case fyne.KeyReturn, fyne.KeyEnter:
		s.elegirActivo()
	case fyne.KeyEscape:
		llamar0(s.cerrar)
	default:
	}
}

func (s *selectorSimbolos) mover(i int) {
	if i != s.activo {
		s.activo = i
		s.Refresh()
	}
}

func (s *selectorSimbolos) elegirActivo() {
	if s.activo < len(s.todos) && s.elegir != nil {
		s.elegir(s.todos[s.activo])
	}
}

// posicion es la fila (contando todas las de todos los apartados) y la columna de la
// casilla `i`.
func (s *selectorSimbolos) posicion(i int) (fila, col int) {
	cols := columnasSimbolos()
	for _, sec := range s.secciones {
		if i < len(sec.simbolos) {
			return fila + i/cols, i % cols
		}
		i -= len(sec.simbolos)
		fila += (len(sec.simbolos) + cols - 1) / cols
	}
	return fila, 0
}

// vecina es la casilla de la fila de encima o de debajo más cercana a la misma columna;
// la misma si no hay más filas.
func (s *selectorSimbolos) vecina(abajo bool) int {
	fila, col := s.posicion(s.activo)
	objetivo := fila - 1
	if abajo {
		objetivo = fila + 1
	}
	mejor, dist := s.activo, 1<<30
	for i := range s.todos {
		f, c := s.posicion(i)
		if f != objetivo {
			continue
		}
		if d := max(c-col, col-c); d < dist {
			mejor, dist = i, d
		}
	}
	return mejor
}

// celdaEn es la casilla bajo un punto, o -1.
func (s *selectorSimbolos) celdaEn(p fyne.Position) int {
	if s.r == nil {
		return -1
	}
	lado := ladoCelda()
	for i, c := range s.r.cajas {
		if p.X >= c.X && p.X < c.X+lado && p.Y >= c.Y && p.Y < c.Y+lado {
			return i
		}
	}
	return -1
}

func (s *selectorSimbolos) Tapped(ev *fyne.PointEvent) {
	if i := s.celdaEn(ev.Position); i >= 0 {
		s.activo = i
		s.elegirActivo()
	}
}

func (s *selectorSimbolos) MouseIn(*desktop.MouseEvent) {}
func (s *selectorSimbolos) MouseMoved(ev *desktop.MouseEvent) {
	if i := s.celdaEn(ev.Position); i >= 0 {
		s.mover(i)
	}
}
func (s *selectorSimbolos) MouseOut() {}

func (s *selectorSimbolos) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &selectorSimbolosRenderer{s: s, activa: canvas.NewRectangle(color.NRGBA{})}
	r.activa.CornerRadius = tema.RadioMD
	for _, sec := range s.secciones {
		r.titulos = append(r.titulos, ui.Texto(sec.titulo, tema.TextoXS, ui.Medio, tema.Color("text-subtle", v)))
		for _, x := range sec.simbolos {
			r.glifos = append(r.glifos, ui.Texto(x, tema.TextoLG, ui.Normal, tema.Color("text", v)))
		}
	}
	s.r = r
	r.Refresh()
	return r
}

type selectorSimbolosRenderer struct {
	s       *selectorSimbolos
	activa  *canvas.Rectangle
	titulos []*canvas.Text
	glifos  []*canvas.Text
	cajas   []fyne.Position
}

func (r *selectorSimbolosRenderer) MinSize() fyne.Size {
	cols := columnasSimbolos()
	lado := ladoCelda()
	ancho := float32(cols)*lado + float32(cols-1)*huecoCeldaSimbolo
	var alto float32
	for k, sec := range r.s.secciones {
		if k > 0 {
			alto += entreSecciones
		}
		filas := (len(sec.simbolos) + cols - 1) / cols
		alto += tema.TextoXS.Linea + bajoRotuloSimbolo + float32(filas)*lado + float32(max(filas-1, 0))*huecoCeldaSimbolo
	}
	return fyne.NewSize(ancho, alto)
}

func (r *selectorSimbolosRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, 1+len(r.titulos)+len(r.glifos))
	objs = append(objs, r.activa)
	for _, t := range r.titulos {
		objs = append(objs, t)
	}
	for _, g := range r.glifos {
		objs = append(objs, g)
	}
	return objs
}

func (r *selectorSimbolosRenderer) Destroy() {}

func (r *selectorSimbolosRenderer) Layout(fyne.Size) {
	cols := columnasSimbolos()
	lado := ladoCelda()
	r.cajas = r.cajas[:0]
	var y float32
	k := 0
	for n, sec := range r.s.secciones {
		if n > 0 {
			y += entreSecciones
		}
		ui.ColocarTexto(r.titulos[n], tema.TextoXS, 0, y)
		y += tema.TextoXS.Linea + bajoRotuloSimbolo
		for i := range sec.simbolos {
			fila, col := i/cols, i%cols
			x := float32(col) * (lado + huecoCeldaSimbolo)
			cy := y + float32(fila)*(lado+huecoCeldaSimbolo)
			r.cajas = append(r.cajas, fyne.NewPos(x, cy))
			g := r.glifos[k]
			m := ui.Medir(g.Text, tema.TextoLG, ui.Normal)
			ui.ColocarTexto(g, tema.TextoLG, x+(lado-m.Width)/2, cy+(lado-tema.TextoLG.Linea)/2)
			k++
		}
		filas := (len(sec.simbolos) + cols - 1) / cols
		y += float32(filas)*lado + float32(max(filas-1, 0))*huecoCeldaSimbolo
	}
	if a := r.s.activo; a < len(r.cajas) {
		r.activa.Move(r.cajas[a])
		r.activa.Resize(fyne.NewSize(lado, lado))
	}
}

// Refresh pinta la casilla activa: `bg-menu-selected` y, con el foco del teclado, el aro.
func (r *selectorSimbolosRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.activa.FillColor = tema.Color("menu-selected", v)
	r.activa.StrokeWidth = 0
	if r.s.enfocado {
		r.activa.StrokeColor, r.activa.StrokeWidth = tema.Color("focus-ring", v), 2
	}
	for _, t := range r.titulos {
		t.Color = tema.Color("text-subtle", v)
	}
	for _, g := range r.glifos {
		g.Color = tema.Color("text", v)
	}
	r.Layout(r.s.Size())
	canvas.Refresh(r.s)
}
