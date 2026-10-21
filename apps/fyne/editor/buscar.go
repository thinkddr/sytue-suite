// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"image/color"
	"strings"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// El panel «Buscar» del carril (`SearchPanel.tsx`), el mismo para todos los lienzos: el
// campo, el contador con las dos opciones y las flechas, el campo de reemplazo y sus dos
// botones. Qué es una coincidencia y cómo se reemplaza es del lienzo ([Buscable]): en
// Docus, un trozo de un bloque del folio; en Calcus, un trozo del texto de una celda.

// Buscable es lo que un lienzo ofrece al panel «Buscar».
type Buscable interface {
	// Buscar busca de nuevo y devuelve cuántas coincidencias hay (como mucho
	// [MaxCoincidencias]). La activa pasa a ser la primera.
	Buscar(q string, mayus, palabra bool) int
	// IrACoincidencia activa la i-ésima, con vuelta, y la lleva a la vista.
	IrACoincidencia(i int)
	// Activa es el índice de la coincidencia activa, y Coincidencias cuántas hay ahora
	// (reemplazar cambia la cuenta).
	Activa() int
	Coincidencias() int
	// Reemplazar cambia la activa por `por` y deja activa la siguiente;
	// ReemplazarTodo las cambia todas en UN paso de deshacer.
	Reemplazar(por, q string, mayus, palabra bool)
	ReemplazarTodo(por, q string, mayus, palabra bool)
	// Editable dice si el lienzo deja reemplazar: sin eso los dos botones se apagan.
	Editable() bool
}

// ResalteBuscar y ResalteActivo son los dos amarillos de «Buscar» de la web: `#fef08a`
// todas las coincidencias y `#fed7aa` la activa (HIGHLIGHT_PALETTE, colores de
// contenido sobre papel blanco). Los pintan el folio de Docus y las celdas de Calcus.
var (
	ResalteBuscar = color.NRGBA{R: 0xfe, G: 0xf0, B: 0x8a, A: 0xff}
	ResalteActivo = color.NRGBA{R: 0xfe, G: 0xd7, B: 0xaa, A: 0xff}
)

// MaxCoincidencias es el tope de lo que se resalta (`MAX_MATCHES`).
const MaxCoincidencias = 500

// PausaBuscar es lo que espera la búsqueda a que se deje de teclear.
const PausaBuscar = 300 * time.Millisecond

// esDePalabra es `WORD_CHAR`: letra, número o guion bajo, con acentos y eñes.
func esDePalabra(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) }

// Casar es `matchIndices`: las apariciones NO solapadas de `aguja` en `pajar`, en runas.
// Sin «coincidir mayúsculas» tampoco mira acentos (`rangos`, buscar.Rangos): «reunion»
// encuentra «reunión».
func Casar(pajar, aguja []rune, mayus, palabra bool) [][2]int {
	if len(aguja) == 0 {
		return nil
	}
	var pares [][2]int
	if mayus {
		for i := 0; i+len(aguja) <= len(pajar); {
			if string(pajar[i:i+len(aguja)]) != string(aguja) {
				i++
				continue
			}
			pares = append(pares, [2]int{i, i + len(aguja)})
			i += len(aguja)
		}
	} else {
		pares = buscar.Rangos(pajar, string(aguja))
	}
	var out [][2]int
	for _, m := range pares {
		i, fin := m[0], m[1]
		if palabra && ((i > 0 && esDePalabra(pajar[i-1])) || (fin < len(pajar) && esDePalabra(pajar[fin]))) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Buscador es el panel «Buscar» entero. `flex flex-col gap-2 p-3`.
type Buscador struct {
	b Buscable
	t func(string, i18n.Vars) string

	consulta, reemplazo *ui.Entrada
	contador            *ui.Rotulo
	mayus, palabra      *opcionBuscar
	anterior, siguiente *ui.BotonIcono
	uno, todos          *BotonPequeno
	objeto              fyne.CanvasObject
	tanda               int // la de la última tecla: solo busca la pausa de esa
	cuenta              int

	// Programar corre f dentro de d en el hilo de la interfaz: es la pausa de lo
	// tecleado. nil es un reloj de verdad; el lienzo que tenga el suyo —el folio, que las
	// pruebas paran— lo pone aquí.
	Programar func(d time.Duration, f func())
}

// NuevoBuscador monta el panel sobre un lienzo. Se construye UNA vez por documento: lo
// tecleado se conserva al cerrar y abrir el panel, como en la web mientras no se recarga.
func NuevoBuscador(bs Buscable, t func(string, i18n.Vars) string) *Buscador {
	b := &Buscador{b: bs, t: t}
	b.consulta = ui.NuevaEntrada(b.tr("find.placeholder"))
	b.consulta.Icono = iconos.Buscar
	b.consulta.AlCambiar(func(string) { b.programar() })
	b.consulta.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		b.ir(1) // Mayús+Intro no llega distinto: la flecha de arriba va hacia atrás
		return true
	}
	b.reemplazo = ui.NuevaEntrada(b.tr("find.replace.placeholder"))
	b.reemplazo.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		b.reemplazarUno()
		return true
	}
	b.contador = ui.NuevoRotulo("", tema.TextoXS, ui.Normal, "text-muted")
	b.mayus = nuevaOpcionBuscar("Aa", b.tr("find.matchCase"), func() { b.buscar() })
	b.palabra = nuevaOpcionBuscar("«ab»", b.tr("find.wholeWord"), func() { b.buscar() })
	b.anterior = ui.NuevoBotonIcono(iconos.ChevronArriba, b.tr("find.previous"), func() { b.ir(-1) })
	b.siguiente = ui.NuevoBotonIcono(iconos.ChevronAbajo, b.tr("find.next"), func() { b.ir(1) })
	b.uno = NuevoBotonPequeno(b.tr("find.replace.one"), b.reemplazarUno)
	b.todos = NuevoBotonPequeno(b.tr("find.replace.all"), func() {
		b.b.ReemplazarTodo(b.reemplazo.Texto(), b.q(), b.mayus.Activa, b.palabra.Activa)
		b.pintar()
	})
	filete := canvas.NewRectangle(tema.Color("border", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))
	filete.SetMinSize(fyne.NewSize(tema.Borde, tema.Espacio(5))) // `h-5 w-px`, y el `mx-0.5` lo pone el relleno
	fila := container.New(filaContador{}, b.contador, b.mayus, b.palabra,
		container.New(layout.NewCustomPaddedLayout(0, 0, tema.Espacio(0.5), tema.Espacio(0.5)), filete),
		b.anterior, b.siguiente)
	botones := container.New(ui.Fila{Hueco: tema.Espacio(1)}, b.uno, b.todos)
	b.objeto = container.New(layout.NewCustomPaddedLayout(MargenPanel, MargenPanel, MargenPanel, MargenPanel),
		container.New(ui.Columna{Hueco: tema.Espacio(2)}, b.consulta, fila, b.reemplazo,
			container.New(AlDerecha{}, botones)))
	b.pintar()
	return b
}

// Objeto es el cuerpo del panel, para `Contexto.Paneles`.
func (b *Buscador) Objeto() fyne.CanvasObject { return b.objeto }

// Consultar pone la consulta y busca YA, sin la pausa de lo tecleado.
func (b *Buscador) Consultar(q string) {
	b.consulta.PonerTexto(q)
	b.tanda++ // lo que hubiera en cola ya no vale
	b.buscar()
}

// Cuenta es cuántas coincidencias dice el contador.
func (b *Buscador) Cuenta() int { return b.cuenta }

func (b *Buscador) q() string { return strings.TrimSpace(b.consulta.Texto()) }

// programar busca [PausaBuscar] después de la última tecla: una tecla nueva deja sin
// efecto la búsqueda que estaba en cola.
func (b *Buscador) programar() {
	b.tanda++
	tanda := b.tanda
	relanzar := func() {
		if tanda == b.tanda {
			b.buscar()
		}
	}
	if b.Programar != nil {
		b.Programar(PausaBuscar, relanzar)
		return
	}
	time.AfterFunc(PausaBuscar, func() { fyne.Do(relanzar) })
}

// buscar vuelve a buscar con las opciones de ahora y lleva a la primera.
func (b *Buscador) buscar() {
	b.cuenta = b.b.Buscar(b.q(), b.mayus.Activa, b.palabra.Activa)
	if b.cuenta > 0 {
		b.b.IrACoincidencia(0)
	}
	b.pintar()
}

func (b *Buscador) ir(d int) {
	b.b.IrACoincidencia(b.b.Activa() + d)
	b.pintar()
}

func (b *Buscador) reemplazarUno() {
	b.b.Reemplazar(b.reemplazo.Texto(), b.q(), b.mayus.Activa, b.palabra.Activa)
	b.pintar()
}

// pintar pone el contador y apaga lo que no se puede usar sin coincidencias.
func (b *Buscador) pintar() {
	b.cuenta = b.b.Coincidencias()
	texto := ""
	switch {
	case b.q() == "":
	case b.cuenta == 0:
		texto = b.tr("find.noMatches")
	default:
		texto = b.trv("find.position", i18n.Vars{"index": b.b.Activa() + 1, "total": b.cuenta})
	}
	b.contador.Texto = texto
	b.contador.Refresh()
	sin := b.cuenta == 0
	for _, x := range []*ui.BotonIcono{b.anterior, b.siguiente} {
		x.Inerte = sin
		x.Refresh()
	}
	for _, x := range []*BotonPequeno{b.uno, b.todos} {
		x.Inerte = sin || !b.b.Editable()
		x.Refresh()
	}
}

func (b *Buscador) tr(k string) string { return b.trv(k, nil) }

func (b *Buscador) trv(k string, v i18n.Vars) string {
	if b.t == nil {
		return k
	}
	return b.t(k, v)
}

// filaContador es la fila del contador: el texto se lleva lo que sobra (`flex-1
// truncate`) y los botones van a su medida con `gap-1`, todo centrado en alto.
type filaContador struct{}

func (filaContador) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			w += m.Width + tema.Espacio(1)
		}
		h = max(h, m.Height)
	}
	return fyne.NewSize(w, h)
}

func (filaContador) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := t.Width
	for i := len(objs) - 1; i > 0; i-- {
		m := objs[i].MinSize()
		x -= m.Width
		objs[i].Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		objs[i].Resize(m)
		x -= tema.Espacio(1)
	}
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(0, (t.Height-m.Height)/2))
	objs[0].Resize(fyne.NewSize(max(x, 0), m.Height))
}

// AlDerecha pega su único hijo a la derecha, a su medida (`flex justify-end`).
type AlDerecha struct{}

// MinSize es la del hijo.
func (AlDerecha) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// Layout lo pega a la derecha.
func (AlDerecha) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	m := objs[0].MinSize()
	objs[0].Move(fyne.NewPos(t.Width-m.Width, 0))
	objs[0].Resize(m)
}

// opcionBuscar es OPT_BTN: `h-7 min-w-7 px-1 text-xs font-semibold text-text-muted
// rounded-sm`, `aria-pressed:bg-selected aria-pressed:text-text`, hover `bg-subtle`.
type opcionBuscar struct {
	widget.BaseWidget
	Texto, Rotulo    string
	Activa           bool
	alCambiar        func()
	encima, enfocado bool
}

func nuevaOpcionBuscar(texto, rotulo string, alCambiar func()) *opcionBuscar {
	o := &opcionBuscar{Texto: texto, Rotulo: rotulo, alCambiar: alCambiar}
	o.ExtendBaseWidget(o)
	return o
}

var (
	_ fyne.Tappable     = (*opcionBuscar)(nil)
	_ fyne.Focusable    = (*opcionBuscar)(nil)
	_ desktop.Hoverable = (*opcionBuscar)(nil)
	_ fyne.Accessible   = (*opcionBuscar)(nil)
)

func (o *opcionBuscar) Tapped(*fyne.PointEvent) {
	o.Activa = !o.Activa
	o.Refresh()
	if o.alCambiar != nil {
		o.alCambiar()
	}
}
func (o *opcionBuscar) MouseIn(*desktop.MouseEvent)    { o.encima = true; o.Refresh() }
func (o *opcionBuscar) MouseMoved(*desktop.MouseEvent) {}
func (o *opcionBuscar) MouseOut()                      { o.encima = false; o.Refresh() }
func (o *opcionBuscar) FocusGained()                   { o.enfocado = true; o.Refresh() }
func (o *opcionBuscar) FocusLost()                     { o.enfocado = false; o.Refresh() }
func (o *opcionBuscar) TypedRune(r rune) {
	if r == ' ' {
		o.Tapped(nil)
	}
}

func (o *opcionBuscar) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(o, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		o.Tapped(nil)
	}
}
func (o *opcionBuscar) AccessibilityLabel() string             { return o.Rotulo }
func (o *opcionBuscar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

func (o *opcionBuscar) MinSize() fyne.Size {
	o.ExtendBaseWidget(o)
	w := ui.Medir(o.Texto, tema.TextoXS, ui.SemiNegrita).Width + 2*tema.Espacio(1)
	return fyne.NewSize(max(w, tema.Espacio(7)), tema.Espacio(7))
}

func (o *opcionBuscar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &opcionRenderer{o: o, caja: ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.RadioSM), aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v)}
	r.texto = ui.Texto(o.Texto, tema.TextoXS, ui.SemiNegrita, tema.Color("text-muted", v))
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.texto)...)
	return r
}

type opcionRenderer struct {
	o     *opcionBuscar
	caja  *ui.Caja
	aro   *ui.Aro
	texto *canvas.Text
	objs  []fyne.CanvasObject
}

func (r *opcionRenderer) MinSize() fyne.Size           { return r.o.MinSize() }
func (r *opcionRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *opcionRenderer) Destroy()                     {}
func (r *opcionRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	w := ui.Medir(r.o.Texto, tema.TextoXS, ui.SemiNegrita).Width
	ui.ColocarTexto(r.texto, tema.TextoXS, (t.Width-w)/2, (t.Height-tema.TextoXS.Linea)/2)
}

func (r *opcionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("text-muted", v)
	switch {
	case r.o.Activa:
		fondo, tinta = tema.Color("selected", v), tema.Color("text", v)
	case r.o.encima:
		fondo, tinta = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	r.caja.Pintar(fondo, color.NRGBA{})
	r.texto.Color = tinta
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.o.enfocado)
	if s := r.o.Size(); s.Width > 0 { // sin tamaño todavía: la caja con borde saldría negativa
		r.Layout(s)
	}
}

// BotonPequeno es REPLACE_BTN: `h-7 rounded-sm border border-border px-2 text-xs
// text-text hover:bg-bg-subtle disabled:opacity-40`, con el rótulo centrado como en un
// `<button>`. Con `Alto` es también el POP_BTN de los paneles de Calcus (`h-6`, y
// `aria-pressed:bg-selected aria-pressed:border-transparent` si está `Pulsado`). Docus
// lo lleva dentro de la ficha de estilo de la galería, que se comporta igual y se pinta
// distinto: por eso el estado del puntero y del foco se leen desde fuera.
type BotonPequeno struct {
	widget.BaseWidget
	Texto            string
	Inerte, Pulsado  bool
	AlPulsar         func()
	Encima, Enfocado bool
	// Alto es el de la caja; cero es el `h-7` de REPLACE_BTN.
	Alto float32
	// Glifo es el icono `sm` (14) de delante del rótulo, con `gap-1.5`: el de «Exportar
	// combinado» de Correspondencia.tsx. nil, no lleva.
	Glifo *iconos.Dibujo
	// Opcion es el botón de un `radiogroup` de Disposición.tsx: pulsado, `border-primary
	// bg-selected text-text`; si no, `border-border bg-surface text-text-muted`.
	Opcion bool
	// SinBorde es el botón de «Quitar»: `text-text-muted hover:bg-bg-subtle`, sin caja.
	SinBorde bool
}

// ladoGlifoPequeno es el `size="sm"` del icono de delante.
const ladoGlifoPequeno = 14

// anchoContenido es lo que miden el glifo, el hueco y el rótulo juntos.
func (b *BotonPequeno) anchoContenido() float32 {
	w := ui.Medir(b.Texto, tema.TextoXS, ui.Normal).Width
	if b.Glifo != nil {
		w += ladoGlifoPequeno + tema.Espacio(1.5)
	}
	return w
}

// NuevoBotonPequeno crea el botón con su rótulo y su acción.
func NuevoBotonPequeno(texto string, alPulsar func()) *BotonPequeno {
	b := &BotonPequeno{Texto: texto, AlPulsar: alPulsar}
	b.ExtendBaseWidget(b)
	return b
}

var (
	_ fyne.Tappable     = (*BotonPequeno)(nil)
	_ fyne.Focusable    = (*BotonPequeno)(nil)
	_ desktop.Hoverable = (*BotonPequeno)(nil)
	_ fyne.Accessible   = (*BotonPequeno)(nil)
)

// Tapped pulsa, salvo inerte.
func (b *BotonPequeno) Tapped(*fyne.PointEvent) {
	if !b.Inerte && b.AlPulsar != nil {
		b.AlPulsar()
	}
}

// MouseIn, MouseMoved y MouseOut siguen el puntero para el `hover`.
func (b *BotonPequeno) MouseIn(*desktop.MouseEvent) { b.Encima = true; b.Refresh() }

// MouseMoved no hace nada.
func (b *BotonPequeno) MouseMoved(*desktop.MouseEvent) {}

// MouseOut apaga el `hover`.
func (b *BotonPequeno) MouseOut() { b.Encima = false; b.Refresh() }

// FocusGained enciende el anillo de foco.
func (b *BotonPequeno) FocusGained() { b.Enfocado = true; b.Refresh() }

// FocusLost lo apaga.
func (b *BotonPequeno) FocusLost() { b.Enfocado = false; b.Refresh() }

// TypedRune pulsa con el espacio.
func (b *BotonPequeno) TypedRune(r rune) {
	if r == ' ' {
		b.Tapped(nil)
	}
}

// TypedKey pulsa con Intro.
func (b *BotonPequeno) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(b, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		b.Tapped(nil)
	}
}

// AccessibilityLabel es el rótulo.
func (b *BotonPequeno) AccessibilityLabel() string { return b.Texto }

// AccessibilityRole lo anuncia como botón.
func (b *BotonPequeno) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// MinSize es el alto (`h-7` si no se dice) y el rótulo con `px-2`.
func (b *BotonPequeno) MinSize() fyne.Size {
	b.ExtendBaseWidget(b)
	alto := b.Alto
	if alto == 0 {
		alto = tema.Espacio(7)
	}
	return fyne.NewSize(b.anchoContenido()+2*tema.Espacio(2), alto)
}

// CreateRenderer pinta la caja con borde, el rótulo y el anillo.
func (b *BotonPequeno) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El relleno es el del panel y no transparente: la caja pinta el borde como un
	// rectángulo entero DEBAJO del relleno, y sin relleno opaco se vería gris. Nace ya
	// con los colores de su estado (pulsado, inerte).
	fondo, borde, tinta := b.colores(v)
	r := &pequenoRenderer{b: b, caja: ui.NuevaCaja(fondo, borde, tema.RadioSM), aro: ui.NuevoAro(tema.RadioSM, ui.SinHueco, v)}
	r.texto = ui.Texto(b.Texto, tema.TextoXS, ui.Normal, tinta)
	r.objs = r.aro.Antes(append(r.caja.Objetos(), r.texto)...)
	if b.Glifo != nil {
		b.Glifo.TenirCon(tinta)
		r.objs = append(r.objs, b.Glifo.Objeto())
	}
	return r
}

// colores son el fondo, el borde y la tinta del botón en su estado.
func (b *BotonPequeno) colores(v tema.Variante) (fondo, borde, tinta color.NRGBA) {
	fondo, borde, tinta = tema.Color("surface", v), tema.Color("border", v), tema.Color("text", v)
	switch {
	case b.Opcion && b.Pulsado:
		fondo = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
		borde = tema.Color("primary", v)
	case b.Opcion:
		tinta = tema.Color("text-muted", v)
		if b.Encima && !b.Inerte {
			fondo = tema.Color("bg-subtle", v)
		}
	case b.SinBorde:
		fondo, tinta = tema.Color("surface", v), tema.Color("text-muted", v)
		if b.Encima && !b.Inerte {
			fondo = tema.Color("bg-subtle", v)
		}
		borde = fondo
	case b.Pulsado:
		// `bg-selected` es translúcido: se aplana sobre el panel, y el borde toma el mismo
		// color (`border-transparent` deja ver el fondo del botón).
		fondo = tokens.SobreSRGB(tema.Color("selected", v), tema.Color("surface", v))
		borde = fondo
	case b.Encima && !b.Inerte:
		fondo = tema.Color("bg-subtle", v)
	}
	if b.Inerte {
		// `disabled:opacity-40`, aplanado contra el panel (Fyne no tiene opacidad de grupo).
		borde, tinta = al40(borde, v), al40(tinta, v)
	}
	return fondo, borde, tinta
}

type pequenoRenderer struct {
	b     *BotonPequeno
	caja  *ui.Caja
	aro   *ui.Aro
	texto *canvas.Text
	objs  []fyne.CanvasObject
}

func (r *pequenoRenderer) MinSize() fyne.Size           { return r.b.MinSize() }
func (r *pequenoRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *pequenoRenderer) Destroy()                     {}
func (r *pequenoRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	r.aro.Colocar(t)
	// Centrado, como en un `<button>`: a su medida es el `px-2`, y estirado en una
	// rejilla (`grid-cols-2`) va en medio.
	x := (t.Width - r.b.anchoContenido()) / 2
	if g := r.b.Glifo; g != nil {
		g.Colocar(x, (t.Height-ladoGlifoPequeno)/2, ladoGlifoPequeno)
		x += ladoGlifoPequeno + tema.Espacio(1.5)
	}
	ui.ColocarTexto(r.texto, tema.TextoXS, x, (t.Height-tema.TextoXS.Linea)/2)
}

func (r *pequenoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, borde, tinta := r.b.colores(v)
	r.caja.Pintar(fondo, borde)
	r.texto.Color = tinta
	r.texto.Refresh()
	if r.b.Glifo != nil {
		r.b.Glifo.TenirCon(tinta)
	}
	r.aro.Pintar(v)
	r.aro.Ver(r.b.Enfocado)
	if s := r.b.Size(); s.Width > 0 { // sin tamaño todavía: la caja con borde saldría negativa
		r.Layout(s)
	}
}
