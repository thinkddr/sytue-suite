// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"math"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// medidaSelector es una fila de SELECT_SIZE en Form.tsx: la misma caja que un campo,
// con el hueco del chevron a la derecha.
//
// `pr-9`/`pr-5` es el relleno de la DERECHA, y `right-3`/`right-1` es lo que el chevron
// se separa del borde. El chevron encoge con la talla, o se come el texto.
type medidaSelector struct{ derecha, chevron, chevronX float32 }

var medidasSelector = [...]medidaSelector{
	CampoMD: {derecha: tema.Espacio(9), chevron: 16, chevronX: tema.Espacio(3)},
	CampoSM: {derecha: tema.Espacio(5), chevron: 12, chevronX: tema.Espacio(1)},
	// ponytail: la cabecera de vista no tiene NINGÚN selector en la web, así que esta
	// fila no está medida: hereda la de `sm` para que el índice no se salga del array
	// —`medidasSelector[t]` no comprueba nada— si alguien construye uno. En cuanto
	// aparezca un selector en el cromo, se mide y se sustituye.
	CampoCromo: {derecha: tema.Espacio(5), chevron: 12, chevronX: tema.Espacio(1)},
}

// MedidaSelector es lo que ocupa un selector: como un campo, con el hueco del chevron.
func MedidaSelector(t TallaCampo, ancho float32) fyne.Size {
	m, s := medidasCampo[t], medidasSelector[t]
	if ancho <= 0 {
		ancho = 2*tema.Borde + m.margen + s.derecha + tema.Rem
	}
	return fyne.NewSize(ancho, AltoCampo(t))
}

// Selector es `<Select>` de Form.tsx: la caja de un campo con el valor elegido y un
// chevron a la derecha.
//
// # La lista abierta no tiene referencia en la web, y hay que saberlo
//
// Un `<select>` despliega la lista del SISTEMA OPERATIVO, no una de la hoja: en macOS,
// en Windows y en GNOME se ve distinta, y no hay ningún número de CSS que copiar. Así
// que la lista de aquí se pinta con las medidas del OTRO desplegable de la suite, el
// que sí es nuestro (`Dropdown.tsx`): panel `rounded-lg` con borde y `p-1`, y filas
// `px-3 py-2 text-base` que se tiñen de `bg-subtle` al pasar por encima.
//
// ponytail: la lista no se desplaza ni busca por teclas. Techo: un selector de país con
// los 31 de la suite obliga a bajar con la rueda por una lista de 1.100 px. Se releva
// metiendo las filas en el panel con desplazamiento que traiga la familia de emergentes.
type Selector struct {
	widget.BaseWidget
	Opciones []string
	Elegida  int
	Talla    TallaCampo
	// Ancho fija el ancho (en la web es `w-full`). Con 0, el mínimo.
	Ancho float32
	// Alto es un `h-*` de la clase sobre la talla (`<Select class="h-8">`): la caja
	// cambia de alto y el texto y el chevron siguen centrados. Con 0, el de la talla.
	Alto     float32
	Invalida bool
	AlElegir func(int)
	// Rotulo es el `aria-label`, YA traducido: el nombre del `<select>` cuando no lo da
	// una `<label>` («Canal para el bot»). Vacío: se anuncia el valor elegido.
	Rotulo string
	// Propio son las medidas de un `<select>` escrito a mano (ver SelectorPropio). Nil
	// es la talla.
	Propio *SelectorPropio

	encima, enfocado, inerte bool
	abierto                  *Flotante
	r                        *selectorRenderer
}

var (
	_ fyne.Widget       = (*Selector)(nil)
	_ fyne.Tappable     = (*Selector)(nil)
	_ fyne.Focusable    = (*Selector)(nil)
	_ fyne.Accessible   = (*Selector)(nil)
	_ desktop.Hoverable = (*Selector)(nil)
)

// AccessibilityLabel es el `aria-label` o, sin él, el valor elegido.
func (s *Selector) AccessibilityLabel() string {
	if s.Rotulo != "" {
		return s.Rotulo
	}
	return s.Valor()
}

// AccessibilityRole: un `<select>` cerrado es un botón que abre su lista (el «pop-up
// button» de macOS); Fyne no tiene el rol de combobox.
func (s *Selector) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// NuevoSelector crea un selector de talla md con esas opciones.
func NuevoSelector(opciones []string, alElegir func(int)) *Selector {
	s := &Selector{Opciones: opciones, AlElegir: alElegir}
	s.ExtendBaseWidget(s)
	return s
}

// Valor es el texto de la opción elegida, o vacío si no hay ninguna.
func (s *Selector) Valor() string {
	if s.Elegida < 0 || s.Elegida >= len(s.Opciones) {
		return ""
	}
	return s.Opciones[s.Elegida]
}

// Inerte es `disabled`.
func (s *Selector) Inerte(si bool) { s.inerte = si; s.encima = false; s.Refresh() }

// Con devuelve el mismo selector con otra talla.
func (s *Selector) Con(t TallaCampo) *Selector { s.Talla = t; return s }

// SelectorPropio es un `<select>` que la web escribe A MANO con clases y deja con la
// flecha del navegador (`appearance: auto`): `border-border rounded border px-1 py-0.5
// text-2xs`, por ejemplo, el del estado de un comentario de CertiPDF. Su ancho, sin
// `w-*`, es la opción más ancha más el relleno, el borde y el hueco de la flecha.
//
// Medido en Chromium a raíz de 16px (2026-10-03, estados de CertiPDF, `text-2xs`
// con px-1): 87 de ancho con la opción más ancha en 56,47; 21 de alto —el `<select>` no
// respeta el `line-height` de la clase: usa el `normal` de la fuente—. La flecha y el
// hueco del texto son del NAVEGADOR, no de la clase, y salen iguales con `text-xs`
// (campos de CertiPDF, 26 de alto): el texto empieza 4 px más allá del relleno, y la
// flecha es un chevrón de 10×7 centrado a 9,25 del borde derecho.
type SelectorPropio struct {
	Campo CampoPropio
	// Alto es el que mide de verdad (el navegador no usa la caja de línea de la clase).
	Alto float32
}

// Las medidas de un `<select>` nativo de Chromium, medidas en CertiPDF.
const (
	// FlechaSelectNativo es lo que el ancho suma a la derecha del texto: el hueco de la
	// flecha más el del texto por la izquierda.
	FlechaSelectNativo float32 = 20.59
	// AltoSelectNativo2XS y AltoSelectNativoXS son los altos con `py-0.5 text-2xs` y
	// `py-1 text-xs`.
	AltoSelectNativo2XS float32 = 21
	AltoSelectNativoXS  float32 = 26
	// interiorSelectNativo es lo que el texto se separa del relleno por la izquierda.
	interiorSelectNativo float32 = 4
	// chevronSelectNativo es el lado del glifo que pinta la flecha a 10 de ancho, y
	// chevronXSelectNativo, lo que su caja se separa del borde derecho.
	chevronSelectNativo  float32 = 17
	chevronXSelectNativo float32 = 0.75
)

// La flecha de un `<select>` nativo NO es un chevrón de trazo: Chromium pinta un
// chevrón MACIZO, de brazos a 45° y 1,94 de grueso, que mide 9,31 × 6,03 y acaba a
// 4,375 del borde derecho, centrado en alto. No cambia con la letra: medido a 8
// aumentos (2026-09-25) con `text-2xs`, `text-xs` y `text-sm`, el mismo.
// flechaNativa es su contorno en la rejilla de 24 de los iconos, para un lado de 10.
const (
	flechaNativa        = "M3.3 0 0 3.3 11.175 14.475 22.35 3.3 19.05 0 11.175 7.875Z"
	ladoFlechaNativa    = 10
	anchoFlechaNativa   = 9.3125
	altoFlechaNativa    = 6.03
	derechaFlechaNativa = 4.375
)

// medidaPropia es el tamaño de un selector con medidas propias.
func (s *Selector) medidaPropia() fyne.Size {
	p := s.Propio
	ancho := s.Ancho
	if ancho <= 0 {
		var opcion float32
		for _, o := range s.Opciones {
			opcion = max(opcion, Medir(o, p.Campo.Texto, Normal).Width)
		}
		// Chromium da al `<select>` sin ancho un ancho ENTERO: medido 79 donde la
		// suma da 79,06 (estados de un comentario) y 64 donde da 64,42 (PNG/JPEG).
		ancho = float32(math.Round(float64(opcion + 2*p.Campo.MargenX + 2*tema.Borde + FlechaSelectNativo)))
	}
	return fyne.NewSize(ancho, p.Alto)
}

// MinSize es la medida declarada.
func (s *Selector) MinSize() fyne.Size {
	s.ExtendBaseWidget(s)
	if s.Propio != nil {
		return s.medidaPropia()
	}
	m := MedidaSelector(s.Talla, s.Ancho)
	if s.Alto > 0 {
		m.Height = s.Alto
	}
	return m
}

// Tapped abre y cierra la lista.
func (s *Selector) Tapped(*fyne.PointEvent) {
	if s.inerte {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(s); c != nil {
		enfocar(c, s)
	}
	s.Alternar()
}

// Alternar abre la lista si está cerrada y la cierra si está abierta.
func (s *Selector) Alternar() {
	if s.abierto != nil {
		s.abierto.Hide()
		s.abierto = nil
		return
	}
	c := fyne.CurrentApp().Driver().CanvasForObject(s)
	if c == nil || len(s.Opciones) == 0 {
		return
	}
	lista := s.lista()
	// El panel se queda en el hueco bajo el selector —o encima, si ahí hay más sitio—
	// y lo que no cabe se desplaza por dentro, con la elegida a la vista: la misma
	// cuenta que un menú (`EncajarMenu`).
	natural := lista.MinSize()
	pos, tope := encajarMenu(fyne.CurrentApp().Driver().AbsolutePositionForObject(s), s.Size(),
		natural, c.Size(), true, false, tema.Espacio(1))
	tam := fyne.NewSize(natural.Width, min(natural.Height, tope))
	lista.centrar(s.Elegida, tam.Height)
	s.abierto = NuevoFlotante(lista, c)
	s.abierto.AlCerrar(func() { s.abierto = nil })
	s.abierto.Resize(tam)
	s.abierto.ShowAtPosition(pos)
	// Con la lista abierta el teclado es de su capa: se enfoca el panel, que le pasa las
	// teclas al selector. Sin esto, tras abrirla con el ratón las flechas no llegaban.
	enfocar(c, lista)
}

// elegir fija la opción y avisa. Fuera de rango no hace nada: una lista vacía no tiene
// opción que elegir y un índice pasado es un fallo de quien lo pasa, no un estado.
func (s *Selector) elegir(i int) {
	if i < 0 || i >= len(s.Opciones) || i == s.Elegida {
		return
	}
	s.Elegida = i
	s.Refresh()
	if s.AlElegir != nil {
		s.AlElegir(i)
	}
}

// FocusGained enciende el aro.
func (s *Selector) FocusGained() { s.enfocado = true; s.Refresh() }

// FocusLost lo apaga y cierra la lista: un desplegable abierto sin foco es un panel
// huérfano flotando sobre la pantalla.
func (s *Selector) FocusLost() {
	s.enfocado = false
	if s.abierto != nil {
		s.abierto.Hide()
		s.abierto = nil
	}
	s.Refresh()
}

// TypedRune abre la lista con la barra espaciadora, como un `<select>` del navegador.
func (s *Selector) TypedRune(r rune) {
	if r == ' ' {
		s.Alternar()
	}
}

// TypedKey mueve la elección con las flechas y abre o cierra con Intro y Escape, que es
// lo que hace un `<select>` con el teclado.
func (s *Selector) TypedKey(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeyUp:
		s.elegir(s.Elegida - 1)
	case fyne.KeyDown:
		s.elegir(s.Elegida + 1)
	case fyne.KeyHome:
		s.elegir(0)
	case fyne.KeyEnd:
		s.elegir(len(s.Opciones) - 1)
	case fyne.KeyReturn, fyne.KeyEnter:
		s.Alternar()
	case fyne.KeyEscape, mobile.KeyBack:
		// Cerrado, Escape es del diálogo de encima: se lo tragaba, y con el foco en un
		// selector el diálogo no se cerraba.
		if s.abierto != nil {
			s.Alternar()
		} else {
			SubirEscape(s, e)
		}
	default: // el resto de teclas no son del selector; las atiende quien tenga el foco después
	}
}

// MouseIn y MouseOut son `hover:border-text-subtle`.
func (s *Selector) MouseIn(*desktop.MouseEvent) {
	if s.inerte {
		return
	}
	s.encima = true
	s.Refresh()
}

// MouseMoved no hace nada.
func (s *Selector) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve el borde a su reposo.
func (s *Selector) MouseOut() {
	if !s.encima {
		return
	}
	s.encima = false
	s.Refresh()
}

// CreateRenderer compone la caja, el aro, el rótulo y el chevron.
func (s *Selector) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde, texto, pista := s.colores(v)
	c, m := s.medidas()
	s.r = &selectorRenderer{
		s:       s,
		aro:     NuevoAro(s.radio(), SinHueco, v),
		caja:    NuevaCaja(relleno, borde, s.radio()),
		rotulo:  Texto(s.Valor(), c.texto, Normal, texto),
		chevron: iconos.TrazoConTinta(iconos.ChevronAbajo, m.chevron, pista),
	}
	if s.Propio != nil {
		s.r.chevron = iconos.RellenoDeRuta(flechaNativa, ladoFlechaNativa, pista)
	}
	s.r.objetos = append(s.r.aro.Objetos(), s.r.caja.Objetos()...)
	s.r.objetos = append(s.r.objetos, s.r.rotulo, s.r.chevron.Objeto())
	return s.r
}

type selectorRenderer struct {
	s       *Selector
	aro     *Aro
	caja    *Caja
	rotulo  *canvas.Text
	chevron *iconos.Dibujo
	objetos []fyne.CanvasObject
}

func (r *selectorRenderer) MinSize() fyne.Size           { return r.s.MinSize() }
func (r *selectorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *selectorRenderer) Destroy()                     {}

func (r *selectorRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	c, m := r.s.medidas()
	// El texto se recorta con puntos suspensivos antes de la flecha: sin esto, una
	// opción larga (la del NAS de Drive a 360) se pintaba encima del chevrón.
	x := tema.Borde + c.margen
	Truncar(r.rotulo, r.s.Valor(), c.texto, Normal, max(t.Width-x-m.chevronX-m.chevron, 0))
	ColocarTexto(r.rotulo, c.texto, x, (t.Height-c.texto.Linea)/2)
	if r.s.Propio != nil {
		r.chevron.Colocar(t.Width-derechaFlechaNativa-anchoFlechaNativa, (t.Height-altoFlechaNativa)/2, ladoFlechaNativa)
		return
	}
	// `right-3` se mide desde la caja de BORDE, como el icono de un campo.
	r.chevron.Colocar(t.Width-m.chevronX-m.chevron, (t.Height-m.chevron)/2, m.chevron)
}

func (r *selectorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, borde, texto, pista := r.s.colores(v)
	r.caja.Pintar(relleno, borde)
	r.aro.Pintar(v)
	r.aro.Ver(r.s.enfocado)
	r.rotulo.Color = texto
	c, _ := r.s.medidas()
	r.rotulo.TextSize = c.texto.Tam
	r.Layout(r.s.Size()) // pone el texto, ya recortado al ancho
	r.rotulo.Refresh()
	r.chevron.TenirCon(pista)
}

// ---- la lista desplegada ----

// AltoOpcion es el alto de una fila de la lista: `py-2` arriba y abajo más la caja de
// línea de `text-base`, como un `MenuItem` de Dropdown.tsx.
var AltoOpcion = 2*tema.Espacio(2) + tema.TextoBase.Linea

// lista construye el panel con una fila por opción. Las medidas son las del panel de
// Dropdown.tsx: borde, `rounded-lg` y `p-1`.
func (s *Selector) lista() *panelLista {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ancho := max(s.Size().Width, MedidaSelector(s.Talla, s.Ancho).Width)
	filas := make([]fyne.CanvasObject, len(s.Opciones))
	for i, o := range s.Opciones {
		filas[i] = nuevaOpcion(s, i, o, ancho-2*(tema.Borde+tema.Espacio(1)))
	}
	p := &panelLista{
		s:     s,
		hijos: filas,
		fondo: NuevaCaja(tema.Color("menu-surface", v), tema.Color("border", v), tema.RadioLG),
		ancho: ancho,
	}
	p.ExtendBaseWidget(p)
	return p
}

// panelLista es el panel del desplegable: la caja con borde y las filas dentro, con el
// `p-1` de la hoja. Con más opciones de las que caben en su hueco —las cientos de zonas
// horarias del evento de Calendar, los veintiséis idiomas— se desplaza POR DENTRO, como
// un menú, y avisa de que queda más abajo.
type panelLista struct {
	widget.BaseWidget
	s     *Selector
	hijos []fyne.CanvasObject
	fondo *Caja
	ancho float32
	// desplazado es lo que las filas han subido dentro del panel.
	desplazado float32
}

var (
	_ fyne.Scrollable = (*panelLista)(nil)
	_ fyne.Focusable  = (*panelLista)(nil)
)

func (p *panelLista) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	return fyne.NewSize(p.ancho, altoFilas(len(p.hijos))+2*(tema.Borde+tema.Espacio(1)))
}

func altoFilas(n int) float32 { return float32(n) * AltoOpcion }

// Scrolled desplaza las filas por dentro del panel, hasta el final.
func (p *panelLista) Scrolled(ev *fyne.ScrollEvent) {
	p.desplazar(p.desplazado-ev.Scrolled.DY, p.Size().Height)
}

// desplazar pone el desplazamiento, acotado a lo que hay, para un panel de ese alto.
func (p *panelLista) desplazar(a, visible float32) {
	a = max(min(a, p.MinSize().Height-visible), 0)
	if a != p.desplazado {
		p.desplazado = a
		p.Refresh()
	}
}

// centrar deja la fila `i` a media altura de un panel de ese alto: al abrir, la elegida
// está a la vista aunque sea la doscientas.
func (p *panelLista) centrar(i int, visible float32) {
	p.desplazar(float32(i)*AltoOpcion-visible/2+AltoOpcion/2, visible)
}

// revelar trae la fila `i` a la vista si ha quedado fuera, sin mover nada si ya se ve.
func (p *panelLista) revelar(i int) {
	m := tema.Borde + tema.Espacio(1)
	arriba := m + float32(i)*AltoOpcion - p.desplazado
	switch alto := p.Size().Height; {
	case arriba < m:
		p.desplazar(p.desplazado-(m-arriba), alto)
	case arriba+AltoOpcion > alto-m:
		p.desplazar(p.desplazado+arriba+AltoOpcion-(alto-m), alto)
	}
}

// El panel se enfoca al abrirse y le pasa las teclas al selector, que es de quien son:
// las flechas mueven la elección, Intro y Escape cierran.
func (p *panelLista) FocusGained()     {}
func (p *panelLista) FocusLost()       {}
func (p *panelLista) TypedRune(r rune) { p.s.TypedRune(r) }

func (p *panelLista) TypedKey(e *fyne.KeyEvent) {
	p.s.TypedKey(e)
	if p.s.abierto == nil {
		return // se ha cerrado
	}
	p.revelar(p.s.Elegida)
	p.Refresh()
}

func (p *panelLista) CreateRenderer() fyne.WidgetRenderer {
	p.ExtendBaseWidget(p)
	r := &panelRenderer{
		p:     p,
		marco: canvas.NewRectangle(color.NRGBA{}),
		aviso: nuevoMasAbajo(tema.RadioLG - tema.Borde),
	}
	// El borde otra vez, encima: las filas que se desplazan pasan por debajo de él.
	r.marco.StrokeWidth, r.marco.CornerRadius = tema.Borde, tema.RadioLG
	r.objetos = slices.Concat(p.fondo.Objetos(), p.hijos, r.aviso.objetos(), []fyne.CanvasObject{r.marco})
	r.Refresh()
	return r
}

type panelRenderer struct {
	p       *panelLista
	marco   *canvas.Rectangle
	aviso   *masAbajo
	objetos []fyne.CanvasObject
}

// IsClip es el recorte del panel: las filas desplazadas no se salen de él.
func (r *panelRenderer) IsClip() {}

func (r *panelRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *panelRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *panelRenderer) Destroy()                     {}

// Refresh repinta las filas: la elegida cambia con las flechas con la lista abierta.
func (r *panelRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	// El panel puede seguir abierto al cambiar la apariencia desde otra app. La
	// caja y sus opciones deben cambiar juntas: tinta clara sobre papel oscuro, o
	// tinta oscura sobre el papel opaco del modo traslúcido.
	r.p.fondo.Pintar(tema.Color("menu-surface", v), tema.Color("border", v))
	for _, h := range r.p.hijos {
		h.Refresh()
	}
	r.Layout(r.p.Size())
}

func (r *panelRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.p.fondo.Redimensionar(t)
	r.marco.StrokeColor = tema.Color("border", v)
	r.marco.Resize(t)
	r.aviso.colocar(t, QuedaMasAbajo(r.p.MinSize().Height, r.p.desplazado, t.Height), v)
	m := tema.Borde + tema.Espacio(1)
	for i, h := range r.p.hijos {
		h.Move(fyne.NewPos(m, m+float32(i)*AltoOpcion-r.p.desplazado))
		h.Resize(fyne.NewSize(t.Width-2*m, AltoOpcion))
	}
}

// opcion es una fila de la lista: `px-3 py-2 text-base rounded-md`, con
// `hover:bg-menu-hover` y la elegida teñida de `menu-selected`.
type opcion struct {
	widget.BaseWidget
	s      *Selector
	i      int
	rotulo string
	ancho  float32
	encima bool
}

var (
	_ fyne.Tappable     = (*opcion)(nil)
	_ desktop.Hoverable = (*opcion)(nil)
)

func nuevaOpcion(s *Selector, i int, rotulo string, ancho float32) *opcion {
	o := &opcion{s: s, i: i, rotulo: rotulo, ancho: ancho}
	o.ExtendBaseWidget(o)
	return o
}

func (o *opcion) MinSize() fyne.Size {
	o.ExtendBaseWidget(o)
	return fyne.NewSize(o.ancho, AltoOpcion)
}

func (o *opcion) Tapped(*fyne.PointEvent) {
	o.s.elegir(o.i)
	o.s.Alternar()
}

func (o *opcion) MouseIn(*desktop.MouseEvent)    { o.encima = true; o.Refresh() }
func (o *opcion) MouseMoved(*desktop.MouseEvent) {}
func (o *opcion) MouseOut()                      { o.encima = false; o.Refresh() }

func (o *opcion) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &opcionRenderer{
		o:     o,
		fondo: canvas.NewRectangle(o.relleno(v)),
		texto: Texto(o.rotulo, tema.TextoBase, Normal, tema.Color("text", v)),
	}
	r.fondo.CornerRadius = tema.RadioMD
	r.objetos = []fyne.CanvasObject{r.fondo, r.texto}
	return r
}

func (o *opcion) relleno(v tema.Variante) color.NRGBA {
	switch {
	case o.i == o.s.Elegida:
		return tema.Color("menu-selected", v)
	case o.encima:
		return tema.Color("menu-hover", v)
	}
	return color.NRGBA{}
}

type opcionRenderer struct {
	o       *opcion
	fondo   *canvas.Rectangle
	texto   *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *opcionRenderer) MinSize() fyne.Size           { return r.o.MinSize() }
func (r *opcionRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *opcionRenderer) Destroy()                     {}

func (r *opcionRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	ColocarTexto(r.texto, tema.TextoBase, tema.Espacio(3), tema.Espacio(2))
}

func (r *opcionRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = r.o.relleno(v)
	r.fondo.Refresh()
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
}

// medidas son las del campo y las de la flecha: las de la talla o las propias.
func (s *Selector) medidas() (medidaCampo, medidaSelector) {
	c, m := medidasCampo[s.Talla], medidasSelector[s.Talla]
	if p := s.Propio; p != nil {
		c.alto, c.margen, c.texto = p.Alto, p.Campo.MargenX+interiorSelectNativo, p.Campo.Texto
		m = medidaSelector{derecha: FlechaSelectNativo, chevron: chevronSelectNativo, chevronX: chevronXSelectNativo}
	}
	return c, m
}

// radio es el de la caja: `rounded-md` en Form.tsx; el propio de la clase si lo hay.
func (s *Selector) radio() float32 {
	if s.Propio != nil && s.Propio.Campo.Radio > 0 {
		return s.Propio.Campo.Radio
	}
	return tema.RadioMD
}

// colores son los del campo; con medidas propias, el borde es el de su token, fijo, y la
// tinta la que diga la clase.
func (s *Selector) colores(v tema.Variante) (relleno, borde, texto, pista color.NRGBA) {
	relleno, borde, texto, pista = colorCampo(v, s.Invalida, s.encima, s.inerte)
	if p := s.Propio; p != nil {
		// La flecha del `<select>` nativo se pinta con `currentColor`: la tinta del texto
		// (medido en Chromium, en claro y en oscuro), no la pista tenue de Form.tsx.
		pista = texto
		if p.Campo.Borde != "" && !s.Invalida {
			borde = atenuar(tema.Color(p.Campo.Borde, v), s.inerte)
		}
		if p.Campo.Tinta != "" {
			texto = atenuar(tema.Color(p.Campo.Tinta, v), s.inerte)
			pista = texto
		}
		if p.Campo.Relleno != "" {
			relleno = atenuar(tema.Color(p.Campo.Relleno, v), s.inerte)
		}
	}
	return relleno, borde, texto, pista
}
