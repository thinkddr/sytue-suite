// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Medidas del carril de miniaturas (§7.2 de SlidesCanvas.tsx). Van en PÍXELES y no en
// rem, y ese detalle costó dinero: lo de dentro se escala a THUMB_W píxeles, así que
// medir la caja en rem la ata al tamaño de letra de la raíz. Con la raíz en 14 —el
// ajuste que tiene la suite— `w-40` daba 139 px y `w-32` 112, mientras el contenido
// seguía midiendo 128: cada miniatura recortaba 16 px de su lado derecho, medido en las
// tres ventanas de escritorio.
const (
	// THUMB_W.
	AnchoMiniatura float32 = 128
	// El carril: la miniatura más `p-2` a cada lado más el borde. La web lo escribe
	// como `THUMB_W + 32`.
	AnchoCarrilDiapos = AnchoMiniatura + 32
)

// Miniatura es una diapositiva del carril: se pulsa, se enfoca y se anuncia.
//
// Es un widget y no un contenedor porque tiene que hacer las tres cosas. El repartidor
// de eventos de Fyne solo desciende por contenedores y widgets, así que un árbol de
// primitivas sueltas es decorado mudo; y un `*fyne.Container` se anuncia siempre como
// «Container», que no es lo que tiene que oír quien navega con lector de pantalla.
type Miniatura struct {
	widget.BaseWidget

	M      *Modelo
	Indice int
	// Activa pinta el anillo de acento de 2 px que la web pone con `box-shadow`.
	Activa bool
	// AlElegir se llama al pulsarla o al activarla con Intro/Espacio.
	AlElegir func(int)
	// AlMover recibe el índice de destino con Ctrl/⌘+Mayús+↑/↓: reordenar sin ratón.
	// nil = el carril no se deja reordenar (solo lectura).
	AlMover func(hasta int)
	// Rotulo traduce una clave del catálogo (para el nombre accesible).
	Rotulo   func(clave string, n, total int) string
	Variante tema.Variante

	enfocada bool
}

var (
	_ fyne.Widget       = (*Miniatura)(nil)
	_ fyne.Tappable     = (*Miniatura)(nil)
	_ fyne.Focusable    = (*Miniatura)(nil)
	_ fyne.Shortcutable = (*Miniatura)(nil)
	_ desktop.Hoverable = (*Miniatura)(nil)
)

// NuevaMiniatura crea la miniatura de la diapositiva `i`.
func NuevaMiniatura(m *Modelo, i int, alElegir func(int)) *Miniatura {
	t := &Miniatura{M: m, Indice: i, AlElegir: alElegir, Variante: tema.Claro}
	t.ExtendBaseWidget(t)
	return t
}

// MedidaMiniatura es lo que ocupa: 128 de ancho y el alto que le toque por la
// proporción del lienzo (16:9 → 72, que es el `h-18` clásico).
func MedidaMiniatura(m *Modelo) fyne.Size {
	return fyne.NewSize(AnchoMiniatura, AnchoMiniatura*m.AltoEscenario()/AnchoEscenario)
}

func (t *Miniatura) Tapped(*fyne.PointEvent) { t.elegir() }

func (t *Miniatura) elegir() {
	if t.AlElegir != nil {
		t.AlElegir(t.Indice)
	}
}

func (t *Miniatura) MouseIn(*desktop.MouseEvent)    {}
func (t *Miniatura) MouseMoved(*desktop.MouseEvent) {}
func (t *Miniatura) MouseOut()                      {}

func (t *Miniatura) FocusGained() { t.enfocada = true; t.Refresh() }
func (t *Miniatura) FocusLost()   { t.enfocada = false; t.Refresh() }

// TypedRune activa con la barra espaciadora, como un `<button>`.
func (t *Miniatura) TypedRune(r rune) {
	if r == ' ' {
		t.elegir()
	}
}

// TypedKey activa con Intro.
func (t *Miniatura) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(t, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter {
		t.elegir()
	}
}

// TypedShortcut es el reordenado por teclado de la web: Ctrl (⌘ en Mac) + Mayús + ↑/↓
// lleva la diapositiva un puesto arriba o abajo. «El arrastre del carril no existe para
// quien navega con teclado, y el orden de las diapositivas no es un adorno».
func (t *Miniatura) TypedShortcut(s fyne.Shortcut) {
	c, ok := s.(*desktop.CustomShortcut)
	if !ok || t.AlMover == nil || c.Modifier&fyne.KeyModifierShift == 0 ||
		c.Modifier&(fyne.KeyModifierControl|fyne.KeyModifierSuper) == 0 {
		return
	}
	var hasta int
	switch c.KeyName {
	case fyne.KeyUp:
		hasta = t.Indice - 1
	case fyne.KeyDown:
		hasta = t.Indice + 1
	default:
		return
	}
	if hasta >= 0 && hasta < len(t.M.Diapos) {
		t.AlMover(hasta)
	}
}

// AccessibilityLabel es el `aria-label` de la web: «Diapositiva {n} de {total}».
func (t *Miniatura) AccessibilityLabel() string {
	if t.Rotulo == nil {
		return "slides.aria.slideOf"
	}
	return t.Rotulo("slides.aria.slideOf", t.Indice+1, len(t.M.Diapos))
}

// AccessibilityRole es botón: elegir una diapositiva es una acción.
func (t *Miniatura) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone la miniatura: la diapositiva escalada, el anillo de activa y
// el número.
func (t *Miniatura) CreateRenderer() fyne.WidgetRenderer {
	l := NuevoLienzo(t.M, t.Indice)
	l.Variante = t.Variante
	l.AlinearDefecto = "center"
	// El número va en `text-2xs` sobre `bg-bg-subtle`, redondeado y con `leading-4`.
	placa := canvas.NewRectangle(tema.Color("bg-subtle", t.Variante))
	placa.CornerRadius = tema.RadioSM
	num := ui.Texto(strconv.Itoa(t.Indice+1), tema.Texto2XS, ui.Normal, tema.Color("text-muted", t.Variante))
	r := &miniaturaRenderer{
		t:      t,
		lienzo: l,
		// La web pinta el anillo con `box-shadow: 0 0 0 2px`, que va POR FUERA. `Aro`
		// es esa misma receta y ya la tiene medida.
		aro:   ui.NuevoAroCon("primary", tema.RadioSM, 0, t.Variante),
		placa: placa,
		num:   num,
	}
	// El aro va al PRINCIPIO: `foco.go` lo dice y aquí se ve — puesto detrás del
	// lienzo se ve el anillo de 2 px que sobresale; puesto delante, el rectángulo del
	// anillo TAPA la miniatura entera, que es lo que pasó la primera vez.
	r.objetos = append(r.aro.Objetos(), l, placa, num)
	return r
}

type miniaturaRenderer struct {
	t       *Miniatura
	lienzo  *Lienzo
	aro     *ui.Aro
	placa   *canvas.Rectangle
	num     *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *miniaturaRenderer) MinSize() fyne.Size           { return MedidaMiniatura(r.t.M) }
func (r *miniaturaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *miniaturaRenderer) Destroy()                     {}

func (r *miniaturaRenderer) Layout(t fyne.Size) {
	r.lienzo.Resize(t)
	r.aro.Colocar(t)
	r.aro.Ver(r.t.Activa || r.t.enfocada)

	// `right-1 bottom-0.5`, con la caja de línea de `leading-4` (14 px) y `px-1`.
	const alto float32 = 14
	m := ui.Medir(r.num.Text, tema.Texto2XS, ui.Normal)
	ancho := m.Width + 2*tema.Espacio(1)
	x, y := t.Width-tema.Espacio(1)-ancho, t.Height-tema.Espacio(0.5)-alto
	r.placa.Resize(fyne.NewSize(ancho, alto))
	r.placa.Move(fyne.NewPos(x, y))
	ui.ColocarTexto(r.num, tema.Escalon{Tam: tema.Texto2XS.Tam, Linea: alto}, x+tema.Espacio(1), y)
}

func (r *miniaturaRenderer) Refresh() {
	r.lienzo.Variante = r.t.Variante
	r.aro.Pintar(r.t.Variante)
	r.placa.FillColor = tema.Color("bg-subtle", r.t.Variante)
	r.num.Color = tema.Color("text-muted", r.t.Variante)
	r.Layout(r.t.Size())
	canvas.Refresh(r.t)
}

// pila apila las miniaturas: `flex-col items-center gap-2 p-2`, o en fila
// (`max-sm:flex-row`) cuando el carril es la tira de un teléfono.
//
// No es `ui.Columna` porque esa ESTIRA los hijos al ancho del contenedor, y una
// miniatura estirada deja la diapositiva centrada dentro de una caja más ancha que
// ella: el anillo de la activa rodea la caja y no la diapositiva, y el número se va al
// borde del carril. Aquí el tamaño es fijo —128 por su proporción— y lo que se centra
// es la miniatura entera.
type pila struct {
	tam        fyne.Size
	horizontal bool
}

func (p pila) MinSize(objs []fyne.CanvasObject) fyne.Size {
	largo := 2 * tema.Espacio(2)
	paso := p.tam.Height
	if p.horizontal {
		paso = p.tam.Width
	}
	if n := len(objs); n > 0 {
		largo += float32(n)*paso + float32(n-1)*tema.Espacio(2)
	}
	if p.horizontal {
		return fyne.NewSize(largo, p.tam.Height+2*tema.Espacio(2))
	}
	return fyne.NewSize(p.tam.Width+2*tema.Espacio(2), largo)
}

func (p pila) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if p.horizontal {
		x, y := tema.Espacio(2), (t.Height-p.tam.Height)/2
		for _, o := range objs {
			o.Resize(p.tam)
			o.Move(fyne.NewPos(x, y))
			x += p.tam.Width + tema.Espacio(2)
		}
		return
	}
	x, y := (t.Width-p.tam.Width)/2, tema.Espacio(2)
	for _, o := range objs {
		o.Resize(p.tam)
		o.Move(fyne.NewPos(x, y))
		y += p.tam.Height + tema.Espacio(2)
	}
}
