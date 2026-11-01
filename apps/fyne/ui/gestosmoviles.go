// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/widget"
)

const (
	bordeMenuMovil       float32 = 24
	decidirGesto         float32 = 12
	abrirMenuMovil       float32 = 64
	cerrarMenuMovil      float32 = 48
	actualizarListaMovil float32 = 72
	dominioGesto         float32 = 1.5
)

// RefrescoLista describes a list that can be refreshed. Arriba reads its actual
// scroll offset; Puede also checks loading, selection, connectivity and lifetime.
// The callback is the app's existing refresh action and runs on the UI thread.
type RefrescoLista struct {
	Arriba, Puede func() bool
	Actualizar    func()
	// Revision changes on account, folder, filter or view changes. A gesture
	// started on a previous revision never refreshes its replacement.
	Revision func() uint64
}

// RevisionLista adapts an app's non-negative request sequence without allowing a
// negative or wrapped signed counter to become an unrelated unsigned revision.
func RevisionLista(sequence int) uint64 {
	if sequence < 0 {
		return 0
	}
	return uint64(sequence)
}

// RefrescarAlTirar marks a list without intercepting its rows, taps or scrolling.
// The mobile canvas arbitrates a single touch sequence before dispatching it.
// Desktop input and accessibility continue to use the original controls.
func RefrescarAlTirar(content fyne.CanvasObject, options *RefrescoLista) fyne.CanvasObject {
	if !esMovil() {
		return content
	}
	w := &listaActualizable{content: content, options: options}
	w.ExtendBaseWidget(w)
	return w
}

type listaActualizable struct {
	widget.BaseWidget
	content fyne.CanvasObject
	options *RefrescoLista
}

func (l *listaActualizable) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.content)
}

func (l *listaActualizable) disponible() bool {
	return l.Visible() && l.options != nil && l.options.Actualizar != nil && l.options.Arriba != nil && l.options.Arriba() &&
		(l.options.Puede == nil || l.options.Puede())
}

// raizGestos owns one filter per canvas. It is part of the content tree, never a
// transparent input overlay; it has no Touchable, Tappable or Draggable methods.
type raizGestos struct {
	widget.BaseWidget
	content     fyne.CanvasObject
	canvas      fyne.Canvas
	gestos      gestosMoviles
	primerPlano bool
}

func conGestosMoviles(content fyne.CanvasObject) fyne.CanvasObject {
	if !esMovil() {
		return content
	}
	w := &raizGestos{content: content, primerPlano: true}
	w.ExtendBaseWidget(w)
	return w
}

func (w *raizGestos) CreateRenderer() fyne.WidgetRenderer {
	return &rendererGestos{WidgetRenderer: widget.NewSimpleRenderer(w.content), root: w}
}

type rendererGestos struct {
	fyne.WidgetRenderer
	root *raizGestos
}

func (r *rendererGestos) Layout(size fyne.Size) {
	r.WidgetRenderer.Layout(size)
	w := r.root
	c := lienzoDe(w)
	if c == nil {
		return
	}
	if c == w.canvas {
		sincronizarExclusionMenuMovil(c)
		return
	}
	filter, supported := c.(mobile.TouchFilterCanvas)
	if !supported {
		return
	}
	w.canvas = c
	w.gestos.canvas = c
	filter.SetTouchFilter(w.gestos.recibir)
	sincronizarExclusionMenuMovil(c)
}

func (r *rendererGestos) Refresh() {
	r.WidgetRenderer.Refresh()
	sincronizarExclusionMenuMovil(r.root.canvas)
}

// Recompute from the live canvas content: a renderer belonging to an old app view
// must never restore Android's small menu exclusion after a dialog or detail opens.
func sincronizarExclusionMenuMovil(c fyne.Canvas) {
	if c == nil {
		return
	}
	root, ok := c.Content().(*raizGestos)
	available := ok && root.canvas == c && root.primerPlano && !root.gestos.bloqueado()
	if _, editing := c.Focused().(mobile.Keyboardable); editing {
		available = false
	}
	if available {
		a := armazonEn(root.content)
		available = a != nil && menuPorGesto(a)
	}
	actualizarExclusionGestos(c, available)
}

type claseGesto uint8

const (
	gestoPendiente claseGesto = iota
	gestoMenu
	gestoCerrarMenu
	gestoActualizar
)

type gestosMoviles struct {
	canvas            fyne.Canvas
	id                int
	inicio            fyne.Position
	hora              time.Time
	menu              *Armazon
	cierre            *Armazon
	lista             *listaActualizable
	clase             claseGesto
	revision          uint64
	activo, capturado bool
}

func (g *gestosMoviles) cancelar() {
	menu, cierre := g.menu, g.cierre
	g.activo, g.capturado = false, false
	g.menu, g.cierre, g.lista = nil, nil, nil
	g.clase = gestoPendiente
	if menu != nil && menu.arrastrandoCajon {
		menu.cancelarArrastreCajon()
	}
	if cierre != nil && cierre.arrastrandoCajon {
		cierre.cancelarArrastreCajon()
	}
}

func (g *gestosMoviles) bloqueado() bool {
	return g.canvas != nil && (g.canvas.Overlays().Top() != nil || dialogoArriba(g.canvas) != nil)
}

func (g *gestosMoviles) recibir(e mobile.FilterEvent) mobile.FilterResult {
	switch e.Phase {
	case mobile.FilterBegin:
		return g.empezar(e)
	case mobile.FilterCancel:
		g.cancelar()
		return mobile.FilterPass
	case mobile.FilterMove, mobile.FilterEnd:
		if !g.activo || e.ID != g.id {
			return mobile.FilterPass
		}
		return g.seguir(e)
	}
	return mobile.FilterPass
}

func (g *gestosMoviles) empezar(e mobile.FilterEvent) mobile.FilterResult {
	sincronizarExclusionMenuMovil(g.canvas)
	g.cancelar()
	if !g.puedeEmpezar(e) {
		return mobile.FilterPass
	}
	left := bordeIzquierdoLienzo(g.canvas)
	for _, object := range e.Path {
		if _, editing := object.(mobile.Keyboardable); editing {
			g.cancelar()
			return mobile.FilterPass
		}
		g.anotarObjetivo(object, e.Position.X, left)
	}
	if g.menu == nil && g.cierre == nil && g.lista == nil {
		return mobile.FilterPass
	}
	g.id, g.inicio, g.hora, g.activo = e.ID, e.Position, e.Time, true
	if g.lista != nil && g.lista.options.Revision != nil {
		g.revision = g.lista.options.Revision()
	}
	return mobile.FilterDefer
}

func (g *gestosMoviles) puedeEmpezar(e mobile.FilterEvent) bool {
	if g.bloqueado() || e.Position.X < 0 || e.Position.Y < 0 {
		return false
	}
	if g.canvas != nil {
		if _, editing := g.canvas.Focused().(mobile.Keyboardable); editing {
			return false
		}
	}
	return true
}

func bordeIzquierdoLienzo(c fyne.Canvas) float32 {
	if c == nil {
		return 0
	}
	position, _ := c.InteractiveArea()
	return position.X
}

func (g *gestosMoviles) anotarObjetivo(object fyne.CanvasObject, posX, left float32) {
	if a, ok := object.(*Armazon); ok {
		if posX >= left && posX <= left+bordeMenuMovil && menuPorGesto(a) {
			g.menu = a
		} else if cierreMenuPorGesto(a) {
			g.cierre = a
		}
	}
	if list, ok := object.(*listaActualizable); ok && list.disponible() {
		g.lista = list
	}
}

func menuPorGesto(a *Armazon) bool {
	return a.Visible() && a.Menu != nil && !a.CajonAbierto && !a.DetalleAbierto &&
		(a.Size().Width < Estrecho || a.Carril != nil)
}

func cierreMenuPorGesto(a *Armazon) bool {
	return a.Visible() && a.Menu != nil && a.CajonAbierto &&
		(a.Size().Width < Estrecho || a.Carril != nil)
}

func (g *gestosMoviles) seguir(e mobile.FilterEvent) mobile.FilterResult {
	if e.Phase == mobile.FilterEnd && !g.enLaMismaVista(e.Path) {
		g.cancelar()
		return mobile.FilterClaim
	}
	if g.bloqueado() || !g.hora.IsZero() && e.Time.Sub(g.hora) > 2*time.Second {
		return g.terminarSinAccion()
	}
	if g.lista != nil && g.lista.options.Revision != nil && g.lista.options.Revision() != g.revision {
		g.cancelar()
		return mobile.FilterClaim
	}
	if !g.capturado {
		if result := g.decidir(e); result != mobile.FilterClaim {
			return result
		}
	}
	if e.Phase != mobile.FilterEnd {
		dx, _, _, _ := g.distancias(e.Position)
		switch g.clase {
		case gestoMenu:
			if g.menu != nil && menuPorGesto(g.menu) {
				g.menu.arrastrarAperturaCajon(dx)
			}
		case gestoCerrarMenu:
			if g.cierre != nil && cierreMenuPorGesto(g.cierre) {
				g.cierre.arrastrarCierreCajon(dx)
			}
		default:
		}
		return mobile.FilterClaim
	}
	return g.soltar(e)
}

func (g *gestosMoviles) enLaMismaVista(path []fyne.CanvasObject) bool {
	return (g.menu == nil || enRutaTactil(path, g.menu)) &&
		(g.cierre == nil || enRutaTactil(path, g.cierre)) &&
		(g.lista == nil || enRutaTactil(path, g.lista))
}

func (g *gestosMoviles) decidir(e mobile.FilterEvent) mobile.FilterResult {
	dx, dy, x, y := g.distancias(e.Position)
	switch {
	case dx >= decidirGesto && x >= dominioGesto*y && g.menu != nil && menuPorGesto(g.menu):
		g.clase, g.capturado = gestoMenu, true
	case dx <= -decidirGesto && x >= dominioGesto*y && g.cierre != nil && cierreMenuPorGesto(g.cierre):
		g.clase, g.capturado = gestoCerrarMenu, true
	case dy >= decidirGesto && y >= dominioGesto*x && g.lista != nil && g.lista.disponible():
		g.clase, g.capturado = gestoActualizar, true
	case x >= decidirGesto || y >= decidirGesto || e.Phase == mobile.FilterEnd:
		g.cancelar()
		return mobile.FilterPass
	default:
		return mobile.FilterDefer
	}
	return mobile.FilterClaim
}

func (g *gestosMoviles) distancias(position fyne.Position) (dx, dy, x, y float32) {
	dx, dy = position.X-g.inicio.X, position.Y-g.inicio.Y
	x, y = float32(math.Abs(float64(dx))), float32(math.Abs(float64(dy)))
	return dx, dy, x, y
}

func (g *gestosMoviles) soltar(e mobile.FilterEvent) mobile.FilterResult {
	dx, dy, x, y := g.distancias(e.Position)
	menu, cierre, list, kind := g.menu, g.cierre, g.lista, g.clase
	g.activo, g.capturado = false, false
	g.menu, g.cierre, g.lista = nil, nil, nil
	g.clase = gestoPendiente
	switch kind {
	case gestoMenu:
		soltarAperturaMenu(menu, dx, x, y, e.Path)
	case gestoCerrarMenu:
		soltarCierreMenu(cierre, dx, x, y, e.Path)
	case gestoActualizar:
		if dy >= actualizarListaMovil && y >= dominioGesto*x && enRutaTactil(e.Path, list) && list.disponible() {
			list.options.Actualizar()
		}
	default:
	}
	return mobile.FilterClaim
}

func soltarAperturaMenu(menu *Armazon, dx, x, y float32, path []fyne.CanvasObject) {
	if menu == nil {
		return
	}
	if dx < abrirMenuMovil || x < dominioGesto*y || !enRutaTactil(path, menu) || !menuPorGesto(menu) {
		menu.cancelarArrastreCajon()
		return
	}
	// Use the existing button action so app-owned drawer state stays in sync.
	bar, ok := menu.barraMovil().(*BarraMovil)
	if !ok || bar.AlMenu == nil {
		menu.AbrirCajon()
		return
	}
	bar.AlMenu()
	if !menu.arrastrandoCajon {
		return
	}
	if !menu.CajonAbierto {
		menu.cancelarArrastreCajon()
		return
	}
	desde := menu.entrada
	menu.arrastrandoCajon = false
	if menu.AlCajon != nil {
		menu.AlCajon(true)
	}
	menu.animarHaciaAbierto(desde)
}

func soltarCierreMenu(cierre *Armazon, dx, x, y float32, path []fyne.CanvasObject) {
	if cierre == nil {
		return
	}
	if -dx >= cerrarMenuMovil && x >= dominioGesto*y && enRutaTactil(path, cierre) && cierreMenuPorGesto(cierre) {
		cierre.cerrarDesdeMando()
		return
	}
	cierre.cancelarArrastreCajon()
}

func enRutaTactil(path []fyne.CanvasObject, expected fyne.CanvasObject) bool {
	for _, object := range path {
		if object == expected {
			return true
		}
	}
	return false
}

func (g *gestosMoviles) terminarSinAccion() mobile.FilterResult {
	claimed := g.capturado
	g.cancelar()
	if claimed {
		return mobile.FilterClaim
	}
	return mobile.FilterPass
}
